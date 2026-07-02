import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, unlinkSync, writeFileSync } from 'fs';
import { homedir, tmpdir } from 'os';
import { dirname, join } from 'path';
import { ConfigEntry } from '../../shared/configUtils';
import { OcrConfig } from '../../shared/types';
import { CliService } from './CliService';
import { applyConfigEntries, RawConfig } from './configDraft';
import { parseConfig, toConfigSetArgs } from './configParse';

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

  private writeRaw(raw: RawConfig): OcrConfig | null {
    const p = this.configPath();
    const hasContent = Boolean(
      raw.provider
      || raw.model
      || (raw.providers && Object.keys(raw.providers).length > 0)
      || (raw.custom_providers && Object.keys(raw.custom_providers).length > 0)
      || (raw.llm && Object.keys(raw.llm).length > 0),
    );
    if (!hasContent) {
      if (existsSync(p)) unlinkSync(p);
      return null;
    }
    const dir = dirname(p);
    if (!existsSync(dir)) mkdirSync(dir, { recursive: true, mode: 0o755 });
    writeFileSync(p, JSON.stringify(raw, null, 2), { encoding: 'utf8', mode: 0o600 });
    return this.read();
  }

  deleteCustomProvider(name: string): OcrConfig | null {
    const raw = this.readRaw();
    if (!raw.custom_providers?.[name]) return this.read();
    delete raw.custom_providers[name];
    if (Object.keys(raw.custom_providers).length === 0) {
      delete raw.custom_providers;
    }
    if (raw.provider === name) {
      delete raw.provider;
      delete raw.model;
    }
    return this.writeRaw(raw);
  }

  /** 在隔离的临时 HOME 上运行 ocr llm test，不修改 ~/.opencodereview/config.json。 */
  async testWithEntries(entries: ConfigEntry[]): Promise<{ ok: boolean; message?: string }> {
    const draft = applyConfigEntries(this.readRaw(), entries);
    const testHome = mkdtempSync(join(tmpdir(), 'ocr-test-home-'));
    const configDir = join(testHome, '.opencodereview');
    const configPath = join(configDir, 'config.json');
    mkdirSync(configDir, { recursive: true, mode: 0o700 });
    writeFileSync(configPath, JSON.stringify(draft, null, 2), { encoding: 'utf8', mode: 0o600 });
    try {
      return await this.cli.testConnection({ home: testHome, configPath });
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
