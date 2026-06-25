import { spawn } from 'child_process';
import { CliResult, CliRunOptions, EnvCheckResult, LogLine } from '../../shared/types';
import { buildReviewArgs, extractCliError, parseCliResult, parseLogLine } from './cliParse';
import { getShellEnv, resolveBin } from './shellEnv';

export class CliService {
  private current: ReturnType<typeof spawn> | null = null;
  private envCache: { env: EnvCheckResult; at: number } | null = null;
  private static readonly ENV_CACHE_TTL_MS = 5 * 60 * 1000;

  constructor(private cliPath: string = 'ocr') {}

  invalidateEnvironmentCache(): void {
    this.envCache = null;
  }

  getCachedEnvironment(): EnvCheckResult | null {
    if (!this.envCache) return null;
    if (Date.now() - this.envCache.at > CliService.ENV_CACHE_TTL_MS) {
      this.envCache = null;
      return null;
    }
    return this.envCache.env;
  }

  async isAvailable(): Promise<boolean> {
    const env = await this.checkEnvironment();
    return env.ocr.ok;
  }

  private probeCommand(bin: string, args: string[]): Promise<{ ok: boolean; version?: string }> {
    return new Promise((resolve) => {
      const proc = spawn(resolveBin(bin), args, { env: getShellEnv() });
      let stdout = '';
      let errored = false;
      proc.stdout?.on('data', (d) => { stdout += d.toString(); });
      proc.on('error', () => { errored = true; resolve({ ok: false }); });
      proc.on('close', (code) => {
        if (errored || code !== 0) {
          resolve({ ok: false });
          return;
        }
        const version = stdout.trim().split('\n')[0]?.trim();
        resolve({ ok: true, version: version || undefined });
      });
    });
  }

  async checkEnvironment(force = false): Promise<EnvCheckResult> {
    if (!force) {
      const cached = this.getCachedEnvironment();
      if (cached) return cached;
    }
    const node = await this.probeCommand('node', ['--version']);
    const npm = node.ok ? await this.probeCommand('npm', ['--version']) : { ok: false };
    const ocr = node.ok && npm.ok
      ? await this.probeCommand(this.cliPath, ['--version'])
      : { ok: false };
    const env = { node, npm, ocr };
    this.envCache = { env, at: Date.now() };
    return env;
  }

  /** 全局安装 ocr CLI，流式回显 npm 日志，按 exit code 返回是否成功。 */
  install(onLog: (l: LogLine) => void): Promise<boolean> {
    return new Promise((resolve) => {
      const args = [
        'install', '-g', '@alibaba-group/open-code-review',
        '--loglevel', 'http', '--no-progress',
      ];
      onLog({ text: `$ npm ${args.join(' ')}`, level: 'info' });
      const proc = spawn(resolveBin('npm'), args, {
        // 非 TTY 下 npm 默认静默进度条；强制关进度条并用行式输出
        env: { ...getShellEnv(), npm_config_progress: 'false', npm_config_color: 'false' },
        shell: process.platform === 'win32',
      });
      // npm 输出可能跨 chunk 断行，按 \r\n 归一并逐行 emit，尾部残行留到下次。
      const emitLines = (() => {
        let buf = '';
        return (chunk: string, level: LogLine['level'], flush = false) => {
          buf += chunk.replace(/\r/g, '\n');
          const parts = buf.split('\n');
          buf = flush ? '' : (parts.pop() ?? '');
          for (const line of parts) if (line.trim()) onLog({ text: line, level });
          if (flush && chunk.trim() && parts.length === 0) onLog({ text: chunk, level });
        };
      })();
      proc.stdout?.on('data', (d) => emitLines(d.toString(), 'info'));
      proc.stderr?.on('data', (d) => emitLines(d.toString(), 'info'));
      proc.on('error', (err) => { onLog({ text: String(err), level: 'error' }); resolve(false); });
      proc.on('close', (code) => {
        emitLines('', 'info', true);
        onLog({ text: code === 0 ? '✓ 安装完成' : `✗ 安装失败 (exit ${code})`, level: code === 0 ? 'info' : 'error' });
        if (code === 0) this.invalidateEnvironmentCache();
        resolve(code === 0);
      });
    });
  }

  /** 运行任意参数，流式回调日志，结束返回 stdout 全文。退出码非 0 时 reject，并带上 CLI 报错文本。 */
  runRaw(
    args: string[],
    cwd: string,
    onLog: (l: LogLine) => void,
    envExtra?: Record<string, string>,
  ): Promise<string> {
    return new Promise((resolve, reject) => {
      const proc = spawn(resolveBin(this.cliPath), args, {
        cwd,
        env: envExtra ? { ...getShellEnv(), ...envExtra } : getShellEnv(),
      });
      this.current = proc;
      let stdout = '';
      let stderr = '';
      proc.stdout.on('data', (d) => { stdout += d.toString(); });
      proc.stderr.on('data', (d) => {
        const text = d.toString();
        stderr += text;
        for (const line of text.split('\n')) {
          const parsed = parseLogLine(line);
          if (parsed) onLog(parsed);
        }
      });
      proc.on('error', (err) => { this.current = null; reject(err); });
      proc.on('close', (code) => {
        this.current = null;
        if (code === 0) { resolve(stdout); return; }
        reject(new Error(extractCliError(stderr) || `CLI exited with code ${code}`));
      });
    });
  }

  async review(opts: CliRunOptions, cwd: string, onLog: (l: LogLine) => void): Promise<CliResult> {
    const stdout = await this.runRaw(buildReviewArgs(opts), cwd, onLog);
    return parseCliResult(stdout);
  }

  async testConnection(options?: { configPath?: string; home?: string }): Promise<{ ok: boolean; message?: string }> {
    const envExtra: Record<string, string> = {};
    if (options?.home) {
      envExtra.HOME = options.home;
      if (process.platform === 'win32') envExtra.USERPROFILE = options.home;
    }
    if (options?.configPath) envExtra.OCR_CONFIG_PATH = options.configPath;
    const env = Object.keys(envExtra).length > 0 ? envExtra : undefined;
    try {
      await this.runRaw(['llm', 'test'], process.cwd(), () => {}, env);
      return { ok: true };
    } catch (e) {
      return { ok: false, message: e instanceof Error ? e.message : String(e) };
    }
  }

  cancel(): void {
    if (this.current && this.current.pid) {
      this.current.kill('SIGTERM');
      const proc = this.current;
      setTimeout(() => { if (!proc.killed) proc.kill('SIGKILL'); }, 3000);
    }
  }
}
