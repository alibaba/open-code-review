// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// src/extension/services/__tests__/gitMap.test.ts
import { execFile } from 'child_process';
import path from 'path';
import { promisify } from 'util';
import {
  buildWorkspaceFiles,
  branchRefCandidates,
  mapStatusCode,
  mergeWorkspaceFiles,
  parsePorcelain,
  parseNameStatus,
  parseUntrackedList,
  pickRepoRoot,
  unquoteGitPath,
} from '../gitMap';

describe('mapStatusCode', () => {
  it('maps Git status codes to FileChange.status', () => {
    // VSCode Status: INDEX_ADDED=1, MODIFIED=5, DELETED=6, UNTRACKED=7 (example values)
    expect(mapStatusCode('A')).toBe('added');
    expect(mapStatusCode('M')).toBe('modified');
    expect(mapStatusCode('D')).toBe('deleted');
    expect(mapStatusCode('R')).toBe('renamed');
    expect(mapStatusCode('?')).toBe('added'); // Treat untracked files as added.
    expect(mapStatusCode('X')).toBe('modified'); // Fall back for unknown codes.
  });
});

describe('parsePorcelain', () => {
  it('parses different file statuses', () => {
    const out = [
      'M  src/a.ts',
      ' M src/b.ts',
      'A  src/c.ts',
      '?? src/d.ts',
      'D  src/e.ts',
    ].join('\n');
    expect(parsePorcelain(out)).toEqual([
      { path: 'src/a.ts', status: 'modified' },
      { path: 'src/b.ts', status: 'modified' },
      { path: 'src/c.ts', status: 'added' },
      { path: 'src/d.ts', status: 'added' },
      { path: 'src/e.ts', status: 'deleted' },
    ]);
  });

  it('uses the new path for renames', () => {
    expect(parsePorcelain('R  old/x.ts -> new/x.ts')).toEqual([
      { path: 'new/x.ts', status: 'renamed' },
    ]);
  });

  it('deduplicates paths with both staged and working tree changes', () => {
    expect(parsePorcelain('MM src/a.ts')).toEqual([
      { path: 'src/a.ts', status: 'modified' },
    ]);
  });

  it('returns an empty array for empty output', () => {
    expect(parsePorcelain('')).toEqual([]);
    expect(parsePorcelain('\n  \n')).toEqual([]);
  });
});

describe('parseNameStatus', () => {
  it('parses git diff/show --name-status output', () => {
    const out = [
      'M\tsrc/a.ts',
      'A\tsrc/b.ts',
      'D\tsrc/c.ts',
    ].join('\n');
    expect(parseNameStatus(out)).toEqual([
      { path: 'src/a.ts', status: 'modified' },
      { path: 'src/b.ts', status: 'added' },
      { path: 'src/c.ts', status: 'deleted' },
    ]);
  });

  it('uses the new path from an R<score> old new rename line', () => {
    expect(parseNameStatus('R100\told/x.ts\tnew/x.ts')).toEqual([
      { path: 'new/x.ts', status: 'renamed' },
    ]);
  });

  it('deduplicates identical paths', () => {
    expect(parseNameStatus('M\tsrc/a.ts\nM\tsrc/a.ts')).toEqual([
      { path: 'src/a.ts', status: 'modified' },
    ]);
  });

  it('returns an empty array for empty output', () => {
    expect(parseNameStatus('')).toEqual([]);
    expect(parseNameStatus('\n \n')).toEqual([]);
  });
});

describe('buildWorkspaceFiles', () => {
  it('merges diff HEAD output with untracked files', () => {
    const files = buildWorkspaceFiles(
      'M\tsrc/a.ts\nA\tsrc/b.ts',
      '',
      'src/c.ts\n',
    );
    expect(files).toEqual([
      { path: 'src/a.ts', status: 'modified' },
      { path: 'src/b.ts', status: 'added' },
      { path: 'src/c.ts', status: 'added' },
    ]);
  });

  it('falls back to staged changes when diff HEAD is empty', () => {
    const files = buildWorkspaceFiles('', 'M\tsrc/staged.ts', '');
    expect(files).toEqual([{ path: 'src/staged.ts', status: 'modified' }]);
  });

  it('deduplicates paths and prefers tracked changes over untracked files', () => {
    const files = buildWorkspaceFiles('M\tsrc/a.ts', '', 'src/a.ts');
    expect(files).toEqual([{ path: 'src/a.ts', status: 'modified' }]);
  });
});

describe('parseUntrackedList', () => {
  it('parses untracked paths and ignores blank lines', () => {
    expect(parseUntrackedList('src/a.ts\n\n src/b.ts \n')).toEqual(['src/a.ts', 'src/b.ts']);
  });
});

