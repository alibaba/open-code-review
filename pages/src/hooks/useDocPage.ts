// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { useCallback, useEffect, useState } from 'react';
import { loadDocPage, type DocPageData, type DocSlug } from '../content/docs';

export type DocPageLoader = (slug: DocSlug, language: string) => Promise<DocPageData>;

interface DocPageState {
  requestKey: string;
  status: 'loading' | 'ready' | 'error';
  data: DocPageData | null;
  error: Error | null;
}

export function useDocPage(
  slug: DocSlug,
  language: string,
  loader: DocPageLoader = loadDocPage,
) {
  const [attempt, setAttempt] = useState(0);
  const requestKey = `${language}:${slug}:${attempt}`;
  const [state, setState] = useState<DocPageState>({
    requestKey: '',
    status: 'loading',
    data: null,
    error: null,
  });

  useEffect(() => {
    let active = true;
    setState({ requestKey, status: 'loading', data: null, error: null });
    loader(slug, language).then(
      data => {
        if (active) setState({ requestKey, status: 'ready', data, error: null });
      },
      reason => {
        if (!active) return;
        const error = reason instanceof Error ? reason : new Error(String(reason));
        setState({ requestKey, status: 'error', data: null, error });
      },
    );
    return () => {
      active = false;
    };
  }, [language, loader, requestKey, slug]);

  const retry = useCallback(() => setAttempt(value => value + 1), []);
  if (state.requestKey !== requestKey) {
    return { status: 'loading' as const, data: null, error: null, retry };
  }
  return { status: state.status, data: state.data, error: state.error, retry };
}
