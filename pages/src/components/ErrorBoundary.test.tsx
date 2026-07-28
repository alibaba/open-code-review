import type { ErrorInfo } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import ErrorBoundary from './ErrorBoundary';

const STORAGE_KEY = 'ocr-chunk-reload-attempted';
const errorInfo = {} as ErrorInfo;

describe('ErrorBoundary', () => {
  let reload: ReturnType<typeof vi.fn>;
  let storage: Map<string, string>;

  beforeEach(() => {
    reload = vi.fn();
    storage = new Map();
    vi.stubGlobal('window', {
      location: { reload },
      sessionStorage: {
        getItem: (key: string) => storage.get(key) ?? null,
        setItem: (key: string, value: string) => storage.set(key, value),
        removeItem: (key: string) => storage.delete(key),
      },
    });
    vi.spyOn(console, 'error').mockImplementation(() => {});
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it('reloads once after a chunk load error', () => {
    const boundary = new ErrorBoundary({
      children: null,
      fallback: null,
      reloadOnChunkError: true,
    });

    boundary.componentDidCatch(
      new Error('Failed to fetch dynamically imported module'),
      errorInfo,
    );

    expect(storage.get(STORAGE_KEY)).toBe('true');
    expect(reload).toHaveBeenCalledOnce();
  });

  it('does not reload again when the session guard is set', () => {
    storage.set(STORAGE_KEY, 'true');
    const boundary = new ErrorBoundary({
      children: null,
      fallback: null,
      reloadOnChunkError: true,
    });

    boundary.componentDidCatch(new Error('Loading chunk 42 failed'), errorInfo);

    expect(reload).not.toHaveBeenCalled();
  });

  it('renders the fallback for non-chunk errors without reloading', () => {
    const error = new Error('render failed');
    const fallback = vi.fn(() => 'fallback');
    const boundary = new ErrorBoundary({
      children: null,
      fallback,
      reloadOnChunkError: true,
    });
    boundary.state = ErrorBoundary.getDerivedStateFromError(error);

    expect(boundary.render()).toBe('fallback');
    expect(fallback).toHaveBeenCalledWith(error, expect.any(Function));
    expect(reload).not.toHaveBeenCalled();
  });

  it('clears the guard and reloads directly from the fallback', () => {
    storage.set(STORAGE_KEY, 'true');
    let reloadBoundary: (() => void) | undefined;
    const boundary = new ErrorBoundary({
      children: null,
      fallback: (_error, reload) => {
        reloadBoundary = reload;
        return null;
      },
    });
    boundary.state = ErrorBoundary.getDerivedStateFromError(new Error('render failed'));
    boundary.render();

    expect(reloadBoundary).toBeDefined();
    reloadBoundary?.();

    expect(storage.has(STORAGE_KEY)).toBe(false);
    expect(reload).toHaveBeenCalledOnce();
  });
});
