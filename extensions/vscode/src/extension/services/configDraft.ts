import { ConfigEntry } from '../../shared/configUtils';
import { isPresetProvider } from '../../shared/providers';

type RawProviderEntry = Record<string, unknown>;

export type RawConfig = {
  provider?: string;
  model?: string;
  providers?: Record<string, RawProviderEntry>;
  custom_providers?: Record<string, RawProviderEntry>;
  llm?: Record<string, unknown>;
  language?: string;
};

function parseModelList(value: string): string[] {
  const trimmed = value.trim();
  if (!trimmed) return [];
  if (trimmed.startsWith('[')) {
    try {
      const parsed = JSON.parse(trimmed) as unknown;
      if (Array.isArray(parsed)) {
        return parsed.filter((item): item is string => typeof item === 'string' && item.trim() !== '');
      }
    } catch {
      // fall through to comma split
    }
  }
  return trimmed.split(',').map((item) => item.trim()).filter(Boolean);
}

function applyProviderField(entry: RawProviderEntry, field: string, value: string): void {
  switch (field) {
    case 'api_key':
      entry.api_key = value;
      break;
    case 'url':
      entry.url = value;
      break;
    case 'protocol':
      entry.protocol = value;
      break;
    case 'model':
      entry.model = value;
      break;
    case 'models':
      entry.models = parseModelList(value);
      break;
    case 'auth_header':
      entry.auth_header = value;
      break;
    default:
      break;
  }
}

function setCustomProviderField(cfg: RawConfig, name: string, field: string, value: string): void {
  if (!cfg.custom_providers) cfg.custom_providers = {};
  const entry = { ...(cfg.custom_providers[name] ?? {}) };
  applyProviderField(entry, field, value);
  cfg.custom_providers[name] = entry;
}

function setProviderValue(cfg: RawConfig, key: string, value: string): void {
  const parts = key.split('.');
  if (parts.length !== 3) return;
  const name = parts[1];
  const field = parts[2];
  if (isPresetProvider(name)) {
    if (!cfg.providers) cfg.providers = {};
    const entry = { ...(cfg.providers[name] ?? {}) };
    applyProviderField(entry, field, value);
    cfg.providers[name] = entry;
    return;
  }
  setCustomProviderField(cfg, name, field, value);
}

function setConfigValue(cfg: RawConfig, key: string, value: string): void {
  if (key.startsWith('providers.')) {
    setProviderValue(cfg, key, value);
    return;
  }
  if (key.startsWith('custom_providers.')) {
    const parts = key.split('.');
    if (parts.length === 3) setCustomProviderField(cfg, parts[1], parts[2], value);
    return;
  }

  switch (key) {
    case 'provider':
      if (cfg.provider !== value) cfg.model = '';
      cfg.provider = value;
      if (isPresetProvider(value)) {
        if (!cfg.providers) cfg.providers = {};
        if (!cfg.providers[value]) cfg.providers[value] = {};
      } else if (value) {
        if (!cfg.custom_providers) cfg.custom_providers = {};
        if (!cfg.custom_providers[value]) cfg.custom_providers[value] = {};
      }
      break;
    case 'model':
      if (cfg.provider) {
        if (isPresetProvider(cfg.provider)) {
          if (!cfg.providers) cfg.providers = {};
          const entry = { ...(cfg.providers[cfg.provider] ?? {}) };
          entry.model = value;
          cfg.providers[cfg.provider] = entry;
        } else {
          setCustomProviderField(cfg, cfg.provider, 'model', value);
        }
      } else {
        cfg.model = value;
      }
      break;
    case 'reasoning_effort': {
      const normalized = value.trim().toLowerCase() === 'default' ? '' : value.trim().toLowerCase();
      if (cfg.provider) {
        const collection = isPresetProvider(cfg.provider) ? cfg.providers : cfg.custom_providers;
        const entry = collection?.[cfg.provider];
        const model = typeof entry?.model === 'string' ? entry.model : cfg.model;
        if (!entry || !model) break;
        const current = entry.model_settings && typeof entry.model_settings === 'object'
          ? { ...(entry.model_settings as Record<string, unknown>) }
          : {};
        if (normalized) current[model] = { reasoning_effort: normalized };
        else delete current[model];
        if (Object.keys(current).length > 0) entry.model_settings = current;
        else delete entry.model_settings;
      } else if (cfg.llm && typeof cfg.llm.model === 'string' && cfg.llm.model) {
        const current = cfg.llm.model_settings && typeof cfg.llm.model_settings === 'object'
          ? { ...(cfg.llm.model_settings as Record<string, unknown>) }
          : {};
        if (normalized) current[cfg.llm.model] = { reasoning_effort: normalized };
        else delete current[cfg.llm.model];
        if (Object.keys(current).length > 0) cfg.llm.model_settings = current;
        else delete cfg.llm.model_settings;
      }
      break;
    }
    case 'llm.url':
      if (!cfg.llm) cfg.llm = {};
      cfg.llm.url = value;
      break;
    case 'llm.auth_token':
      if (!cfg.llm) cfg.llm = {};
      cfg.llm.auth_token = value;
      break;
    case 'llm.auth_header':
      if (!cfg.llm) cfg.llm = {};
      cfg.llm.auth_header = value;
      break;
    case 'llm.model':
      if (!cfg.llm) cfg.llm = {};
      cfg.llm.model = value;
      break;
    case 'llm.use_anthropic':
      if (!cfg.llm) cfg.llm = {};
      cfg.llm.use_anthropic = value === 'true';
      break;
    default:
      break;
  }
}

/** 在内存中将 config set 条目合并到原始 config JSON（不写磁盘）。 */
export function applyConfigEntries(base: RawConfig, entries: ConfigEntry[]): RawConfig {
  const draft: RawConfig = JSON.parse(JSON.stringify(base));
  for (const entry of entries) {
    setConfigValue(draft, entry.key, entry.value);
  }
  return draft;
}
