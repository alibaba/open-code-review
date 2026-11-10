// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { act, renderHook, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { useCommandSearch } from './useCommandSearch';

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>(resolvePromise => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

describe('useCommandSearch', () => {
  it('keeps synchronous blog-style search behavior', async () => {
    const runSearch = vi.fn((query: string) => [
      { slug: 'post', title: query, snippet: 'result' },
    ]);
    const { result } = renderHook(() => useCommandSearch(runSearch, 'en'));

    act(() => result.current.setSearchQuery('term'));
    await waitFor(() => expect(result.current.searchStatus).toBe('ready'));
    expect(result.current.searchResults).toEqual([
      { slug: 'post', title: 'term', snippet: 'result' },
    ]);
  });

  it('ignores stale asynchronous results after the query changes', async () => {
    const first = deferred<{ slug: string; title: string; snippet: string }[]>();
    const second = deferred<{ slug: string; title: string; snippet: string }[]>();
    const runSearch = vi.fn((query: string) => query === 'first' ? first.promise : second.promise);
    const { result } = renderHook(() => useCommandSearch(runSearch, 'en'));

    act(() => result.current.setSearchQuery('first'));
    await waitFor(() => expect(runSearch).toHaveBeenCalledWith('first', 'en'));
    act(() => result.current.setSearchQuery('second'));
    await waitFor(() => expect(runSearch).toHaveBeenCalledWith('second', 'en'));

    await act(async () => {
      first.resolve([{ slug: 'old', title: 'Old', snippet: 'Old' }]);
      await first.promise;
    });
    expect(result.current.searchStatus).toBe('loading');
    expect(result.current.searchResults).toEqual([]);

    await act(async () => {
      second.resolve([{ slug: 'new', title: 'New', snippet: 'New' }]);
      await second.promise;
    });
    await waitFor(() => expect(result.current.searchStatus).toBe('ready'));
    expect(result.current.searchResults[0].slug).toBe('new');
  });

  it('reports a failed search and retries the current query', async () => {
    const runSearch = vi.fn()
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValueOnce([{ slug: 'ready', title: 'Ready', snippet: 'Ready' }]);
    const { result } = renderHook(() => useCommandSearch(runSearch, 'en'));

    act(() => result.current.setSearchQuery('term'));
    await waitFor(() => expect(result.current.searchStatus).toBe('error'));
    expect(result.current.searchError?.message).toBe('offline');
    act(() => result.current.retrySearch());
    await waitFor(() => expect(result.current.searchStatus).toBe('ready'));
    expect(result.current.searchResults[0].slug).toBe('ready');
    expect(runSearch).toHaveBeenCalledTimes(2);
  });
});
