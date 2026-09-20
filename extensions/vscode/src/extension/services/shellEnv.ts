// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { spawnSync } from 'child_process';

const DELIM = '_OCR_ENV_DELIM_';

/** Parse key=value pairs between the delimiter markers in login shell `env` output. */
export function parseEnvBlock(stdout: string): Record<string, string> {
  const start = stdout.indexOf(DELIM);
  const end = stdout.lastIndexOf(DELIM);
  if (start === -1 || end === -1 || end <= start) return {};
  const block = stdout.slice(start + DELIM.length, end);
  const env: Record<string, string> = {};
  for (const line of block.split('\n')) {
    const eq = line.indexOf('=');
    if (eq > 0) env[line.slice(0, eq)] = line.slice(eq + 1);
  }
  return env;
}

let cached: NodeJS.ProcessEnv | null = null;

/**
 * VS Code launched from the GUI inherits a minimal PATH without nvm, Homebrew, or global npm binaries.
 * Resolve and cache the environment using the interactive login shell, which loads ~/.zshrc, ~/.zprofile, etc.
 * On Windows, the terminal and GUI share the environment, so use process.env directly.
 */
export function getShellEnv(): NodeJS.ProcessEnv {
  if (cached) return cached;
  if (process.platform === 'win32' || process.env.OCR_SKIP_SHELL_RESOLVE) {
    cached = process.env;
    return cached;
  }
  try {
    const shell = process.env.SHELL || '/bin/zsh';
    const res = spawnSync(shell, ['-ilc', `echo ${DELIM}; env; echo ${DELIM}`], {
      encoding: 'utf8',
      timeout: 5000,
    });
    const parsed = parseEnvBlock(res.stdout || '');
    cached = Object.keys(parsed).length ? { ...process.env, ...parsed } : process.env;
  } catch {
    cached = process.env;
  }
  return cached;
}

const binCache = new Map<string, string>();

/**
 * Resolve a command with `command -v` in an interactive login shell, including nvm and Homebrew
 * setups that expose binaries through shell functions or dynamic PATH entries. On failure, use the original
 * command name so spawn can search the injected PATH. On Windows, return the original name directly.
 */
export function resolveBin(name: string): string {
  if (process.platform === 'win32' || process.env.OCR_SKIP_SHELL_RESOLVE) return name;
  const hit = binCache.get(name);
  if (hit) return hit;
  let resolved = name;
  try {
    const shell = process.env.SHELL || '/bin/zsh';
    if (!/^[a-zA-Z0-9._/-]+$/.test(name)) return name;
    const res = spawnSync(shell, ['-ilc', `command -v '${name.replace(/'/g, "'\\''")}'`], {
      encoding: 'utf8',
      timeout: 5000,
    });
    const path = (res.stdout || '').trim().split('\n').pop()?.trim();
    if (path && path.startsWith('/')) resolved = path;
  } catch {
    // Fall back to the original command name.
  }
  binCache.set(name, resolved);
  return resolved;
}
