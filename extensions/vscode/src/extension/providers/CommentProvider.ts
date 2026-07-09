import { t, resolveLocale, SupportedLocale } from '../../shared/i18n';
import * as vscode from 'vscode';
import { ReviewComment, CommentStatus, CommentSyncState, ReviewContext, ReviewMode } from '../../shared/types';
import { COMMENT_CONTROLLER_ID } from '../../shared/constants';
import { LineOffsetTracker } from './lineOffset';
import { GitService } from '../services/GitService';
import {
  MountableCommentAnchor,
  resolveCommentAnchor,
  CommentAnchorDeps,
  SidebarOnlyReason,
} from './commentAnchor';

export class CommentProvider {
  private controller: vscode.CommentController;
  private threads = new Map<number, vscode.CommentThread>();
  private mounts = new Map<number, MountableCommentAnchor>();
  private jumpable = new Set<number>();
  private jumpBlockReasons = new Map<number, SidebarOnlyReason | 'mount-failed'>();
  private comments: ReviewComment[] = [];
  private status = new Map<number, CommentStatus>();
  private offsets = new LineOffsetTracker();
  private syncListeners: Array<(s: CommentSyncState[]) => void> = [];
  private reviewContext: ReviewContext = { mode: ReviewMode.Workspace };

  private locale: SupportedLocale;

  constructor(
    private extensionUri: vscode.Uri,
    private git: GitService,
  ) {
    this.locale = resolveLocale(vscode.env.language);
    this.controller = vscode.comments.createCommentController(COMMENT_CONTROLLER_ID, t(this.locale, 'ext.commentController'));
  }

  onSync(fn: (s: CommentSyncState[]) => void): void {
    this.syncListeners.push(fn);
  }

  private emitSync(): void {
    const states: CommentSyncState[] = this.comments.map((_, i) => ({
      index: i,
      status: this.status.get(i) ?? 'pending',
      jumpable: this.jumpable.has(i),
    }));
    this.syncListeners.forEach((fn) => fn(states));
  }

  /** 展示审查评论：能解析到 git/工作区快照的挂 thread，否则仅侧边栏。 */
  async show(comments: ReviewComment[], ctx: ReviewContext): Promise<void> {
    this.clear();
    this.reviewContext = ctx;
    this.comments = comments;

    const root = vscode.workspace.workspaceFolders?.[0].uri.fsPath;
    if (!root) return;

    await this.git.prepareReviewFileStatus(ctx);
    const deps = this.buildAnchorDeps(root, ctx);

    let firstShown = -1;
    for (let i = 0; i < this.comments.length; i++) {
      const c = this.comments[i];
      this.status.set(i, 'pending');

      const anchor = await resolveCommentAnchor(c, ctx, deps);
      if (anchor.kind === 'sidebar') {
        this.jumpBlockReasons.set(i, anchor.reason);
        continue;
      }

      try {
        await vscode.workspace.openTextDocument(anchor.uri);
        const body = this.renderBody(c, i, 'pending', anchor.locateNote);
        const thread = this.controller.createCommentThread(anchor.uri, anchor.range, [{
          body,
          mode: vscode.CommentMode.Preview,
          author: { name: t(this.locale, 'ext.comment.pending') },
        }]);
        thread.canReply = false;
        thread.label = `${t(this.locale, 'ext.comment.threadLabel')} (${i + 1} / ${this.comments.length})`;
        thread.contextValue = this.threadContextValue(c, ctx);
        thread.collapsibleState = vscode.CommentThreadCollapsibleState.Expanded;
        this.threads.set(i, thread);
        this.mounts.set(i, anchor);
        this.jumpable.add(i);
        if (firstShown < 0) firstShown = i;
      } catch {
        this.jumpBlockReasons.set(i, 'mount-failed');
      }
    }

    if (firstShown >= 0) await this.jumpTo(firstShown);
    this.emitSync();
  }

  private buildAnchorDeps(root: string, ctx: ReviewContext): CommentAnchorDeps {
    return {
      repoRoot: root,
      fileStatus: (path) => this.git.getReviewFileStatus(path),
      readAtRef: (ref, path) => this.git.readFileAtRef(ref, path),
      readWorkspace: (path) => this.git.readWorkspaceFile(path),
      buildDiffUris: (path, status) => this.git.buildCommentDiffUris(path, status, ctx),
      toGitUri: async (path, ref) => {
        const uri = await this.git.createGitFileUri(path, ref);
        if (!uri) throw new Error('git uri unavailable');
        return uri;
      },
    };
  }

  private threadContextValue(c: ReviewComment, ctx: ReviewContext): string {
    if (ctx.mode !== ReviewMode.Workspace) return 'pendingNoSuggestion';
    return this.hasSuggestion(c) ? 'pending' : 'pendingNoSuggestion';
  }

  private hasSuggestion(c: ReviewComment): boolean {
    return !!(c.suggestionCode && c.suggestionCode.trim());
  }

  private renderBody(
    c: ReviewComment,
    _index: number,
    _status: CommentStatus,
    locateNote?: string,
  ): vscode.MarkdownString {
    let md = locateNote ? `${locateNote}\n\n${c.content}` : c.content;
    if (this.hasSuggestion(c)) {
      md += `\n***\n\`\`\`diff\n${c.suggestionCode}\n\`\`\``;
    } else {
      md += `\n***\n${t(this.locale, 'ext.comment.noSuggestion')}`;
    }
    const s = new vscode.MarkdownString(md);
    s.isTrusted = true;
    return s;
  }

