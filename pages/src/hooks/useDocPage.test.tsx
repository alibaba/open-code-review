// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { act, renderHook, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import type { DocPageData, DocSlug } from '../content/docs';
import { useDocPage, type DocPageLoader } from './useDocPage';

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>(resolvePromise => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

describe('useDocPage', () => {
  it('ignores an older response after navigation changes the requested document', async () => {
    const first = deferred<DocPageData>();
    const second = deferred<DocPageData>();
    const loader: DocPageLoader = vi.fn(slug => slug === 'quickstart' ? first.promise : second.promise);
    const { result, rerender } = renderHook(
      ({ slug }: { slug: DocSlug }) => useDocPage(slug, 'en', loader),
      { initialProps: { slug: 'quickstart' as DocSlug } },
    );

    await waitFor(() => expect(loader).toHaveBeenCalledWith('quickstart', 'en'));
    rerender({ slug: 'faq' });
    await waitFor(() => expect(loader).toHaveBeenCalledWith('faq', 'en'));

    await act(async () => {
      first.resolve({ slug: 'quickstart', title: 'Old', content: 'Old body' });
      await first.promise;
    });
    expect(result.current.status).toBe('loading');
    expect(result.current.data).toBeNull();

    await act(async () => {
      second.resolve({ slug: 'faq', title: 'Current', content: 'Current body' });
      await second.promise;
    });
    await waitFor(() => expect(result.current.status).toBe('ready'));
    expect(result.current.data?.slug).toBe('faq');
  });

  it('exposes an error and retries without reloading the page', async () => {
    const loader = vi.fn()
      .mockRejectedValueOnce(new Error('download failed'))
      .mockResolvedValueOnce({ slug: 'quickstart', title: 'Ready', content: 'Body' });
    const { result } = renderHook(() => useDocPage('quickstart', 'en', loader));

    await waitFor(() => expect(result.current.status).toBe('error'));
    expect(result.current.error?.message).toBe('download failed');
    act(() => result.current.retry());
    await waitFor(() => expect(result.current.status).toBe('ready'));
    expect(result.current.data?.title).toBe('Ready');
    expect(loader).toHaveBeenCalledTimes(2);
  });
});
