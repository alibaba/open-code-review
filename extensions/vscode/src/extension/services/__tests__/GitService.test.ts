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
import { EMPTY_SIDE_SCHEME, EmptySideProvider, emptySideUri } from '../../providers/EmptySideProvider';

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
  // #1816: the platform null device is not readable through VS Code's file
  // system provider, so `\\.\NUL` made the diff fail to open on Windows.
  //
  // The repair is not a git URI with an empty ref either. An empty treeish is
  // not a sentinel for "nothing" -- VS Code's getObjectDetails sends `!treeish`
  // to lsfiles, so it addresses the index. The two tests below pin the git-level
  // consequences that makes, which are why this uses its own scheme instead.
  it('resolves an added file\'s left side to an empty document', async () => {
    const uri: any = await (new GitService() as any)
      .resolveDiffSide({}, '/repo', 'added.ts', 'HEAD', 'added', 'left');

    // Pinned to the literal, not EMPTY_SIDE_SCHEME: an assertion against the
    // same constant the code uses moves with it and cannot fail.
    expect(uri.scheme).toBe('ocr-empty');
    expect(EMPTY_SIDE_SCHEME).toBe('ocr-empty');
    // 'git' is the spelling that looks right and resolves to the index.
    expect(uri.scheme).not.toBe('git');
    expect(new EmptySideProvider().provideTextDocumentContent()).toBe('');
    expect(String(uri.path)).not.toMatch(/NUL|dev\/null/);
  });

  it('resolves a deleted file\'s right side to an empty document', async () => {
    const uri: any = await (new GitService() as any)
      .resolveDiffSide({}, '/repo', 'gone.ts', 'HEAD', 'deleted', 'right');

    expect(uri.scheme).toBe('ocr-empty');
    expect(uri.scheme).not.toBe('git');
    expect(new EmptySideProvider().provideTextDocumentContent()).toBe('');
  });

  it('names the file in the URI so the diff title still reads sensibly', () => {
    expect(emptySideUri('src/added.ts').path).toBe('/src/added.ts');
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

      const calls: Array<{ fsPath: string; ref: string }> = [];
      const api = { toGitUri: (uri: any, ref: string) => { calls.push({ fsPath: uri.fsPath, ref }); return { scheme: 'git', fsPath: uri.fsPath, ref }; } };
      const uri: any = await (new GitService() as any)
        .resolveDiffSide(api, repoRoot, 'kept.ts', 'HEAD', 'modified', 'left');

      expect(uri.ref).toBe('HEAD');
      expect(calls).toEqual([{ fsPath: `${repoRoot}/kept.ts`, ref: 'HEAD' }]);
    } finally {
      await rm(repoRoot, { recursive: true, force: true });
    }
  });
});

// The reason the empty side cannot be `toGitUri(uri, '')`. VS Code resolves an
// empty treeish against the index, and these are what the index actually holds
// at the two moments the diff is opened. Asserting the git boundary directly
// rather than mocking it is the whole point: a mocked toGitUri would report
// success for either spelling.
describe('an empty git ref addresses the index, not nothing', () => {
  let repoRoot = '';
  const run = (args: string[]) => execFileAsync('git', args, { cwd: repoRoot });

  beforeEach(async () => {
    repoRoot = await mkdtemp(path.join(tmpdir(), 'ocr-vscode-empty-ref-'));
    await run(['init', '-q']);
    await run(['config', 'user.email', 'test@example.com']);
    await run(['config', 'user.name', 'Test User']);
  });
  afterEach(async () => { await rm(repoRoot, { recursive: true, force: true }); });

  it('holds the added file right after a root commit, so an empty ref would hide the addition', async () => {
    await writeFile(path.join(repoRoot, 'first.ts'), 'export const x = 1;\n');
    await run(['add', 'first.ts']);
    await run(['commit', '-q', '-m', 'root commit']);

    const fromIndex = (await run(['show', ':first.ts'])).stdout;
    const fromCommit = (await run(['show', 'HEAD:first.ts'])).stdout;

    expect(fromIndex).toBe(fromCommit);
    expect(fromIndex.trim()).not.toBe('');
  });

  it('has no entry after a committed deletion, so an empty ref would fail to open', async () => {
    await writeFile(path.join(repoRoot, 'gone.ts'), 'export const x = 1;\n');
    await run(['add', 'gone.ts']);
    await run(['commit', '-q', '-m', 'add']);
    await run(['rm', '-q', 'gone.ts']);
    await run(['commit', '-q', '-m', 'delete']);

    await expect(run(['show', ':gone.ts'])).rejects.toThrow();
  });

  it('bases a root commit on a real, empty tree object in this repository', async () => {
    await writeFile(path.join(repoRoot, 'first.ts'), 'export const x = 1;\n');
    await run(['add', 'first.ts']);
    await run(['commit', '-q', '-m', 'root commit']);

    const base = await (new GitService() as any).commitBaseRef(repoRoot, 'HEAD');

    // Asserted as properties rather than against the SHA-1 constant
    // `4b825dc...`: a repository created with --object-format=sha256 has a
    // different empty tree, and hardcoding either one is the bug this avoids.
    expect((await run(['cat-file', '-t', base])).stdout.trim()).toBe('tree');
    expect((await run(['ls-tree', base])).stdout).toBe('');
    expect((await run(['rev-parse', '--verify', base])).stdout.trim()).toBe(base);
    expect((await run(['diff', '--name-status', `${base}..HEAD`])).stdout).toContain('A\tfirst.ts');
  });
});
