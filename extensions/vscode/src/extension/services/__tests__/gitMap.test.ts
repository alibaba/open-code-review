// src/extension/services/__tests__/gitMap.test.ts
import { mapStatusCode, parsePorcelain, parseNameStatus, pickRepoRoot } from '../gitMap';

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
