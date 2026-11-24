// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { HostToWebview, WebviewToHost } from '../shared/messages';

/**
 * Host bridge contract shared by both targets. Entry apps import './bridge';
 * webpack's NormalModuleReplacementPlugin swaps that specifier for the target
 * implementation (bridge.vsc.ts or bridge.idea.ts) at bundle time, so this
 * facade is never bundled or executed. It exists so tsc and jest can resolve
 * the import without a bundler in the loop.
 */
export interface Bridge {
  post(msg: WebviewToHost): void;
  onMessage(handler: (msg: HostToWebview) => void): () => void;
}

export declare const bridge: Bridge;
