// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { execFile } from 'child_process';
import { mkdir, mkdtemp, realpath, rm, writeFile } from 'fs/promises';
import { tmpdir } from 'os';
import path from 'path';
import { promisify } from 'util';
import * as vscode from 'vscode';
import { HostToWebview, WebviewToHost } from '@shared/messages';
import { ReviewMode } from '@shared/types';
import { initialState, reducer } from '../../../../../frontend/src/webview/store';
import { GitService } from '../../services/GitService';
import { CommentProvider } from '../CommentProvider';
import { SidebarProvider } from '../SidebarProvider';

const execFileAsync = promisify(execFile);
const result = { status: 'success', comments: [], warnings: [] };

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => { resolve = done; });
  return { promise, resolve };
}

describe('workspace selection', () => {
  let temp: string;
  let first: vscode.WorkspaceFolder;
  let second: vscode.WorkspaceFolder;
  const originalFolders = vscode.workspace.workspaceFolders;

  beforeEach(async () => {
    temp = await realpath(await mkdtemp(path.join(tmpdir(), 'ocr-workspaces-')));
    first = { name: 'first', index: 0, uri: vscode.Uri.file(path.join(temp, 'first')) };
    second = { name: 'second', index: 1, uri: vscode.Uri.file(path.join(temp, 'second')) };
    for (const folder of [first, second]) {
      await mkdir(folder.uri.fsPath);
      await execFileAsync('git', ['init', '-q'], { cwd: folder.uri.fsPath });
      await writeFile(path.join(folder.uri.fsPath, 'shared.ts'), `${folder.name} project\n`);
    }
    (vscode.workspace as any).workspaceFolders = [first, second];
    jest.spyOn(vscode.window, 'showWorkspaceFolderPick').mockResolvedValue(second);
  });

  afterEach(async () => {
    jest.restoreAllMocks();
    (vscode.workspace as any).workspaceFolders = originalFolders;
    await rm(temp, { recursive: true, force: true });
  });

  function sidebar(git = new GitService(), commentProvider?: CommentProvider) {
    const cli = { review: jest.fn().mockResolvedValue(result), cancel: jest.fn() };
    const comments = { onSync: jest.fn(), show: jest.fn(), clear: jest.fn() };
    const posts: HostToWebview[] = [];
    const watch = jest.spyOn(GitService.prototype, 'watchWorkspaceChanges')
      .mockImplementation(() => ({ dispose: jest.fn() }));
    let handle!: (message: WebviewToHost) => Promise<void>;
    const provider = new SidebarProvider(first.uri, cli as any, { read: () => null } as any, git, (commentProvider ?? comments) as any);
    provider.resolveWebviewView({
      webview: {
        asWebviewUri: (uri: vscode.Uri) => uri,
        onDidReceiveMessage: (callback: typeof handle) => { handle = callback; },
        postMessage: (message: HostToWebview) => { posts.push(message); },
      },
      onDidDispose: () => {},
    } as any);
    return { handle, cli, comments, posts, watch };
  }

  it('runs the CLI from the selected Git root when the workspace is a subdirectory', async () => {
    const nested = { ...second, uri: vscode.Uri.file(path.join(second.uri.fsPath, 'src')) };
    await mkdir(nested.uri.fsPath);
    const { handle, cli } = sidebar(new GitService(undefined, nested));

    await handle({ type: 'startReview', options: { mode: ReviewMode.Workspace } });

    expect(cli.review.mock.calls[0][1]).toBe(second.uri.fsPath);
  });

  it('switches the file list and watcher without publishing an old project response', async () => {
    const git = new GitService();
    const oldState = await git.getState(ReviewMode.Workspace);
    const pending = deferred<typeof oldState>();
    jest.spyOn(git, 'getState').mockReturnValue(pending.promise);
    const { handle, posts, comments, watch } = sidebar(git);
    const stale = handle({ type: 'getGitState', mode: ReviewMode.Workspace });

    await handle({ type: 'selectWorkspace' });
    pending.resolve(oldState);
    await stale;

    expect(posts).toHaveLength(1);
    expect(posts[0]).toMatchObject({
      type: 'workspaceChanged',
      gitState: { workspaceFolder: { path: second.uri.fsPath }, workspaceFiles: [{ path: 'shared.ts', status: 'added' }] },
    });
    expect(comments.clear).toHaveBeenCalled();
    expect(watch.mock.results[0].value.dispose).toHaveBeenCalled();
    expect(watch).toHaveBeenCalledTimes(2);
  });

  it('keeps the selected project through review completion and comment mounting', async () => {
    const git = new GitService(undefined, second);
    const { handle, cli, comments } = sidebar(git);
    const review = deferred<any>();
    const mounting = deferred<void>();
    cli.review.mockReturnValue(review.promise);
    const shown = deferred<void>();
    comments.show.mockImplementation(() => { shown.resolve(); return mounting.promise; });
    const running = handle({ type: 'startReview', options: { mode: ReviewMode.Workspace } });
    await handle({ type: 'selectWorkspace' });
    expect(vscode.window.showWorkspaceFolderPick).not.toHaveBeenCalled();

    review.resolve({ ...result, comments: [{ path: 'shared.ts', content: 'comment', startLine: 1, endLine: 1 }] });
    await shown.promise;
    await handle({ type: 'selectWorkspace' });
    expect(vscode.window.showWorkspaceFolderPick).not.toHaveBeenCalled();
    expect(comments.show.mock.calls[0][2]).toBe(git);
    mounting.resolve();
    await running;
  });

  it('keeps review results available when inline comment mounting fails', async () => {
    const { handle, cli, comments, posts } = sidebar();
    const reviewed = { ...result, comments: [{ path: 'shared.ts', content: 'Review finding', startLine: 1, endLine: 1 }] };
    cli.review.mockResolvedValue(reviewed);
    comments.show.mockRejectedValue(new Error('Editor is unavailable'));

    await expect(handle({ type: 'startReview', options: { mode: ReviewMode.Workspace } })).resolves.toBeUndefined();

    expect(posts).toContainEqual({ type: 'reviewDone', result: reviewed });
    expect(posts).toContainEqual({
      type: 'logLine', line: { level: 'warn', text: '[ocr] Unable to display inline comments: Editor is unavailable' },
    });
    await handle({ type: 'selectWorkspace' });
    expect(posts).toContainEqual(expect.objectContaining({ type: 'workspaceChanged' }));
  });

  it('keeps results visible without jump links when the repository disappears during review', async () => {
    const git = new GitService(undefined, second);
    const comments = new CommentProvider(first.uri, git);
    const { handle, cli, posts } = sidebar(git, comments);
    const reviewed = { ...result, comments: [{ path: 'shared.ts', content: 'Review finding', startLine: 1, endLine: 1 }] };
    cli.review.mockImplementation(async () => {
      await rm(path.join(second.uri.fsPath, '.git'), { recursive: true, force: true });
      return reviewed;
    });

    await handle({ type: 'startReview', options: { mode: ReviewMode.Workspace } });

    expect(posts).toContainEqual({
      type: 'commentSync', comments: [{ index: 0, status: 'pending', jumpable: false }],
    });
    const state = posts.reduce(reducer, initialState);
    expect(state.view).toBe('done');
    expect(state.session.result).toEqual(reviewed);
    expect(state.commentJumpable[0]).toBe(false);
    comments.dispose();
  });

  it('keeps visible project options tied to their repository while the next project loads', async () => {
    const git = new GitService();
    const selected = git.forWorkspace(second);
    const state = await selected.getState(ReviewMode.Workspace);
    const pending = deferred<typeof state>();
    const loading = deferred<void>();
    jest.spyOn(git, 'forWorkspace').mockReturnValue(selected);
    jest.spyOn(selected, 'getState').mockImplementation(() => { loading.resolve(); return pending.promise; });
    const { handle, cli } = sidebar(git);
    const switching = handle({ type: 'selectWorkspace' });
    await loading.promise;

    await handle({ type: 'startReview', options: { mode: ReviewMode.Workspace } });
    pending.resolve(state);
    await switching;

    expect(cli.review.mock.calls[0][1]).toBe(first.uri.fsPath);
  });

  it('does not start a review outside a Git repository', async () => {
    const git = new GitService(undefined, { name: 'non-git', index: 2, uri: vscode.Uri.file(temp) });
    const { handle, cli, posts } = sidebar(git);

    await handle({ type: 'startReview', options: { mode: ReviewMode.Workspace } });

    expect(cli.review).not.toHaveBeenCalled();
    expect(posts).toContainEqual(expect.objectContaining({ type: 'stateChange', state: 'failed' }));
  });

  it('mounts and applies comments in the reviewed repository after selection changes', async () => {
    const firstGit = new GitService();
    const secondGit = new GitService(undefined, second);
    const open = jest.spyOn(vscode.workspace, 'openTextDocument').mockResolvedValue({
      lineCount: 2, lineAt: () => ({ text: 'second project' }), save: async () => true,
    } as any);
    const apply = jest.spyOn(vscode.workspace, 'applyEdit').mockResolvedValue(true);
    const comments = new CommentProvider(first.uri, firstGit);
    await comments.show([{
      path: 'shared.ts', content: 'Use the corrected value.', suggestionCode: 'corrected', startLine: 1, endLine: 1,
    }], { mode: ReviewMode.Workspace }, secondGit);
    (vscode.workspace as any).workspaceFolders = [first];

    await comments.apply(0);

    expect(open.mock.calls.map(([uri]) => (uri as vscode.Uri).fsPath)).toEqual([
      `${second.uri.fsPath}/shared.ts`, `${second.uri.fsPath}/shared.ts`,
    ]);
    expect(apply).toHaveBeenCalledTimes(1);
    comments.dispose();
  });
});
