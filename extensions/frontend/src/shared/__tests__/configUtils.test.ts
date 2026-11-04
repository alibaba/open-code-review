// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { buildCustomCreateSaveEntries, buildCustomUpdateSaveEntries } from '../configUtils';

const params = {
  name: 'gateway',
  protocol: 'openai',
  url: ' https://example.com/v1 ',
  model: ' model-one ',
  models: '',
  apiKey: '',
  apiKeyChanged: false,
  authHeader: '',
};

describe('custom provider save entries', () => {
  it.each(['', '   '])('writes explicit clears for empty optional fields (%p)', (empty) => {
    expect(buildCustomUpdateSaveEntries({ ...params, models: empty, authHeader: empty })).toEqual([
      { key: 'custom_providers.gateway.protocol', value: 'openai' },
      { key: 'custom_providers.gateway.url', value: 'https://example.com/v1' },
      { key: 'custom_providers.gateway.model', value: 'model-one' },
      { key: 'custom_providers.gateway.models', value: '' },
      { key: 'custom_providers.gateway.auth_header', value: '' },
      { key: 'provider', value: 'gateway' },
    ]);
  });

  it('trims optional values and saves a changed API key', () => {
    expect(buildCustomUpdateSaveEntries({
      ...params, models: ' model-one, model-two ', authHeader: ' bearer ',
      apiKey: ' secret ', apiKeyChanged: true,
    })).toEqual(expect.arrayContaining([
      { key: 'custom_providers.gateway.models', value: 'model-one, model-two' },
      { key: 'custom_providers.gateway.auth_header', value: 'bearer' },
      { key: 'custom_providers.gateway.api_key', value: 'secret' },
    ]));
  });

  it('does not overwrite an unchanged or blank API key', () => {
    for (const keyParams of [
      { apiKey: 'secret', apiKeyChanged: false },
      { apiKey: '   ', apiKeyChanged: true },
    ]) {
      expect(buildCustomUpdateSaveEntries({ ...params, ...keyParams }))
        .not.toEqual(expect.arrayContaining([expect.objectContaining({ key: 'custom_providers.gateway.api_key' })]));
    }
  });

  it('still omits empty optional fields when creating a provider', () => {
    const entries = buildCustomCreateSaveEntries({ ...params, models: '   ', authHeader: '   ' });
    for (const key of ['custom_providers.gateway.models', 'custom_providers.gateway.auth_header']) {
      expect(entries).not.toEqual(expect.arrayContaining([expect.objectContaining({ key })]));
    }
  });
});
