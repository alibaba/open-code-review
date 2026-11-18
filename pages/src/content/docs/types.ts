// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

/* Shared docs-content types — importing this file must never pull in a Markdown body. */

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
  | 'acp'
  | 'copilot-cli'
  | 'contributing'
  | 'faq'
  | 'chatgpt-preview';

/* A bundle that translates every slug. Assigning to this type fails at
 * compile time when a key is missing or misspelled. */
export type CompleteLocalizedDocs = Record<DocSlug, string>;

/* An incremental bundle: missing slugs fall back to English at runtime. */
export type LocalizedDocs = Partial<Record<DocSlug, string>>;

export interface DocSearchResult {
  slug: DocSlug;
  title: string;
  snippet: string;
}
