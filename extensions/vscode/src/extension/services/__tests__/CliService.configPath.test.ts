// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { mkdtempSync, writeFileSync, readFileSync, existsSync, rmSync } from 'fs';
import { tmpdir } from 'os';
import { join } from 'path';
import { CliService } from '../CliService';
import { ConfigService } from '../ConfigService';

process.env.OCR_SKIP_SHELL_RESOLVE = '1';
jest.mock('vscode', () => ({ env: { language: 'en' } }), { virtual: true });

describe('bounded asynchronous config path queries', () => {
  let root: string;
  let bin: string;
  const windows = process.platform === 'win32';
  const quote = (value: string) => `'${value.replace(/'/g, "'\\''")}'`;

  beforeEach(() => {
    root = mkdtempSync(join(tmpdir(), 'ocr-path-query-'));
    const script = join(root, 'fixture.js');
    writeFileSync(script, `// SPDX-License-Identifier: Apache-2.0
const fs = require('fs');
const root = ${JSON.stringify(root)};
if (process.argv[2] === '--version') { console.log('v1.0.0'); process.exit(0); }
fs.appendFileSync(root + '/calls', 'query\\n');
fs.writeFileSync(root + '/pid', String(process.pid));
const mode = fs.readFileSync(root + '/mode', 'utf8');
if (mode === 'hang') setInterval(() => {}, 1000);
else if (mode === 'large') process.stdout.write('x'.repeat(70000));
else setTimeout(() => {
  if (mode === 'old') { console.error('Error: unknown command path for ocr config'); process.exitCode = 1; }
  else if (mode === 'fail') { console.error('Error: unavailable'); process.exitCode = 1; }
  else console.log(JSON.stringify({path: root + '/config.json'}));
}, 300);
`);
    bin = join(root, windows ? 'ocr.cmd' : 'ocr');
    writeFileSync(bin, windows ? `@echo off\r\n"${process.execPath}" "${script}" %*\r\n`
      : `#!/bin/sh\n# SPDX-License-Identifier: Apache-2.0\nexec ${quote(process.execPath)} ${quote(script)} "$@"\n`, { mode: 0o755 });
    writeFileSync(join(root, 'mode'), 'ok');
  });

  afterEach(() => {
    if (existsSync(join(root, 'pid'))) {
      try { process.kill(Number(readFileSync(join(root, 'pid'), 'utf8')), 'SIGKILL'); } catch { /* Already exited. */ }
    }
    rmSync(root, { recursive: true, force: true });
  });

  it('shares concurrent resolution, keeps the event loop responsive and caches success', async () => {
    const cli = new CliService(bin);
    const first = cli.configPath();
    expect(cli.configPath()).toBe(first);
    let complete = false;
    void first.then(() => { complete = true; });
    await new Promise((resolve) => setTimeout(resolve, 20));
    expect(complete).toBe(false);
    expect(await first).toBe(join(root, 'config.json'));
    expect(await cli.configPath()).toBe(join(root, 'config.json'));
    expect(readFileSync(join(root, 'calls'), 'utf8')).toBe('query\n');
  });

  it('retries failed queries without pinning another storage root', async () => {
    const cli = new CliService(bin);
    writeFileSync(join(root, 'mode'), 'fail');
    await expect(cli.configPath()).rejects.toThrow('unavailable');
    writeFileSync(join(root, 'mode'), 'ok');
    expect(await cli.configPath()).toBe(join(root, 'config.json'));
    expect(readFileSync(join(root, 'calls'), 'utf8')).toBe('query\nquery\n');
  });

  it('times out a hung process and cleans it up', async () => {
    writeFileSync(join(root, 'mode'), 'hang');
    const cli = new CliService(bin, 1500);
    await expect(cli.configPath()).rejects.toThrow('timed out');
    const pid = Number(readFileSync(join(root, 'pid'), 'utf8'));
    let alive = true;
    for (let i = 0; i < 100 && alive; i++) {
      try { process.kill(pid, 0); } catch { alive = false; }
      if (alive) await new Promise((resolve) => setTimeout(resolve, 20));
    }
    expect(alive).toBe(false);
  });

  it('detects an old CLI despite a working version probe, preserves config and provides an upgrade path', async () => {
    writeFileSync(join(root, 'mode'), 'old');
    writeFileSync(join(root, 'config.json'), '{"model":"saved-model"}');
    const cli = new CliService(bin);
    const env = await cli.checkEnvironment();
    expect(env.ocr.ok).toBe(true);
    expect(env.configPath).toMatchObject({ ok: false, upgradeRequired: true });
    expect(env.configPath?.message).toContain('npm install -g @alibaba-group/open-code-review');
    const config = new ConfigService(cli);
    await expect(config.read()).rejects.toThrow('Upgrade');
    await expect(config.set('model', 'wrong-model')).rejects.toThrow('Upgrade');
    expect(readFileSync(join(root, 'config.json'), 'utf8')).toBe('{"model":"saved-model"}');
    writeFileSync(join(root, 'mode'), 'ok');
    cli.invalidateEnvironmentCache();
    expect((await cli.checkEnvironment()).configPath?.ok).toBe(true);
    expect((await config.read())?.model).toBe('saved-model');
  });

  it('does not classify arbitrary path failures as an incompatible CLI', async () => {
    writeFileSync(join(root, 'mode'), 'fail');
    const env = await new CliService(bin).checkEnvironment();
    expect(env.configPath).toMatchObject({ ok: false, upgradeRequired: false, message: 'unavailable' });
  });

  it('rejects oversized output', async () => {
    writeFileSync(join(root, 'mode'), 'large');
    await expect(new CliService(bin).configPath()).rejects.toThrow('too large');
  });
});
