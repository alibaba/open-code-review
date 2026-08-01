// src/extension/services/__tests__/ReviewSession.test.ts
import { resultToState } from '../ReviewSession';

describe('resultToState', () => {
  it('有 comments → done', () => {
    expect(resultToState({ status: 'success', comments: [{} as any], warnings: [] })).toBe('done');
  });
  it('success 但无 comments → empty', () => {
    expect(resultToState({ status: 'success', comments: [], warnings: [] })).toBe('empty');
  });
  it('skipped 无 comments → empty', () => {
    expect(resultToState({ status: 'skipped', comments: [], warnings: [] })).toBe('empty');
  });
  it('completed_with_errors 无 comments → failed', () => {
    expect(resultToState({ status: 'completed_with_errors', comments: [], warnings: [] })).toBe('failed');
  });
  it('completed_with_errors 有 comments → done', () => {
    expect(resultToState({ status: 'completed_with_errors', comments: [{} as any], warnings: [] })).toBe('done');
  });
});
