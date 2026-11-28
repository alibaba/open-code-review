// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { execFile, spawn } from 'child_process';
import { mkdtemp, rm, writeFile } from 'fs/promises';
import { tmpdir } from 'os';
import path from 'path';
import { promisify } from 'util';
import * as vscode from 'vscode';
import { ReviewMode } from '@shared/types';
import { commitDiffRefs, GitService } from '../GitService';

const execFileAsync = promisify(execFile);

describe('commitDiffRefs', () => {
  it('keeps the historical refs, mount point, and label for commits with a parent', () => {
    expect(commitDiffRefs('abc123', 'modified', true)).toEqual({
      leftRef: 'abc123^',
      rightRef: 'abc123',
      mountRef: 'abc123',
      mountSide: 'right',
      label: 'abc123^ ↔ abc123',
      rootLeft: false,
    });
    expect(commitDiffRefs('abc123', 'added', true)).toEqual({
      leftRef: null,
      rightRef: 'abc123',
      mountRef: 'abc123',
      mountSide: 'right',
      label: 'abc123^ ↔ abc123',
      rootLeft: false,
    });
    expect(commitDiffRefs('abc123', 'deleted', true)).toEqual({
      leftRef: 'abc123^',
      rightRef: null,
      mountRef: 'abc123^',
      mountSide: 'left',
      label: 'abc123^ ↔ abc123',
      rootLeft: false,
    });
  });

  it('keeps root-commit diffs viewable without dead parent refs', () => {
    expect(commitDiffRefs('abc123', 'added', false)).toEqual({
      leftRef: null,
      rightRef: 'abc123',
      mountRef: 'abc123',
      mountSide: 'right',
      label: 'abc123 ↔ abc123',
      rootLeft: true,
    });
    expect(commitDiffRefs('abc123', 'modified', false)).toEqual({
      leftRef: null,
      rightRef: 'abc123',
      mountRef: 'abc123',
      mountSide: 'right',
      label: 'abc123 ↔ abc123',
      rootLeft: true,
    });
    expect(commitDiffRefs('abc123', 'deleted', false)).toEqual({
      leftRef: null,
      rightRef: null,
      mountRef: 'abc123',
      mountSide: 'left',
      label: 'abc123 ↔ abc123',
      rootLeft: true,
    });
  });
});

