import * as vscode from 'vscode';
import { activateOcr, deactivateOcr } from './index';

export function activate(context: vscode.ExtensionContext): void {
  activateOcr(context);
}

export function deactivate(): void {
  deactivateOcr();
}
