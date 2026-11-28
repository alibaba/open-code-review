// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'fs';
import { homedir, tmpdir } from 'os';
import { join } from 'path';
import { CliService } from '../CliService';
import { ConfigService } from '../ConfigService';

jest.mock('os', () => ({ ...jest.requireActual('os'), homedir: jest.fn() }));

describe('ConfigService.deleteCustomProvider', () => {
  const service = new ConfigService(new CliService());
  let home: string;
  let configPath: string;

  beforeEach(() => {
    home = mkdtempSync(join(tmpdir(), 'ocr-vscode-config-'));
    jest.mocked(homedir).mockReturnValue(home);
    const configDir = join(home, '.opencodereview');
    mkdirSync(configDir);
    configPath = join(configDir, 'config.json');
  });

  afterEach(() => {
    rmSync(home, { recursive: true, force: true });
  });

  it.each([
    { label: 'language, MCP and telemetry', settings: {
      language: 'English', mcp_servers: { docs: { command: 'docs' } }, telemetry: { enabled: false },
    } },
    { label: 'an unknown false setting', settings: { future_setting: false } },
  ])('preserves $label when deleting the last active provider', ({ settings }) => {
    writeFileSync(configPath, JSON.stringify({
      ...settings, provider: 'custom', model: 'old-model', custom_providers: { custom: {} },
    }));

    service.deleteCustomProvider('custom');

    expect(JSON.parse(readFileSync(configPath, 'utf8'))).toEqual(settings);
  });

  it('preserves exact numbers and raw keys in unrelated settings', () => {
    writeFileSync(configPath, '{"custom_providers":{"custom":{}},"__proto__":{"id":9007199254740993}}');

    service.deleteCustomProvider('custom');

    const saved = readFileSync(configPath, 'utf8');
    expect(Object.keys(JSON.parse(saved))).toEqual(['__proto__']);
    expect(saved).toMatch(/"id"\s*:\s*9007199254740993\b/);
  });

  it.each([
    ['{"custom_providers":{"other":{},"custom":{}},"custom_providers":{"custom":{}}}', /duplicate key "custom_providers"/],
    ['{"provider":"custom","provider":null,"custom_providers":{"custom":{}}}', /duplicate key "provider"/],
    ['{"custom_providers":{"custom":{},"custom":{},"other":{}}}', /duplicate provider name "custom"/],
    ['{"Provider":"custom","custom_providers":{"custom":{}}}', /Rename the top-level key "Provider" to "provider"/],
  ] as const)('leaves ambiguous configuration keys untouched in %s', (text, message) => {
    writeFileSync(configPath, text);

    expect(() => service.deleteCustomProvider('custom')).toThrow(message);
    expect(readFileSync(configPath, 'utf8')).toBe(text);
  });

  it('keeps the active provider and model when deleting an inactive provider', () => {
    const active = { provider: 'other', model: 'active-model', custom_providers: { other: {} }, LLM: { TimeoutSec: 90 } };
    writeFileSync(configPath, JSON.stringify({ ...active, custom_providers: { other: {}, custom: {} } }));

    service.deleteCustomProvider('custom');

    expect(JSON.parse(readFileSync(configPath, 'utf8'))).toEqual(active);
  });

  it('removes the file when only empty provider settings remain', () => {
    writeFileSync(configPath, JSON.stringify({
      provider: 'custom', model: 'old-model', custom_providers: { custom: {} }, providers: {}, llm: {},
    }));

    expect(service.deleteCustomProvider('custom')).toBeNull();
    expect(existsSync(configPath)).toBe(false);
  });
});
