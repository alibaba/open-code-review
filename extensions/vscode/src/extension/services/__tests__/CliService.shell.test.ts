// src/extension/services/__tests__/CliService.shell.test.ts
//
// Windows 下 node 是 node.exe（可被 spawn 直接执行），但 npm/ocr 是 npm.cmd / ocr.cmd 包装脚本，
// Node 的 child_process.spawn 不加 shell 无法执行 .cmd，导致环境检测误报"未检测到 npm"（issue #453）
// 以及 review 无法运行。这里 mock child_process.spawn，断言各平台传入的 shell 选项，
// 从而在非 Windows 机器上也能验证该跨平台修复，且保证 macOS/Linux 行为不变（shell:false）。
process.env.OCR_SKIP_SHELL_RESOLVE = '1';
import { EventEmitter } from 'events';
import { spawn } from 'child_process';
import { CliService } from '../CliService';

jest.mock('child_process');

const mockSpawn = spawn as unknown as jest.Mock;
const originalPlatform = process.platform;

const setPlatform = (platform: string) =>
  Object.defineProperty(process, 'platform', { value: platform, configurable: true });

// 立即回显版本号并以 code 0 结束的假子进程，覆盖 probeCommand / runRaw 用到的最小接口。
const fakeProc = () => {
  const proc = new EventEmitter() as EventEmitter & {
    stdout: EventEmitter; stderr: EventEmitter; pid: number; kill: () => void; killed: boolean;
  };
  proc.stdout = new EventEmitter();
  proc.stderr = new EventEmitter();
  proc.pid = 4242;
  proc.kill = () => {};
  proc.killed = false;
  setImmediate(() => {
    proc.stdout.emit('data', Buffer.from('v1.0.0\n'));
    proc.emit('close', 0);
  });
  return proc;
};

const shellOf = (callIndex: number): boolean | undefined =>
  (mockSpawn.mock.calls[callIndex]?.[2] as { shell?: boolean } | undefined)?.shell;

describe('CliService spawn shell 选项（跨平台）', () => {
  beforeEach(() => {
    mockSpawn.mockReset();
    mockSpawn.mockImplementation(() => fakeProc());
  });

  afterEach(() => {
    Object.defineProperty(process, 'platform', { value: originalPlatform, configurable: true });
  });

  it('Windows：probeCommand 的每次 spawn 都带 shell:true（否则 npm.cmd 无法执行）', async () => {
    setPlatform('win32');
    await new CliService('node').checkEnvironment(true);
    expect(mockSpawn).toHaveBeenCalled();
    for (const call of mockSpawn.mock.calls) {
      expect((call[2] as { shell?: boolean }).shell).toBe(true);
    }
  });

  it('非 Windows：probeCommand 的 spawn 不启用 shell（shell:false，行为不变）', async () => {
    setPlatform('darwin');
    await new CliService('node').checkEnvironment(true);
    expect(mockSpawn).toHaveBeenCalled();
    for (const call of mockSpawn.mock.calls) {
      expect((call[2] as { shell?: boolean }).shell).toBe(false);
    }
  });

  it('runRaw 跟随平台设置 shell 选项', async () => {
    setPlatform('win32');
    await new CliService('ocr').runRaw(['--version'], '.', () => {});
    expect(shellOf(0)).toBe(true);

    mockSpawn.mockClear();
    setPlatform('linux');
    await new CliService('ocr').runRaw(['--version'], '.', () => {});
    expect(shellOf(0)).toBe(false);
  });
});
