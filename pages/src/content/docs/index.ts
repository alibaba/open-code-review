// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

/* Docs content index — imports all markdown files and provides a lookup by slug + language */

import type { Language } from '../../i18n/types';

// English docs
import enQuickstart from './en/quickstart.md';
import enInstallation from './en/installation.md';
import enConfiguration from './en/configuration.md';
import enCliReference from './en/cli-reference.md';
import enReviewRules from './en/review-rules.md';
import enArchitecture from './en/architecture.md';
import enTools from './en/tools.md';
import enMcp from './en/mcp.md';
import enViewer from './en/viewer.md';
import enTelemetry from './en/telemetry.md';
import enAgentSkill from './en/integrations/agent-skill.md';
import enClaudeCode from './en/integrations/claude-code.md';
import enCicd from './en/integrations/ci.md';
import enDelegate from './en/integrations/delegate.md';
import enContributing from './en/contributing.md';
import enFaq from './en/faq.md';

// Chinese docs
import zhQuickstart from './zh/quickstart.md';
import zhInstallation from './zh/installation.md';
import zhConfiguration from './zh/configuration.md';
import zhCliReference from './zh/cli-reference.md';
import zhReviewRules from './zh/review-rules.md';
import zhArchitecture from './zh/architecture.md';
import zhTools from './zh/tools.md';
import zhMcp from './zh/mcp.md';
import zhViewer from './zh/viewer.md';
import zhTelemetry from './zh/telemetry.md';
import zhAgentSkill from './zh/integrations/agent-skill.md';
import zhClaudeCode from './zh/integrations/claude-code.md';
import zhCicd from './zh/integrations/ci.md';
import zhDelegate from './zh/integrations/delegate.md';
import zhContributing from './zh/contributing.md';
import zhFaq from './zh/faq.md';

// Japanese docs
import jaQuickstart from './ja/quickstart.md';
import jaInstallation from './ja/installation.md';
import jaConfiguration from './ja/configuration.md';
import jaCliReference from './ja/cli-reference.md';
import jaReviewRules from './ja/review-rules.md';
import jaArchitecture from './ja/architecture.md';
import jaTools from './ja/tools.md';
import jaMcp from './ja/mcp.md';
import jaViewer from './ja/viewer.md';
import jaTelemetry from './ja/telemetry.md';
import jaAgentSkill from './ja/integrations/agent-skill.md';
import jaClaudeCode from './ja/integrations/claude-code.md';
import jaCicd from './ja/integrations/ci.md';
import jaDelegate from './ja/integrations/delegate.md';
import jaContributing from './ja/contributing.md';
import jaFaq from './ja/faq.md';

// Russian docs (incremental — partial LocalizedDocs)
import ruQuickstart from './ru/quickstart.md';
import ruInstallation from './ru/installation.md';
import ruConfiguration from './ru/configuration.md';
import ruCliReference from './ru/cli-reference.md';
import ruReviewRules from './ru/review-rules.md';
import ruArchitecture from './ru/architecture.md';
import ruTools from './ru/tools.md';
import ruMcp from './ru/mcp.md';
import ruViewer from './ru/viewer.md';
import ruTelemetry from './ru/telemetry.md';
import ruAgentSkill from './ru/integrations/agent-skill.md';
import ruClaudeCode from './ru/integrations/claude-code.md';
import ruCicd from './ru/integrations/ci.md';
import ruDelegate from './ru/integrations/delegate.md';
import ruContributing from './ru/contributing.md';
import ruFaq from './ru/faq.md';

export type DocSlug =
  | 'quickstart'
  | 'installation'
  | 'configuration'
  | 'cli-reference'
  | 'review-rules'
  | 'architecture'
  | 'tools'
  | 'mcp'
  | 'viewer'
  | 'telemetry'
  | 'agent-skill'
  | 'claude-code'
  | 'cicd'
  | 'delegate'
  | 'contributing'
  | 'faq';

type LocalizedDocs = Partial<Record<DocSlug, string>>;

const enDocs: Record<DocSlug, string> = {
  'quickstart': enQuickstart,
  'installation': enInstallation,
  'configuration': enConfiguration,
  'cli-reference': enCliReference,
  'review-rules': enReviewRules,
  'architecture': enArchitecture,
  'tools': enTools,
  'mcp': enMcp,
  'viewer': enViewer,
  'telemetry': enTelemetry,
  'agent-skill': enAgentSkill,
  'claude-code': enClaudeCode,
  'cicd': enCicd,
  'delegate': enDelegate,
  'contributing': enContributing,
  'faq': enFaq,
};

