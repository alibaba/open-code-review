// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import {
  findLinesByExistingCode,
  resolveCommentAnchor,
  CommentAnchorDeps,
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

// Both snapshots have valid line 2; only explicit side metadata disambiguates it.
import * as vscode from 'vscode';
import { ReviewMode } from '@shared/types';
import { parseCliResult } from '../../services/cliParse';

describe('CLI side metadata through editor anchoring', () => {
  function fixture() {
    const left = vscode.Uri.file('/old/a.ts');
    const right = vscode.Uri.file('/new/a.ts');
    const deps: CommentAnchorDeps = {
      repoRoot: '/repo',
      fileStatus: jest.fn(async () => 'modified' as const),
      readAtRef: jest.fn(async (ref) => ref === 'base' ? 'header\nlegacy()\nend' : 'new1()\nnew2()\nend'),
      readWorkspace: jest.fn(async () => 'new1()\nnew2()\nend'),
      buildDiffUris: jest.fn(async () => ({left, right, title: 'a.ts', mountRef: 'head', mountSide: 'right' as const, leftRef: 'base', rightRef: 'head'})),
      toGitUri: jest.fn(async (_path, ref) => ref === 'base' ? left : right),
    };
    return {deps, left, right};
  }

  function comment(side?: 'LEFT' | 'RIGHT') {
    return parseCliResult(JSON.stringify({status: 'success', comments: [{path: 'a.ts', content: 'issue', existing_code: 'legacy()', start_line: 2, end_line: 2, side}]})).comments[0];
  }

  it.each([ReviewMode.Branch, ReviewMode.Commit])('anchors LEFT on the old snapshot in %s mode', async (mode) => {
    const {deps, left} = fixture();
    const anchor = await resolveCommentAnchor(comment('LEFT'), {mode}, deps);
    expect(anchor).toMatchObject({kind: 'mountable', side: 'left', uri: left, range: {start: {line: 1}, end: {line: 1}}});
    expect(deps.readAtRef).toHaveBeenCalledTimes(1);
    expect(deps.readAtRef).toHaveBeenCalledWith('base', 'a.ts');
  });

  it.each(['RIGHT', undefined] as const)('keeps %s on the new snapshot', async (side) => {
    const {deps, right} = fixture();
    expect(await resolveCommentAnchor(comment(side), {mode: ReviewMode.Branch}, deps)).toMatchObject({kind: 'mountable', side: 'right', uri: right});
  });

  it('does not fall back to RIGHT when the explicit LEFT snapshot is unavailable', async () => {
    const {deps} = fixture();
    deps.readAtRef = jest.fn(async (ref) => ref === 'base' ? null : 'new1()\nnew2()');
    expect(await resolveCommentAnchor(comment('LEFT'), {mode: ReviewMode.Branch}, deps)).toEqual({kind: 'sidebar', reason: 'unresolved'});
    expect(deps.readAtRef).toHaveBeenCalledTimes(1);
  });

  it('keeps workspace LEFT findings in the sidebar', async () => {
    const {deps} = fixture();
    expect(await resolveCommentAnchor(comment('LEFT'), {mode: ReviewMode.Workspace}, deps)).toEqual({kind: 'sidebar', reason: 'unresolved'});
    expect(deps.readWorkspace).not.toHaveBeenCalled();
  });
});
