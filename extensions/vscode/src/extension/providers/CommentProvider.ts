import * as vscode from 'vscode';
import { ReviewComment, CommentStatus, CommentSyncState } from '../../shared/types';
import { COMMENT_CONTROLLER_ID } from '../../shared/constants';
import { LineOffsetTracker } from './lineOffset';

export class CommentProvider {
  private controller: vscode.CommentController;
  // 以 comment 在 result.comments 中的原始下标为 key，与 webview 共用同一索引空间。
  // 打不开的文件（如目录）没有 thread，但下标依旧保留，避免错位。
  private threads = new Map<number, vscode.CommentThread>();
  private comments: ReviewComment[] = [];
  private status = new Map<number, CommentStatus>();
  private offsets = new LineOffsetTracker();
  private syncListeners: Array<(s: CommentSyncState[]) => void> = [];

  constructor(private extensionUri: vscode.Uri) {
    this.controller = vscode.comments.createCommentController(COMMENT_CONTROLLER_ID, 'Open Code Review');
  }

  onSync(fn: (s: CommentSyncState[]) => void): void {
    this.syncListeners.push(fn);
  }

  private emitSync(): void {
    const states: CommentSyncState[] = this.comments.map((_, i) => ({
      index: i, status: this.status.get(i) ?? 'pending',
    }));
    this.syncListeners.forEach((fn) => fn(states));
  }

  /**
   * 展示审查评论。
   * @param inEditor 是否在编辑器内创建 CommentThread。仅工作区模式为 true；
   *   分支对比/单次提交模式下被审查代码不在当前工作区，行号会错位，故只在侧边栏展示。
   */
  async show(comments: ReviewComment[], inEditor = true): Promise<void> {
    this.clear();
    // 不重排：保持与 webview（result.comments）相同的顺序与下标
    this.comments = comments;
    const root = vscode.workspace.workspaceFolders?.[0].uri.fsPath;
    if (!root) return;

    // 非工作区模式：只登记评论与状态供侧边栏同步，不在编辑器内放置 thread。
    if (!inEditor) {
      for (let i = 0; i < this.comments.length; i++) this.status.set(i, 'pending');
      this.emitSync();
      return;
    }

    let firstShown = -1;
    for (let i = 0; i < this.comments.length; i++) {
      const c = this.comments[i];
      this.status.set(i, 'pending');
      try {
        const uri = vscode.Uri.file(`${root}/${c.path}`);
        const doc = await vscode.workspace.openTextDocument(uri);
        const range = new vscode.Range(Math.max(0, c.startLine - 1), 0, Math.max(0, c.endLine - 1), 0);
        const body = this.renderBody(c, i, 'pending');
        const thread = this.controller.createCommentThread(doc.uri, range, [{
          body, mode: vscode.CommentMode.Preview,
          author: { name: '⏳ [未处理]' },
        }]);
        thread.canReply = false;
        thread.label = `Code Review (${i + 1} / ${this.comments.length})`;
        // 有代码建议 → 'pending'（显示应用+忽略）；无建议 → 'pendingNoSuggestion'（仅忽略）
        thread.contextValue = this.hasSuggestion(c) ? 'pending' : 'pendingNoSuggestion';
        thread.collapsibleState = vscode.CommentThreadCollapsibleState.Expanded;
        this.threads.set(i, thread);
        if (firstShown < 0) firstShown = i;
      } catch { /* 文件打不开（如目录）则无 thread，但保留下标 */ }
    }
    if (firstShown >= 0) await this.jumpTo(firstShown);
    this.emitSync();
  }

  private hasSuggestion(c: ReviewComment): boolean {
    return !!(c.suggestionCode && c.suggestionCode.trim());
  }

  private renderBody(c: ReviewComment, _index: number, _status: CommentStatus): vscode.MarkdownString {
    let md = c.content;
    if (this.hasSuggestion(c)) {
      md += `\n***\n\`\`\`diff\n${c.suggestionCode}\n\`\`\``;
    } else {
      md += `\n***\n_💡 无代码建议，请手动处理_`;
    }
    const s = new vscode.MarkdownString(md);
    s.isTrusted = true;
    return s;
  }

  async apply(index: number): Promise<void> {
    const c = this.comments[index];
    if (!c) return;
    const root = vscode.workspace.workspaceFolders?.[0].uri.fsPath;
    if (!root) return;
    const uri = vscode.Uri.file(`${root}/${c.path}`);
    const doc = await vscode.workspace.openTextDocument(uri);
    const before = doc.lineCount;
    const start = Math.max(0, this.offsets.adjusted(c.path, c.startLine) - 1);
    const end = Math.min(doc.lineCount - 1, this.offsets.adjusted(c.path, c.endLine) - 1);
    if (end < start) {
      vscode.window.showErrorMessage('应用失败：代码位置已失效，请刷新后重试。');
      return;
    }
    const range = new vscode.Range(start, 0, end, doc.lineAt(end).text.length);
    const hasSuggestion = !!(c.suggestionCode && c.suggestionCode.trim());

    // 用 WorkspaceEdit 而非 editor.edit：后者要求目标编辑器为活动编辑器，
    // 从评论标题栏按钮触发时焦点在评论控件上，会静默返回 false 导致“点不动”。
    const edit = new vscode.WorkspaceEdit();
    if (hasSuggestion) edit.replace(uri, range, c.suggestionCode!);
    else edit.delete(uri, range);
    const ok = await vscode.workspace.applyEdit(edit);
    if (!ok) {
      vscode.window.showErrorMessage('应用失败：无法修改文件，请检查文件是否被占用或处于只读状态。');
      return;
    }
    await doc.save();
    this.offsets.record(c.path, c.startLine, doc.lineCount - before);
    await vscode.window.showTextDocument(doc, { selection: new vscode.Range(start, 0, start, 0), preview: false });
    this.setStatus(index, 'applied');
  }

  discard(index: number): void { this.setStatus(index, 'discarded'); }
  falsePositive(index: number): void { this.setStatus(index, 'falsePositive'); }

  private setStatus(index: number, status: CommentStatus): void {
    this.status.set(index, status);
    const thread = this.threads.get(index);
    if (thread) {
      const label = { applied: '✅ [已应用]', discarded: '✅ [已忽略]', falsePositive: '✅ [已误报]', pending: '⏳ [未处理]' }[status];
      thread.comments = [{ ...thread.comments[0], author: { name: label }, body: this.renderBody(this.comments[index], index, status) }] as any;
      thread.contextValue = status;
      thread.collapsibleState = vscode.CommentThreadCollapsibleState.Collapsed;
    }
    this.emitSync();
  }

  async jumpTo(index: number): Promise<void> {
    const thread = this.threads.get(index);
    if (!thread) {
      const c = this.comments[index];
      if (c) vscode.window.showWarningMessage(`无法定位到 ${c.path}：该路径不是可打开的文件。`);
      return;
    }
    await vscode.window.showTextDocument(thread.uri, { selection: thread.range, preview: false });
    thread.collapsibleState = vscode.CommentThreadCollapsibleState.Expanded;
  }

  indexOfThread(thread: vscode.CommentThread): number {
    for (const [i, t] of this.threads) if (t === thread) return i;
    return -1;
  }

  clear(): void {
    this.threads.forEach((t) => t.dispose());
    this.threads.clear();
    this.comments = [];
    this.status.clear();
    this.offsets.clear();
  }

  dispose(): void {
    this.clear();
    this.controller.dispose();
  }
}
