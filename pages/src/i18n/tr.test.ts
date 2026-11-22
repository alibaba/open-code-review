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

  it.each(files)('translates %s with all source sections and stable anchors', path => {
    const source = readFileSync(join(docsRoot, 'en', path), 'utf8').replace(/^---[\s\S]*?---\s*/, '');
    const translated = readFileSync(join(docsRoot, 'tr', path), 'utf8').replace(/^---[\s\S]*?---\s*/, '');
    expect(translated).not.toBe(source);
    expect(extractHeadingInfo(translated).map(({ id, level }) => ({ id, level })))
      .toEqual(extractHeadingInfo(source).map(({ id, level }) => ({ id, level })));
    expect(translated).not.toMatch(/\{\{(?:CODE\d+|[A-Z]+)\}\}/);
  });

});
