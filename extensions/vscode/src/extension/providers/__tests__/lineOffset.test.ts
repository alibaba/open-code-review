// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// src/extension/providers/__tests__/lineOffset.test.ts
import { LineOffsetTracker } from '../lineOffset';

describe('LineOffsetTracker', () => {
  it('returns the original line number when there are no changes', () => {
    const t = new LineOffsetTracker();
    expect(t.adjusted('a.ts', 10)).toBe(10);
  });
  it('shifts subsequent line numbers forward after an insertion', () => {
    const t = new LineOffsetTracker();
    t.record('a.ts', 5, +2); // Insert two lines starting at line 5.
    expect(t.adjusted('a.ts', 10)).toBe(12);
    expect(t.adjusted('a.ts', 3)).toBe(3); // Earlier lines are unaffected.
  });
  it('shifts subsequent line numbers backward after a deletion', () => {
    const t = new LineOffsetTracker();
    t.record('a.ts', 5, -1);
    expect(t.adjusted('a.ts', 10)).toBe(9);
  });
  it('tracks offsets independently for each file', () => {
    const t = new LineOffsetTracker();
    t.record('a.ts', 1, +5);
    expect(t.adjusted('b.ts', 10)).toBe(10);
  });
});
