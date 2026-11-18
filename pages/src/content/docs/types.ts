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

export type LocalizedDocs = Partial<Record<DocSlug, string>>;

export interface DocSearchResult {
  slug: DocSlug;
  title: string;
  snippet: string;
}
