// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { describe, it, expect } from 'vitest';
import {
  getDocContent,
  getDocTitle,
  hasTranslation,
  isPartialLocale,
  loadLanguageContent,
  searchDocs,
} from './index';
import type { LoadedDocContent } from './index';
import type { DocSlug, LocalizedDocs } from './types';

const LANGUAGES = ['en', 'zh', 'ja', 'ko', 'ru', 'tr'] as const;

/* The order the previous synchronous search iterated (the historical enDocs
 * key order). Result ordering must stay identical after the lazy-loading
 * split, so it is pinned here. */
const EXPECTED_SLUG_ORDER: readonly DocSlug[] = [
  'quickstart',
  'installation',
  'configuration',
  'chatgpt-preview',
  'cli-reference',
  'review-rules',
  'reviewing-requirements',
  'architecture',
  'tools',
  'mcp',
  'viewer',
  'telemetry',
  'agent-skill',
  'claude-code',
  'cicd',
  'delegate',
  'acp',
  'copilot-cli',
  'contributing',
  'faq',
];

describe('docs directory', () => {
  it('availability matches the real language bundles', async () => {
    for (const lang of LANGUAGES) {
      const docs = await loadLanguageContent(lang);
      const bundledSlugs = Object.keys(docs) as DocSlug[];
      for (const slug of bundledSlugs) {
        expect(EXPECTED_SLUG_ORDER).toContain(slug);
      }
      for (const slug of EXPECTED_SLUG_ORDER) {
        expect(hasTranslation(slug, lang)).toBe(slug in docs);
      }
    }
  });

  it('reports the current locales as complete', () => {
    for (const lang of LANGUAGES) {
      expect(isPartialLocale(lang)).toBe(false);
    }
  });

  it('search results follow the canonical slug order', () => {
    const docs = Object.fromEntries(
      EXPECTED_SLUG_ORDER.map(slug => [slug, `body of ${slug} mentions shared-token`]),
    ) as LocalizedDocs;
    const results = searchDocs('shared-token', { language: 'en', docs });
    expect(results.map(r => r.slug)).toEqual([...EXPECTED_SLUG_ORDER]);
  });

  it('search skips empty entries and matches English fallbacks for partial locales', () => {
    const docs: LocalizedDocs = { quickstart: 'alpha-token only in the locale' };
    const english: LocalizedDocs = { faq: 'beta-token only in english' };
    const loaded: LoadedDocContent = { language: 'ru', docs, english };

    expect(searchDocs('alpha-token', loaded).map(r => r.slug)).toEqual(['quickstart']);
    expect(searchDocs('beta-token', loaded).map(r => r.slug)).toEqual(['faq']);
    expect(searchDocs('not-in-any-bundle', loaded)).toEqual([]);
    expect(searchDocs('   ', loaded)).toEqual([]);
  });

  it('content and title come from the loaded language with frontmatter stripped', async () => {
    const docs = await loadLanguageContent('en');
    const loaded: LoadedDocContent = { language: 'en', docs };

    const content = getDocContent('quickstart', loaded);
    expect(content.startsWith('---')).toBe(false);
    expect(content).toContain('code review');
    expect(getDocTitle('quickstart', loaded)).toBe('QuickStart');
  });

  it('title falls back to the slug when the body has no frontmatter', () => {
    const loaded: LoadedDocContent = { language: 'en', docs: { faq: 'plain body without frontmatter' } };
    expect(getDocTitle('faq', loaded)).toBe('faq');
    expect(getDocContent('faq', loaded)).toBe('plain body without frontmatter');
  });

  it('search over real English content returns contextual snippets', async () => {
    const docs = await loadLanguageContent('en');
    const results = searchDocs('Prerequisites', { language: 'en', docs });

    expect(results.length).toBeGreaterThan(0);
    for (const result of results) {
      expect(result.snippet.length).toBeGreaterThan(0);
      expect(EXPECTED_SLUG_ORDER).toContain(result.slug);
    }
    expect(results.map(r => r.slug)).toEqual(
      [...EXPECTED_SLUG_ORDER].filter(slug => results.some(r => r.slug === slug)),
    );
  });
});
