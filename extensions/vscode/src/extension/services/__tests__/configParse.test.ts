// src/extension/services/__tests__/configParse.test.ts
import { parseConfig, toConfigSetArgs } from '../configParse';

describe('parseConfig', () => {
  it('完整 config 转 camelCase', () => {
    const raw = JSON.stringify({
      llm: { url: 'u', auth_token: 't', model: 'm', use_anthropic: true, auth_header: 'x-api-key' },
      language: 'Chinese',
    });
    expect(parseConfig(raw)).toEqual({
      llm: { url: 'u', authToken: 't', model: 'm', useAnthropic: true, authHeader: 'x-api-key' },
      language: 'Chinese',
    });
  });

  it('缺字段时给默认值', () => {
    const cfg = parseConfig('{}');
    expect(cfg.llm.url).toBe('');
    expect(cfg.llm.useAnthropic).toBe(false);
    expect(cfg.language).toBe('Chinese');
  });

  it('空字符串 → null', () => {
    expect(parseConfig('')).toBeNull();
  });
});

describe('toConfigSetArgs', () => {
  it('生成 config set 参数', () => {
    expect(toConfigSetArgs('llm.model', 'opus')).toEqual(['config', 'set', 'llm.model', 'opus']);
  });
});
