// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { describe, expect, it } from 'vitest';
import { getDocContent, getDocTitle, hasTranslation, isPartialLocale, loadLanguageContent, searchDocs } from '../content/docs';
import type { DocSlug } from '../content/docs';
import { en } from './en';
import { id } from './id';

const placeholders = (text: string) => text.match(/\{[^{}]+\}/g)?.sort() ?? [];

describe('Indonesian localization', () => {
  it('covers the site catalog and preserves placeholders and line breaks', () => {
    expect(Object.keys(id).sort()).toEqual(Object.keys(en).sort());
    for (const key of Object.keys(en) as (keyof typeof en)[]) {
      expect(id[key].trim(), key).not.toBe('');
      expect(placeholders(id[key]), key).toEqual(placeholders(en[key]));
      expect(id[key].split('\n'), key).toHaveLength(en[key].split('\n').length);
    }
  });

  it('uses English for all document bodies, titles, and search results', async () => {
    const [docs, english] = await Promise.all([loadLanguageContent('id'), loadLanguageContent('en')]);
    const fallback = { language: 'id' as const, docs, english };
    const original = { language: 'en' as const, docs: english };

    expect(isPartialLocale('id')).toBe(true);
    expect(docs).toEqual({});
    for (const slug of Object.keys(english) as DocSlug[]) {
      expect(hasTranslation(slug, 'id'), slug).toBe(false);
      expect(getDocContent(slug, fallback), slug).toBe(getDocContent(slug, original));
      expect(getDocTitle(slug, fallback), slug).toBe(getDocTitle(slug, original));
    }
    const results = searchDocs('ocr review', original);
    expect(results.length).toBeGreaterThan(0);
    expect(searchDocs('ocr review', fallback)).toEqual(results);
  });
});
