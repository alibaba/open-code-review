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
 * Fetch the live download count for an npm package.
 * The data source is the official npm stats API (CORS-enabled, callable directly from a purely static page).
 * When the request fails, `error` is true so callers can degrade gracefully.
 *
 * Currently used by the "NPM community downloads" stat in HighlightsSection: while the
 * request is in flight or has failed, the component falls back to the static i18n value.
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

    // On a slow network or an unresponsive API, abort the request after a timeout and degrade,
    // so the UI does not stay stuck in the loading state indefinitely
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), 8000);

    (async () => {
      try {
        // pkg may be a scoped package name (containing `/`), so encode it before interpolation to keep the URL path valid
        const r = await fetch(`https://api.npmjs.org/downloads/point/${period}/${encodeURIComponent(pkg)}`, {
          signal: controller.signal,
        });
        if (!r.ok) throw new Error(`npm downloads API responded ${r.status}`);
        const data: { downloads?: number } = await r.json();
        if (cancelled) return;
        if (typeof data.downloads !== 'number') throw new Error('unexpected payload');
        setState({ downloads: data.downloads, loading: false, error: false });
      } catch {
        if (cancelled) return;
        setState({ downloads: null, loading: false, error: true });
      } finally {
        // Clear the timeout as soon as the request settles, so it doesn't linger and abort a finished request
        clearTimeout(timeout);
      }
    })();

    return () => {
      cancelled = true;
      clearTimeout(timeout);
      controller.abort();
    };
  }, [pkg, period]);

  return state;
}