  async apply(index: number): Promise<void> {
    if (this.reviewContext.mode !== ReviewMode.Workspace) {
      vscode.window.showWarningMessage(t(this.locale, 'ext.comment.applyWorkspaceOnly'));
      return;
    }
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
      vscode.window.showErrorMessage(t(this.locale, 'ext.comment.applyFailedStale'));
      return;
    }
    const range = new vscode.Range(start, 0, end, doc.lineAt(end).text.length);
    const hasSuggestion = !!(c.suggestionCode && c.suggestionCode.trim());

    const edit = new vscode.WorkspaceEdit();
    if (hasSuggestion) edit.replace(uri, range, c.suggestionCode!);
    else edit.delete(uri, range);
    const ok = await vscode.workspace.applyEdit(edit);
    if (!ok) {
      vscode.window.showErrorMessage(t(this.locale, 'ext.comment.applyFailedLocked'));
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
    const mount = this.mounts.get(index);
    if (thread) {
      const label = {
        applied: t(this.locale, 'ext.comment.statusApplied'),
        discarded: t(this.locale, 'ext.comment.statusDiscarded'),
        falsePositive: t(this.locale, 'ext.comment.statusFalsePositive'),
        pending: t(this.locale, 'ext.comment.pending'),
      }[status];
      thread.comments = [{
        ...thread.comments[0],
        author: { name: label },
        body: this.renderBody(this.comments[index], index, status, mount?.locateNote),
      }] as any;
      thread.contextValue = status;
      thread.collapsibleState = vscode.CommentThreadCollapsibleState.Collapsed;
    }
    this.emitSync();
  }

  async jumpTo(index: number): Promise<void> {
    const mount = this.mounts.get(index);
    const thread = this.threads.get(index);
    if (!mount || !thread) {
      this.showJumpFailed(index);
      return;
    }

    if (mount.diff) {
      const opened = await this.openDiffEditor(mount);
      if (opened) {
        await this.revealInDiffSide(mount.uri, mount.range, mount.side);
      } else {
        await vscode.window.showTextDocument(mount.uri, { selection: mount.range, preview: false });
      }
    } else {
      await vscode.window.showTextDocument(mount.uri, { selection: mount.range, preview: false });
    }
    thread.collapsibleState = vscode.CommentThreadCollapsibleState.Expanded;
  }

  private showJumpFailed(index: number): void {
    const c = this.comments[index];
    if (!c) return;

    const reason = this.jumpBlockReasons.get(index) ?? this.inferJumpBlockReason(c);
    const key = reason === 'missing-file' || reason === 'mount-failed'
      ? 'ext.comment.jumpFileMissing'
      : 'ext.comment.jumpLineUnresolved';
    vscode.window.showWarningMessage(
      t(this.locale, key).replace('{path}', c.path),
    );
  }

  /** 未记录原因时根据评论元数据推断（如 L0 多为行号未解析）。 */
  private inferJumpBlockReason(c: ReviewComment): SidebarOnlyReason | 'mount-failed' {
    if (c.startLine <= 0 && c.endLine <= 0) return 'unresolved';
    return 'missing-file';
  }

  private async openDiffEditor(mount: MountableCommentAnchor): Promise<boolean> {
    if (!mount.diff) return false;
    try {
      await vscode.commands.executeCommand(
        'vscode.diff',
        mount.diff.left,
        mount.diff.right,
        mount.diff.title,
        { preview: false },
      );
      return true;
    } catch {
      return false;
    }
  }

  /** 在已打开的 diff 编辑器中定位到挂载侧行，不再额外打开单文件 tab。 */
  private async revealInDiffSide(
    uri: vscode.Uri,
    range: vscode.Range,
    side: MountableCommentAnchor['side'],
  ): Promise<void> {
    for (let attempt = 0; attempt < 8; attempt++) {
      const editor = this.findEditorForUri(uri, side);
      if (editor) {
        editor.selection = new vscode.Selection(range.start, range.end);
        editor.revealRange(range, vscode.TextEditorRevealType.InCenter);
        return;
      }
      await new Promise((r) => setTimeout(r, 40));
    }
  }

  private findEditorForUri(
    uri: vscode.Uri,
    side?: MountableCommentAnchor['side'],
  ): vscode.TextEditor | undefined {
    for (const editor of vscode.window.visibleTextEditors) {
      if (this.urisMatch(editor.document.uri, uri)) return editor;
    }

    const tab = vscode.window.tabGroups.activeTabGroup.activeTab;
    if (tab?.input instanceof vscode.TabInputTextDiff) {
      const { original, modified } = tab.input;
      const prefer = side === 'left' ? original : side === 'right' ? modified : undefined;
      const candidates = prefer ? [prefer, original, modified] : [original, modified];
      for (const candidate of candidates) {
        if (!this.urisMatch(candidate, uri)) continue;
        const editor = vscode.window.visibleTextEditors.find(
          (ed) => this.urisMatch(ed.document.uri, candidate),
        );
        if (editor) return editor;
      }
    }
    return undefined;
  }

  private urisMatch(a: vscode.Uri, b: vscode.Uri): boolean {
    if (a.toString() === b.toString()) return true;
    if (a.scheme !== b.scheme) return false;
    if (a.scheme === 'git') {
      const refA = new URLSearchParams(a.query).get('ref');
      const refB = new URLSearchParams(b.query).get('ref');
      return a.path === b.path && refA === refB;
    }
    return a.fsPath === b.fsPath;
  }

  indexOfThread(thread: vscode.CommentThread): number {
    for (const [i, th] of this.threads) if (th === thread) return i;
    return -1;
  }

  clear(): void {
    this.threads.forEach((th) => th.dispose());
    this.threads.clear();
    this.mounts.clear();
    this.jumpable.clear();
    this.jumpBlockReasons.clear();
    this.comments = [];
    this.status.clear();
    this.offsets.clear();
    this.reviewContext = { mode: ReviewMode.Workspace };
  }

  dispose(): void {
    this.clear();
    this.controller.dispose();
  }
}
