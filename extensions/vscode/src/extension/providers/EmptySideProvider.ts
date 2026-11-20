// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import * as vscode from 'vscode';

/**
 * Scheme for the blank side of a diff: the "before" of an added file, the
 * "after" of a deleted one.
 *
 * The obvious spellings are both wrong. The platform null device cannot be
 * read through VS Code's file system provider -- on Windows `\\.\NUL` fails
 * with "Unavailable (FileSystemError)" and the editor reports that the diff
 * could not be opened (#1816). A `git:` URI with an empty ref is worse,
 * because an empty treeish is not a sentinel for "nothing": VS Code routes it
 * to `ls-files`, so it addresses the *index*. Right after a root commit the
 * index still holds the new file and the "empty" side renders its full
 * contents, hiding the addition; after a committed deletion the path is absent
 * from the index and the read fails instead. It would also make a historical
 * diff depend on whatever happens to be staged now.
 *
 * VS Code's own git provider does return empty bytes, but only for the ref
 * equal to that repository's empty tree, and `getEmptyTree()` is internal to
 * the built-in extension rather than part of the API it exposes. Owning a
 * scheme here is both simpler and independent of that.
 */
export const EMPTY_SIDE_SCHEME = 'ocr-empty';

/** Always resolves to an empty document, whatever the path. */
export class EmptySideProvider implements vscode.TextDocumentContentProvider {
  provideTextDocumentContent(): string {
    return '';
  }
}

/**
 * A URI for the blank side of `relPath`'s diff.
 *
 * The path is carried through so the editor's diff title still names the file
 * rather than showing a bare scheme.
 */
export function emptySideUri(relPath: string): vscode.Uri {
  return vscode.Uri.from({ scheme: EMPTY_SIDE_SCHEME, path: `/${relPath.replace(/^\/+/, '')}` });
}
