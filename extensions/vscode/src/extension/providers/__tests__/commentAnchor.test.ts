// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import {
  findLinesByExistingCode,
  normalizeLine,
  resolveLinesInContent,
  splitAndNormalize,
  splitAndNormalizeDiffSnippet,
} from '../commentAnchor';

describe('commentAnchor line resolution', () => {
  const content = ['line1', 'for (let i = 0; i <= 30, i++) {', '  console.log(i);', '}', 'line5'].join('\n');

  it('normalizeLine preserves leading signs', () => {
    expect(normalizeLine('+added')).toBe('+added');
    expect(normalizeLine('-removed')).toBe('-removed');
  });

  it('splitAndNormalizeDiffSnippet strips one diff marker', () => {
    expect(splitAndNormalizeDiffSnippet('++added\n--removed')).toEqual(['+added', '-removed']);
  });

  it('splitAndNormalize skips blank lines', () => {
    expect(splitAndNormalize('a\n\n b ')).toEqual(['a', 'b']);
  });

  it('resolveLinesInContent uses explicit line numbers when in range', () => {
    expect(resolveLinesInContent(content, 2, 2)).toEqual({ start: 2, end: 2, relocated: false });
  });

  it('resolveLinesInContent falls back to existingCode when line out of range', () => {
    const code = 'for (let i = 0; i <= 30, i++) {';
    const result = resolveLinesInContent(content, 99, 99, code);
    expect(result).toEqual({ start: 2, end: 2, relocated: true });
  });

  it('findLinesByExistingCode matches consecutive non-blank lines', () => {
    const found = findLinesByExistingCode(content, 'console.log(i);');
    expect(found).toEqual({ start: 3, end: 3 });
  });

  it('returns null when neither line nor existingCode resolves', () => {
    expect(resolveLinesInContent(content, 99, 99)).toBeNull();
    expect(resolveLinesInContent(content, 0, 0)).toBeNull();
  });

  it('does not match a snippet missing the YAML item dash', () => {
    expect(findLinesByExistingCode('items:\n  - name: app', 'name: app')).toBeNull();
  });

  it('prefers a literal YAML list item over an earlier mapping', () => {
    const yaml = 'defaults:\n  name: app\nitems:\n  - name: app';
    expect(findLinesByExistingCode(yaml, '- name: app')).toEqual({ start: 4, end: 4 });
  });

  it('matches a diff-style snippet after literal matching fails', () => {
    const yaml = 'defaults:\n  name: app\nitems:\n  - name: app';
    expect(findLinesByExistingCode(yaml, '+  - name: app')).toEqual({ start: 4, end: 4 });
  });
});
