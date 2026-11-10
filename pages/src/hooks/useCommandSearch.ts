// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { useCallback, useEffect, useRef, useState } from 'react';
import type { Dispatch, KeyboardEvent as ReactKeyboardEvent, SetStateAction } from 'react';

interface SearchResult<S extends string> {
  slug: S;
  title: string;
  snippet: string;
}

type SearchRunner<S extends string> = (
  query: string,
  language: string,
) => SearchResult<S>[] | Promise<SearchResult<S>[]>;

interface SearchState<S extends string> {
  requestKey: string;
  status: 'idle' | 'loading' | 'ready' | 'error';
  results: SearchResult<S>[];
  error: Error | null;
}

export function useCommandSearch<S extends string>(
  runSearch: SearchRunner<S>,
  language: string,
) {
  const [searchOpen, setSearchOpen] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');
  const [searchSelectedIdx, setSearchSelectedIdx] = useState(0);
  const [searchAttempt, setSearchAttempt] = useState(0);
  const searchInputRef = useRef<HTMLInputElement>(null);
  const requestKey = `${language}:${searchQuery}:${searchAttempt}`;
  const [searchState, setSearchState] = useState<SearchState<S>>({
    requestKey: '',
    status: 'idle',
    results: [],
    error: null,
  });

  useEffect(() => {
    if (!searchQuery.trim()) {
      setSearchState({ requestKey, status: 'idle', results: [], error: null });
      return;
    }

    let active = true;
    setSearchState({ requestKey, status: 'loading', results: [], error: null });
    try {
      Promise.resolve(runSearch(searchQuery, language)).then(
        results => {
          if (active) setSearchState({ requestKey, status: 'ready', results, error: null });
        },
        reason => {
          if (!active) return;
          const error = reason instanceof Error ? reason : new Error(String(reason));
          setSearchState({ requestKey, status: 'error', results: [], error });
        },
      );
    } catch (reason) {
      const error = reason instanceof Error ? reason : new Error(String(reason));
      setSearchState({ requestKey, status: 'error', results: [], error });
    }
    return () => {
      active = false;
    };
  }, [language, requestKey, runSearch, searchQuery]);

  const currentSearchState = searchState.requestKey === requestKey
    ? searchState
    : { requestKey, status: searchQuery.trim() ? 'loading' as const : 'idle' as const, results: [], error: null };
  const searchResults = currentSearchState.results;
  const retrySearch = useCallback(() => setSearchAttempt(value => value + 1), []);

  /* Reset selection when results change */
  useEffect(() => {
    setSearchSelectedIdx(0);
  }, [searchResults]);

  /* ⌘K / Ctrl+K keyboard shortcut */
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
        // Don't intercept when focused in other input/textarea (unless search is already open)
        const activeEl = document.activeElement;
        if (activeEl && (activeEl.tagName === 'INPUT' || activeEl.tagName === 'TEXTAREA') && !searchOpen) return;
        e.preventDefault();
        setSearchOpen(prev => !prev);
      }
      if (e.key === 'Escape') {
        setSearchOpen(false);
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [searchOpen]);

  /* Focus input when search opens */
  useEffect(() => {
    if (searchOpen) {
      setTimeout(() => searchInputRef.current?.focus(), 50);
    } else {
      setSearchQuery('');
    }
  }, [searchOpen]);

  return {
    searchOpen,
    setSearchOpen,
    searchQuery,
    setSearchQuery,
    searchSelectedIdx,
    setSearchSelectedIdx,
    searchInputRef,
    searchResults,
    searchStatus: currentSearchState.status,
    searchError: currentSearchState.error,
    retrySearch,
  };
}

export function useSearchKeyboardNav<S extends string>(
  searchResults: readonly SearchResult<S>[],
  searchSelectedIdx: number,
  setSearchSelectedIdx: Dispatch<SetStateAction<number>>,
  onSelect: (slug: S) => void,
) {
  return useCallback(
    (e: ReactKeyboardEvent) => {
      if (e.key === 'ArrowDown') {
        e.preventDefault();
        setSearchSelectedIdx(prev => Math.min(prev + 1, searchResults.length - 1));
      } else if (e.key === 'ArrowUp') {
        e.preventDefault();
        setSearchSelectedIdx(prev => Math.max(prev - 1, 0));
      } else if (e.key === 'Enter' && searchResults.length > 0) {
        e.preventDefault();
        onSelect(searchResults[searchSelectedIdx].slug);
      }
    },
    [searchResults, searchSelectedIdx, setSearchSelectedIdx, onSelect],
  );
}
