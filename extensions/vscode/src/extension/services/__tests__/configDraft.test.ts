// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { applyConfigEntries } from '../configDraft';

describe('applyConfigEntries', () => {
  it('merges built-in provider entries', () => {
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

  it('merges custom provider entries', () => {
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

  it('clears models and auth_header when update entries carry empty values', () => {
    const draft = applyConfigEntries(
      {
        custom_providers: {
          'my-llm': {
            protocol: 'openai',
            url: 'https://api.example.com/v1',
            model: 'gpt-4',
            api_key: 'sk-custom',
            models: ['gpt-4', 'gpt-4-mini'],
            auth_header: 'x-api-key',
          },
        },
      },
      [
        { key: 'custom_providers.my-llm.models', value: '' },
        { key: 'custom_providers.my-llm.auth_header', value: '' },
      ],
    );
    expect(draft.custom_providers?.['my-llm']).toEqual({
      protocol: 'openai',
      url: 'https://api.example.com/v1',
      model: 'gpt-4',
      api_key: 'sk-custom',
    });
  });

  it('updates models and auth_header when update entries carry new values', () => {
    const draft = applyConfigEntries(
      {
        custom_providers: {
          'my-llm': { model: 'gpt-4', models: ['a'], auth_header: 'x-api-key' },
        },
      },
      [
        { key: 'custom_providers.my-llm.models', value: 'gpt-4, gpt-4o' },
        { key: 'custom_providers.my-llm.auth_header', value: 'authorization' },
      ],
    );
    expect(draft.custom_providers?.['my-llm']?.models).toEqual(['gpt-4', 'gpt-4o']);
    expect(draft.custom_providers?.['my-llm']?.auth_header).toBe('authorization');
  });

  it('merges manual llm entries and clears the provider', () => {
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
