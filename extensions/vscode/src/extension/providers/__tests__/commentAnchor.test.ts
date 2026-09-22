// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import {
  findLinesByExistingCode,
  normalizeLine,
  resolveLinesInContent,
  snippetForms,
} from '../commentAnchor';

describe('commentAnchor line resolution', () => {
  const content = ['line1', 'for (let i = 0; i <= 30, i++) {', '  console.log(i);', '}', 'line5'].join('\n');
  const yaml = ['defaults:', '  name: app', 'items:', '  - name: app'].join('\n');

  it('normalizeLine trims and leaves diff markers alone', () => {
    expect(normalizeLine('  +added  ')).toBe('+added');
    expect(normalizeLine('-removed')).toBe('-removed');
  });

  it('snippetForms tries the code as written before the diff-quoted reading', () => {
    expect(snippetForms('+  - name: app')).toEqual([['+  - name: app'], ['- name: app']]);
  });

  it('snippetForms drops a line that is nothing but a marker', () => {
    expect(snippetForms('+\n+foo')).toEqual([['+', '+foo'], ['foo']]);
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

  it('findLinesByExistingCode matches across blank lines in the file', () => {
    const spaced = ['package main', '', 'func foo() {', '', '  return 1', '}'].join('\n');
    expect(findLinesByExistingCode(spaced, 'func foo() {\n  return 1\n}')).toEqual({ start: 3, end: 6 });
  });

  it('findLinesByExistingCode does not confuse a YAML list item with the mapping line above it', () => {
    expect(findLinesByExistingCode(yaml, '- name: app')).toEqual({ start: 4, end: 4 });
  });

  it('findLinesByExistingCode matches a YAML list item quoted with its diff marker', () => {
    expect(findLinesByExistingCode(yaml, '+  - name: app')).toEqual({ start: 4, end: 4 });
  });

  it('findLinesByExistingCode keeps a bare mapping line on the mapping line', () => {
    expect(findLinesByExistingCode(yaml, 'name: app')).toEqual({ start: 2, end: 2 });
  });

  it('returns null when neither line nor existingCode resolves', () => {
    expect(resolveLinesInContent(content, 99, 99)).toBeNull();
    expect(resolveLinesInContent(content, 0, 0)).toBeNull();
  });
});
