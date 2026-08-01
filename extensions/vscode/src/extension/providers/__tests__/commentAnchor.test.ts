import {
  findLinesByExistingCode,
  normalizeLine,
  resolveLinesInContent,
  splitAndNormalize,
} from '../commentAnchor';

describe('commentAnchor line resolution', () => {
  const content = ['line1', 'for (let i = 0; i <= 30, i++) {', '  console.log(i);', '}', 'line5'].join('\n');

  it('normalizeLine strips diff markers', () => {
    expect(normalizeLine('+added')).toBe('added');
    expect(normalizeLine('-removed')).toBe('removed');
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
});
