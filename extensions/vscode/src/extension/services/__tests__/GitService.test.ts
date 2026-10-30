// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { execFile } from 'child_process';
import { mkdtemp, rm, writeFile } from 'fs/promises';
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
    repoRoot = await mkdtemp(path.join(tmpdir(), 'ocr-vscode-git-'));
    await execFileAsync('git', ['init', '-q'], { cwd: repoRoot });
    (vscode.workspace as any).workspaceFolders = [{ uri: { fsPath: repoRoot } }];
  });

  afterEach(async () => {
    (vscode.workspace as any).workspaceFolders = originalWorkspaceFolders;
    await rm(repoRoot, { recursive: true, force: true });
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

describe('GitService commit base ref', () => {
  let repoRoot: string;
  const originalWorkspaceFolders = vscode.workspace.workspaceFolders;
  const EMPTY_TREE = '4b825dc642cb6eb9a060e54bf8d69288fbee4904';

  const git = (args: string[]) => execFileAsync('git', args, { cwd: repoRoot });

  beforeEach(async () => {
    repoRoot = await mkdtemp(path.join(tmpdir(), 'ocr-vscode-git-base-'));
    await git(['init', '-q']);
    await git(['config', 'user.email', 'test@example.com']);
    await git(['config', 'user.name', 'Test User']);
    (vscode.workspace as any).workspaceFolders = [{ uri: { fsPath: repoRoot } }];
  });

  afterEach(async () => {
    (vscode.workspace as any).workspaceFolders = originalWorkspaceFolders;
    await rm(repoRoot, { recursive: true, force: true });
  });

  const commit = async (file: string, message: string) => {
    await writeFile(path.join(repoRoot, file), `export const x = '${file}';\n`);
    await git(['add', file]);
    await git(['commit', '-q', '-m', message]);
    return (await git(['rev-parse', 'HEAD'])).stdout.trim();
  };

  // `<sha>^` does not resolve for an initial commit, so building it blindly
  // leaves the diff pointing at a revision that does not exist.
  it('falls back to the empty tree for a commit with no parent', async () => {
    const root = await commit('first.ts', 'root commit');

    const base = await (new GitService() as any).commitBaseRef(repoRoot, root);

    expect(base).toBe(EMPTY_TREE);
    // The fallback must be a range git can actually diff, which is the whole
    // point of choosing the empty tree over `<sha>^`.
    const out = await git(['diff', '--name-status', `${base}..${root}`]);
    expect(out.stdout.trim()).toBe('A\tfirst.ts');
  });

  it('uses the first parent for a commit that has one', async () => {
    await commit('first.ts', 'root commit');
    const second = await commit('second.ts', 'second commit');

    const base = await (new GitService() as any).commitBaseRef(repoRoot, second);

    expect(base).toBe(`${second}^`);
    const out = await git(['diff', '--name-status', `${base}..${second}`]);
    expect(out.stdout.trim()).toBe('A\tsecond.ts');
  });

  // A merge keeps the first-parent base, matching getCommitFiles.
  it('uses the first parent for a merge commit', async () => {
    await commit('base.ts', 'base');
    await git(['branch', '-M', 'main']);
    await git(['checkout', '-q', '-b', 'feature']);
    await commit('feature.ts', 'feature');
    await git(['checkout', '-q', 'main']);
    await commit('main.ts', 'main');
    await git(['merge', '--no-ff', '-q', 'feature', '-m', 'merge']);
    const merge = (await git(['rev-parse', 'HEAD'])).stdout.trim();

    const base = await (new GitService() as any).commitBaseRef(repoRoot, merge);

    expect(base).toBe(`${merge}^`);
  });
});

describe('GitService empty diff side', () => {
  // VS Code opens both sides of a diff through its file system provider. The
  // null device is not readable through it on Windows -- the reported symptom
  // in #1816 is a failed read of `\\.\NUL` -- so the empty side has to be a
  // git URI with an empty ref, the way workspace mode already builds it.
  const fakeApi = () => {
    const calls: Array<{ fsPath: string; ref: string }> = [];
    return {
      calls,
      api: { toGitUri: (uri: any, ref: string) => { calls.push({ fsPath: uri.fsPath, ref }); return { scheme: 'git', fsPath: uri.fsPath, ref }; } },
    };
  };

  it('represents an added file\'s left side as an empty git ref, not a device path', async () => {
    const { api, calls } = fakeApi();

    const uri: any = await (new GitService() as any)
      .resolveDiffSide(api, '/repo', 'added.ts', 'HEAD', 'added', 'left');

    expect(uri.scheme).toBe('git');
    expect(uri.ref).toBe('');
    expect(calls).toEqual([{ fsPath: '/repo/added.ts', ref: '' }]);
    expect(String(uri.fsPath)).not.toMatch(/NUL|dev\/null/);
  });

  it('represents a deleted file\'s right side as an empty git ref', async () => {
    const { api } = fakeApi();

    const uri: any = await (new GitService() as any)
      .resolveDiffSide(api, '/repo', 'gone.ts', 'HEAD', 'deleted', 'right');

    expect(uri.scheme).toBe('git');
    expect(uri.ref).toBe('');
  });

  it('still builds a real git side when the file exists at the ref', async () => {
    const repoRoot = await mkdtemp(path.join(tmpdir(), 'ocr-vscode-git-side-'));
    try {
      const run = (args: string[]) => execFileAsync('git', args, { cwd: repoRoot });
      await run(['init', '-q']);
      await run(['config', 'user.email', 'test@example.com']);
      await run(['config', 'user.name', 'Test User']);
      await writeFile(path.join(repoRoot, 'kept.ts'), 'export const kept = true;\n');
      await run(['add', 'kept.ts']);
      await run(['commit', '-q', '-m', 'kept']);

      const { api } = fakeApi();
      const uri: any = await (new GitService() as any)
        .resolveDiffSide(api, repoRoot, 'kept.ts', 'HEAD', 'modified', 'left');

      expect(uri.ref).toBe('HEAD');
    } finally {
      await rm(repoRoot, { recursive: true, force: true });
    }
  });
});
