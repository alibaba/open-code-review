// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// src/extension/services/__tests__/ReviewSession.test.ts
import { resultToState } from '../ReviewSession';

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
});