describe('GitService commit-mode diffs', () => {
  let repoRoot: string;
  const originalWorkspaceFolders = vscode.workspace.workspaceFolders;
  const originalGetExtension = vscode.extensions.getExtension;

  const fakeGitApi = () => ({
    repositories: [{ rootUri: { fsPath: repoRoot } }],
    toGitUri: (uri: { fsPath: string }, ref: string) => ({ scheme: 'git', fsPath: uri.fsPath, ref }),
  });

  beforeEach(async () => {
    repoRoot = await mkdtemp(path.join(tmpdir(), 'ocr-vscode-commit-'));
    await execFileAsync('git', ['init', '-q'], { cwd: repoRoot });
    await execFileAsync('git', ['config', 'user.email', 'test@example.com'], { cwd: repoRoot });
    await execFileAsync('git', ['config', 'user.name', 'Test User'], { cwd: repoRoot });
    (vscode.workspace as any).workspaceFolders = [{ uri: { fsPath: repoRoot } }];
    (vscode.extensions as any).getExtension = (id: string) => (
      id === 'vscode.git' ? { isActive: true, exports: { getAPI: () => fakeGitApi() } } : null
    );
  });

  afterEach(async () => {
    (vscode.workspace as any).workspaceFolders = originalWorkspaceFolders;
    (vscode.extensions as any).getExtension = originalGetExtension;
    await rm(repoRoot, { recursive: true, force: true });
  });

  // spawn with a closed stdin: execFile keeps the stdin pipe open and
  // `hash-object --stdin` would wait for EOF forever.
  const emptyTreeSha = async () =>
    await new Promise<string>((resolve, reject) => {
      const child = spawn('git', ['hash-object', '-t', 'tree', '--stdin'], { cwd: repoRoot, stdio: ['ignore', 'pipe', 'ignore'] });
      let out = '';
      child.stdout!.on('data', (d: Buffer) => { out += d; });
      child.on('error', reject);
      child.on('close', (code) => { if (code === 0) resolve(out.trim()); else reject(new Error(`exit ${code}`)); });
    });

  const commitFile = async (name: string, second: boolean) => {
    await writeFile(path.join(repoRoot, name), `export const ${name.replace('.ts', '')} = ${second ? 'v2' : 'v1'};\n`);
    await execFileAsync('git', ['add', name], { cwd: repoRoot });
    await execFileAsync('git', ['commit', '-q', '-m', `add ${name}`], { cwd: repoRoot });
    return (await execFileAsync('git', ['rev-parse', 'HEAD'], { cwd: repoRoot })).stdout.trim();
  };

  it('treats a root commit as fully added: empty-tree left side and no dead parent refs', async () => {
    const rootSha = await commitFile('root.ts', false);
    const emptyTree = await emptyTreeSha();
    const service = new GitService();

    const uris = await service.buildCommentDiffUris('root.ts', 'added', { mode: ReviewMode.Commit, commit: rootSha });

    expect(uris).not.toBeNull();
    expect(uris!.title).toBe(`root.ts (${rootSha} ↔ ${rootSha})`);
    expect(uris!.leftRef).toBeNull();
    expect(uris!.mountRef).toBe(rootSha);
    expect(uris!.mountSide).toBe('right');
    const left = uris!.left as unknown as { scheme: string; ref: string; fsPath: string };
    expect(left.scheme).toBe('git');
    // The empty-tree id — VS Code's git extension renders it as zero bytes,
    // so the file shows as fully added (an empty ref would render the index
    // content instead).
    expect(left.ref).toBe(emptyTree);
    expect(left.fsPath).toContain('root.ts');
    const right = uris!.right as unknown as { ref: string };
    expect(right.ref).toBe(rootSha);
  });

  it('opens the sidebar diff for a root commit with the same empty-tree left side', async () => {
    const rootSha = await commitFile('root.ts', false);
    const emptyTree = await emptyTreeSha();
    const calls: Array<[unknown, unknown, string]> = [];
    const originalExecute = vscode.commands.executeCommand;
    (vscode.commands as any).executeCommand = async (command: string, left: unknown, right: unknown, title: string) => {
      calls.push([left, right, title]);
    };

    try {
      await new GitService().openDiff({ path: 'root.ts', status: 'added', mode: ReviewMode.Commit, commit: rootSha });
    } finally {
      (vscode.commands as any).executeCommand = originalExecute;
    }

    expect(calls).toHaveLength(1);
    const [left, right, title] = calls[0];
    expect(title).toBe(`root.ts (${rootSha} ↔ ${rootSha})`);
    expect((left as { scheme: string; ref: string }).ref).toBe(emptyTree);
    expect((right as { ref: string }).ref).toBe(rootSha);
  });

  it('keeps parent-relative refs and label for commits with a parent', async () => {
    await commitFile('file.ts', false);
    const childSha = await commitFile('file.ts', true);
    const service = new GitService();

    const uris = await service.buildCommentDiffUris('file.ts', 'modified', { mode: ReviewMode.Commit, commit: childSha });

    expect(uris).not.toBeNull();
    expect(uris!.title).toBe(`file.ts (${childSha}^ ↔ ${childSha})`);
    expect(uris!.leftRef).toBe(`${childSha}^`);
    expect(uris!.rightRef).toBe(childSha);
    expect(uris!.mountRef).toBe(childSha);
    expect(uris!.mountSide).toBe('right');
    const left = uris!.left as unknown as { scheme: string; ref: string };
    expect(left.scheme).toBe('git');
    expect(left.ref).toBe(`${childSha}^`);
    const right = uris!.right as unknown as { ref: string };
    expect(right.ref).toBe(childSha);
  });
});

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
