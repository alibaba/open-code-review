// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { HostToWebview, WebviewToHost } from '../shared/messages';
import type { Bridge } from './bridge';

interface VsCodeApi { postMessage(msg: unknown): void; }
declare function acquireVsCodeApi(): VsCodeApi;

const vscode = acquireVsCodeApi();

export const bridge: Bridge = {
  post(msg: WebviewToHost): void {
    vscode.postMessage(msg);
  },
  onMessage(handler: (msg: HostToWebview) => void): () => void {
    const listener = (e: MessageEvent) => handler(e.data as HostToWebview);
    window.addEventListener('message', listener);
    return () => window.removeEventListener('message', listener);
  },
};
