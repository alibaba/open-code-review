// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, unlinkSync, writeFileSync } from 'fs';
import { homedir, tmpdir } from 'os';
import { join } from 'path';
import * as vscode from 'vscode';
import { applyEdits, findNodeAtLocation, getNodeValue, modify, parseTree, ParseError } from 'jsonc-parser';
import { resolveLocale, t } from '@shared/i18n';
import { ConfigEntry } from '@shared/configUtils';
import { OcrConfig } from '@shared/types';
import { CliService } from './CliService';
import { applyConfigEntries, RawConfig } from './configDraft';
import { parseConfig, toConfigSetArgs } from './configParse';

function removeProperty(text: string, path: string[]): string {
  return applyEdits(text, modify(text, path, undefined, {}));
}

export class ConfigService {
  constructor(private cli: CliService) {}

  private configPath(): string {
    return join(homedir(), '.opencodereview', 'config.json');
  }

  read(): OcrConfig | null {
    const p = this.configPath();
    if (!existsSync(p)) return null;
    try {
      return parseConfig(readFileSync(p, 'utf8'));
    } catch {
      return null;
    }
  }

  private readRaw(): RawConfig {
    const p = this.configPath();
    if (!existsSync(p)) return {};
    try {
      return JSON.parse(readFileSync(p, 'utf8')) as RawConfig;
    } catch {
      return {};
    }
  }

  deleteCustomProvider(name: string): OcrConfig | null {
    const p = this.configPath();
    if (!existsSync(p)) return null;
    let text = readFileSync(p, 'utf8');
    const errors: ParseError[] = [];
    const root = parseTree(text, errors, { disallowComments: true });
    if (errors.length || root?.type !== 'object') return this.read();

    // Go can merge repeated maps and ignore null when decoding a string field.
    const locale = resolveLocale(vscode.env.language);
    const keys: string[] = root.children?.map(property => property.children![0].value) ?? [];
    const raw = getNodeValue(root) as RawConfig;
    const { custom_providers: bucket, providers, llm, provider, model, ...otherSettings } = raw;
    const active = name !== '' && provider === name;
    for (const key of keys) {
      // Go's JSON field matching also folds Unicode long s to ASCII s.
      const canonical = key.replace(/\u017f/g, 's').toLowerCase();
      if (canonical !== 'custom_providers' && canonical !== 'provider' && !(active && canonical === 'model')) continue;
      if (key !== canonical) {
        throw new Error(t(locale, 'ext.config.renameKey').replace('{key}', key).replace('{canonical}', canonical));
      }
      if (keys.indexOf(key) !== keys.lastIndexOf(key)) {
        throw new Error(t(locale, 'ext.config.duplicateKey').replace('{key}', key));
      }
    }
    if (!bucket || typeof bucket !== 'object' || Array.isArray(bucket)
        || !Object.prototype.hasOwnProperty.call(bucket, name)) return this.read();
    const entries = findNodeAtLocation(root, ['custom_providers'])?.children ?? [];
    if (entries.filter(entry => entry.children![0].value === name).length > 1) {
      throw new Error(t(locale, 'ext.config.duplicateProvider').replace('{name}', () => JSON.stringify(name)));
    }

    const keepBucket = Object.keys(bucket).length > 1;
    if (!keepBucket && keys.length === new Set(keys).size && (active || (!provider && !model))
        && !Object.keys(providers ?? {}).length && !Object.keys(llm ?? {}).length && !Object.keys(otherSettings).length) {
      unlinkSync(p);
      return null;
    }
    // Edit the original text so unrelated numbers and unknown fields retain their original representation.
    text = removeProperty(text, keepBucket ? ['custom_providers', name] : ['custom_providers']);
    if (active) {
      text = removeProperty(text, ['provider']);
      text = removeProperty(text, ['model']);
    }
    writeFileSync(p, text, { encoding: 'utf8', mode: 0o600 });
    return this.read();
  }

  /** Run ocr llm test with an isolated temporary HOME without modifying ~/.opencodereview/config.json. */
  async testWithEntries(entries: ConfigEntry[]): Promise<{ ok: boolean; message?: string }> {
    const draft = applyConfigEntries(this.readRaw(), entries);
    const testHome = mkdtempSync(join(tmpdir(), 'ocr-test-home-'));
    const configDir = join(testHome, '.opencodereview');
    const configPath = join(configDir, 'config.json');
    mkdirSync(configDir, { recursive: true, mode: 0o700 });
    writeFileSync(configPath, JSON.stringify(draft, null, 2), { encoding: 'utf8', mode: 0o600 });
    try {
      return await this.cli.testConnection({ home: testHome });
    } finally {
      if (existsSync(testHome)) rmSync(testHome, { recursive: true, force: true });
    }
  }

  async set(key: string, value: string): Promise<OcrConfig | null> {
    await this.cli.runRaw(toConfigSetArgs(key, value), process.cwd(), () => {});
    return this.read();
  }

  async setMany(entries: { key: string; value: string }[]): Promise<OcrConfig | null> {
    for (const entry of entries) {
      await this.cli.runRaw(toConfigSetArgs(entry.key, entry.value), process.cwd(), () => {});
    }
    return this.read();
  }
}
