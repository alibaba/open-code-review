import { FileChange } from '../../shared/types';

export function mapStatusCode(code: string): FileChange['status'] {
  switch (code) {
    case 'A': return 'added';
    case '?': return 'added';
    case 'D': return 'deleted';
    case 'R': return 'renamed';
    case 'M': return 'modified';
    default: return 'modified';
  }
}

/**
 * 解析 `git status --porcelain` 输出。
 * 每行格式：XY<space>path，X=暂存区状态，Y=工作区状态，'??'=未跟踪。
 * 重命名行格式：`R  old -> new`，取 new。
 */
export function parsePorcelain(output: string): FileChange[] {
  const files: FileChange[] = [];
  const seen = new Set<string>();
  for (const rawLine of output.split('\n')) {
    if (!rawLine.trim()) continue;
    const x = rawLine[0];
    const y = rawLine[1];
    let path = rawLine.slice(3);
    let code: string;
    if (x === '?' && y === '?') {
      code = '?';
    } else if (x === 'R' || y === 'R') {
      code = 'R';
      const arrow = path.indexOf(' -> ');
      if (arrow >= 0) path = path.slice(arrow + 4);
    } else {
      // 取暂存区状态优先，否则工作区状态
      const c = x !== ' ' && x !== '?' ? x : y;
      code = c;
    }
    path = unquoteGitPath(path);
    if (seen.has(path)) continue;
    seen.add(path);
    files.push({ path, status: mapStatusCode(code) });
  }
  return files;
}

/** 解析 `git ls-files --others --exclude-standard` 输出的未跟踪路径列表。 */
export function parseUntrackedList(output: string): string[] {
  return output.split('\n').map((line) => unquoteGitPath(line.trim())).filter(Boolean);
}

/** 合并已跟踪变更与未跟踪文件，按路径去重。 */
export function mergeWorkspaceFiles(tracked: FileChange[], untrackedPaths: string[]): FileChange[] {
  const files: FileChange[] = [];
  const seen = new Set<string>();
  for (const file of tracked) {
    if (seen.has(file.path)) continue;
    seen.add(file.path);
    files.push(file);
  }
  for (const path of untrackedPaths) {
    if (seen.has(path)) continue;
    seen.add(path);
    files.push({ path, status: 'added' });
  }
  return files;
}

/**
 * 从 git diff / ls-files 输出构建工作区文件列表。
 * 与 OCR CLI workspace 模式一致：先 diff HEAD，空则回退 staged，再合并未跟踪文件。
 */
export function buildWorkspaceFiles(diffHeadOut: string, diffCachedOut: string, untrackedOut: string): FileChange[] {
  let tracked = parseNameStatus(diffHeadOut);
  if (tracked.length === 0) {
    tracked = parseNameStatus(diffCachedOut);
  }
  return mergeWorkspaceFiles(tracked, parseUntrackedList(untrackedOut));
}

/**
 * 从候选仓库根路径中选出与 workspace 匹配的那个。
 * VSCode git 扩展异步扫描嵌套仓库,repositories 顺序不稳定,直接取 [0] 会漂移到子仓库。
 * 优先级:精确等于 workspace 根 > workspace 的最深祖先 > 第一个。
 */
export function pickRepoRoot(roots: string[], workspacePath?: string): string | null {
  if (roots.length === 0) return null;
  if (!workspacePath) return roots[0];

  const exact = roots.find((r) => r === workspacePath);
  if (exact) return exact;

  const ancestors = roots.filter((r) => workspacePath.startsWith(r.endsWith('/') ? r : r + '/'));
  if (ancestors.length > 0) {
    return ancestors.reduce((deepest, r) => (r.length > deepest.length ? r : deepest));
  }

  return roots[0];
}

/** 生成用于 rev-parse 验证的分支引用候选列表。 */
export function branchRefCandidates(ref: string): string[] {
  const candidates = [ref];
  if (!ref.includes('/')) {
    candidates.push(`origin/${ref}`);
  }
  if (ref === 'master') {
    candidates.push('main', 'origin/main');
  } else if (ref === 'main') {
    candidates.push('master', 'origin/master');
  }
  return [...new Set(candidates)];
}

/**
 * 解码 Git quotepath 转义路径（core.quotepath=true 时中文等会显示为 "\344\273..."）。
 */
export function unquoteGitPath(path: string): string {
  if (!path.startsWith('"') || !path.endsWith('"')) return path;

  const bytes: number[] = [];
  const inner = path.slice(1, -1);
  for (let i = 0; i < inner.length; i++) {
    if (inner[i] !== '\\' || i + 1 >= inner.length) {
      bytes.push(inner.charCodeAt(i));
      continue;
    }
    if (i + 3 < inner.length && /^\d{3}$/.test(inner.slice(i + 1, i + 4))) {
      bytes.push(parseInt(inner.slice(i + 1, i + 4), 8));
      i += 3;
      continue;
    }
    i += 1;
    const esc = inner[i];
    if (esc === 'n') bytes.push(0x0a);
    else if (esc === 't') bytes.push(0x09);
    else if (esc === 'r') bytes.push(0x0d);
    else if (esc === '\\') bytes.push(0x5c);
    else if (esc === '"') bytes.push(0x22);
    else bytes.push(esc.charCodeAt(0));
  }
  return Buffer.from(bytes).toString('utf8');
}

/**
 * 解析 `git diff --name-status` / `git show --name-status` 输出。
 * 每行制表符分隔：status<TAB>path,重命名为 R<score><TAB>old<TAB>new(取 new)。
 */
export function parseNameStatus(output: string): FileChange[] {
  const files: FileChange[] = [];
  const seen = new Set<string>();
  for (const rawLine of output.split('\n')) {
    if (!rawLine.trim()) continue;
    const parts = rawLine.split('\t');
    if (parts.length < 2) continue;
    const codeChar = parts[0][0];
    const path = unquoteGitPath(parts.length >= 3 ? parts[parts.length - 1] : parts[1]);
    if (seen.has(path)) continue;
    seen.add(path);
    files.push({ path, status: mapStatusCode(codeChar) });
  }
  return files;
}
