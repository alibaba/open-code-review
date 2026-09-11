// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { PROVIDER_PRESETS as generatedPresets } from '../providers.generated';
import { isPresetProvider, lookupPreset, mergeModelLists, PROVIDER_PRESETS, usesAmbientAuth } from '../providers';
import { buildOfficialSaveEntries, detectInitialTab, isConfigReady } from '../configUtils';
import { OcrConfig } from '../types';

describe('generated provider presets', () => {
  it('exposes the complete generated registry through the existing module', () => {
    expect(PROVIDER_PRESETS).toEqual(generatedPresets);
    expect(PROVIDER_PRESETS[0].name).toBe('anthropic');
  });

  it.each(generatedPresets)('preserves metadata, model order and the default for $name', (preset) => {
    const found = lookupPreset(`  ${preset.name.toUpperCase()}  `);
    expect(found).toEqual(preset);
    expect(isPresetProvider(`  ${preset.name.toUpperCase()}  `)).toBe(true);
    const userModels = preset.models.length > 0 ? ['user-model', preset.models[0]] : ['user-model'];
    const models = mergeModelLists(found?.models ?? [], userModels);
    expect(models).toEqual([...preset.models, 'user-model']);
    if (preset.models.length > 0) {
      expect(models[0]).toBe(preset.models[0]);
    } else {
      expect(models).toEqual(['user-model']);
    }
  });

  it.each(['bedrock', 'openai-responses'])('uses the built-in configuration path for %s', (name) => {
    const preset = lookupPreset(name);
    expect(preset).toBeDefined();
    const model = preset?.models[0] ?? '';
    const config: OcrConfig = {
      provider: name,
      model: '',
      providers: { [name]: { model } },
      customProviders: {},
      llm: { url: '', authToken: '', model: '', useAnthropic: false },
      language: 'English',
    };
    expect(detectInitialTab(config)).toBe('official');
    expect(isConfigReady(config)).toBe(true);
    expect(buildOfficialSaveEntries(name, model, '', false)).toEqual([
      { key: 'provider', value: name },
      { key: `providers.${name}.model`, value: model },
    ]);
  });

  it('preserves the Bedrock authentication and Responses protocol metadata', () => {
    expect(lookupPreset('bedrock')).toMatchObject({
      protocol: 'anthropic-bedrock', ambientAuth: true, envVar: '', baseUrl: '',
    });
    expect(lookupPreset('openai-responses')?.protocol).toBe('openai-responses');
    expect(lookupPreset('anthropic')?.authHeader).toBe('x-api-key');
  });

  it('derives ambient authentication from the effective protocol', () => {
    const bedrock = lookupPreset('bedrock');
    const openai = lookupPreset('openai');
    expect(bedrock).toBeDefined();
    expect(openai).toBeDefined();
    expect(usesAmbientAuth(bedrock!)).toBe(true);
    expect(usesAmbientAuth(bedrock!, 'openai')).toBe(false);
    expect(usesAmbientAuth(openai!, 'anthropic-bedrock')).toBe(true);
    expect(usesAmbientAuth(openai!, ' OPENAI ')).toBe(false);
  });

  it('keeps unknown providers outside the built-in preset path', () => {
    expect(lookupPreset('my-custom-provider')).toBeUndefined();
    expect(isPresetProvider('my-custom-provider')).toBe(false);
  });
});
