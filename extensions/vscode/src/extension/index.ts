import * as vscode from 'vscode';
import { SIDEBAR_VIEW_ID } from '../shared/constants';
import { CliService } from './services/CliService';
import { ConfigService } from './services/ConfigService';
import { GitService } from './services/GitService';
import { CommentProvider } from './providers/CommentProvider';
import { SidebarProvider } from './providers/SidebarProvider';
import { registerCommands } from './commands';

export interface OcrAdapter {
  extensionUri?: vscode.Uri;
  telemetry?: (event: string, data: Record<string, unknown>) => void;
  cliPath?: string;
}

let disposables: vscode.Disposable[] = [];

export function activateOcr(context: vscode.ExtensionContext, adapter: OcrAdapter = {}): void {
  const extensionUri = adapter.extensionUri ?? context.extensionUri;
  const output = vscode.window.createOutputChannel('Open Code Review');
  const cli = new CliService(adapter.cliPath ?? 'ocr');
  const config = new ConfigService(cli);
  const git = new GitService(output);
  const comments = new CommentProvider(extensionUri);

  const sidebar = new SidebarProvider(extensionUri, cli, config, git, comments);
  const viewReg = vscode.window.registerWebviewViewProvider(SIDEBAR_VIEW_ID, sidebar);

  const cmdReg = registerCommands(comments);

  disposables.push(viewReg, cmdReg, comments, output);
  context.subscriptions.push(...disposables);
}

export function deactivateOcr(): void {
  disposables.forEach((d) => d.dispose());
  disposables = [];
}
