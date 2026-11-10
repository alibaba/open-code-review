// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import type { Language } from '../../i18n/types';
import {
  DOC_DIRECTORY,
  type DocDirectory,
  type DocDirectoryEntry,
  type DocSlug,
} from './directory';

export { DOC_DIRECTORY, DOC_SLUGS } from './directory';
export type { DocDirectory, DocDirectoryEntry, DocSlug } from './directory';

export type DocLanguageBundle = Readonly<Partial<Record<DocSlug, string>>>;
export type DocLanguageLoader = () => Promise<DocLanguageBundle>;

export interface DocPageData {
  slug: DocSlug;
  title: string;
  content: string;
}

export interface DocSearchResult {
  slug: DocSlug;
  title: string;
  snippet: string;
}

export interface DocsRepository {
  loadLanguage: (language: Language) => Promise<DocLanguageBundle>;
  loadPage: (slug: DocSlug, language: string) => Promise<DocPageData>;
  search: (query: string, language: string) => Promise<DocSearchResult[]>;
}

const SUPPORTED_LANGUAGES: readonly Language[] = ['en', 'zh', 'ja', 'ko', 'ru'];

function normalizeLanguage(language: string): Language {
  return SUPPORTED_LANGUAGES.includes(language as Language) ? language as Language : 'en';
}

function stripFrontmatter(markdown: string): string {
  if (markdown.startsWith('---')) {
    const end = markdown.indexOf('---', 3);
    if (end !== -1) return markdown.slice(end + 3).trim();
  }
  return markdown;
}

function titleFromFrontmatter(markdown: string, fallback: string): string {
  if (markdown.startsWith('---')) {
    const end = markdown.indexOf('---', 3);
    if (end !== -1) {
      const match = markdown.slice(3, end).match(/title:\s*(.+)/);
      if (match) return match[1].trim();
    }
  }
  return fallback;
}

function contentFor(bundle: DocLanguageBundle, slug: DocSlug, language: Language): string {
  const content = bundle[slug];
  if (content === undefined) {
    throw new Error(`Documentation directory lists ${slug} for ${language}, but its content is missing.`);
  }
  return content;
}

export function createDocsRepository(
  loaders: Readonly<Record<Language, DocLanguageLoader>>,
  directory: DocDirectory = DOC_DIRECTORY,
): DocsRepository {
  const cached = new Map<Language, DocLanguageBundle>();
  const inFlight = new Map<Language, Promise<DocLanguageBundle>>();
  const entries = new Map<DocSlug, DocDirectoryEntry>(directory.map(entry => [entry.slug, entry]));

  const loadLanguage = (language: Language): Promise<DocLanguageBundle> => {
    const cachedBundle = cached.get(language);
    if (cachedBundle) return Promise.resolve(cachedBundle);

    const pending = inFlight.get(language);
    if (pending) return pending;

    const request = Promise.resolve()
      .then(() => loaders[language]())
      .then(
        bundle => {
          cached.set(language, bundle);
          inFlight.delete(language);
          return bundle;
        },
        error => {
          inFlight.delete(language);
          throw error;
        },
      );
    inFlight.set(language, request);
    return request;
  };

  const hasTranslation = (entry: DocDirectoryEntry, language: Language): boolean => (
    entry.languages.includes(language)
  );

  const loadPage = async (slug: DocSlug, requestedLanguage: string): Promise<DocPageData> => {
    const entry = entries.get(slug);
    if (!entry) throw new Error(`Unknown documentation slug: ${slug}`);

    const language = normalizeLanguage(requestedLanguage);
    const localized = await loadLanguage(language);
    let raw: string;
    if (hasTranslation(entry, language)) {
      raw = contentFor(localized, slug, language);
    } else {
      const english = language === 'en' ? localized : await loadLanguage('en');
      raw = contentFor(english, slug, 'en');
    }

    return {
      slug,
      title: titleFromFrontmatter(raw, slug),
      content: stripFrontmatter(raw),
    };
  };

  const search = async (query: string, requestedLanguage: string): Promise<DocSearchResult[]> => {
    if (!query.trim()) return [];

    const language = normalizeLanguage(requestedLanguage);
    const localized = await loadLanguage(language);
    const needsEnglish = language !== 'en' && directory.some(entry => !hasTranslation(entry, language));
    const english = needsEnglish ? await loadLanguage('en') : language === 'en' ? localized : null;
    const lowerQuery = query.toLowerCase();
    const results: DocSearchResult[] = [];

    for (const entry of directory) {
      const raw = hasTranslation(entry, language)
        ? contentFor(localized, entry.slug, language)
        : contentFor(english!, entry.slug, 'en');
      const content = stripFrontmatter(raw);
      const index = content.toLowerCase().indexOf(lowerQuery);
      if (index === -1) continue;

      const start = Math.max(0, index - 30);
      const end = Math.min(content.length, index + query.length + 60);
      let snippet = content.slice(start, end).replace(/[#*_`[\]()`]/g, '').replace(/\n/g, ' ').trim();
      if (start > 0) snippet = `...${snippet}`;
      if (end < content.length) snippet = `${snippet}...`;
      results.push({
        slug: entry.slug,
        title: titleFromFrontmatter(raw, entry.slug),
        snippet,
      });
    }

    return results;
  };

  return { loadLanguage, loadPage, search };
}

const repository = createDocsRepository({
  en: () => import(/* webpackChunkName: "docs-content-en" */ './bundles/en').then(module => module.default),
  zh: () => import(/* webpackChunkName: "docs-content-zh" */ './bundles/zh').then(module => module.default),
  ja: () => import(/* webpackChunkName: "docs-content-ja" */ './bundles/ja').then(module => module.default),
  ko: () => import(/* webpackChunkName: "docs-content-ko" */ './bundles/ko').then(module => module.default),
  ru: () => import(/* webpackChunkName: "docs-content-ru" */ './bundles/ru').then(module => module.default),
});

export const loadDocLanguage = repository.loadLanguage;
export const loadDocPage = repository.loadPage;
export const searchDocs = repository.search;
