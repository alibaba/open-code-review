// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { spawnSync } from 'child_process';
import { copyFileSync, existsSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'fs';
import { tmpdir } from 'os';
import { delimiter, dirname, join, resolve } from 'path';
import { CliService } from '../CliService';
import { ConfigService } from '../ConfigService';

jest.mock('vscode', () => ({ env: { language: 'en' } }), { virtual: true });

const integration = process.env.OCR_TEST_CLI ? describe : describe.skip;

integration('configuration through an npm-installed launcher', () => {
  const windows = process.platform === 'win32';
  const originalEnv = process.env;
  let root: string;
  let prefix: string;
  let native: string;
  let configPath: string;

  beforeAll(() => {
    root = mkdtempSync(join(tmpdir(), 'ocr npm shim '));
    prefix = join(root, 'npm prefix with spaces');
    const fixture = join(root, 'package');
    const repository = resolve(__dirname, '../../../../../..');
    for (const dir of ['bin', 'scripts']) mkdirSync(join(fixture, dir), { recursive: true });
    for (const file of ['bin/ocr.js', 'scripts/platform.js', 'scripts/version.js']) {
      copyFileSync(join(repository, file), join(fixture, file));
    }
    native = join(fixture, 'bin', windows ? 'opencodereview.exe' : 'opencodereview');
    copyFileSync(resolve(process.env.OCR_TEST_CLI!), native);
    writeFileSync(join(fixture, 'package.json'), JSON.stringify({
      name: 'ocr-config-path-fixture', version: '1.0.0', bin: { ocr: 'bin/ocr.js' },
    }));
    configPath = join(root, 'custom storage', 'config.json');
    mkdirSync(dirname(configPath), { recursive: true });
    writeFileSync(configPath, JSON.stringify({ provider: 'openai', model: 'saved-model' }));
    const env = { ...originalEnv };
    const inheritedPath = Object.entries(env).find(([key]) => key.toLowerCase() === 'path')?.[1] || '';
    for (const key of Object.keys(env)) if (key.toLowerCase() === 'path') delete env[key];
    Object.assign(env, {
      [windows ? 'Path' : 'PATH']: `${windows ? prefix : join(prefix, 'bin')}${delimiter}${inheritedPath}`,
      HOME: root, USERPROFILE: root, OCR_HOME: dirname(configPath),
      OCR_SKIP_SHELL_RESOLVE: '1', OCR_NO_UPDATE: '1', OCR_ENABLE_TELEMETRY: '1',
      OTEL_EXPORTER_OTLP_ENDPOINT: '', npm_config_cache: join(root, 'npm cache'),
    });
    process.env = env;
    const npmArgs = [
      'install', '--global', '--prefix', prefix, '--ignore-scripts', '--no-audit', '--no-fund', '--offline', fixture,
    ];
    const installed = spawnSync('npm', windows ? npmArgs.map((arg) => `"${arg}"`) : npmArgs,
      { env, encoding: 'utf8', shell: windows, timeout: 30000 });
    if (installed.error || installed.status !== 0) {
      throw new Error(`npm install failed: ${installed.error || installed.stderr}`);
    }
    expect(existsSync(join(prefix, windows ? 'ocr.cmd' : 'bin/ocr'))).toBe(true);
  }, 40000);

  afterAll(() => {
    process.env = originalEnv;
    if (root) rmSync(root, { recursive: true, force: true });
  });

  it.each(['command', 'absolute launcher', 'native executable'])('reads existing config using %s', async (mode) => {
    const bin = mode === 'command' ? 'ocr'
      : mode === 'absolute launcher' ? join(prefix, windows ? 'ocr.cmd' : 'bin/ocr') : native;
    const cli = new CliService(bin);
    expect(await cli.configPath()).toBe(configPath);
    const config = await new ConfigService(cli).read();
    expect(config?.provider).toBe('openai');
    expect(config?.model).toBe('saved-model');
  });
});
