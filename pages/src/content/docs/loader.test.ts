// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { describe, it, expect, vi, beforeEach } from 'vitest';

/* Language bundles are mocked so the loader's caching semantics can be
 * observed directly: how many times each bundle was imported. */
const state = vi.hoisted(() => ({
  loads: {} as Record<string, number>,
  failEn: false,
}));

vi.mock('./en', () => {
  state.loads.en = (state.loads.en ?? 0) + 1;
  if (state.failEn) throw new Error('en chunk unavailable');
  return { default: { quickstart: 'en-quickstart' } };
});
vi.mock('./zh', () => {
  state.loads.zh = (state.loads.zh ?? 0) + 1;
  return { default: { quickstart: 'zh-quickstart' } };
});
vi.mock('./ja', () => {
  state.loads.ja = (state.loads.ja ?? 0) + 1;
  return { default: { quickstart: 'ja-quickstart' } };
});
vi.mock('./ko', () => {
  state.loads.ko = (state.loads.ko ?? 0) + 1;
  return { default: { quickstart: 'ko-quickstart' } };
});
vi.mock('./ru', () => {
  state.loads.ru = (state.loads.ru ?? 0) + 1;
  return { default: { quickstart: 'ru-quickstart' } };
});

import { loadLanguageContent } from './index';

beforeEach(() => {
  state.failEn = false;
});

describe('loadLanguageContent', () => {
  it('shares one in-flight load across concurrent callers', async () => {
    const first = loadLanguageContent('zh');
    const second = loadLanguageContent('zh');
    expect(second).toBe(first);
    await first;
    expect(state.loads.zh).toBe(1);
  });

  it('keeps a successful load cached for the session', async () => {
    const first = await loadLanguageContent('ja');
    expect(first).toEqual({ quickstart: 'ja-quickstart' });

    const again = loadLanguageContent('ja');
    await expect(again).resolves.toEqual({ quickstart: 'ja-quickstart' });
    expect(again).toBeInstanceOf(Promise);
    expect(state.loads.ja).toBe(1);
  });

  it('returns the same cached promise after a successful load', async () => {
    const first = loadLanguageContent('ko');
    await first;
    const second = loadLanguageContent('ko');
    expect(second).toBe(first);
    expect(state.loads.ko).toBe(1);
  });

  it('does not cache a failed load, so a retry starts a fresh import', async () => {
    state.failEn = true;
    vi.resetModules();
    const first = loadLanguageContent('en');
    // A throwing mock factory surfaces as a module-load failure; the exact
    // message is vitest's, so only the rejection itself is asserted here.
    await expect(first).rejects.toThrow();
    expect(state.loads.en).toBe(1);

    state.failEn = false;
    vi.resetModules();
    const retry = loadLanguageContent('en');
    await expect(retry).resolves.toEqual({ quickstart: 'en-quickstart' });
    expect(state.loads.en).toBe(2);
  });
});
