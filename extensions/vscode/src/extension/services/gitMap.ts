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
    if (seen.has(path)) continue;
    seen.add(path);
    files.push({ path, status: mapStatusCode(code) });
  }
  return files;
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
    const path = parts.length >= 3 ? parts[parts.length - 1] : parts[1];
    if (seen.has(path)) continue;
    seen.add(path);
    files.push({ path, status: mapStatusCode(codeChar) });
  }
  return files;
}
