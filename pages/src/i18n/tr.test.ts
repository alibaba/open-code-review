// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { readFileSync, readdirSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';
import { getDocContent, loadLanguageContent, searchDocs } from '../content/docs';
import { en } from './en';
import { tr } from './tr';
import { extractHeadingInfo } from '../utils/headingId';

const docsRoot = join(dirname(fileURLToPath(import.meta.url)), '..', 'content', 'docs');
const files = readdirSync(join(docsRoot, 'en'), { recursive: true })
  .map(String).filter(path => path.endsWith('.md'));
const placeholders = (text: string) => text.match(/\{[^{}]+\}/g)?.sort() ?? [];

describe('Turkish localization', () => {
  it('covers the site catalog without losing interpolation parameters', () => {
    expect(Object.keys(tr).sort()).toEqual(Object.keys(en).sort());
    for (const key of Object.keys(en) as (keyof typeof en)[]) {
      expect(tr[key].trim(), key).not.toBe('');
      expect(placeholders(tr[key]), key).toEqual(placeholders(en[key]));
    }
  });

  it('loads and searches the Turkish requirements guide without English fallback', async () => {
    const docs = await loadLanguageContent('tr');
    const loaded = { language: 'tr' as const, docs };
    const query = 'Sepet boşsa';
    expect(getDocContent('reviewing-requirements', loaded)).toContain(query);
    expect(searchDocs(query, loaded)).toContainEqual({
      slug: 'reviewing-requirements',
      title: tr['docs.sidebar.reviewingRequirements'],
      snippet: expect.stringContaining(query),
    });
  });

  it('finds Turkish words regardless of case in the published guides', async () => {
    const docs = await loadLanguageContent('tr');
    const loaded = { language: 'tr' as const, docs };
    for (const query of ['incelemeyi çalıştırma', 'İNCELEMEYİ ÇALIŞTIRMA']) {
      expect(searchDocs(query, loaded)).toEqual(expect.arrayContaining(
        ['agent-skill', 'claude-code'].map(slug => expect.objectContaining({
          slug, snippet: expect.stringContaining('İncelemeyi çalıştırma'),
        })),
      ));
    }
  });

  it('still finds English CLI terms in Turkish documentation', () => {
    const loaded = { language: 'tr' as const, docs: { quickstart: 'içindeki CLI' } };
    expect(searchDocs('cli', loaded).map(result => result.slug)).toEqual(['quickstart']);
    expect(searchDocs('İÇİNDEKİ cli', loaded).map(result => result.slug)).toEqual(['quickstart']);
    expect(searchDocs('[cli]', loaded)).toEqual([]);
  });

  it('distinguishes dotted and dotless I in Turkish words', () => {
    const loaded = { language: 'tr' as const, docs: { quickstart: 'KIR' } };
    expect(searchDocs('kır', loaded)).toHaveLength(1);
    expect(searchDocs('kir', loaded)).toEqual([]);
  });

  it.each([
    ['openid connect', 'chatgpt-preview', 'OpenID Connect'],
    ['GITHUB RELEASE', 'installation', 'GitHub Release'],
    ['intellij idea', 'contributing', 'IntelliJ IDEA'],
  ])('finds "%s" in the Turkish %s guide', async (query, slug, name) => {
    const docs = await loadLanguageContent('tr');
    expect(searchDocs(query, { language: 'tr', docs })).toContainEqual(
      expect.objectContaining({ slug, snippet: expect.stringContaining(name) }),
    );
  });

  it('matches English identifiers and acronym prefixes as users type them', () => {
    const loaded = { language: 'tr' as const, docs: { quickstart: '`OCR_LLM_TIMEOUT` / SARIF' } };
    for (const query of ['ocr_llm_timeout', 'SARI', 'sari']) {
      expect(searchDocs(query, loaded)).toHaveLength(1);
    }
  });

  it('keeps the match in the snippet after decomposed dotted letters', () => {
    const loaded = { language: 'tr' as const, docs: { quickstart: 'I\u0307'.repeat(70) + ' hedef' } };
    expect(searchDocs('hedef', loaded)[0]?.snippet).toContain('hedef');
  });

  it.each(files)('translates %s with all source sections and stable anchors', path => {
    const source = readFileSync(join(docsRoot, 'en', path), 'utf8').replace(/^---[\s\S]*?---\s*/, '');
    const translated = readFileSync(join(docsRoot, 'tr', path), 'utf8').replace(/^---[\s\S]*?---\s*/, '');
    expect(translated).not.toBe(source);
    expect(extractHeadingInfo(translated).map(({ id, level }) => ({ id, level })))
      .toEqual(extractHeadingInfo(source).map(({ id, level }) => ({ id, level })));
    expect(translated).not.toMatch(/\{\{(?:CODE\d+|[A-Z]+)\}\}/);
  });

});
