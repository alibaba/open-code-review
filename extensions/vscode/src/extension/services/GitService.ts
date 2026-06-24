import * as vscode from 'vscode';
import { execFile } from 'child_process';
import { GitState, CommitInfo, FileChange, ReviewMode } from '../../shared/types';
import { parsePorcelain, parseNameStatus, pickRepoRoot } from './gitMap';

export class GitService {
  private api: any | null = null;

  constructor(private log?: vscode.OutputChannel) {}

  private trace(msg: string): void {
    this.log?.appendLine(`[git] ${msg}`);
  }

  private async ensureApi(): Promise<any | null> {
    if (this.api) return this.api;
    const ext = vscode.extensions.getExtension('vscode.git');
    if (!ext) return null;
    const exports = ext.isActive ? ext.exports : await ext.activate();
    if (!exports?.getAPI) return null;
    this.api = exports.getAPI(1);
    return this.api;
  }

  /**
   * 选出与 workspace 匹配的仓库。嵌套仓库场景下 repositories 顺序不稳定,
   * 不能直接取 [0],否则会漂移到子仓库。
   */
  private selectRepo(api: any): any | null {
    const repos: any[] = api.repositories;
    if (!repos || repos.length === 0) return null;
    const ws = vscode.workspace.workspaceFolders?.[0].uri.fsPath;
    const root = pickRepoRoot(repos.map((r) => r.rootUri?.fsPath ?? ''), ws);
    return repos.find((r) => (r.rootUri?.fsPath ?? '') === root) ?? repos[0];
  }

  /** 等待至少一个仓库就绪（git 扩展异步扫描，首次可能为空）。 */
  private async waitForRepo(timeoutMs = 5000): Promise<any | null> {
    const api = await this.ensureApi();
    if (!api) return null;
    if (api.repositories.length > 0) return this.selectRepo(api);

    return new Promise((resolve) => {
      let done = false;
      const finish = (repo: any | null) => {
        if (done) return;
        done = true;
        disposable?.dispose();
        clearInterval(poll);
        clearTimeout(timer);
        resolve(repo);
      };
      const disposable = api.onDidOpenRepository?.(() => finish(this.selectRepo(api)));
      const poll = setInterval(() => {
        if (api.repositories.length > 0) finish(this.selectRepo(api));
      }, 200);
      const timer = setTimeout(() => finish(this.selectRepo(api)), timeoutMs);
    });
  }

  async getState(mode: ReviewMode): Promise<GitState> {
    const empty: GitState = { branches: [], currentBranch: '', recentCommits: [], workspaceFiles: [] };
    const repo = await this.waitForRepo();
    if (!repo) {
      this.trace(`getState(${mode}): no repo`);
      return empty;
    }

    let currentBranch = '';
    try {
      currentBranch = repo.state.HEAD?.name || '';
    } catch { /* ignore */ }

    let branches: string[] = [];
    try {
      const refs = await repo.getBranches({ remote: true });
      branches = refs.map((r: any) => r.name).filter(Boolean);
    } catch { /* ignore */ }

    let recentCommits: CommitInfo[] = [];
    try {
      const commits = await repo.log({ maxEntries: 20 });
      recentCommits = commits.map((c: any) => ({
        sha: c.hash.slice(0, 7),
        message: c.message.split('\n')[0],
        relativeTime: formatRelative(c.authorDate),
      }));
    } catch { /* ignore */ }

    // 工作区变更直接走 git status --porcelain，避免依赖扩展懒填充的 state 数组。
    let workspaceFiles: FileChange[] = [];
    try {
      const root: string = repo.rootUri?.fsPath
        ?? vscode.workspace.workspaceFolders?.[0].uri.fsPath
        ?? process.cwd();
      const out = await runGit(root, ['status', '--porcelain']);
      workspaceFiles = parsePorcelain(out);
      this.trace(`getState(${mode}): root=${root} porcelainBytes=${out.length} files=${workspaceFiles.length}`);
    } catch (e) {
      this.trace(`getState(${mode}): status failed: ${e instanceof Error ? e.message : String(e)}`);
    }

    return { branches, currentBranch, recentCommits, workspaceFiles };
  }

  /** 分支对比：merge-base 三点 diff。 */
  async getBranchDiff(from: string, to: string): Promise<FileChange[]> {
    const root = await this.repoRoot();
    if (!root || !from || !to) return [];
    try {
      const out = await runGit(root, ['diff', '--name-status', `${from}...${to}`]);
      const files = parseNameStatus(out);
      this.trace(`getBranchDiff(${from}...${to}): files=${files.length}`);
      return files;
    } catch (e) {
      this.trace(`getBranchDiff failed: ${e instanceof Error ? e.message : String(e)}`);
      return [];
    }
  }

