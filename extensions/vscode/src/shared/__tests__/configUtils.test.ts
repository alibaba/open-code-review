import {
  buildOfficialSaveEntries,
  reasoningEffortOptions,
} from '../configUtils';

describe('reasoning effort config helpers', () => {
  it('uses protocol-specific effort options', () => {
    expect(reasoningEffortOptions('anthropic')).toEqual(['', 'low', 'medium', 'high', 'xhigh', 'max']);
    expect(reasoningEffortOptions('openai')).toEqual(['', 'none', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max']);
  });

  it('saves provider default as an explicit reset', () => {
    expect(buildOfficialSaveEntries('anthropic', 'claude-opus-4-8', '', false, '')).toContainEqual({
      key: 'reasoning_effort',
      value: 'default',
    });
  });
});
