import * as vscode from 'vscode';
import { SIDEBAR_VIEW_ID } from '../shared/constants';
import { CliService } from './services/CliService';
import { ConfigService } from './services/ConfigService';
import { GitService } from './services/GitService';
import { CommentProvider } from './providers/CommentProvider';
import { SidebarProvider } from './providers/SidebarProvider';
import { ConfigPanelProvider } from './providers/ConfigPanelProvider';
import { registerCommands } from './commands';

let disposables: vscode.Disposable[] = [];

export function activate(context: vscode.ExtensionContext): void {
  const extensionUri = context.extensionUri;
  const output = vscode.window.createOutputChannel('Open Code Review');
  const cli = new CliService('ocr');
  const config = new ConfigService(cli);
  const git = new GitService(output);
  const comments = new CommentProvider(extensionUri, git);

  const sidebar = new SidebarProvider(extensionUri, cli, config, git, comments);
  const configPanel = new ConfigPanelProvider(extensionUri, cli, config, (cfg) => sidebar.pushConfig(cfg));
  sidebar.bindConfigPanel((focus) => configPanel.open(focus));

  const viewReg = vscode.window.registerWebviewViewProvider(
    SIDEBAR_VIEW_ID,
    sidebar,
    { webviewOptions: { retainContextWhenHidden: true } },
  );
  const cmdReg = registerCommands(comments, () => configPanel.open());

  disposables.push(viewReg, cmdReg, comments, output, configPanel);
  context.subscriptions.push(...disposables);
}

export function deactivate(): void {
  disposables.forEach((d) => d.dispose());
  disposables = [];
}
