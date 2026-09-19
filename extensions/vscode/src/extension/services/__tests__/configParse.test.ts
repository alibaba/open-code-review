// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// src/extension/services/__tests__/configParse.test.ts
import { parseConfig, toConfigSetArgs } from '../configParse';

describe('parseConfig', () => {
  it('converts a complete config to camelCase', () => {
    const raw = JSON.stringify({
      provider: 'anthropic',
      providers: {
        anthropic: { api_key: 'k', model: 'claude-opus-4-6', models: ['claude-opus-4-6'] },
      },
      custom_providers: {
        'my-llm': { url: 'https://x', protocol: 'openai', model: 'm', api_key: 'k2' },
      },
      llm: { url: 'u', auth_token: 't', model: 'm', use_anthropic: true, auth_header: 'x-api-key' },
      language: 'Chinese',
    });
    expect(parseConfig(raw)).toEqual({
      provider: 'anthropic',
      model: '',
      providers: {
        anthropic: { apiKey: 'k', url: '', protocol: '', model: 'claude-opus-4-6', models: ['claude-opus-4-6'], authHeader: '' },
      },
      customProviders: {
        'my-llm': { apiKey: 'k2', url: 'https://x', protocol: 'openai', model: 'm', authHeader: '' },
      },
      llm: { url: 'u', authToken: 't', model: 'm', useAnthropic: true, authHeader: 'x-api-key' },
      language: 'Chinese',
    });
  });

  it('provides defaults for missing fields', () => {
    const cfg = parseConfig('{}');
    expect(cfg?.llm.url).toBe('');
    expect(cfg?.llm.useAnthropic).toBe(true);
    expect(cfg?.providers).toEqual({});
    expect(cfg?.customProviders).toEqual({});
    expect(cfg?.language).toBe('Chinese');
  });

  it('returns null for an empty string', () => {
    expect(parseConfig('')).toBeNull();
  });

  it('ignores providers when it is an array', () => {
    const cfg = parseConfig(JSON.stringify({ providers: ['bad'], custom_providers: [] }));
    expect(cfg?.providers).toEqual({});
    expect(cfg?.customProviders).toEqual({});
  });
});

describe('toConfigSetArgs', () => {
  it('builds config set arguments', () => {
    expect(toConfigSetArgs('llm.model', 'opus')).toEqual(['config', 'set', 'llm.model', 'opus']);
    expect(toConfigSetArgs('providers.anthropic.api_key', 'sk')).toEqual(['config', 'set', 'providers.anthropic.api_key', 'sk']);
  });
});
