// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// src/extension/services/__tests__/ReviewSession.test.ts
import { ReviewSession, resultToState } from '../ReviewSession';

describe('resultToState', () => {
  it('returns done when comments are present', () => {
    expect(resultToState({ status: 'success', comments: [{} as any], warnings: [] })).toBe('done');
  });
  it('returns empty for success without comments', () => {
    expect(resultToState({ status: 'success', comments: [], warnings: [] })).toBe('empty');
  });
  it('returns empty for skipped without comments', () => {
    expect(resultToState({ status: 'skipped', comments: [], warnings: [] })).toBe('empty');
  });
  it('returns failed for completed_with_errors without comments', () => {
    expect(resultToState({ status: 'completed_with_errors', comments: [], warnings: [] })).toBe('failed');
  });
  it('returns done for completed_with_errors with comments', () => {
    expect(resultToState({ status: 'completed_with_errors', comments: [{} as any], warnings: [] })).toBe('done');
  });

  // `ocr review` reports its manifest terminal state: complete | partial | failed | skipped.
  it('returns failed for partial without comments', () => {
    expect(resultToState({ status: 'partial' as any, comments: [], warnings: [] })).toBe('failed');
  });
  it('returns failed for failed without comments', () => {
    expect(resultToState({ status: 'failed' as any, comments: [], warnings: [] })).toBe('failed');
  });
  it('returns done for partial with comments', () => {
    expect(resultToState({ status: 'partial' as any, comments: [{} as any], warnings: [] })).toBe('done');
  });
  it('returns empty for complete without comments', () => {
    expect(resultToState({ status: 'complete' as any, comments: [], warnings: [] })).toBe('empty');
  });
});

describe('ReviewSession.run', () => {
  it('reports a partial review without comments as failed with the CLI message', async () => {
    const message = 'Review partially complete: 0 finding(s); 2 of 5 selected item(s) failed.';
    const cli = {
      review: jest.fn().mockResolvedValue({ status: 'partial', comments: [], warnings: [], message }),
      cancel: jest.fn(),
    };
    const onState = jest.fn();
    const session = new ReviewSession(cli as any, '/repo');
    await session.run({} as any, { onState, onLog: jest.fn(), onDone: jest.fn() });
    expect(onState).toHaveBeenLastCalledWith('failed', message);
  });
});
