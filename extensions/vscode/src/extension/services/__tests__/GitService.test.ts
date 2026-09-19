// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { execFile } from 'child_process';
import { mkdir, mkdtemp, realpath, rm, writeFile } from 'fs/promises';
import { tmpdir } from 'os';
import path from 'path';
import { promisify } from 'util';
import * as vscode from 'vscode';
import { ReviewMode } from '@shared/types';
import { GitService } from '../GitService';

const execFileAsync = promisify(execFile);

describe('GitService workspace files', () => {
  let repoRoot: string;
  const originalWorkspaceFolders = vscode.workspace.workspaceFolders;

  beforeEach(async () => {
    repoRoot = await realpath(await mkdtemp(path.join(tmpdir(), 'ocr-vscode-git-')));
    await execFileAsync('git', ['init', '-q'], { cwd: repoRoot });
    (vscode.workspace as any).workspaceFolders = [{ uri: { fsPath: repoRoot } }];
  });

  afterEach(async () => {
    jest.restoreAllMocks();
    (vscode.workspace as any).workspaceFolders = originalWorkspaceFolders;
    await rm(repoRoot, { recursive: true, force: true });
  });

  it('uses the selected workspace for changed files, contents, and diff URIs', async () => {
    const secondRoot = path.join(repoRoot, 'second');
    await mkdir(secondRoot);
    await execFileAsync('git', ['init', '-q'], { cwd: secondRoot });
    await writeFile(path.join(repoRoot, 'first.ts'), 'first project\n');
    await writeFile(path.join(secondRoot, 'second.ts'), 'second project\n');
    const secondFolder = { name: 'second', index: 1, uri: vscode.Uri.file(secondRoot) };
    (vscode.workspace as any).workspaceFolders.push(secondFolder);
    const toGitUri = (uri: vscode.Uri, ref: string) => ({ ...uri, scheme: 'git', ref });
    jest.spyOn(vscode.extensions, 'getExtension').mockReturnValue({
      isActive: true,
      exports: { getAPI: () => ({ repositories: [{ rootUri: vscode.Uri.file(repoRoot) }], toGitUri }) },
    } as any);
    const openDiff = jest.spyOn(vscode.commands, 'executeCommand');

    const git = new GitService(undefined, secondFolder);
    const state = await git.getState(ReviewMode.Workspace);

    expect(state.workspaceFiles).toEqual([{ path: 'second.ts', status: 'added' }]);
    expect(await git.readWorkspaceFile('second.ts')).toBe('second project\n');
    await git.openDiff({ path: 'second.ts', status: 'added', mode: ReviewMode.Workspace });
    expect(openDiff.mock.calls[0][2]).toEqual(vscode.Uri.file(`${secondRoot}/second.ts`));
  });

  it('uses the selected repository for branch and commit metadata', async () => {
    const secondRoot = path.join(repoRoot, 'second');
    await mkdir(secondRoot);
    await execFileAsync('git', ['init', '-q'], { cwd: secondRoot });
    const secondFolder = { name: 'second', index: 1, uri: vscode.Uri.file(secondRoot) };
    const repositories = [repoRoot, secondRoot].map((root) => ({
      rootUri: vscode.Uri.file(root),
      state: { HEAD: { name: root === secondRoot ? 'second-main' : 'first-main' } },
      getBranches: async () => [{ name: root === secondRoot ? 'second-feature' : 'first-feature' }],
      log: async () => [{ hash: '1234567890', message: root === secondRoot ? 'second commit' : 'first commit' }],
    }));
    jest.spyOn(vscode.extensions, 'getExtension').mockReturnValue({
      isActive: true, exports: { getAPI: () => ({ repositories }) },
    } as any);
    const git = new GitService(undefined, secondFolder);

    expect(await git.getState(ReviewMode.Branch)).toMatchObject({
      currentBranch: 'second-main', branches: ['second-feature'],
    });
    expect((await git.getState(ReviewMode.Commit)).recentCommits[0].message).toBe('second commit');
  });

  it.each(['already discovered', 'open event', 'polling'])('waits for the selected repository when an unrelated one is %s', async (discovery) => {
    const unrelated = { rootUri: vscode.Uri.file('/unrelated') };
    const selected = {
      rootUri: vscode.Uri.file(repoRoot),
      state: { HEAD: { name: 'selected-main' } },
      getBranches: async () => [{ name: 'selected-feature' }],
    };
    let onOpen = () => {};
    const api = {
      repositories: discovery === 'already discovered' ? [unrelated] : [],
      onDidOpenRepository: (listener: () => void) => { onOpen = listener; return { dispose: () => {} }; },
    };
    jest.spyOn(vscode.extensions, 'getExtension').mockReturnValue({
      isActive: true, exports: { getAPI: () => api },
    } as any);
    const pending = new GitService().getState(ReviewMode.Branch);
    await new Promise((resolve) => setImmediate(resolve));
    if (discovery !== 'already discovered') {
      api.repositories.push(unrelated);
      onOpen();
      await new Promise((resolve) => setImmediate(resolve));
    }
    api.repositories.push(selected);
    if (discovery !== 'polling') onOpen();

    expect(await pending).toMatchObject({ currentBranch: 'selected-main', branches: ['selected-feature'] });
  });

  it('does not display parent repository refs before a nested repository is discovered', async () => {
    const secondRoot = path.join(repoRoot, 'second');
    await mkdir(secondRoot);
    await execFileAsync('git', ['init', '-q'], { cwd: secondRoot });
    jest.spyOn(vscode.extensions, 'getExtension').mockReturnValue({
      isActive: true,
      exports: { getAPI: () => ({ repositories: [{
        rootUri: vscode.Uri.file(repoRoot),
        state: { HEAD: { name: 'parent-main' } },
        getBranches: async () => [{ name: 'parent-feature' }],
      }] }) },
    } as any);

    const git = new GitService(undefined, { name: 'second', index: 1, uri: vscode.Uri.file(secondRoot) });

    expect(await git.getState(ReviewMode.Branch)).toMatchObject({ currentBranch: '', branches: [] });
  });

  it('does not use another repository when the selected folder is not a Git repository', async () => {
    const folder = { name: 'non-git', index: 1, uri: vscode.Uri.file(tmpdir()) };
    jest.spyOn(vscode.extensions, 'getExtension').mockReturnValue({
      isActive: true,
      exports: { getAPI: () => ({ repositories: [{ rootUri: vscode.Uri.file(repoRoot) }] }) },
    } as any);

    expect(await new GitService(undefined, folder).readWorkspaceFile('first.ts')).toBeNull();
    expect(await new GitService(undefined, folder).getRepositoryRoot()).toBeNull();
  });

  it('includes staged and untracked files before the first commit', async () => {
    await writeFile(path.join(repoRoot, 'staged.ts'), 'export const staged = true;\n');
    await execFileAsync('git', ['add', 'staged.ts'], { cwd: repoRoot });
    await writeFile(path.join(repoRoot, 'untracked.ts'), 'export const untracked = true;\n');

    const state = await new GitService().getState(ReviewMode.Workspace);

    expect(state.workspaceFiles).toEqual([
      { path: 'staged.ts', status: 'added' },
      { path: 'untracked.ts', status: 'added' },
    ]);
  });

  it('lists merge commit files relative to the first parent', async () => {
    await execFileAsync('git', ['config', 'user.email', 'test@example.com'], { cwd: repoRoot });
    await execFileAsync('git', ['config', 'user.name', 'Test User'], { cwd: repoRoot });

    await writeFile(path.join(repoRoot, 'base.ts'), 'export const base = true;\n');
    await execFileAsync('git', ['add', 'base.ts'], { cwd: repoRoot });
    await execFileAsync('git', ['commit', '-q', '-m', 'base'], { cwd: repoRoot });
    await execFileAsync('git', ['branch', '-M', 'main'], { cwd: repoRoot });

    await execFileAsync('git', ['checkout', '-q', '-b', 'feature'], { cwd: repoRoot });
    await writeFile(path.join(repoRoot, 'feature.ts'), 'export const feature = true;\n');
    await execFileAsync('git', ['add', 'feature.ts'], { cwd: repoRoot });
    await execFileAsync('git', ['commit', '-q', '-m', 'feature'], { cwd: repoRoot });

    await execFileAsync('git', ['checkout', '-q', 'main'], { cwd: repoRoot });
    await writeFile(path.join(repoRoot, 'main.ts'), 'export const main = true;\n');
    await execFileAsync('git', ['add', 'main.ts'], { cwd: repoRoot });
    await execFileAsync('git', ['commit', '-q', '-m', 'main'], { cwd: repoRoot });
    await execFileAsync('git', ['merge', '--no-ff', '-q', 'feature', '-m', 'merge'], { cwd: repoRoot });

    const mergeSha = (await execFileAsync('git', ['rev-parse', 'HEAD'], { cwd: repoRoot })).stdout.trim();
    const files = await new GitService().getCommitFiles(mergeSha);

    expect(files).toEqual([{ path: 'feature.ts', status: 'added' }]);
  });
});
