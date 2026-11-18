// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { buildReviewArgs, extractCliError, parseCliResult, parseLogLine } from '../cliParse';
import { ReviewMode } from '@shared/types';

describe('buildReviewArgs', () => {
  it('adds --format json in workspace mode', () => {
    expect(buildReviewArgs({ mode: ReviewMode.Workspace }))
      .toEqual(['review', '--format', 'json']);
  });

  it('adds --from and --to in branch mode', () => {
    expect(buildReviewArgs({ mode: ReviewMode.Branch, from: 'main', to: 'dev' }))
      .toEqual(['review', '--from', 'main', '--to', 'dev', '--format', 'json']);
  });

  it('adds --commit in commit mode', () => {
    expect(buildReviewArgs({ mode: ReviewMode.Commit, commit: 'abc123' }))
      .toEqual(['review', '--commit', 'abc123', '--format', 'json']);
  });

  it('appends --background for customPrompt', () => {
    expect(buildReviewArgs({ mode: ReviewMode.Workspace, customPrompt: '关注安全' })) // allow-non-english: fixture verifies Unicode prompt forwarding
      .toEqual(['review', '--format', 'json', '--background', '关注安全']); // allow-non-english: fixture verifies Unicode prompt forwarding
  });

  it('appends --concurrency for concurrency', () => {
    expect(buildReviewArgs({ mode: ReviewMode.Workspace, concurrency: 4 }))
      .toEqual(['review', '--format', 'json', '--concurrency', '4']);
  });
});

describe('parseCliResult', () => {
  it('parses success, comments, and summary with camelCase fields', () => {
    const raw = JSON.stringify({
      status: 'success',
      comments: [{
        path: 'src/a.ts', content: 'bug', start_line: 10, end_line: 12,
        suggestion_code: 'fix', existing_code: 'old',
      }],
      summary: {
        files_reviewed: 2, comments: 1, total_tokens: 100,
        input_tokens: 80, output_tokens: 20, elapsed: '5s',
      },
    });
    const r = parseCliResult(raw);
    expect(r.status).toBe('success');
    expect(r.comments[0]).toEqual({
      path: 'src/a.ts', content: 'bug', startLine: 10, endLine: 12,
      suggestionCode: 'fix', existingCode: 'old', thinking: undefined,
    });
    expect(r.summary?.filesReviewed).toBe(2);
  });

  it('returns no comments for skipped status', () => {
    const raw = JSON.stringify({ status: 'skipped', message: 'No supported files changed.', comments: [] });
    const r = parseCliResult(raw);
    expect(r.status).toBe('skipped');
    expect(r.comments).toEqual([]);
  });

  it('ignores non-JSON noise before the JSON result', () => {
    const raw = '[ocr] some log\n{"status":"success","comments":[]}';
    const r = parseCliResult(raw);
    expect(r.status).toBe('success');
  });
});

describe('extractCliError', () => {
  it('prefers an Error: line and removes its prefix', () => {
    const stderr = '[ocr] starting\nError: llm request failed: 401 unauthorized\n';
    expect(extractCliError(stderr)).toBe('llm request failed: 401 unauthorized');
  });
  it('uses the last Error line when there are multiple errors', () => {
    const stderr = 'Error: first\nError: last';
    expect(extractCliError(stderr)).toBe('last');
  });
  it('uses the last non-empty line when there is no Error line', () => {
    expect(extractCliError('foo\nbar\n\n')).toBe('bar');
  });
  it('returns an empty string for empty stderr', () => {
    expect(extractCliError('')).toBe('');
  });
});

describe('parseLogLine', () => {
  it('classifies a regular [ocr] line as info', () => {
    expect(parseLogLine('[ocr] Reviewing src/a.ts')).toEqual({ text: '[ocr] Reviewing src/a.ts', level: 'info' });
  });
  it('classifies a line containing Retrying as warn', () => {
    expect(parseLogLine('[llm] Retrying in 1.46s (attempt 1/3)').level).toBe('warn');
  });
  it('classifies a line containing WARNING as warn', () => {
    expect(parseLogLine('[ocr] WARNING [x] f: m').level).toBe('warn');
  });
  it('returns null for a blank line', () => {
    expect(parseLogLine('   ')).toBeNull();
  });
});
