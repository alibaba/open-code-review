// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { spawn } from 'child_process';
import { EventEmitter } from 'events';
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'fs';
import { tmpdir } from 'os';
import { dirname, join } from 'path';
import { PassThrough } from 'stream';
import { CliService } from '../CliService';
import { ConfigService } from '../ConfigService';

let mockShellEnv: NodeJS.ProcessEnv;
jest.mock('../shellEnv', () => ({
  getShellEnv: () => mockShellEnv,
  getShellEnvAsync: async () => mockShellEnv,
  resolveBin: (bin: string) => bin,
}));
jest.mock('child_process', () => ({ spawn: jest.fn() }));

describe('configuration storage', () => {
  let root: string;
  let configPath: string;
  let cwd: string;
  let testedConfig: string | undefined;
  let testedModel: string | undefined;
  let cli: CliService;
  let config: ConfigService;

  beforeEach(() => {
    root = mkdtempSync(join(tmpdir(), 'ocr-storage-'));
    cwd = join(root, 'workspace');
    configPath = join(cwd, 'custom storage', 'config.json');
    mkdirSync(dirname(configPath), { recursive: true });
    writeFileSync(configPath, JSON.stringify({
      provider: 'custom',
      custom_providers: { custom: { model: 'saved-model' } },
    }));
    mockShellEnv = { ...process.env, OCR_HOME: 'custom storage' };
    testedConfig = testedModel = undefined;
    jest.spyOn(process, 'cwd').mockImplementation(() => cwd);
    jest.mocked(spawn).mockImplementation((_bin, args, options) => {
      const proc = Object.assign(new EventEmitter(), {
        stdout: new PassThrough(), stderr: new PassThrough(),
      });
      const env = (options as { env: NodeJS.ProcessEnv }).env;
      queueMicrotask(() => {
        const path = join(env.OCR_HOME!, 'config.json');
        if (args?.[0] === 'config' && args[1] === 'path') {
          proc.stdout.end(JSON.stringify({ path: configPath }));
        } else if (args?.[0] === 'config') {
          expect(env.OCR_HOME).toBe(dirname(configPath));
          const raw = JSON.parse(readFileSync(path, 'utf8'));
          raw.custom_providers.custom.model = args[3];
          writeFileSync(path, JSON.stringify(raw));
        } else if (args?.[0] === 'llm') {
          expect(env.OCR_HOME).toBe(join(env.HOME!, '.opencodereview'));
          testedConfig = path;
          testedModel = JSON.parse(readFileSync(path, 'utf8')).custom_providers.custom.model;
        } else {
          expect(env.OCR_HOME).toBe(dirname(configPath));
        }
        proc.emit('close', 0);
      });
      return proc as ReturnType<typeof spawn>;
    });
    cli = new CliService();
    config = new ConfigService(cli);
  });

  afterEach(() => {
    jest.restoreAllMocks();
    jest.mocked(spawn).mockReset();
    rmSync(root, { recursive: true, force: true });
  });

  it.each(['absolute', 'relative'])('uses the CLI-resolved %s location for reads, writes and deletes', async (mode) => {
    mockShellEnv.OCR_HOME = mode === 'absolute' ? dirname(configPath) : 'custom storage';
    expect((await config.read())?.customProviders.custom.model).toBe('saved-model');
    expect(spawn).toHaveBeenCalledWith('ocr', ['config', 'path', '--json'], expect.objectContaining({
      cwd, env: mockShellEnv,
    }));
    cwd = root;
    expect((await config.set('model', 'updated-model'))?.customProviders.custom.model).toBe('updated-model');
    await cli.runRaw(['review'], cwd, () => {});
    expect(await config.deleteCustomProvider('custom')).toBeNull();
    expect(existsSync(configPath)).toBe(false);
    expect(jest.mocked(spawn).mock.calls.filter(([, args]) => args?.[0] === 'config' && args[1] === 'path')).toHaveLength(1);
    expect(existsSync(join(root, '.opencodereview', 'config.json'))).toBe(false);
  });

  it('resolves storage before the first batch write', async () => {
    await config.setMany([{ key: 'model', value: 'batch-model' }]);
    expect((await config.read())?.customProviders.custom.model).toBe('batch-model');
    expect(jest.mocked(spawn).mock.calls.filter(([, args]) => args?.[0] === 'config' && args[1] === 'path')).toHaveLength(1);
  });

  it.each(['ocr', 'C:\\Program Files\\ocr\\ocr.cmd'])('supports the Windows launcher %s', async (bin) => {
    const platform = Object.getOwnPropertyDescriptor(process, 'platform')!;
    Object.defineProperty(process, 'platform', { value: 'win32' });
    try {
      expect(await new CliService(bin).configPath()).toBe(configPath);
      expect(spawn).toHaveBeenCalledWith(`"${bin}"`, ['config', 'path', '--json'], expect.objectContaining({
        shell: true,
      }));
    } finally {
      Object.defineProperty(process, 'platform', platform);
    }
  });

  it('tests the draft in temporary OCR_HOME even after pinning saved storage', async () => {
    const saved = readFileSync(configPath, 'utf8');
    expect((await config.read())?.customProviders.custom.model).toBe('saved-model');
    cwd = root;
    expect(await config.testWithEntries([{ key: 'model', value: 'draft-model' }])).toEqual({ ok: true });
    expect(testedModel).toBe('draft-model');
    expect(testedConfig).not.toBe(configPath);
    expect(existsSync(testedConfig!)).toBe(false);
    expect(readFileSync(configPath, 'utf8')).toBe(saved);
    await cli.runRaw(['review'], cwd, () => {});
  });

  it('allows setup UI to load when lookup fails, without writing a fallback config', async () => {
    jest.mocked(spawn).mockImplementation(() => {
      const proc = Object.assign(new EventEmitter(), { stdout: new PassThrough(), stderr: new PassThrough() });
      queueMicrotask(() => { proc.stderr.end('Error: unavailable'); proc.emit('close', 1); });
      return proc as ReturnType<typeof spawn>;
    });
    expect(await config.read()).toBeNull();
    await expect(config.set('model', 'wrong-model')).rejects.toThrow('unavailable');
    expect(readFileSync(configPath, 'utf8')).toContain('saved-model');
  });

  it('rejects relative paths returned by an incompatible CLI', async () => {
    jest.mocked(spawn).mockImplementationOnce(() => {
      const proc = Object.assign(new EventEmitter(), { stdout: new PassThrough(), stderr: new PassThrough() });
      queueMicrotask(() => { proc.stdout.end('{"path":"relative/config.json"}'); proc.emit('close', 0); });
      return proc as ReturnType<typeof spawn>;
    });
    await expect(cli.configPath()).rejects.toThrow('invalid config path');
  });
});
