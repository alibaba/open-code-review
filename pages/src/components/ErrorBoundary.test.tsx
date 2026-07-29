import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import ErrorBoundary from './ErrorBoundary';

const Thrower: React.FC<{ msg: string }> = ({ msg }) => {
  throw new Error(msg);
};

function mockReload(): ReturnType<typeof vi.fn> {
  const reloadSpy = vi.fn();
  vi.spyOn(window, 'location', 'get').mockReturnValue({
    reload: reloadSpy,
    href: 'http://localhost/',
  } as unknown as Location);
  return reloadSpy;
}

describe('ErrorBoundary', () => {
  beforeEach(() => {
    sessionStorage.clear();
    vi.restoreAllMocks();
  });

  it('reloads the page on a chunk load error', () => {
    const reloadSpy = mockReload();
    render(
      <ErrorBoundary fallback="boom" reloadOnChunkError>
        <Thrower msg="Loading chunk 1 failed." />
      </ErrorBoundary>
    );
    expect(reloadSpy).toHaveBeenCalled();
  });

  it('does not reload twice when the guard is already set', () => {
    const reloadSpy = mockReload();
    sessionStorage.setItem('ocr-chunk-reload-attempted', 'true');
    render(
      <ErrorBoundary fallback="boom" reloadOnChunkError>
        <Thrower msg="Loading chunk 1 failed." />
      </ErrorBoundary>
    );
    expect(reloadSpy).not.toHaveBeenCalled();
  });

  it('renders fallback without reloading on a non-chunk error', () => {
    const reloadSpy = mockReload();
    render(
      <ErrorBoundary fallback="fallback-ui">
        <Thrower msg="ordinary bug" />
      </ErrorBoundary>
    );
    expect(screen.getByText('fallback-ui')).toBeTruthy();
    expect(reloadSpy).not.toHaveBeenCalled();
  });

  it('reset() clears the guard and reloads the page', () => {
    const reloadSpy = mockReload();
    const fallback = (_e: Error, reset: () => void) => (
      <button onClick={reset} type="button">reload</button>
    );
    render(
      <ErrorBoundary fallback={fallback}>
        <Thrower msg="ordinary bug" />
      </ErrorBoundary>
    );
    sessionStorage.setItem('ocr-chunk-reload-attempted', 'true');
    fireEvent.click(screen.getByText('reload'));
    expect(sessionStorage.getItem('ocr-chunk-reload-attempted')).toBeNull();
    expect(reloadSpy).toHaveBeenCalled();
  });
});
