// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { t, resolveLocale } from '@shared/i18n';
import * as vscode from 'vscode';
import { readFile } from 'fs/promises';
import { execFile } from 'child_process';
import { GitState, FileChange, ReviewMode, ReviewContext } from '@shared/types';
import { buildWorkspaceFiles, branchRefCandidates, parseNameStatus, pickRepoRoot } from './gitMap';

const WORKSPACE_REFRESH_DEBOUNCE_MS = 300;

export class GitService {
  private api: any | null = null;
  private cache: GitState = { branches: [], currentBranch: '', recentCommits: [], workspaceFiles: [] };
  private reviewFileStatus = new Map<string, FileChange['status']>();

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
   * Choose the repository matching the workspace. Nested repository ordering is unstable,
   * so taking [0] directly may select a child repository.
   */
  private selectRepo(api: any): any | null {
    const repos: any[] = api.repositories;
    if (!repos || repos.length === 0) return null;
    const ws = vscode.workspace.workspaceFolders?.[0].uri.fsPath;
    const root = pickRepoRoot(repos.map((r) => r.rootUri?.fsPath ?? ''), ws);
    return repos.find((r) => (r.rootUri?.fsPath ?? '') === root) ?? repos[0];
  }

  /** Wait for at least one repository (the Git extension scans asynchronously and may initially return none). */
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

    if (mode === ReviewMode.Workspace) {
      await this.refreshWorkspaceFiles();
      return { ...this.cache };
    }

    const repo = await this.waitForRepo();
    if (!repo) {
      this.trace(`getState(${mode}): no repo`);
      return empty;
    }

    try {
      this.cache.currentBranch = repo.state.HEAD?.name || '';
    } catch { /* ignore */ }

    if (mode === ReviewMode.Branch) {
      await this.refreshBranches(repo);
    } else if (mode === ReviewMode.Commit) {
      await this.refreshRecentCommits(repo);
    }

