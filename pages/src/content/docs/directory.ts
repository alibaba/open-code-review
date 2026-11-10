// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import type { Language } from '../../i18n/types';

export const DOC_SLUGS = [
  'quickstart',
  'installation',
  'configuration',
  'chatgpt-preview',
  'cli-reference',
  'review-rules',
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
  'contributing',
  'faq',
] as const;

export type DocSlug = typeof DOC_SLUGS[number];

export interface DocDirectoryEntry {
  slug: DocSlug;
  languages: readonly Language[];
}

export type DocDirectory = readonly DocDirectoryEntry[];

const ALL_LANGUAGES: readonly Language[] = ['en', 'zh', 'ja', 'ko', 'ru'];

export const DOC_DIRECTORY: DocDirectory = DOC_SLUGS.map(slug => ({
  slug,
  languages: ALL_LANGUAGES,
}));
