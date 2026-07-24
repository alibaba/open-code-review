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
  afterEach(() => {
    jest.useRealTimers();
    mockedSpawn.mockReset();
  });

  it('子进程忽略 SIGTERM 时，超时后发送 SIGKILL', () => {
    jest.useFakeTimers();
    const proc = createMockProcess();
    mockedSpawn.mockReturnValue(proc as unknown as ReturnType<typeof spawn>);

    const svc = new CliService('node');
    void svc.runRaw([], '.', () => {});
    svc.cancel();
    jest.advanceTimersByTime(3000);

    expect(proc.kill).toHaveBeenNthCalledWith(1, 'SIGTERM');
    expect(proc.kill).toHaveBeenNthCalledWith(2, 'SIGKILL');
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
  });
});
