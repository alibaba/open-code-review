import { spawnSync } from 'child_process';

const DELIM = '_OCR_ENV_DELIM_';

/** 从登录 shell 的 `env` 输出中解析出 key=value（取两个分隔标记之间的内容）。 */
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
 * GUI 启动的 VSCode 继承的是精简 PATH，不含 nvm / homebrew / npm 全局 bin。
 * 通过用户的登录交互式 shell（加载 ~/.zshrc、~/.zprofile 等）解析真实环境变量并缓存。
 * Windows 下终端与 GUI 环境一致，直接用 process.env。
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
 * 通过登录交互式 shell 解析命令的绝对路径（`command -v`），覆盖 nvm / homebrew
 * 等用 shell function 或动态 PATH 暴露二进制的情况。解析失败时回退到原命令名
 * （交给 spawn 在注入的 PATH 中查找）。Windows 直接返回原名。
 */
export function resolveBin(name: string): string {
  if (process.platform === 'win32' || process.env.OCR_SKIP_SHELL_RESOLVE) return name;
  const hit = binCache.get(name);
  if (hit) return hit;
  let resolved = name;
  try {
    const shell = process.env.SHELL || '/bin/zsh';
    if (!/^[a-zA-Z0-9._\/-]+$/.test(name)) return name;
    const res = spawnSync(shell, ['-ilc', `command -v '${name.replace(/'/g, "'\\''")}'`], {
      encoding: 'utf8',
      timeout: 5000,
    });
    const path = (res.stdout || '').trim().split('\n').pop()?.trim();
    if (path && path.startsWith('/')) resolved = path;
  } catch {
    // 回退到原命令名
  }
  binCache.set(name, resolved);
  return resolved;
}