    return { ...this.cache };
  }

  /**
   * Subscribe to VS Code Git repository state changes and debounce workspace file list refreshes.
   * Keep staged, working tree, and untracked changes up to date in the sidebar workspace mode.
   */
  watchWorkspaceChanges(onUpdate: (state: GitState) => void): vscode.Disposable {
    const cleanups: vscode.Disposable[] = [];
    let debounceTimer: ReturnType<typeof setTimeout> | undefined;
    let repoDisposable: vscode.Disposable | undefined;
    let cancelled = false;

    const scheduleRefresh = () => {
      if (debounceTimer) clearTimeout(debounceTimer);
      debounceTimer = setTimeout(() => {
        debounceTimer = undefined;
        void this.refreshWorkspaceFiles().then(() => {
          if (!cancelled) onUpdate({ ...this.cache });
        });
      }, WORKSPACE_REFRESH_DEBOUNCE_MS);
    };

    const attachRepo = (repo: any) => {
      repoDisposable?.dispose();
      repoDisposable = repo?.state?.onDidChange?.(scheduleRefresh);
    };

    void this.ensureApi().then(async (api) => {
      if (!api || cancelled) return;

      const repo = await this.waitForRepo();
      if (!cancelled && repo) attachRepo(repo);

      if (api.onDidOpenRepository) {
        cleanups.push(api.onDidOpenRepository(() => {
          if (cancelled) return;
          const selected = this.selectRepo(api);
          if (selected) attachRepo(selected);
        }));
      }
    });

    return new vscode.Disposable(() => {
      cancelled = true;
      if (debounceTimer) clearTimeout(debounceTimer);
      repoDisposable?.dispose();
      for (const d of cleanups) d.dispose();
    });
  }

  /** In workspace mode, refresh only changed files without waiting for the Git extension or fetching branches and history. */
  private async refreshWorkspaceFiles(): Promise<void> {
    const root = await this.repoRootFast();
    if (!root) {
      this.cache.workspaceFiles = [];
      return;
    }

    try {
      const [diffHeadOut, untrackedOut] = await Promise.all([
        // HEAD does not exist before the first commit; fall back to the index below.
        runGit(root, ['diff', '--name-status', 'HEAD']).catch(() => ''),
        runGit(root, ['ls-files', '--others', '--exclude-standard']),
      ]);
      let diffCachedOut = '';
      if (!diffHeadOut.trim()) {
        diffCachedOut = await runGit(root, ['diff', '--cached', '--name-status']);
      }
      this.cache.workspaceFiles = buildWorkspaceFiles(diffHeadOut, diffCachedOut, untrackedOut);
      this.trace(`refreshWorkspaceFiles: root=${root} files=${this.cache.workspaceFiles.length}`);
    } catch (e) {
      this.trace(`refreshWorkspaceFiles failed: ${e instanceof Error ? e.message : String(e)}`);
      this.cache.workspaceFiles = [];
    }
  }

  /** Resolve the repository root with git rev-parse without waiting for VS Code Git extension initialization. */
  private async repoRootFast(): Promise<string | null> {
    const ws = vscode.workspace.workspaceFolders?.[0].uri.fsPath;
    if (!ws) return null;
    try {
      const out = await runGit(ws, ['rev-parse', '--show-toplevel']);
      const root = out.trim();
      return root || null;
    } catch {
      return null;
    }
  }

  private async refreshBranches(repo: any): Promise<void> {
    try {
      const refs = await repo.getBranches({ remote: true });
      this.cache.branches = refs.map((r: any) => r.name).filter(Boolean);
    } catch {
      this.cache.branches = [];
    }
  }

  private async refreshRecentCommits(repo: any): Promise<void> {
    try {
      const commits = await repo.log({ maxEntries: 20 });
      this.cache.recentCommits = commits.map((c: any) => ({
        sha: c.hash.slice(0, 7),
        message: c.message.split('\n')[0],
        relativeTime: formatRelative(c.authorDate),
      }));
    } catch {
      this.cache.recentCommits = [];
    }
  }

  /** Compare branches using a three-dot diff from the merge base. */
  async getBranchDiff(from: string, to: string): Promise<FileChange[]> {
    const root = await this.repoRoot();
    if (!root || !from || !to) return [];

    const resolvedFrom = await this.resolveGitRef(root, from);
    const resolvedTo = await this.resolveGitRef(root, to);
    if (!resolvedFrom || !resolvedTo) {
      this.trace(`getBranchDiff: unresolved ref from=${from} to=${to}`);
      return [];
    }

    try {
      const out = await runGit(root, ['diff', '--name-status', `${resolvedFrom}...${resolvedTo}`]);
      const files = parseNameStatus(out);
      this.trace(`getBranchDiff(${resolvedFrom}...${resolvedTo}): files=${files.length}`);
      return files;
    } catch (e) {
      this.trace(`getBranchDiff failed: ${e instanceof Error ? e.message : String(e)}`);
      return [];
    }
  }

  private async resolveGitRef(root: string, ref: string): Promise<string | null> {
    for (const candidate of branchRefCandidates(ref)) {
      try {
        const out = await runGit(root, ['rev-parse', '--verify', candidate]);
        if (out.trim()) return candidate;
      } catch { /* try next candidate */ }
    }
    return null;
  }

  /** List files changed in a single commit relative to its parent. */
  async getCommitFiles(sha: string): Promise<FileChange[]> {
    const root = await this.repoRoot();
    if (!root || !sha) return [];
    try {
      const out = await runGit(root, [
        'show',
        '--diff-merges=first-parent',
        '--name-status',
        '--format=',
        '--end-of-options',
        sha,
      ]);
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
    if (!repo) return this.repoRootFast();
    return repo.rootUri?.fsPath
      ?? vscode.workspace.workspaceFolders?.[0].uri.fsPath
      ?? process.cwd();
  }

  /** Open a review file in the native VS Code diff view. Each of the three modes determines its left and right sides. */
  async openDiff(opts: {
    path: string; status: FileChange['status'];
    mode: ReviewMode; from?: string; to?: string; commit?: string;
  }): Promise<void> {
    const api = await this.ensureApi();
    const root = await this.repoRoot();
    if (!api || !root) return;

    const fileUri = vscode.Uri.file(`${root}/${opts.path}`);

    if (opts.status === 'binary') {
      if (opts.mode === ReviewMode.Workspace) {
        try { await vscode.window.showTextDocument(fileUri, { preview: true }); } catch { /* ignore */ }
      }
      return;
    }

    if (opts.mode === ReviewMode.Workspace) {
      const emptyRef = '';
      const left = api.toGitUri(fileUri, opts.status === 'added' ? emptyRef : 'HEAD');
      const right = opts.status === 'deleted' ? api.toGitUri(fileUri, emptyRef) : fileUri;
      const label = t(resolveLocale(vscode.env.language), 'ext.git.workspaceVsHead');
      await this.presentDiff(left, right, `${opts.path} (${label})`);
      return;
    }

    if (opts.mode === ReviewMode.Commit && opts.commit) {
      const parent = `${opts.commit}^`;
      const label = `${opts.commit}^ ↔ ${opts.commit}`;
      const leftRef = opts.status === 'added' ? null : parent;
      const rightRef = opts.status === 'deleted' ? null : opts.commit;
      await this.presentRefDiff(api, root, opts.path, opts.status, leftRef, rightRef, `${opts.path} (${label})`, {
        fallbackRange: `${parent}..${opts.commit}`,
      });
      return;
    }

    if (opts.mode === ReviewMode.Branch && opts.from && opts.to) {
      const resolvedFrom = await this.resolveGitRef(root, opts.from);
      const resolvedTo = await this.resolveGitRef(root, opts.to);
      if (!resolvedFrom || !resolvedTo) return;
      const base = (await this.mergeBase(root, resolvedFrom, resolvedTo)) || resolvedFrom;
      const label = `${resolvedFrom}...${resolvedTo}`;
      const leftRef = opts.status === 'added' ? null : base;
      const rightRef = opts.status === 'deleted' ? null : resolvedTo;
      await this.presentRefDiff(api, root, opts.path, opts.status, leftRef, rightRef, `${opts.path} (${label})`, {
        fallbackRange: `${resolvedFrom}...${resolvedTo}`,
      });
    }
  }

  /** In branch or commit mode, build both sides from Git refs or empty files for the native VS Code diff editor. */
  private async presentRefDiff(
    api: any,
    root: string,
    relPath: string,
    status: FileChange['status'],
    leftRef: string | null,
    rightRef: string | null,
    title: string,
    opts: { fallbackRange: string },
  ): Promise<void> {
    const left = await this.resolveDiffSide(api, root, relPath, leftRef, status, 'left');
    const right = await this.resolveDiffSide(api, root, relPath, rightRef, status, 'right');
    const opened = await this.presentDiff(left, right, title);
    if (!opened) {
      await this.presentPatchDiff(root, opts.fallbackRange, relPath, title);
    }
  }

  private async resolveDiffSide(
    api: any,
    root: string,
    relPath: string,
    ref: string | null,
    status: FileChange['status'],
    side: 'left' | 'right',
  ): Promise<vscode.Uri> {
    if (side === 'left' && (status === 'added' || !ref)) return this.emptySideUri();
    if (side === 'right' && (status === 'deleted' || !ref)) return this.emptySideUri();
    if (!ref || !(await this.pathExistsAtRef(root, ref, relPath))) return this.emptySideUri();
    return api.toGitUri(vscode.Uri.file(`${root}/${relPath}`), ref);
  }

  /** Provide an empty side (equivalent to /dev/null) for diffs of added or deleted files. */
  private emptySideUri(): vscode.Uri {
    return vscode.Uri.file(process.platform === 'win32' ? '\\\\.\\NUL' : '/dev/null');
  }

  private async presentDiff(
    left: vscode.Uri,
    right: vscode.Uri,
    title: string,
  ): Promise<boolean> {
    try {
      await vscode.commands.executeCommand('vscode.diff', left, right, title, { preview: true });
      return true;
    } catch (e) {
      this.trace(`presentDiff failed: ${e instanceof Error ? e.message : String(e)}`);
      return false;
    }
  }

  /** As a last resort, show patch text only when the native diff fails. */
  private async presentPatchDiff(
    root: string,
    range: string,
    relPath: string,
    title: string,
  ): Promise<void> {
    let patch = '';
    try {
      patch = await runGit(root, ['diff', range, '--', relPath]);
    } catch (e) {
      this.trace(`presentPatchDiff failed: ${e instanceof Error ? e.message : String(e)}`);
    }
    const doc = await vscode.workspace.openTextDocument({
      content: patch || `# ${title}\n\n(no changes)`,
      language: 'diff',
    });
    await vscode.window.showTextDocument(doc, { preview: true });
  }

  private async pathExistsAtRef(root: string, ref: string, relPath: string): Promise<boolean> {
    try {
      await runGit(root, ['cat-file', '-e', `${ref}:${relPath}`]);
      return true;
    } catch {
      return false;
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

  /** Cache file statuses before a commit or branch review for use when attaching comments. */
  async prepareReviewFileStatus(ctx: ReviewContext): Promise<void> {
    this.reviewFileStatus.clear();
    if (ctx.mode === ReviewMode.Commit && ctx.commit) {
      const files = await this.getCommitFiles(ctx.commit);
      for (const f of files) this.reviewFileStatus.set(f.path, f.status);
    } else if (ctx.mode === ReviewMode.Branch && ctx.from && ctx.to) {
      const files = await this.getBranchDiff(ctx.from, ctx.to);
      for (const f of files) this.reviewFileStatus.set(f.path, f.status);
    }
  }

  async getReviewFileStatus(path: string): Promise<FileChange['status'] | null> {
    return this.reviewFileStatus.get(path) ?? null;
  }

  async readFileAtRef(ref: string, relPath: string): Promise<string | null> {
    const root = await this.repoRoot();
    if (!root) return null;
    try {
      return await runGit(root, ['show', `${ref}:${relPath}`]);
    } catch {
      return null;
    }
  }

  async readWorkspaceFile(relPath: string): Promise<string | null> {
    const root = await this.repoRoot();
    if (!root) return null;
    try {
      return await readFile(`${root}/${relPath}`, 'utf8');
    } catch {
      return null;
    }
  }

  /** Build the URIs for both diff sides and the Git ref used to attach comments. */
  async buildCommentDiffUris(
    relPath: string,
    status: FileChange['status'],
    ctx: ReviewContext,
  ): Promise<{
    left: vscode.Uri;
    right: vscode.Uri;
    title: string;
    mountRef: string;
    mountSide: 'left' | 'right';
    leftRef: string | null;
    rightRef: string | null;
  } | null> {
    const api = await this.ensureApi();
    const root = await this.repoRoot();
    if (!api || !root) return null;

    if (ctx.mode === ReviewMode.Commit && ctx.commit) {
      const parent = `${ctx.commit}^`;
      const label = `${ctx.commit}^ ↔ ${ctx.commit}`;
      const leftRef = status === 'added' ? null : parent;
      const rightRef = status === 'deleted' ? null : ctx.commit;
      const mountRef = status === 'deleted' ? parent : ctx.commit;
      const mountSide: 'left' | 'right' = status === 'deleted' ? 'left' : 'right';
      const left = await this.resolveDiffSide(api, root, relPath, leftRef, status, 'left');
      const right = await this.resolveDiffSide(api, root, relPath, rightRef, status, 'right');
      return { left, right, title: `${relPath} (${label})`, mountRef, mountSide, leftRef, rightRef };
    }

    if (ctx.mode === ReviewMode.Branch && ctx.from && ctx.to) {
      const resolvedFrom = await this.resolveGitRef(root, ctx.from);
      const resolvedTo = await this.resolveGitRef(root, ctx.to);
      if (!resolvedFrom || !resolvedTo) return null;
      const base = (await this.mergeBase(root, resolvedFrom, resolvedTo)) || resolvedFrom;
      const label = `${resolvedFrom}...${resolvedTo}`;
      const leftRef = status === 'added' ? null : base;
      const rightRef = status === 'deleted' ? null : resolvedTo;
      const mountRef = status === 'deleted' ? base : resolvedTo;
      const mountSide: 'left' | 'right' = status === 'deleted' ? 'left' : 'right';
      const left = await this.resolveDiffSide(api, root, relPath, leftRef, status, 'left');
      const right = await this.resolveDiffSide(api, root, relPath, rightRef, status, 'right');
      return { left, right, title: `${relPath} (${label})`, mountRef, mountSide, leftRef, rightRef };
    }

    return null;
  }

  async createGitFileUri(relPath: string, ref: string): Promise<vscode.Uri | null> {
    const api = await this.ensureApi();
    const root = await this.repoRoot();
    if (!api || !root) return null;
    return api.toGitUri(vscode.Uri.file(`${root}/${relPath}`), ref);
  }
}

function runGit(cwd: string, args: string[]): Promise<string> {
  return new Promise((resolve, reject) => {
    execFile('git', ['-c', 'core.quotepath=false', ...args], { cwd, maxBuffer: 10 * 1024 * 1024 }, (err, stdout) => {
      if (err) reject(err);
      else resolve(stdout);
    });
  });
}

function formatRelative(date?: Date): string {
  if (!date) return '';
  const locale = resolveLocale(vscode.env.language);
  const diff = Date.now() - date.getTime();
  const h = Math.floor(diff / 3.6e6);
  if (h < 1) return t(locale, 'ext.git.justNow');
  if (h === 1) return t(locale, 'ext.git.hourAgo');
  if (h < 24) return t(locale, 'ext.git.hoursAgo').replace('{h}', String(h));
  const d = Math.floor(h / 24);
  if (d === 1) return t(locale, 'ext.git.yesterday');
  return t(locale, 'ext.git.daysAgo').replace('{d}', String(d));
}
