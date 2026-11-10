// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { applyConfigEntries } from '../configDraft';
import { buildCustomCreateSaveEntries } from '@shared/configUtils';

describe('applyConfigEntries', () => {
  it.each(['mimo-v2.6-flash', 'foo.api_key', 'openai.extra_body'])('creates %s before applying form fields', (name) => {
    const entries = buildCustomCreateSaveEntries({
      name, protocol: 'openai', url: 'https://example.invalid/v1', model: 'some-model',
      models: 'some-model,other-model', apiKey: 'test-key', authHeader: 'authorization',
    });
    expect(entries[0]).toEqual({ key: 'provider', value: name });
    const base = { provider: 'openai', model: 'saved-model' };
    const draft = applyConfigEntries(base, entries);
    expect(draft.provider).toBe(name);
    expect(draft.custom_providers).toEqual({
      [name]: {
        protocol: 'openai', url: 'https://example.invalid/v1', model: 'some-model',
        models: ['some-model', 'other-model'], api_key: 'test-key', auth_header: 'authorization',
      },
    });
    expect(base).toEqual({ provider: 'openai', model: 'saved-model' });
  });

  it.each(['providers', 'custom_providers'])('requires existing dotted names for %s', (prefix) => {
    const base = { provider: 'openai', model: 'saved-model' };
    for (const name of ['mimo-v2.6-flash', 'foo.api_key', 'openai.extra_body']) {
      const entry = { key: `${prefix}.${name}.model`, value: 'some-model' };
      expect(applyConfigEntries(base, [entry])).toEqual(base);
      const existing = { ...base, custom_providers: { [name]: {} } };
      const draft = applyConfigEntries(existing, [entry]);
      expect(draft.provider).toBe('openai');
      expect(draft.custom_providers).toEqual({ [name]: { model: 'some-model' } });
      expect(existing.custom_providers[name]).toEqual({});
    }
  });

  it.each([' ', '.a', 'a.', 'a..b', 'a. .b', 'anthropic.'])('rejects invalid name %s', (name) => {
    const base = { provider: 'openai', model: 'saved-model', custom_providers: { [name]: {} } };
    expect(applyConfigEntries(base, [{ key: 'provider', value: name }])).toEqual(base);
    for (const prefix of ['providers', 'custom_providers']) {
      expect(applyConfigEntries(base, [{ key: `${prefix}.${name}.url`, value: 'https://example.invalid/v1' }])).toEqual(base);
    }
  });

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
