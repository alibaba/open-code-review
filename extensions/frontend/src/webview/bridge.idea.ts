import { HostToWebview, WebviewToHost } from '../shared/messages';

/**
 * frontend/ 目录里唯一相对上游改写的文件，其余与上游逐字节相同（改动请回上游改）。
 *
 * 上游走 `acquireVsCodeApi()` + `addEventListener('message')`；IDEA 没这套 API，宿主注入两个全局：
 * `window.__ocrPost(json)`（JBCefJSQuery 注入，回传 Kotlin）、`window.__ocrReceive(msg)`（Kotlin 调，推进页面）。
 *
 * `onMessage` 保持"可多次订阅+返回退订"语义：上游 useEffect 这么用，直接覆盖成单 handler 会把第二个订阅者顶掉。
 */

declare global {
  interface Window {
    __ocrPost?: (json: string) => void;
    __ocrReceive?: (msg: unknown) => void;
  }
}

type Handler = (msg: HostToWebview) => void;

const handlers = new Set<Handler>();

// 宿主侧 executeJavaScript 调的就是这个函数。注册一次，之后只往 handlers 里分发。
window.__ocrReceive = (msg: unknown): void => {
  handlers.forEach((handler) => {
    try {
      handler(msg as HostToWebview);
    } catch (err) {
      // 一个订阅者抛异常不该让别的订阅者收不到消息。
      console.error('[ocr] message handler failed', err);
    }
  });
};

export const bridge = {
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
