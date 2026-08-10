// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { execFile } from 'child_process';
import { mkdtemp, rm, writeFile } from 'fs/promises';
import { tmpdir } from 'os';
import path from 'path';
import { promisify } from 'util';
import * as vscode from 'vscode';
import { ReviewMode } from '../../../shared/types';
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
});
