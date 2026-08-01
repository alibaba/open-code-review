import { OcrConfig, ProviderEntry } from '../../shared/types';

function parseProviderEntry(raw: Record<string, unknown> | undefined): ProviderEntry {
  if (!raw) return {};
  const models = Array.isArray(raw.models)
    ? raw.models.filter((m): m is string => typeof m === 'string')
    : undefined;
  return {
    apiKey: typeof raw.api_key === 'string' ? raw.api_key : '',
    url: typeof raw.url === 'string' ? raw.url : '',
    protocol: typeof raw.protocol === 'string' ? raw.protocol : '',
    model: typeof raw.model === 'string' ? raw.model : '',
    models,
    authHeader: typeof raw.auth_header === 'string' ? raw.auth_header : '',
  };
}

function parseProviderMap(raw: Record<string, unknown> | undefined): Record<string, ProviderEntry> {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return {};
  const out: Record<string, ProviderEntry> = {};
  for (const [name, entry] of Object.entries(raw)) {
    out[name] = parseProviderEntry(entry as Record<string, unknown>);
  }
  return out;
}

export function parseConfig(raw: string): OcrConfig | null {
  if (!raw || !raw.trim()) return null;
  const j = JSON.parse(raw);
  const llm = j.llm || {};
  return {
    provider: typeof j.provider === 'string' ? j.provider : '',
    model: typeof j.model === 'string' ? j.model : '',
    providers: parseProviderMap(j.providers),
    customProviders: parseProviderMap(j.custom_providers),
    llm: {
      url: llm.url || '',
      authToken: llm.auth_token || '',
      model: llm.model || '',
      useAnthropic: llm.use_anthropic !== false,
      authHeader: llm.auth_header || '',
    },
    language: j.language || 'Chinese',
  };
}

export function toConfigSetArgs(key: string, value: string): string[] {
  return ['config', 'set', key, value];
}
