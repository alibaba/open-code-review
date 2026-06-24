import { applyConfigEntries } from '../configDraft';

describe('applyConfigEntries', () => {
  it('合并官方 provider 条目', () => {
    const draft = applyConfigEntries({}, [
      { key: 'provider', value: 'anthropic' },
      { key: 'providers.anthropic.model', value: 'claude-opus-4-8' },
      { key: 'providers.anthropic.api_key', value: 'sk-test' },
    ]);
    expect(draft.provider).toBe('anthropic');
    expect(draft.providers?.anthropic).toEqual({
      model: 'claude-opus-4-8',
      api_key: 'sk-test',
    });
  });

  it('合并自定义 provider 条目', () => {
    const draft = applyConfigEntries({}, [
      { key: 'custom_providers.my-llm.protocol', value: 'openai' },
      { key: 'custom_providers.my-llm.url', value: 'https://api.example.com/v1' },
      { key: 'custom_providers.my-llm.model', value: 'gpt-4' },
      { key: 'custom_providers.my-llm.api_key', value: 'sk-custom' },
      { key: 'provider', value: 'my-llm' },
    ]);
    expect(draft.provider).toBe('my-llm');
    expect(draft.custom_providers?.['my-llm']).toMatchObject({
      protocol: 'openai',
      url: 'https://api.example.com/v1',
      model: 'gpt-4',
      api_key: 'sk-custom',
    });
  });

  it('合并 manual llm 条目并清空 provider', () => {
    const draft = applyConfigEntries({ provider: 'anthropic' }, [
      { key: 'provider', value: '' },
      { key: 'model', value: '' },
      { key: 'llm.url', value: 'https://api.anthropic.com/v1/messages' },
      { key: 'llm.model', value: 'claude-opus-4-6' },
      { key: 'llm.auth_token', value: 'sk-manual' },
      { key: 'llm.use_anthropic', value: 'true' },
    ]);
    expect(draft.provider).toBe('');
    expect(draft.llm).toMatchObject({
      url: 'https://api.anthropic.com/v1/messages',
      model: 'claude-opus-4-6',
      auth_token: 'sk-manual',
      use_anthropic: true,
    });
  });
});
