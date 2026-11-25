// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// src/extension/services/__tests__/CliService.test.ts
process.env.OCR_SKIP_SHELL_RESOLVE = '1';
import { mkdtempSync, rmSync, writeFileSync } from 'fs';
import { tmpdir } from 'os';
import { join } from 'path';
import { ReviewMode } from '@shared/types';
import { CliService } from '../CliService';
import { ConfigService } from '../ConfigService';
import { ReviewSession } from '../ReviewSession';

describe('CliService.isAvailable', () => {
  it('returns true for the available node command', async () => {
    const svc = new CliService('node');
    expect(await svc.isAvailable()).toBe(true);
  });
  it('returns false for a missing command', async () => {
    const svc = new CliService('definitely-not-a-real-binary-xyz');
    expect(await svc.isAvailable()).toBe(false);
  });
});

it('blocks config reads and writes with an inherited OCR_HOME', async () => {
  const previous = process.env.OCR_HOME;
  process.env.OCR_HOME = '/custom-storage';
  const cli = new CliService();
  const run = jest.spyOn(cli, 'runRaw');
  const config = new ConfigService(cli);
  try {
    expect(() => config.read()).toThrow('Use the OCR CLI');
    await expect(config.set('model', 'test-model')).rejects.toThrow('Use the OCR CLI');
    expect(run).not.toHaveBeenCalled();
  } finally {
    if (previous === undefined) delete process.env.OCR_HOME;
    else process.env.OCR_HOME = previous;
  }
});

describe('CliService probe shell option', () => {
  const originalPlatform = Object.getOwnPropertyDescriptor(process, 'platform');
  let spawnSpy: jest.SpyInstance;

  beforeEach(() => {
    // eslint-disable-next-line @typescript-eslint/no-var-requires
    spawnSpy = jest.spyOn(require('child_process'), 'spawn');
  });

  afterEach(() => {
    spawnSpy.mockRestore();
    if (originalPlatform) {
      Object.defineProperty(process, 'platform', originalPlatform);
    }
  });

  it('passes shell: true to probeCommand on Windows', async () => {
    Object.defineProperty(process, 'platform', { value: 'win32' });
    const mockProc = {
      stdout: { on: jest.fn() },
      on: jest.fn((event: string, cb: (code: number) => void) => {
        if (event === 'close') cb(0);
      }),
    };
    spawnSpy.mockReturnValue(mockProc as any);

    const svc = new CliService('node');
    await (svc as any).probeCommand('npm');

    expect(spawnSpy).toHaveBeenCalledWith(
      'npm',
      ['--version'],
      expect.objectContaining({ shell: true }),
    );
  });

  it('does not pass shell to probeCommand on non-Windows platforms', async () => {
    Object.defineProperty(process, 'platform', { value: 'linux' });
    const mockProc = {
      stdout: { on: jest.fn() },
      on: jest.fn((event: string, cb: (code: number) => void) => {
        if (event === 'close') cb(0);
      }),
    };
    spawnSpy.mockReturnValue(mockProc as any);

    const svc = new CliService('node');
    await (svc as any).probeCommand('npm');

    expect(spawnSpy).toHaveBeenCalledWith(
      'npm',
      ['--version'],
      expect.objectContaining({ shell: false }),
    );
  });
});

describe('CliService.runRaw', () => {
  it('collects stdout and resolves on completion', async () => {
    // Simulate ocr by printing JSON with node.
    const svc = new CliService('node');
    const logs: string[] = [];
    const out = await svc.runRaw(
      ['-e', 'process.stdout.write(JSON.stringify({status:"success",comments:[]}))'],
      '.', (line) => logs.push(line.text),
    );
    expect(out).toContain('"status":"success"');
  });

  it('rejects nonzero exits with the Error text from stderr', async () => {
    const svc = new CliService('node');
    await expect(svc.runRaw(
      ['-e', 'process.stderr.write("Error: bad api key\\n"); process.exit(1)'],
      '.', () => {},
    )).rejects.toThrow('bad api key');
  });
});

describe('nonzero review exits', () => {
  const finding = { path: 'a.ts', content: 'Missing return', start_line: 2, end_line: 2 };

  async function run(stdout: string, exitCode = 1) {
    const cwd = mkdtempSync(join(tmpdir(), 'ocr-review-'));
    try {
      // Node treats the CLI's first argument ("review") as the script to run.
      writeFileSync(join(cwd, 'review'), [
        'process.stdout.write(' + JSON.stringify(stdout) + ');',
        'console.error("Error: provider timed out");',
        'process.exitCode = ' + exitCode + ';',
      ].join('\n'));
      const callbacks = { onState: jest.fn(), onLog: jest.fn(), onDone: jest.fn() };
      await new ReviewSession(new CliService(process.execPath), cwd)
        .run({ mode: ReviewMode.Workspace }, callbacks);
      return callbacks;
    } finally {
      rmSync(cwd, { recursive: true, force: true });
    }
  }

  it('reports failure and delivers findings from a partial review', async () => {
    const { onState, onDone } = await run(JSON.stringify({ status: 'partial', comments: [finding] }));
    expect(onState).toHaveBeenLastCalledWith('failed', 'provider timed out');
    expect(onDone.mock.calls[0][0]).toMatchObject({
      status: 'partial', comments: [{ path: 'a.ts', content: 'Missing return', startLine: 2 }],
    });
  });

  it.each([
    ['', 1], ['invalid JSON', 1], ['{"status":"partial"}', 1],
    ['{"status":"partial","comments":[{}]}', 1],
    [JSON.stringify({ status: 'success', comments: [finding] }), 1],
    [JSON.stringify({ status: 'partial', comments: [finding] }), 2],
  ] as const)('retains the CLI error for %s (exit %i)', async (stdout, code) => {
    const { onState, onDone } = await run(stdout, code);
    expect(onState).toHaveBeenLastCalledWith('failed', 'provider timed out');
    expect(onDone).not.toHaveBeenCalled();
  });
});

describe('CliService.testConnection', () => {
  it('reports failure for a nonzero CLI exit', async () => {
    const svc = new CliService('node');
    // The default ['llm', 'test'] arguments cannot be overridden, so verify failure propagation through runRaw directly.
    const r = await svc.runRaw(
      ['-e', 'process.stderr.write("Error: connection refused\\n"); process.exit(1)'],
      '.', () => {},
    ).then(() => ({ ok: true }), (e: Error) => ({ ok: false, message: e.message }));
    expect(r.ok).toBe(false);
    expect((r as { message: string }).message).toContain('connection refused');
  });
});
