// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import * as vscode from 'vscode';
import { ReviewComment, ReviewContext, ReviewMode } from '@shared/types';

export type CommentMountSide = 'left' | 'right' | 'workspace';

/** A resolved anchor that can be attached to an editor or diff. */
export interface MountableCommentAnchor {
  kind: 'mountable';
  uri: vscode.Uri;
  range: vscode.Range;
  side: CommentMountSide;
  /** Open the native diff when navigating in commit or branch mode. */
  diff?: {
    left: vscode.Uri;
    right: vscode.Uri;
    title: string;
  };
  /** Explanation shown when line numbers are relocated using existingCode. */
  locateNote?: string;
}

/** An anchor that cannot be resolved in a snapshot and is shown only in the sidebar. */
export interface SidebarOnlyCommentAnchor {
  kind: 'sidebar';
  reason: 'binary' | 'unresolved' | 'missing-file';
}

export type SidebarOnlyReason = SidebarOnlyCommentAnchor['reason'];

export type CommentAnchorResult = MountableCommentAnchor | SidebarOnlyCommentAnchor;

export interface CommentAnchorDeps {
  repoRoot: string;
  fileStatus: (path: string) => Promise<'added' | 'modified' | 'deleted' | 'renamed' | 'binary' | null>;
  readAtRef: (ref: string, path: string) => Promise<string | null>;
  readWorkspace: (path: string) => Promise<string | null>;
  buildDiffUris: (
    path: string,
    status: 'added' | 'modified' | 'deleted' | 'renamed' | 'binary',
  ) => Promise<{
    left: vscode.Uri;
    right: vscode.Uri;
    title: string;
    mountRef: string;
    mountSide: 'left' | 'right';
    leftRef: string | null;
    rightRef: string | null;
  } | null>;
  toGitUri: (path: string, ref: string) => Promise<vscode.Uri>;
}

/** Trim a line, and nothing else: on the content side a leading '+' or '-' is code, not a diff marker. */
export function normalizeLine(s: string): string {
  return s.trim();
}

/** Trim each line and drop the blank ones, so a snippet's blank lines never become match targets of their own. */
function normalizeLines(lines: string[]): string[] {
  return lines.map(normalizeLine).filter((line) => line !== '');
}

/**
 * Remove one leading diff marker from each line: the reading for a snippet quoted
 * out of diff output ("+  - name: app" becomes "- name: app").
 *
 * Exactly one marker per line, and only from the first character, so a deleted
 * YAML list item keeps its own dash ("-- name: app" becomes "- name: app"). A line
 * that is nothing but a marker loses the marker, is left blank, and is dropped.
 */
function stripDiffMarkers(lines: string[]): string[] {
  return normalizeLines(
    lines.map((line) => (line.startsWith('+') || line.startsWith('-') ? line.slice(1) : line)),
  );
}

/**
 * The readings an existingCode snippet may be matched against, in priority order.
 *
 * Verbatim first: the model copied the code out of the file, where a leading '-' is
 * code, a YAML list item being the everyday case. Diff-quoted second: the model
 * copied it out of the diff instead, where that first character is a marker. Both
 * readings are returned even when they are identical, so callers stop at the first
 * one that matches.
 */
export function snippetForms(existingCode: string): string[][] {
  const lines = existingCode.split('\n');
  const forms: string[][] = [];
  const verbatim = normalizeLines(lines);
  if (verbatim.length > 0) forms.push(verbatim);
  const stripped = stripDiffMarkers(lines);
  if (stripped.length > 0) forms.push(stripped);
  return forms;
}

/** Find existingCode in the file content using a sliding-window match and return 1-based line numbers. */
export function findLinesByExistingCode(content: string, existingCode: string): { start: number; end: number } | null {
  const forms = snippetForms(existingCode);
  if (forms.length === 0) return null;

  const fileLines = content.split('\n');
  const normalized: string[] = [];
  const lineNums: number[] = [];
  for (let i = 0; i < fileLines.length; i++) {
    const n = normalizeLine(fileLines[i].replace(/\r$/, ''));
    if (!n) continue;
    normalized.push(n);
    lineNums.push(i + 1);
  }

  for (const targetLines of forms) {
    if (normalized.length < targetLines.length) continue;
    for (let i = 0; i <= normalized.length - targetLines.length; i++) {
      let matched = true;
      for (let j = 0; j < targetLines.length; j++) {
        if (normalized[i + j] !== targetLines[j]) {
          matched = false;
          break;
        }
      }
      if (matched) {
        return { start: lineNums[i], end: lineNums[i + targetLines.length - 1] };
      }
    }
  }
  return null;
}