describe('mergeWorkspaceFiles', () => {
  it('merges and deduplicates paths', () => {
    expect(mergeWorkspaceFiles(
      [{ path: 'a.ts', status: 'modified' }],
      ['b.ts', 'a.ts'],
    )).toEqual([
      { path: 'a.ts', status: 'modified' },
      { path: 'b.ts', status: 'added' },
    ]);
  });
});

describe('unquoteGitPath', () => {
  it('decodes Chinese paths with Git quotepath octal escapes', () => {
    const quoted = '"\\344\\273\\243\\347\\240\\201\\344\\277\\256\\346\\224\\271\\346\\234\\200\\345\\260\\217\\345\\271\\262\\351\\242\\204\\350\\247\\204\\345\\210\\231.md"';
    expect(unquoteGitPath(quoted)).toBe('代码修改最小干预规则.md'); // allow-non-english: fixture verifies UTF-8 Git path decoding
  });

  it('returns ordinary paths unchanged', () => {
    expect(unquoteGitPath('src/a.ts')).toBe('src/a.ts');
  });
});

describe('parseNameStatus', () => {
  it('parses quotepath-escaped paths', () => {
    expect(parseNameStatus('A\t"\\344\\273\\243\\347\\240\\201.md"')).toEqual([
      { path: '代码.md', status: 'added' }, // allow-non-english: fixture verifies UTF-8 Git path decoding
    ]);
  });
});

describe('branchRefCandidates', () => {
  it('adds an origin/ candidate for local branch names', () => {
    expect(branchRefCandidates('dev')).toEqual(['dev', 'origin/dev']);
  });

  it('adds main fallback candidates for master', () => {
    expect(branchRefCandidates('master')).toEqual(['master', 'origin/master', 'main', 'origin/main']);
  });

  it('does not add another prefix to remote references', () => {
    expect(branchRefCandidates('origin/main')).toEqual(['origin/main']);
  });
});

describe('pickRepoRoot', () => {
  const ws = '/Users/lost/tre/copilot-union/code-chat';

  it('prefers an exact workspace root match over a nested child repository', () => {
    // Select the parent code-chat repository even when the child chat-ui appears first.
    const roots = ['/Users/lost/tre/copilot-union/code-chat/chat-ui', ws];
    expect(pickRepoRoot(roots, ws)).toBe(ws);
  });

  it('selects an ancestor repository when there is no exact workspace match', () => {
    const parent = '/Users/lost/tre/copilot-union';
    const roots = ['/Users/lost/tre/copilot-union/code-chat/chat-ui', parent];
    expect(pickRepoRoot(roots, ws)).toBe(parent);
  });

  it('selects the deepest ancestor when multiple ancestors match', () => {
    const grand = '/Users/lost/tre';
    const parent = '/Users/lost/tre/copilot-union';
    const roots = [grand, parent];
    expect(pickRepoRoot(roots, ws)).toBe(parent);
  });

  it('falls back to the first candidate when none match', () => {
    const roots = ['/some/other/repo', '/another/repo'];
    expect(pickRepoRoot(roots, ws)).toBe('/some/other/repo');
  });

  it('returns null when there are no candidates', () => {
    expect(pickRepoRoot([], ws)).toBeNull();
  });

  it('falls back to the first candidate when the workspace path is missing', () => {
    const roots = ['/a/repo', '/b/repo'];
    expect(pickRepoRoot(roots, undefined)).toBe('/a/repo');
  });

  it('selects a Windows repository containing the workspace subdirectory', () => {
    const roots = ['C:\\other', 'C:\\repo'];
    expect(pickRepoRoot(roots, 'C:\\repo\\packages\\app')).toBe('C:\\repo');
  });

  it('does not treat a Windows path with the same prefix as an ancestor', () => {
    const roots = ['C:\\repo', 'C:\\repository'];
    expect(pickRepoRoot(roots, 'C:\\repository\\src')).toBe('C:\\repository');
  });

  it('handles mixed separators between a UNC root and workspace path', () => {
    const roots = ['//server/share/other', '//server/share/repo'];
    expect(pickRepoRoot(roots, '\\\\server\\share\\repo\\src')).toBe('//server/share/repo');
  });
});

describe('getCommitFiles: git show revision placement', () => {
  const repoRoot = path.resolve(__dirname, '../../../../../..');
  const execGit = (args: string[]) =>
    promisify(execFile)('git', ['-c', 'core.quotepath=false', ...args], { cwd: repoRoot })
      .then((r) => r.stdout.trim());

  it('requires the revision before -- to avoid treating it as a pathspec and returning an empty list', async () => {
    const good = await execGit(['show', '--name-status', '--format=', 'HEAD']);
    const bad = await execGit(['show', '--name-status', '--format=', '--', 'HEAD']);
    expect(good.length).toBeGreaterThan(0);
    expect(bad).toBe('');
    expect(parseNameStatus(good).length).toBeGreaterThan(0);
    expect(parseNameStatus(bad)).toEqual([]);
  });
});
