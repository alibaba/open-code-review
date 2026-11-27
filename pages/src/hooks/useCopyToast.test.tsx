// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, renderHook } from '@testing-library/react';
import { useCopyToast } from './useCopyToast';

describe('useCopyToast', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    // jsdom has no secure context, so the hook takes the execCommand path.
    document.execCommand = vi.fn(() => true);
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('hides the toast after the duration', () => {
    const { result } = renderHook(() => useCopyToast(1200));

    act(() => result.current.handleCopy('first'));
    expect(result.current.toastVisible).toBe(true);

    act(() => vi.advanceTimersByTime(1199));
    expect(result.current.toastVisible).toBe(true);

    act(() => vi.advanceTimersByTime(1));
    expect(result.current.toastVisible).toBe(false);
  });

  it('restarts the hide timer when copying again while the toast is visible', () => {
    const { result } = renderHook(() => useCopyToast(1200));

    act(() => result.current.handleCopy('first'));
    act(() => vi.advanceTimersByTime(940));
    act(() => result.current.handleCopy('second'));

    // The first copy's deadline has passed, but the second copy
    // should keep the toast up for its own full duration.
    act(() => vi.advanceTimersByTime(1000));
    expect(result.current.toastVisible).toBe(true);

    act(() => vi.advanceTimersByTime(200));
    expect(result.current.toastVisible).toBe(false);
  });

  it('restarts the hide timer on the Clipboard API path too', async () => {
    const writeText = vi.fn(() => Promise.resolve());
    vi.stubGlobal('isSecureContext', true);
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true });
    try {
      const { result } = renderHook(() => useCopyToast(1200));

      await act(async () => result.current.handleCopy('first'));
      expect(result.current.toastVisible).toBe(true);
      act(() => vi.advanceTimersByTime(940));
      await act(async () => result.current.handleCopy('second'));

      act(() => vi.advanceTimersByTime(1000));
      expect(result.current.toastVisible).toBe(true);
      act(() => vi.advanceTimersByTime(200));
      expect(result.current.toastVisible).toBe(false);
      expect(writeText).toHaveBeenCalledTimes(2);
      expect(document.execCommand).not.toHaveBeenCalled();
    } finally {
      vi.unstubAllGlobals();
      Reflect.deleteProperty(navigator, 'clipboard');
    }
  });

  it('does not show the toast when the copy fails', () => {
    document.execCommand = vi.fn(() => false);
    const { result } = renderHook(() => useCopyToast(1200));

    act(() => result.current.handleCopy('text'));
    expect(result.current.toastVisible).toBe(false);
  });
});
