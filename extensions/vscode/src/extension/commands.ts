import * as vscode from 'vscode';
import { COMMANDS } from '../shared/constants';
import { CommentProvider } from './providers/CommentProvider';

export function registerCommands(comments: CommentProvider): vscode.Disposable {
  const subs: vscode.Disposable[] = [];
  const reg = (id: string, fn: (...args: any[]) => any) =>
    subs.push(vscode.commands.registerCommand(id, fn));

  // 标题栏按钮传入的是 CommentThread，侧边栏 / Markdown 链接传入的是 index
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
