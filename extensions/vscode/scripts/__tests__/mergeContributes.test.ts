const { mergeContributes } = require('../mergeContributes');

describe('mergeContributes', () => {
  const ocr = {
    commands: [{ command: 'ocr.review.start', title: 'x' }],
    views: { 'ocr-container': [{ id: 'ocr.sidebar', type: 'webview', name: 'CR' }] },
  };

  it('把 ocr 命令并入空 aone contributes', () => {
    const result = mergeContributes({}, ocr);
    expect(result.commands).toHaveLength(1);
    expect(result.commands[0].command).toBe('ocr.review.start');
  });

  it('幂等：重复 merge 不产生重复条目', () => {
    const once = mergeContributes({}, ocr);
    const twice = mergeContributes(once, ocr);
    expect(twice.commands).toHaveLength(1);
  });

  it('保留 aone 自己的非 ocr 命令', () => {
    const aone = { commands: [{ command: 'aone.foo', title: 'foo' }] };
    const result = mergeContributes(aone, ocr);
    expect(result.commands.map((c) => c.command).sort()).toEqual(['aone.foo', 'ocr.review.start']);
  });

  it('再次 merge 时移除旧 ocr.* 命令后重插（更新场景）', () => {
    const aone = { commands: [{ command: 'ocr.old.removed', title: 'old' }, { command: 'aone.foo', title: 'foo' }] };
    const result = mergeContributes(aone, ocr);
    const cmds = result.commands.map((c) => c.command).sort();
    expect(cmds).toEqual(['aone.foo', 'ocr.review.start']); // 旧的 ocr.old.removed 被清掉
  });
});
