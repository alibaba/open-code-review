// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

process.env.OCR_SKIP_SHELL_RESOLVE = '1';
import { spawn } from 'child_process';
import { EventEmitter } from 'events';
import { PassThrough } from 'stream';
import { CliService } from '../CliService';

jest.mock('child_process', () => ({
  ...jest.requireActual('child_process'),
  spawn: jest.fn(),
}));

const mockedSpawn = jest.mocked(spawn);
const platformDescriptor = Object.getOwnPropertyDescriptor(process, 'platform');

function setPlatform(platform: NodeJS.Platform): void {
  Object.defineProperty(process, 'platform', { value: platform });
}

function createMockProcess() {
  const proc = Object.assign(new EventEmitter(), {
    pid: 123,
    killed: false,
    exitCode: null as number | null,
    signalCode: null as NodeJS.Signals | null,
    stdout: new PassThrough(),
    stderr: new PassThrough(),
    kill: jest.fn<(signal?: NodeJS.Signals | number) => boolean>(),
  });
  proc.kill.mockImplementation((signal) => {
    proc.killed = true;
    if (signal === 'SIGKILL') proc.signalCode = 'SIGKILL';
    return true;
  });
  return proc;
}

describe('CliService.cancel', () => {
  beforeEach(() => {
    jest.spyOn(process, 'kill').mockImplementation(() => true);
  });

  afterEach(() => {
    jest.useRealTimers();
    mockedSpawn.mockReset();
    jest.restoreAllMocks();
    if (platformDescriptor) Object.defineProperty(process, 'platform', platformDescriptor);
  });

  it('子进程忽略 SIGTERM 时，超时后发送 SIGKILL', () => {
    jest.useFakeTimers();
    const proc = createMockProcess();
    mockedSpawn.mockReturnValue(proc as unknown as ReturnType<typeof spawn>);

    const svc = new CliService('node');
    void svc.runRaw([], '.', () => {});
    svc.cancel();
    jest.advanceTimersByTime(3000);

    expect(mockedSpawn).toHaveBeenNthCalledWith(
      1,
      'node',
      [],
      expect.objectContaining({ detached: true }),
    );
    expect(proc.kill).toHaveBeenCalledTimes(1);
    expect(proc.kill).toHaveBeenCalledWith('SIGTERM');
    expect(process.kill).toHaveBeenCalledWith(-123, 'SIGKILL');
  });

  it('子进程已正常退出时，不再发送 SIGKILL', async () => {
    jest.useFakeTimers();
    const proc = createMockProcess();
    mockedSpawn.mockReturnValue(proc as unknown as ReturnType<typeof spawn>);

    const svc = new CliService('node');
    const run = svc.runRaw([], '.', () => {});
    svc.cancel();
    proc.exitCode = 0;
    proc.emit('close', 0);
    await run;
    jest.advanceTimersByTime(3000);

    expect(proc.kill).toHaveBeenCalledTimes(1);
    expect(proc.kill).toHaveBeenCalledWith('SIGTERM');
    expect(process.kill).not.toHaveBeenCalled();
  });

  it('falls back to the launcher PID when the POSIX group kill fails', () => {
    jest.useFakeTimers();
    jest.mocked(process.kill).mockImplementation(() => {
      throw Object.assign(new Error('not permitted'), { code: 'EPERM' });
    });
    const proc = createMockProcess();
    mockedSpawn.mockReturnValue(proc as unknown as ReturnType<typeof spawn>);

    const svc = new CliService('node');
    void svc.runRaw([], '.', () => {});
    svc.cancel();
    jest.advanceTimersByTime(3000);

    expect(process.kill).toHaveBeenCalledWith(-123, 'SIGKILL');
    expect(proc.kill).toHaveBeenNthCalledWith(1, 'SIGTERM');
    expect(proc.kill).toHaveBeenNthCalledWith(2, 'SIGKILL');
  });

  it('kills the complete process tree on Windows', () => {
    setPlatform('win32');
    const proc = createMockProcess();
    const treeKill = createMockProcess();
    mockedSpawn
      .mockReturnValueOnce(proc as unknown as ReturnType<typeof spawn>)
      .mockReturnValueOnce(treeKill as unknown as ReturnType<typeof spawn>);

    const svc = new CliService('node');
    void svc.runRaw([], '.', () => {});
    svc.cancel();

    expect(mockedSpawn).toHaveBeenNthCalledWith(
      1,
      'node',
      [],
      expect.objectContaining({ detached: false }),
    );
    expect(mockedSpawn).toHaveBeenNthCalledWith(
      2,
      'taskkill',
      ['/pid', '123', '/t', '/f'],
      { stdio: 'ignore', windowsHide: true },
    );
    expect(proc.kill).not.toHaveBeenCalled();
  });

  it('falls back when taskkill cannot be started on Windows', () => {
    setPlatform('win32');
    const proc = createMockProcess();
    const treeKill = createMockProcess();
    mockedSpawn
      .mockReturnValueOnce(proc as unknown as ReturnType<typeof spawn>)
      .mockReturnValueOnce(treeKill as unknown as ReturnType<typeof spawn>);

    const svc = new CliService('node');
    void svc.runRaw([], '.', () => {});
    svc.cancel();
    treeKill.emit('error', new Error('taskkill unavailable'));

    expect(proc.kill).toHaveBeenCalledWith('SIGKILL');
  });

  it('falls back when taskkill exits unsuccessfully on Windows', () => {
    setPlatform('win32');
    const proc = createMockProcess();
    const treeKill = createMockProcess();
    mockedSpawn
      .mockReturnValueOnce(proc as unknown as ReturnType<typeof spawn>)
      .mockReturnValueOnce(treeKill as unknown as ReturnType<typeof spawn>);

    const svc = new CliService('node');
    void svc.runRaw([], '.', () => {});
    svc.cancel();
    treeKill.emit('close', 1);

    expect(proc.kill).toHaveBeenCalledWith('SIGKILL');
  });
});
