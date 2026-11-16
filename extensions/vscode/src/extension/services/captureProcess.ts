// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { spawn } from 'child_process';

export function captureProcess(
  bin: string, args: string[], cwd: string, env: NodeJS.ProcessEnv,
  timeoutMs: number, shell = false,
): Promise<{ stdout: string; stderr: string; code: number | null }> {
  if (timeoutMs <= 0) return Promise.reject(new Error('OCR config path query timed out'));
  return new Promise((resolve, reject) => {
    const windows = process.platform === 'win32';
    const proc = spawn(bin, args, { cwd, env, shell, detached: !windows, windowsHide: true });
    let stdout = '';
    let stderr = '';
    let settled = false;
    const finish = (error?: Error, code: number | null = null) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      if (error) {
        if (windows && proc.pid) {
          const killer = spawn('taskkill', ['/pid', String(proc.pid), '/t', '/f'], { stdio: 'ignore', windowsHide: true, timeout: 2000 });
          killer.on('error', () => proc.kill('SIGKILL'));
          killer.on('close', (status) => { if (status !== 0) proc.kill('SIGKILL'); });
        } else {
          try { if (proc.pid) process.kill(-proc.pid, 'SIGKILL'); } catch { proc.kill('SIGKILL'); }
        }
      }
      proc.stdin?.destroy();
      proc.stdout?.destroy();
      proc.stderr?.destroy();
      if (error) reject(error);
      else resolve({ stdout, stderr, code });
    };
    const timer = setTimeout(() => finish(new Error('OCR config path query timed out')), timeoutMs);
    proc.stdin?.end();
    proc.stdout?.on('data', (chunk) => {
      stdout += chunk.toString();
      if (Buffer.byteLength(stdout) > 65536) finish(new Error('OCR config path response is too large'));
    });
    proc.stderr?.on('data', (chunk) => { stderr = (stderr + chunk.toString()).slice(-16384); });
    proc.on('error', (error) => finish(error));
    proc.on('close', (code) => finish(undefined, code));
  });
}
