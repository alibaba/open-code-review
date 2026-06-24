import { readFileSync, existsSync } from 'fs';
import { homedir } from 'os';
import { join } from 'path';
import { OcrConfig } from '../../shared/types';
import { CliService } from './CliService';
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

  async set(key: string, value: string): Promise<OcrConfig | null> {
    await this.cli.runRaw(toConfigSetArgs(key, value), process.cwd(), () => {});
    return this.read();
  }
}
