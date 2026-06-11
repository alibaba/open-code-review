import { HostToWebview, WebviewToHost } from '../shared/messages';

interface VsCodeApi { postMessage(msg: unknown): void; }
declare function acquireVsCodeApi(): VsCodeApi;

const vscode = acquireVsCodeApi();

export const bridge = {
  post(msg: WebviewToHost): void {
    vscode.postMessage(msg);
  },
  onMessage(handler: (msg: HostToWebview) => void): void {
    window.addEventListener('message', (e) => handler(e.data as HostToWebview));
  },
};
