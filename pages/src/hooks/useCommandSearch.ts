import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { Dispatch, KeyboardEvent as ReactKeyboardEvent, SetStateAction } from 'react';

interface SearchResult<S extends string> {
  slug: S;
  title: string;
  snippet: string;
}

export function useCommandSearch<S extends string>(
  runSearch: (query: string, language: string) => SearchResult<S>[],
  language: string,
) {
  const [searchOpen, setSearchOpen] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');
  const [searchSelectedIdx, setSearchSelectedIdx] = useState(0);
  const searchInputRef = useRef<HTMLInputElement>(null);

  const searchResults = useMemo(
    () => runSearch(searchQuery, language),
    [runSearch, searchQuery, language],
  );

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
