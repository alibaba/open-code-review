// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import * as vscode from 'vscode';
import { COMMANDS } from '@shared/constants';
import { CommentProvider } from './providers/CommentProvider';

export function registerCommands(
  comments: CommentProvider,
  openConfig: () => void,
): vscode.Disposable {
  const subs: vscode.Disposable[] = [];
  const reg = (id: string, fn: (...args: any[]) => any) =>
    subs.push(vscode.commands.registerCommand(id, fn));

  reg(COMMANDS.configOpen, openConfig);

  // Title-bar buttons pass a CommentThread; sidebar and Markdown links pass an index.
  const idxOf = (arg: vscode.CommentThread | number): number =>
    typeof arg === 'number' ? arg : comments.indexOfThread(arg);

  reg(COMMANDS.commentApply, (arg: vscode.CommentThread | number) => {
    const i = idxOf(arg);
    if (i >= 0) comments.apply(i);
  });
  reg(COMMANDS.commentDiscard, (arg: vscode.CommentThread | number) => {
    const i = idxOf(arg);
    if (i >= 0) comments.discard(i);
  });
  reg(COMMANDS.commentFalsePositive, (arg: vscode.CommentThread | number) => {
    const i = idxOf(arg);
    if (i >= 0) comments.falsePositive(i);
  });

  return vscode.Disposable.from(...subs);
}
