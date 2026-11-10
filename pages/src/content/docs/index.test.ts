// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { describe, expect, it, vi } from 'vitest';
import type { Language } from '../../i18n/types';
import {
  createDocsRepository,
  type DocDirectory,
  type DocLanguageBundle,
  type DocLanguageLoader,
} from '.';

function loaderSet(overrides: Partial<Record<Language, DocLanguageLoader>>) {
  const empty = async (): Promise<DocLanguageBundle> => ({});
  return {
    en: vi.fn(overrides.en ?? empty),
    zh: vi.fn(overrides.zh ?? empty),
    ja: vi.fn(overrides.ja ?? empty),
    ko: vi.fn(overrides.ko ?? empty),
    ru: vi.fn(overrides.ru ?? empty),
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

describe('documentation content repository', () => {
  it('loads only the selected language and shares cached and in-flight requests', async () => {
    const pending = deferred<DocLanguageBundle>();
    const loaders = loaderSet({ zh: () => pending.promise });
    const directory: DocDirectory = [{ slug: 'quickstart', languages: ['en', 'zh'] }];
    const repository = createDocsRepository(loaders, directory);

    const first = repository.loadPage('quickstart', 'zh');
    const second = repository.loadPage('quickstart', 'zh');
    await vi.waitFor(() => expect(loaders.zh).toHaveBeenCalledTimes(1));
    expect(loaders.en).not.toHaveBeenCalled();

    pending.resolve({ quickstart: '---\ntitle: Local title\n---\nLocal body' });
    await expect(Promise.all([first, second])).resolves.toEqual([
      { slug: 'quickstart', title: 'Local title', content: 'Local body' },
      { slug: 'quickstart', title: 'Local title', content: 'Local body' },
    ]);
    await repository.loadPage('quickstart', 'zh');
    expect(loaders.zh).toHaveBeenCalledTimes(1);
  });

  it('does not cache a failed download and retries the same language', async () => {
    const loaders = loaderSet({
      zh: vi.fn()
        .mockRejectedValueOnce(new Error('offline'))
        .mockResolvedValueOnce({ quickstart: '---\ntitle: Retry title\n---\nRetry body' }),
    });
    const directory: DocDirectory = [{ slug: 'quickstart', languages: ['en', 'zh'] }];
    const repository = createDocsRepository(loaders, directory);

    await expect(repository.loadPage('quickstart', 'zh')).rejects.toThrow('offline');
    expect(loaders.en).not.toHaveBeenCalled();
    await expect(repository.loadPage('quickstart', 'zh')).resolves.toMatchObject({
      title: 'Retry title',
      content: 'Retry body',
    });
    expect(loaders.zh).toHaveBeenCalledTimes(2);
  });

  it('loads English only when directory metadata marks a translation as missing', async () => {
    const loaders = loaderSet({
      zh: async () => ({ quickstart: '---\ntitle: Local title\n---\nLocal body' }),
      en: async () => ({ faq: '---\ntitle: FAQ\n---\nEnglish fallback' }),
    });
    const directory: DocDirectory = [
      { slug: 'quickstart', languages: ['en', 'zh'] },
      { slug: 'faq', languages: ['en'] },
    ];
    const repository = createDocsRepository(loaders, directory);

    await expect(repository.loadPage('quickstart', 'zh')).resolves.toMatchObject({ title: 'Local title' });
    expect(loaders.en).not.toHaveBeenCalled();
    await expect(repository.loadPage('faq', 'zh')).resolves.toMatchObject({
      title: 'FAQ',
      content: 'English fallback',
    });
    expect(loaders.en).toHaveBeenCalledTimes(1);
  });

  it('preserves search order, snippets, titles, and English coverage for a partial locale', async () => {
    const loaders = loaderSet({
      zh: async () => ({
        quickstart: '---\ntitle: Local title\n---\nBefore **Needle** after',
      }),
      en: async () => ({
        faq: '---\ntitle: English FAQ\n---\nNeedle in fallback',
      }),
    });
    const directory: DocDirectory = [
      { slug: 'quickstart', languages: ['en', 'zh'] },
      { slug: 'faq', languages: ['en'] },
    ];
    const repository = createDocsRepository(loaders, directory);

    await expect(repository.search('needle', 'zh')).resolves.toEqual([
      { slug: 'quickstart', title: 'Local title', snippet: 'Before Needle after' },
      { slug: 'faq', title: 'English FAQ', snippet: 'Needle in fallback' },
    ]);
    expect(loaders.zh).toHaveBeenCalledTimes(1);
    expect(loaders.en).toHaveBeenCalledTimes(1);
    await repository.search('fallback', 'zh');
    expect(loaders.zh).toHaveBeenCalledTimes(1);
    expect(loaders.en).toHaveBeenCalledTimes(1);
  });

  it('does not download content for an empty search', async () => {
    const loaders = loaderSet({});
    const directory: DocDirectory = [{ slug: 'quickstart', languages: ['en', 'zh'] }];
    const repository = createDocsRepository(loaders, directory);

    await expect(repository.search('   ', 'zh')).resolves.toEqual([]);
    expect(loaders.zh).not.toHaveBeenCalled();
    expect(loaders.en).not.toHaveBeenCalled();
  });
});
