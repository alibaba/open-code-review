// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { HostToWebview, WebviewToHost } from '../shared/messages';
import type { Bridge } from './bridge';

/**
 * The only file under frontend/ that diverges from upstream; everything else is
 * byte-identical to upstream (send changes there instead).
 *
 * Upstream uses `acquireVsCodeApi()` + `addEventListener('message')`; IDEA has
 * no such API, so the host injects two globals:
 * `window.__ocrPost(json)` (injected via JBCefJSQuery, delivers to Kotlin) and
 * `window.__ocrReceive(msg)` (called by Kotlin, pushes into the page).
 *
 * `onMessage` keeps the "multi-subscribe + unsubscribe handle" semantics:
 * upstream useEffect relies on them, and replacing this with a single-handler
 * assignment would silently drop the second subscriber.
 */

declare global {
  interface Window {
    __ocrPost?: (json: string) => void;
    __ocrReceive?: (msg: unknown) => void;
  }
}

type Handler = (msg: HostToWebview) => void;

const handlers = new Set<Handler>();

// The host calls this exact function via executeJavaScript. Registered once,
// it only dispatches into handlers from then on.
window.__ocrReceive = (msg: unknown): void => {
  handlers.forEach((handler) => {
    try {
      handler(msg as HostToWebview);
    } catch (err) {
      // One subscriber throwing must not stop the others from receiving the message.
      console.error('[ocr] message handler failed', err);
    }
  });
};

export const bridge: Bridge = {
  post(msg: WebviewToHost): void {
    const post = window.__ocrPost;
    if (!post) {
      console.error('[ocr] host bridge is not injected yet', msg);
      return;
    }
    post(JSON.stringify(msg));
  },
  onMessage(handler: Handler): () => void {
    handlers.add(handler);
    return () => {
      handlers.delete(handler);
    };
  },
};