export function resolveLinesInContent(
  content: string,
  startLine: number,
  endLine: number,
  existingCode?: string,
): { start: number; end: number; relocated: boolean } | null {
  const lineCount = content.split('\n').length;
  const start = startLine > 0 ? startLine : 0;
  const end = endLine > 0 ? endLine : start;

  if (start > 0 && end > 0 && start <= lineCount && end <= lineCount && start <= end) {
    return { start, end, relocated: false };
  }

  if (existingCode?.trim()) {
    const found = findLinesByExistingCode(content, existingCode);
    if (found) return { ...found, relocated: true };
  }
  return null;
}

function toRange(start: number, end: number): vscode.Range {
  return new vscode.Range(Math.max(0, start - 1), 0, Math.max(0, end - 1), 0);
}

export async function resolveCommentAnchor(
  comment: ReviewComment,
  ctx: ReviewContext,
  deps: CommentAnchorDeps,
): Promise<CommentAnchorResult> {
  const status = await deps.fileStatus(comment.path);
  if (status === 'binary') return { kind: 'sidebar', reason: 'binary' };
  if (status === null && ctx.mode !== ReviewMode.Workspace) {
    return { kind: 'sidebar', reason: 'missing-file' };
  }

  const effectiveStatus = status ?? 'modified';

  if (ctx.mode === ReviewMode.Workspace) {
    const content = await deps.readWorkspace(comment.path);
    if (content === null) return { kind: 'sidebar', reason: 'missing-file' };
    const lines = resolveLinesInContent(content, comment.startLine, comment.endLine, comment.existingCode);
    if (!lines) return { kind: 'sidebar', reason: 'unresolved' };
    const uri = vscode.Uri.file(`${deps.repoRoot}/${comment.path}`);
    return {
      kind: 'mountable',
      uri,
      range: toRange(lines.start, lines.end),
      side: 'workspace',
      locateNote: lines.relocated ? formatLocateNote(comment.startLine, lines.start) : undefined,
    };
  }

  const diff = await deps.buildDiffUris(comment.path, effectiveStatus);
  if (!diff) return { kind: 'sidebar', reason: 'missing-file' };

  const primaryRef = diff.mountRef;
  const primaryContent = await deps.readAtRef(primaryRef, comment.path);
  let lines = primaryContent
    ? resolveLinesInContent(primaryContent, comment.startLine, comment.endLine, comment.existingCode)
    : null;

  let mountRef = primaryRef;
  let mountSide = diff.mountSide;

  if (!lines && effectiveStatus !== 'added') {
    const altRef = diff.mountSide === 'right' ? diff.leftRef : diff.rightRef;
    const altSide: CommentMountSide = diff.mountSide === 'right' ? 'left' : 'right';
    if (altRef) {
      const altContent = await deps.readAtRef(altRef, comment.path);
      const altLines = altContent
        ? resolveLinesInContent(altContent, comment.startLine, comment.endLine, comment.existingCode)
        : null;
      if (altLines) {
        lines = altLines;
        mountRef = altRef;
        mountSide = altSide;
      }
    }
  }

  if (!lines) return { kind: 'sidebar', reason: 'unresolved' };

  const uri = await deps.toGitUri(comment.path, mountRef);
  return {
    kind: 'mountable',
    uri,
    range: toRange(lines.start, lines.end),
    side: mountSide,
    diff: { left: diff.left, right: diff.right, title: diff.title },
    locateNote: lines.relocated ? formatLocateNote(comment.startLine, lines.start) : undefined,
  };
}

function formatLocateNote(originalLine: number, resolvedLine: number): string {
  if (originalLine > 0 && originalLine !== resolvedLine) {
    return `⚠ Original line L${originalLine} could not be matched; showing L${resolvedLine} instead.`;
  }
  return '⚠ Line number was re-located from code content.';
}
