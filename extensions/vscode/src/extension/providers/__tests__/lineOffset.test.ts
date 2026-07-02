// src/extension/providers/__tests__/lineOffset.test.ts
import { LineOffsetTracker } from '../lineOffset';

describe('LineOffsetTracker', () => {
  it('无变更时返回原行号', () => {
    const t = new LineOffsetTracker();
    expect(t.adjusted('a.ts', 10)).toBe(10);
  });
  it('在某行之前插入若干行，后续行号顺移', () => {
    const t = new LineOffsetTracker();
    t.record('a.ts', 5, +2); // 第5行起增加2行
    expect(t.adjusted('a.ts', 10)).toBe(12);
    expect(t.adjusted('a.ts', 3)).toBe(3); // 之前的行不受影响
  });
  it('删除行使后续行号回退', () => {
    const t = new LineOffsetTracker();
    t.record('a.ts', 5, -1);
    expect(t.adjusted('a.ts', 10)).toBe(9);
  });
  it('不同文件互不影响', () => {
    const t = new LineOffsetTracker();
    t.record('a.ts', 1, +5);
    expect(t.adjusted('b.ts', 10)).toBe(10);
  });
});
