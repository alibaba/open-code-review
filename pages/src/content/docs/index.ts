// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

/* Docs content directory and on-demand language loader.
 *
 * This module must stay lightweight: it holds slugs, navigation order and
 * language availability, but never imports a Markdown body. Each language's
 * bodies live behind their own bundle (en.ts, zh.ts, ...) and are pulled in
 * through loadLanguageContent's explicit import() boundaries, so opening a
 * page downloads only the selected language's content.
 *
 * The directory is the source of truth for what a translation may contain:
 * a transient download failure is never treated as a missing translation. */

import type { Language } from '../../i18n/types';
import type { DocSlug, DocSearchResult, LocalizedDocs } from './types';

export type { DocSlug, DocSearchResult, LocalizedDocs };

/* Canonical slug order — search iterates this order, so it must match the
 * historical enDocs key order to keep result ordering stable. */
const DOC_SLUG_ORDER: readonly DocSlug[] = [
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

/* Which docs exist per language. When a doc is added without a translation,
 * remove its slug from that language's list so the page falls back to
 * English instead of promising a translation the bundle cannot deliver. */
const availableTranslations: Record<Language, readonly DocSlug[]> = {
  en: DOC_SLUG_ORDER,
  zh: DOC_SLUG_ORDER,
  ja: DOC_SLUG_ORDER,
  ko: DOC_SLUG_ORDER,
  ru: DOC_SLUG_ORDER,
  tr: DOC_SLUG_ORDER,
};

export function hasTranslation(slug: DocSlug, language: Language): boolean {
  return availableTranslations[language].includes(slug);
}

export function isPartialLocale(language: Language): boolean {
  return availableTranslations[language].length !== DOC_SLUG_ORDER.length;
}

/* One import() boundary per language so webpack emits independent
 * chunks; a computed specifier would merge them into a single context. */
const languageLoaders: Record<Language, () => Promise<{ default: LocalizedDocs }>> = {
  en: () => import('./en'),
  zh: () => import('./zh'),
  ja: () => import('./ja'),
  ko: () => import('./ko'),
  ru: () => import('./ru'),
  tr: () => import('./tr'),
};

/* Successful loads stay cached for the session; concurrent callers share the
 * in-flight promise; failed loads are evicted so a retry starts a fresh
 * import instead of replaying the rejection. */
const languageContentCache = new Map<Language, Promise<LocalizedDocs>>();

export function loadLanguageContent(language: Language): Promise<LocalizedDocs> {
  const cached = languageContentCache.get(language);
  if (cached) return cached;
  const pending = languageLoaders[language]().then(mod => mod.default);
  languageContentCache.set(language, pending);
  pending.catch(() => languageContentCache.delete(language));
  return pending;
}

/* ─── Loaded-content accessors ───
 * Pages and search read the same LoadedDocContent, so rendering and full-text
 * search always agree on what has been loaded. English is attached only for
 * partial locales that need fallback entries. */

export interface LoadedDocContent {
  language: Language;
  docs: LocalizedDocs;
  english?: LocalizedDocs;
}

function rawFor(slug: DocSlug, loaded: LoadedDocContent): string {
  return loaded.docs[slug] ?? loaded.english?.[slug] ?? '';
}

/**
 * Strip YAML frontmatter from markdown content
 */
function stripFrontmatter(md: string): string {
  if (md.startsWith('---')) {
    const end = md.indexOf('---', 3);
    if (end !== -1) {
      return md.slice(end + 3).trim();
    }
  }
  return md;
}

/**
 * Get the markdown content for a doc slug from the loaded language content.
 */
export function getDocContent(slug: DocSlug, loaded: LoadedDocContent): string {
  return stripFrontmatter(rawFor(slug, loaded));
}

/**
 * Get the title from frontmatter
 */
export function getDocTitle(slug: DocSlug, loaded: LoadedDocContent): string {
  const raw = rawFor(slug, loaded);
  if (raw.startsWith('---')) {
    const end = raw.indexOf('---', 3);
    if (end !== -1) {
      const fm = raw.slice(3, end);
      const match = fm.match(/title:\s*(.+)/);
      if (match) return match[1].trim();
    }
  }
  return slug;
}

function turkishSearchPattern(query: string): RegExp {
  const literal = query.normalize().toLocaleLowerCase('tr').replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  return new RegExp(literal.replace(/[i\u0131]/gu, letter => `[${letter}I]`));
}

declare const __ENGLISH_DOC_IDENTIFIERS__: string[];
const englishDocIdentifiers = new Set(__ENGLISH_DOC_IDENTIFIERS__);

function foldTurkishDocument(content: string): string {
  const englishIdentifier = /^(?:AI|ANSI|API|CI|CLI|ID|IDE|IDEA|Intel|Linux|SARIF|TUI|UI|URI|UUID|Windows)$/i;
  return content.replace(/[\p{L}\p{M}\p{N}_]+/gu, word => {
    if (!/[Ii\u0130]/u.test(word)) return word.toLowerCase();
    const lower = word.toLowerCase();
    if (englishDocIdentifiers.has(lower) || englishIdentifier.test(word)) return lower.replace(/i/g, 'I');
    return word.toLocaleLowerCase('tr');
  });
}

/**
 * Search the loaded language docs for a query string. Returns matching slugs
 * with context. Iterates the canonical slug order so partial locales with
 * English attached still search the English fallbacks, preserving the
 * previous all-language search coverage.
 */
export function searchDocs(query: string, loaded: LoadedDocContent): DocSearchResult[] {
  if (!query.trim()) return [];
  const results: DocSearchResult[] = [];
  const lowerQuery = query.toLowerCase();
  const turkishPattern = loaded.language === 'tr' ? turkishSearchPattern(query) : undefined;
  for (const slug of DOC_SLUG_ORDER) {
    const raw = rawFor(slug, loaded);
    if (!raw) continue;
    const isTurkish = turkishPattern !== undefined && loaded.docs[slug] !== undefined;
    const body = stripFrontmatter(raw);
    const content = isTurkish ? body.normalize() : body;
    const idx = isTurkish ? foldTurkishDocument(content).search(turkishPattern) : content.toLowerCase().indexOf(lowerQuery);
    if (idx !== -1) {
      // Extract snippet around match
      const start = Math.max(0, idx - 30);
      const end = Math.min(content.length, idx + query.length + 60);
      let snippet = content.slice(start, end).replace(/[#*_`[\]()]/g, '').replace(/\n/g, ' ').trim();
      if (start > 0) snippet = '...' + snippet;
      if (end < content.length) snippet = snippet + '...';
      const title = getDocTitle(slug, loaded);
      results.push({ slug, title, snippet });
    }
  }
  return results;
}
