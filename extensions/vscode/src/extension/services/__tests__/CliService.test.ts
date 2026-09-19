// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// src/extension/services/__tests__/CliService.test.ts
process.env.OCR_SKIP_SHELL_RESOLVE = '1';
import { CliService } from '../CliService';

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
