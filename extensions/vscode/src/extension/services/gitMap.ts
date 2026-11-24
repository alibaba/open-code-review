// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { FileChange } from '@shared/types';
import path from 'path';

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
 * Parse `git status --porcelain` output.
 * Each line is XY<space>path: X is the index status, Y is the working tree status, and '??' means untracked.
 * Rename lines have the form `R  old -> new`; use the new path.
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
      // Prefer the index status, otherwise use the working tree status.
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

/** Parse untracked paths from `git ls-files --others --exclude-standard` output. */
export function parseUntrackedList(output: string): string[] {
  return output.split('\n').map((line) => unquoteGitPath(line.trim())).filter(Boolean);
}

/** Merge tracked changes with untracked files, deduplicating by path. */
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
 * Build the workspace file list from git diff and ls-files output.
 * Match OCR CLI workspace mode: use diff HEAD, fall back to staged changes if empty, then merge untracked files.
 */
export function buildWorkspaceFiles(diffHeadOut: string, diffCachedOut: string, untrackedOut: string): FileChange[] {
  let tracked = parseNameStatus(diffHeadOut);
  if (tracked.length === 0) {
    tracked = parseNameStatus(diffCachedOut);
  }
  return mergeWorkspaceFiles(tracked, parseUntrackedList(untrackedOut));
}

/**
 * Choose the candidate repository root that matches the workspace.
 * The VS Code Git extension scans nested repositories asynchronously, so choosing [0] may select a child repository.
 * Priority: exact workspace root match > deepest workspace ancestor > first candidate.
 */
export function pickRepoRoot(roots: string[], workspacePath?: string): string | null {
  if (roots.length === 0) return null;
  if (!workspacePath) return roots[0];

  const candidates = roots.map((root) => {
    const isWindowsPath = [root, workspacePath].some((p) => /^[a-z]:[\\/]/i.test(p) || p.includes('\\'));
    const pathApi = isWindowsPath ? path.win32 : path.posix;
    return { root, relative: pathApi.relative(root, workspacePath), pathApi };
  });

  const exact = candidates.find(({ relative }) => relative === '');
  if (exact) return exact.root;

  const ancestors = candidates.filter(({ relative, pathApi }) =>
    relative !== '..' && !relative.startsWith(`..${pathApi.sep}`) && !pathApi.isAbsolute(relative));
  if (ancestors.length > 0) {
    return ancestors.reduce((deepest, candidate) =>
      candidate.relative.length < deepest.relative.length ? candidate : deepest).root;
  }

  return roots[0];
}

/** Build candidate branch references for rev-parse validation. */
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
 * Decode Git quotepath escapes (with core.quotepath=true, non-ASCII paths use octal escapes such as "\344\273...").
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
 * Parse `git diff --name-status` or `git show --name-status` output.
 * Lines are tab-separated: status<TAB>path, or R<score><TAB>old<TAB>new for renames (use new).
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
