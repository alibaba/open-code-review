// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { PROVIDER_PRESETS } from './providers.generated';

// Update internal/llm/providers.go and run go generate ./internal/llm to change presets.
export { PROVIDER_PRESETS };

export interface OcrProviderPreset {
  name: string;
  displayName: string;
  protocol: 'anthropic' | 'openai' | 'openai-responses' | 'anthropic-bedrock';
  baseUrl: string;
  authHeader?: string;
  envVar: string;
  ambientAuth?: boolean;
  models: string[];
}

const presetMap = new Map(PROVIDER_PRESETS.map((p) => [p.name.toLowerCase(), p]));

export function lookupPreset(name: string): OcrProviderPreset | undefined {
  return presetMap.get(name.trim().toLowerCase());
}

export function isPresetProvider(name: string): boolean {
  return presetMap.has(name.trim().toLowerCase());
}

export function usesAmbientAuth(preset: OcrProviderPreset, protocolOverride?: string): boolean {
  const protocol = protocolOverride?.trim().toLowerCase();
  if (protocol) return protocol === 'anthropic-bedrock';
  return preset.protocol === 'anthropic-bedrock' || preset.ambientAuth === true;
}

export function mergeModelLists(...lists: string[][]): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const list of lists) {
    for (const raw of list) {
      const model = raw.trim();
      if (!model || seen.has(model)) continue;
      seen.add(model);
      out.push(model);
    }
  }
  return out;
}
