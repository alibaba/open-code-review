// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import React from 'react';

const CHUNK_RELOAD_STORAGE_KEY = 'ocr-chunk-reload-attempted';

export type FallbackRender = (error: Error, reset: () => void) => React.ReactNode;

interface ErrorBoundaryProps {
  children: React.ReactNode;
  fallback: React.ReactNode | FallbackRender;
  reloadOnChunkError?: boolean;
}

interface ErrorBoundaryState {
  error: Error | null;
}

/** Identify webpack and browser dynamic-import failures without reloading ordinary errors. */
export function isChunkLoadError(error: Error): boolean {
  return (
    error.name === 'ChunkLoadError' ||
    /Loading( CSS)? chunk .* failed/i.test(error.message) ||
    /failed to fetch dynamically imported module/i.test(error.message)
  );
}

/**
 * Reload after a user confirms a persistent chunk failure. Clearing the
 * automatic-reload guard lets the new runtime make one guarded recovery
 * attempt, while the boundary still prevents an endless reload loop.
 */
export function reloadAfterChunkError(): void {
  try {
    window.sessionStorage.removeItem(CHUNK_RELOAD_STORAGE_KEY);
  } catch {}
  window.location.reload();
}

export default class ErrorBoundary extends React.Component<ErrorBoundaryProps, ErrorBoundaryState> {
  state: ErrorBoundaryState = { error: null };

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { error };
  }

  componentDidCatch(error: Error, info: React.ErrorInfo): void {
    console.error('ErrorBoundary caught an error:', error, info);
    if (this.props.reloadOnChunkError && isChunkLoadError(error) && this.markChunkReloadAttempt()) {
      window.location.reload();
    }
  }

  private markChunkReloadAttempt(): boolean {
    try {
      if (window.sessionStorage.getItem(CHUNK_RELOAD_STORAGE_KEY)) return false;
      window.sessionStorage.setItem(CHUNK_RELOAD_STORAGE_KEY, 'true');
      return true;
    } catch {
      return false;
    }
  }

  private reset = (): void => {
    reloadAfterChunkError();
  };

  render(): React.ReactNode {
    const { error } = this.state;
    if (!error) return this.props.children;

    const { fallback } = this.props;
    return typeof fallback === 'function' ? fallback(error, this.reset) : fallback;
  }
}
