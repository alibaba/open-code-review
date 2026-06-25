import { isPresetProvider, lookupPreset } from './providers';
import { OcrConfig } from './types';

export type ProviderTab = 'official' | 'custom';

export type ConfigPanelFocus = {
  step?: 1 | 2;
  tab?: ProviderTab;
  customView?: 'list' | 'form';
  customSelection?: string;
};

export interface ConfigEntry {
  key: string;
  value: string;
}

export function detectInitialTab(config: OcrConfig | null): ProviderTab {
  if (!config) return 'official';
  if (config.provider) {
    return isPresetProvider(config.provider) ? 'official' : 'custom';
  }
  return 'official';
}

export type ActiveProviderKind = 'official' | 'custom' | 'legacy';

export interface ActiveProviderSummary {
  kind: ActiveProviderKind;
  name: string;
  displayName: string;
  model: string;
  detail?: string;
}

/** 描述 config.json 中当前生效的 Provider（非表单草稿）。 */
export function describeActiveProvider(config: OcrConfig | null): ActiveProviderSummary | null {
  if (!config) return null;

  if (config.provider) {
    const preset = lookupPreset(config.provider);
    if (preset) {
      const entry = config.providers[config.provider];
      const model = entry?.model || config.model || '';
      if (!model) return null;
      return {
        kind: 'official',
        name: config.provider,
        displayName: preset.displayName,
        model,
      };
    }
    const entry = config.customProviders[config.provider];
    if (!entry?.model) return null;
    return {
      kind: 'custom',
      name: config.provider,
      displayName: config.provider,
      model: entry.model,
      detail: entry.url,
    };
  }

  if (config.llm.url && config.llm.model && config.llm.authToken) {
    return {
      kind: 'legacy',
      name: 'legacy',
      displayName: 'Legacy LLM 端点',
      model: config.llm.model,
      detail: config.llm.url,
    };
  }

  return null;
}

/** 判断配置是否足以发起审查（与 resolver 要求对齐，官方 provider 允许仅依赖环境变量中的 API Key） */
export function isConfigReady(config: OcrConfig | null): boolean {
  if (!config) return false;

  if (config.provider) {
    const preset = lookupPreset(config.provider);
    const entry = preset
      ? config.providers[config.provider]
      : config.customProviders[config.provider];
    if (!entry?.model) return false;
    if (preset) return true;
    return Boolean(entry.url && entry.protocol && entry.apiKey);
  }

  return Boolean(config.llm.url && config.llm.model && config.llm.authToken);
}

export function buildOfficialSaveEntries(
  providerName: string,
  model: string,
  apiKey: string,
  apiKeyChanged: boolean,
): ConfigEntry[] {
  const entries: ConfigEntry[] = [
    { key: 'provider', value: providerName },
    { key: `providers.${providerName}.model`, value: model },
  ];
  if (apiKeyChanged && apiKey.trim()) {
    entries.push({ key: `providers.${providerName}.api_key`, value: apiKey.trim() });
  }
  return entries;
}

export function buildCustomCreateSaveEntries(params: {
  name: string;
  protocol: string;
  url: string;
  model: string;
  models: string;
  apiKey: string;
  authHeader: string;
}): ConfigEntry[] {
  const entries: ConfigEntry[] = [
    { key: `custom_providers.${params.name}.protocol`, value: params.protocol },
    { key: `custom_providers.${params.name}.url`, value: params.url.trim() },
    { key: `custom_providers.${params.name}.model`, value: params.model.trim() },
    { key: `custom_providers.${params.name}.api_key`, value: params.apiKey.trim() },
    { key: 'provider', value: params.name.trim() },
  ];
  const models = params.models.trim();
  if (models) {
    entries.splice(3, 0, { key: `custom_providers.${params.name}.models`, value: models });
  }
  if (params.authHeader.trim()) {
    entries.push({ key: `custom_providers.${params.name}.auth_header`, value: params.authHeader.trim() });
  }
  return entries;
}

export function buildCustomUpdateSaveEntries(params: {
  name: string;
  protocol: string;
  url: string;
  model: string;
  models: string;
  apiKey: string;
  apiKeyChanged: boolean;
  authHeader: string;
}): ConfigEntry[] {
  const entries: ConfigEntry[] = [
    { key: `custom_providers.${params.name}.protocol`, value: params.protocol },
    { key: `custom_providers.${params.name}.url`, value: params.url.trim() },
    { key: `custom_providers.${params.name}.model`, value: params.model.trim() },
    { key: 'provider', value: params.name },
  ];
  const models = params.models.trim();
  if (models) {
    entries.splice(3, 0, { key: `custom_providers.${params.name}.models`, value: models });
  }
  if (params.apiKeyChanged && params.apiKey.trim()) {
    entries.push({ key: `custom_providers.${params.name}.api_key`, value: params.apiKey.trim() });
  }
  if (params.authHeader.trim()) {
    entries.push({ key: `custom_providers.${params.name}.auth_header`, value: params.authHeader.trim() });
  }
  return entries;
}

export function listCustomProviderNames(config: OcrConfig | null): string[] {
  return Object.keys(config?.customProviders ?? {}).sort();
}
