import { OcrConfig } from '../../shared/types';

export function parseConfig(raw: string): OcrConfig | null {
  if (!raw || !raw.trim()) return null;
  const j = JSON.parse(raw);
  const llm = j.llm || {};
  return {
    llm: {
      url: llm.url || '',
      authToken: llm.auth_token || '',
      model: llm.model || '',
      useAnthropic: Boolean(llm.use_anthropic),
      authHeader: llm.auth_header || '',
    },
    language: j.language || 'Chinese',
  };
}

export function toConfigSetArgs(key: string, value: string): string[] {
  return ['config', 'set', key, value];
}