const zhDocs: Record<DocSlug, string> = {
  'quickstart': zhQuickstart,
  'installation': zhInstallation,
  'configuration': zhConfiguration,
  'cli-reference': zhCliReference,
  'review-rules': zhReviewRules,
  'architecture': zhArchitecture,
  'tools': zhTools,
  'mcp': zhMcp,
  'viewer': zhViewer,
  'telemetry': zhTelemetry,
  'agent-skill': zhAgentSkill,
  'claude-code': zhClaudeCode,
  'cicd': zhCicd,
  'delegate': zhDelegate,
  'contributing': zhContributing,
  'faq': zhFaq,
};

const jaDocs: Record<DocSlug, string> = {
  'quickstart': jaQuickstart,
  'installation': jaInstallation,
  'configuration': jaConfiguration,
  'cli-reference': jaCliReference,
  'review-rules': jaReviewRules,
  'architecture': jaArchitecture,
  'tools': jaTools,
  'mcp': jaMcp,
  'viewer': jaViewer,
  'telemetry': jaTelemetry,
  'agent-skill': jaAgentSkill,
  'claude-code': jaClaudeCode,
  'cicd': jaCicd,
  'delegate': jaDelegate,
  'contributing': jaContributing,
  'faq': jaFaq,
};

const ruDocs: LocalizedDocs = {
  'quickstart': ruQuickstart,
  'installation': ruInstallation,
  'configuration': ruConfiguration,
  'cli-reference': ruCliReference,
  'review-rules': ruReviewRules,
  'architecture': ruArchitecture,
  'tools': ruTools,
  'mcp': ruMcp,
  'viewer': ruViewer,
  'telemetry': ruTelemetry,
  'agent-skill': ruAgentSkill,
  'claude-code': ruClaudeCode,
  'cicd': ruCicd,
  'delegate': ruDelegate,
  'contributing': ruContributing,
  'faq': ruFaq,
};

const docsMap: Record<Language, LocalizedDocs> = {
  en: enDocs,
  zh: zhDocs,
  ja: jaDocs,
  // Korean UI strings ship first; doc pages fall back to English until ko/ exists.
  ko: {},
  ru: ruDocs,
};

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
 * Get raw content for a slug in the given language, with English fallback.
 */
function getRawContent(slug: DocSlug, language: string): string {
  const langDocs = docsMap[language as Language] || docsMap.en;
  return langDocs[slug] ?? enDocs[slug] ?? '';
}

/**
 * Get the markdown content for a given doc slug and language.
 * Falls back to English if the language is not found.
 */
export function getDocContent(slug: DocSlug, language: string): string {
  return stripFrontmatter(getRawContent(slug, language));
}

/**
 * Get the title from frontmatter
 */
export function getDocTitle(slug: DocSlug, language: string): string {
  const raw = getRawContent(slug, language);
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

/**
 * Search across all docs for a query string. Returns matching slugs with context.
 * Iterates every English slug so partial locales still search English fallbacks.
 */
export function searchDocs(query: string, language: string): { slug: DocSlug; title: string; snippet: string }[] {
  if (!query.trim()) return [];
  const langDocs = docsMap[language as Language] || docsMap.en;
  const results: { slug: DocSlug; title: string; snippet: string }[] = [];
  const lowerQuery = query.toLowerCase();
  const slugs = Object.keys(enDocs) as DocSlug[];
  for (const slug of slugs) {
    const raw = langDocs[slug] ?? enDocs[slug] ?? '';
    const content = stripFrontmatter(raw);
    const lowerContent = content.toLowerCase();
    const idx = lowerContent.indexOf(lowerQuery);
    if (idx !== -1) {
      // Extract snippet around match
      const start = Math.max(0, idx - 30);
      const end = Math.min(content.length, idx + query.length + 60);
      let snippet = content.slice(start, end).replace(/[#*_`[\]()]/g, '').replace(/\n/g, ' ').trim();
      if (start > 0) snippet = '...' + snippet;
      if (end < content.length) snippet = snippet + '...';
      const title = getDocTitle(slug, language);
      results.push({ slug, title, snippet });
    }
  }
  return results;
}
