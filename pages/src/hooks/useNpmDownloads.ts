// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { useEffect, useState } from 'react';

type Period = 'last-day' | 'last-week' | 'last-month' | 'last-year';

interface NpmDownloadsState {
  downloads: number | null;
  loading: boolean;
  error: boolean;
}

/**
 * 实时获取某个 npm 包的下载量。
 * 数据源为 npm 官方统计 API（支持 CORS，可在纯静态页面直接调用）。
 * 请求失败时 error 为 true，调用方可据此优雅降级。
 *
 * 当前用于 HighlightsSection 的「NPM 社区下载量」指标：请求进行中或失败时，
 * 组件回退到 i18n 中的兜底静态值。
 */
export function useNpmDownloads(pkg: string, period: Period = 'last-month'): NpmDownloadsState {
  const [state, setState] = useState<NpmDownloadsState>({
    downloads: null,
    loading: true,
    error: false,
  });

  useEffect(() => {
    let cancelled = false;
    setState({ downloads: null, loading: true, error: false });

    // 弱网或 API 无响应时，超时后 abort 请求并降级，避免界面长期卡在 loading
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), 8000);

    // pkg 可能是 scoped 包名（含 `/`），编码后再插值以保证 URL 路径合法
    fetch(`https://api.npmjs.org/downloads/point/${period}/${encodeURIComponent(pkg)}`, {
      signal: controller.signal,
    })
      .then((r) => {
        if (!r.ok) throw new Error(`npm downloads API responded ${r.status}`);
        return r.json();
      })
      .then((data: { downloads?: number }) => {
        if (cancelled) return;
        if (typeof data.downloads !== 'number') throw new Error('unexpected payload');
        setState({ downloads: data.downloads, loading: false, error: false });
      })
      .catch(() => {
        if (cancelled) return;
        setState({ downloads: null, loading: false, error: true });
      });

    return () => {
      cancelled = true;
      clearTimeout(timeout);
      controller.abort();
    };
  }, [pkg, period]);

  return state;
}
