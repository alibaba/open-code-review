// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { spawn, spawnSync } from 'child_process';
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
  resolveBin: (bin: string) => bin,
}));
jest.mock('child_process', () => ({ spawn: jest.fn(), spawnSync: jest.fn() }));

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
    jest.mocked(spawnSync).mockReturnValue({
      status: 0, stdout: JSON.stringify({ path: configPath }), stderr: '',
    } as ReturnType<typeof spawnSync>);
    jest.mocked(spawn).mockImplementation((_bin, args, options) => {
      const proc = Object.assign(new EventEmitter(), {
        stdout: new PassThrough(), stderr: new PassThrough(),
      });
      const env = (options as { env: NodeJS.ProcessEnv }).env;
      queueMicrotask(() => {
        const path = join(env.OCR_HOME!, 'config.json');
        if (args?.[0] === 'config') {
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
    jest.mocked(spawnSync).mockReset();
    rmSync(root, { recursive: true, force: true });
  });

  it.each(['absolute', 'relative'])('uses the CLI-resolved %s location for reads, writes and deletes', async (mode) => {
    mockShellEnv.OCR_HOME = mode === 'absolute' ? dirname(configPath) : 'custom storage';
    expect(config.read()?.customProviders.custom.model).toBe('saved-model');
    expect(spawnSync).toHaveBeenCalledWith('ocr', ['config', 'path', '--json'], expect.objectContaining({
      cwd, env: mockShellEnv,
    }));
    cwd = root;
    expect((await config.set('model', 'updated-model'))?.customProviders.custom.model).toBe('updated-model');
    await cli.runRaw(['review'], cwd, () => {});
    expect(config.deleteCustomProvider('custom')).toBeNull();
    expect(existsSync(configPath)).toBe(false);
    expect(spawnSync).toHaveBeenCalledTimes(1);
    expect(existsSync(join(root, '.opencodereview', 'config.json'))).toBe(false);
  });

  it('resolves storage before the first batch write', async () => {
    await config.setMany([{ key: 'model', value: 'batch-model' }]);
    expect(config.read()?.customProviders.custom.model).toBe('batch-model');
    expect(spawnSync).toHaveBeenCalledTimes(1);
  });

  it('tests the draft in temporary OCR_HOME even after pinning saved storage', async () => {
    const saved = readFileSync(configPath, 'utf8');
    expect(config.read()?.customProviders.custom.model).toBe('saved-model');
    cwd = root;
    expect(await config.testWithEntries([{ key: 'model', value: 'draft-model' }])).toEqual({ ok: true });
    expect(testedModel).toBe('draft-model');
    expect(testedConfig).not.toBe(configPath);
    expect(existsSync(testedConfig!)).toBe(false);
    expect(readFileSync(configPath, 'utf8')).toBe(saved);
    await cli.runRaw(['review'], cwd, () => {});
  });

  it('allows setup UI to load when CLI path lookup fails, without writing a fallback config', async () => {
    jest.mocked(spawnSync).mockReturnValue({ status: 1, stdout: '', stderr: 'Error: unavailable' } as ReturnType<typeof spawnSync>);
    expect(config.read()).toBeNull();
    await expect(config.set('model', 'wrong-model')).rejects.toThrow('unavailable');
    expect(spawn).not.toHaveBeenCalled();
  });

  it('rejects relative paths returned by an incompatible CLI', () => {
    jest.mocked(spawnSync).mockReturnValue({ status: 0, stdout: '{"path":"relative/config.json"}', stderr: '' } as ReturnType<typeof spawnSync>);
    expect(() => cli.configPath()).toThrow('invalid config path');
  });
});