  /** 单次提交：该 commit 相对父提交的改动文件。 */
  async getCommitFiles(sha: string): Promise<FileChange[]> {
    const root = await this.repoRoot();
    if (!root || !sha) return [];
    try {
      const out = await runGit(root, ['show', '--name-status', '--format=', '--', sha]);
      const files = parseNameStatus(out);
      this.trace(`getCommitFiles(${sha}): files=${files.length}`);
      return files;
    } catch (e) {
      this.trace(`getCommitFiles failed: ${e instanceof Error ? e.message : String(e)}`);
      return [];
    }
  }

  private async repoRoot(): Promise<string | null> {
    const repo = await this.waitForRepo();
    if (!repo) return null;
    return repo.rootUri?.fsPath
      ?? vscode.workspace.workspaceFolders?.[0].uri.fsPath
      ?? process.cwd();
  }

  /** 在 VSCode 原生 diff 视图中打开某个待审查文件。三种模式各自决定 diff 的左右两侧。 */
  async openDiff(opts: {
    path: string; status: FileChange['status'];
    mode: ReviewMode; from?: string; to?: string; commit?: string;
  }): Promise<void> {
    const api = await this.ensureApi();
    const root = await this.repoRoot();
    if (!api || !root) return;

    const fileUri = vscode.Uri.file(`${root}/${opts.path}`);

    // 二进制无法做文本 diff，直接打开文件本身。
    if (opts.status === 'binary') {
      try { await vscode.window.showTextDocument(fileUri, { preview: true }); } catch { /* ignore */ }
      return;
    }

    // toGitUri(uri, '') 返回空文档，用于新增/删除时缺失的一侧。
    const emptyRef = '';
    let left: vscode.Uri;
    let right: vscode.Uri;
    let label: string;

    if (opts.mode === 'workspace') {
      left = api.toGitUri(fileUri, opts.status === 'added' ? emptyRef : 'HEAD');
      right = opts.status === 'deleted' ? api.toGitUri(fileUri, emptyRef) : fileUri;
      label = '工作区 ↔ HEAD';
    } else if (opts.mode === 'commit' && opts.commit) {
      left = api.toGitUri(fileUri, opts.status === 'added' ? emptyRef : `${opts.commit}^`);
      right = opts.status === 'deleted' ? api.toGitUri(fileUri, emptyRef) : api.toGitUri(fileUri, opts.commit);
      label = `${opts.commit}^ ↔ ${opts.commit}`;
    } else if (opts.mode === 'branch' && opts.from && opts.to) {
      // 文件列表用三点 diff（merge-base），逐文件 diff 也应以 merge-base 为基准。
      const base = (await this.mergeBase(root, opts.from, opts.to)) || opts.from;
      left = api.toGitUri(fileUri, opts.status === 'added' ? emptyRef : base);
      right = opts.status === 'deleted' ? api.toGitUri(fileUri, emptyRef) : api.toGitUri(fileUri, opts.to);
      label = `${opts.from}...${opts.to}`;
    } else {
      return;
    }

    const title = `${opts.path} (${label})`;
    try {
      await vscode.commands.executeCommand('vscode.diff', left, right, title, { preview: true });
    } catch (e) {
      this.trace(`openDiff failed: ${e instanceof Error ? e.message : String(e)}`);
    }
  }

  private async mergeBase(root: string, from: string, to: string): Promise<string | null> {
    try {
      const out = await runGit(root, ['merge-base', from, to]);
      return out.trim() || null;
    } catch {
      return null;
    }
  }
}

function runGit(cwd: string, args: string[]): Promise<string> {
  return new Promise((resolve, reject) => {
    execFile('git', args, { cwd, maxBuffer: 10 * 1024 * 1024 }, (err, stdout) => {
      if (err) reject(err);
      else resolve(stdout);
    });
  });
}

function formatRelative(date?: Date): string {
  if (!date) return '';
  const diff = Date.now() - date.getTime();
  const h = Math.floor(diff / 3.6e6);
  if (h < 1) return '刚刚';
  if (h < 24) return `${h} 小时前`;
  const d = Math.floor(h / 24);
  if (d === 1) return '昨天';
  return `${d} 天前`;
}
