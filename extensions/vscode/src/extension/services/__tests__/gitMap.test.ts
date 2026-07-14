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
  it('VSCode git Status 枚举映射到 FileChange.status', () => {
    // VSCode Status: INDEX_ADDED=1, MODIFIED=5, DELETED=6, UNTRACKED=7 (示例值)
    expect(mapStatusCode('A')).toBe('added');
    expect(mapStatusCode('M')).toBe('modified');
    expect(mapStatusCode('D')).toBe('deleted');
    expect(mapStatusCode('R')).toBe('renamed');
    expect(mapStatusCode('?')).toBe('added'); // untracked 视为 added
    expect(mapStatusCode('X')).toBe('modified'); // 未知兜底
  });
});

describe('parsePorcelain', () => {
  it('解析各种状态', () => {
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

  it('重命名取新路径', () => {
    expect(parsePorcelain('R  old/x.ts -> new/x.ts')).toEqual([
      { path: 'new/x.ts', status: 'renamed' },
    ]);
  });

  it('去重同一路径（同时暂存+工作区变更）', () => {
    expect(parsePorcelain('MM src/a.ts')).toEqual([
      { path: 'src/a.ts', status: 'modified' },
    ]);
  });

  it('空输出返回空数组', () => {
    expect(parsePorcelain('')).toEqual([]);
    expect(parsePorcelain('\n  \n')).toEqual([]);
  });
});

describe('parseNameStatus', () => {
  it('解析 git diff/show --name-status 输出', () => {
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

  it('重命名行 R<score> old new 取新路径', () => {
    expect(parseNameStatus('R100\told/x.ts\tnew/x.ts')).toEqual([
      { path: 'new/x.ts', status: 'renamed' },
    ]);
  });

  it('去重同一路径', () => {
    expect(parseNameStatus('M\tsrc/a.ts\nM\tsrc/a.ts')).toEqual([
      { path: 'src/a.ts', status: 'modified' },
    ]);
  });

  it('空输出返回空数组', () => {
    expect(parseNameStatus('')).toEqual([]);
    expect(parseNameStatus('\n \n')).toEqual([]);
  });
});

describe('buildWorkspaceFiles', () => {
  it('合并 diff HEAD 与未跟踪文件', () => {
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

  it('diff HEAD 为空时回退 staged', () => {
    const files = buildWorkspaceFiles('', 'M\tsrc/staged.ts', '');
    expect(files).toEqual([{ path: 'src/staged.ts', status: 'modified' }]);
  });

  it('按路径去重，已跟踪优先于未跟踪', () => {
    const files = buildWorkspaceFiles('M\tsrc/a.ts', '', 'src/a.ts');
    expect(files).toEqual([{ path: 'src/a.ts', status: 'modified' }]);
  });
});

describe('parseUntrackedList', () => {
  it('解析未跟踪路径并忽略空行', () => {
    expect(parseUntrackedList('src/a.ts\n\n src/b.ts \n')).toEqual(['src/a.ts', 'src/b.ts']);
  });
});

describe('mergeWorkspaceFiles', () => {
  it('合并并去重', () => {
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
  it('解码 Git quotepath 八进制转义的中文路径', () => {
    const quoted = '"\\344\\273\\243\\347\\240\\201\\344\\277\\256\\346\\224\\271\\346\\234\\200\\345\\260\\217\\345\\271\\262\\351\\242\\204\\350\\247\\204\\345\\210\\231.md"';
    expect(unquoteGitPath(quoted)).toBe('代码修改最小干预规则.md');
  });

  it('普通路径原样返回', () => {
    expect(unquoteGitPath('src/a.ts')).toBe('src/a.ts');
  });
});

describe('parseNameStatus', () => {
  it('解析 quotepath 转义路径', () => {
    expect(parseNameStatus('A\t"\\344\\273\\243\\347\\240\\201.md"')).toEqual([
      { path: '代码.md', status: 'added' },
    ]);
  });
});

describe('branchRefCandidates', () => {
  it('本地分支名补充 origin/ 前缀', () => {
    expect(branchRefCandidates('dev')).toEqual(['dev', 'origin/dev']);
  });

  it('master 回退到 main 候选', () => {
    expect(branchRefCandidates('master')).toEqual(['master', 'origin/master', 'main', 'origin/main']);
  });

  it('已是远程引用时不重复拼接', () => {
    expect(branchRefCandidates('origin/main')).toEqual(['origin/main']);
  });
});

describe('pickRepoRoot', () => {
  const ws = '/Users/lost/tre/copilot-union/code-chat';

  it('精确匹配 workspace 根优先(嵌套子仓库不漂移)', () => {
    // 子仓库 chat-ui 排在前面也应选中父 code-chat
    const roots = ['/Users/lost/tre/copilot-union/code-chat/chat-ui', ws];
    expect(pickRepoRoot(roots, ws)).toBe(ws);
  });

  it('无精确匹配时选 workspace 的祖先仓库', () => {
    const parent = '/Users/lost/tre/copilot-union';
    const roots = ['/Users/lost/tre/copilot-union/code-chat/chat-ui', parent];
    expect(pickRepoRoot(roots, ws)).toBe(parent);
  });

  it('多个祖先时选最深(最长路径)的祖先', () => {
    const grand = '/Users/lost/tre';
    const parent = '/Users/lost/tre/copilot-union';
    const roots = [grand, parent];
    expect(pickRepoRoot(roots, ws)).toBe(parent);
  });

  it('都不匹配时退回第一个', () => {
    const roots = ['/some/other/repo', '/another/repo'];
    expect(pickRepoRoot(roots, ws)).toBe('/some/other/repo');
  });

  it('空候选返回 null', () => {
    expect(pickRepoRoot([], ws)).toBeNull();
  });

  it('无 workspace 路径时退回第一个', () => {
    const roots = ['/a/repo', '/b/repo'];
    expect(pickRepoRoot(roots, undefined)).toBe('/a/repo');
  });
});

describe('getCommitFiles: git show revision placement', () => {
  const repoRoot = path.resolve(__dirname, '../../../../../..');
  const execGit = (args: string[]) =>
    promisify(execFile)('git', ['-c', 'core.quotepath=false', ...args], { cwd: repoRoot })
      .then((r) => r.stdout.trim());

  it('revision 必须在 -- 之前，否则会被当成 pathspec 导致空列表', async () => {
    const good = await execGit(['show', '--name-status', '--format=', 'HEAD']);
    const bad = await execGit(['show', '--name-status', '--format=', '--', 'HEAD']);
    expect(good.length).toBeGreaterThan(0);
    expect(bad).toBe('');
    expect(parseNameStatus(good).length).toBeGreaterThan(0);
    expect(parseNameStatus(bad)).toEqual([]);
  });
});
