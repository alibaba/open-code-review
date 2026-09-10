// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import DOMPurify from 'dompurify';
import { Marked, type Token, type Tokens } from 'marked';

const explicitHeadingIdPattern = /\s+\{#([a-zA-Z0-9][a-zA-Z0-9_.:-]*)\}\s*$/;

/** Split an optional trailing `{#id}` marker from the visible heading text. */
export function parseExplicitHeadingId(text: string): { text: string; id?: string } {
  const match = text.match(explicitHeadingIdPattern);
  if (!match || match.index === undefined) {
    return { text };
  }

  return {
    text: text.slice(0, match.index).trimEnd(),
    id: match[1],
  };
}

/**
 * Shared utility to generate heading IDs from text.
 * Used by both extractHeadings (DocsPage TOC) and MarkdownRenderer (heading renderer)
 * to ensure consistent anchor IDs.
 */
export function generateHeadingId(text: string): string {
  // Strip HTML tags via DOMPurify (a single-pass regex is unreliable and can be
  // bypassed by nested tags). Keeps text content, decodes HTML entities so the
  // TOC side (raw markdown) and renderer side (marked HTML output) agree.
  const plain = DOMPurify.sanitize(text, { ALLOWED_TAGS: [], ALLOWED_ATTR: [] })
    .replace(/[`*_[\]()]/g, '')
    .trim();
  return plain.toLowerCase().replace(/[^a-z0-9\u4e00-\u9fff]+/g, '-').replace(/^-|-$/g, '');
}

/**
 * Creates a per-document heading ID resolver that keeps every rendered heading
 * addressable, even when a document repeats the same title (for example,
 * several "Schema" or "Output" sections).
 */
export function createHeadingIdResolver(): (baseId: string) => string {
  const usedIds = new Set<string>();

  return (baseId: string) => {
    if (!usedIds.has(baseId)) {
      usedIds.add(baseId);
      return baseId;
    }

    let suffix = 2;
    let candidate = `${baseId}-${suffix}`;
    while (usedIds.has(candidate)) {
      suffix += 1;
      candidate = `${baseId}-${suffix}`;
    }
    usedIds.add(candidate);
    return candidate;
  };
}

export interface HeadingInfo {
  id: string;
  text: string;
  level: number;
}

function normalizeHeadingText(text: string): string {
  return text
    .replace(/\[([^\]]+)]\([^)]*\)/g, '$1')
    .replace(/[`*_[\]()]/g, '')
    .trim();
}

/**
 * Walk the same Markdown token stream used by the renderer so all heading IDs
 * are resolved in exactly the same order, including setext headings and nested
 * tokens inside blockquotes/lists.
 */
export function extractHeadingInfo(markdown: string): HeadingInfo[] {
  const parser = new Marked({ gfm: true, breaks: false });
  const headings: HeadingInfo[] = [];
  const resolveHeadingId = createHeadingIdResolver();

  const visit = (tokens: Token[]) => {
    for (const token of tokens) {
      if (token.type === 'heading') {
        const heading = token as Tokens.Heading;
        const { text: headingText, id: explicitId } = parseExplicitHeadingId(heading.text);
        const text = normalizeHeadingText(headingText);
        headings.push({
          id: resolveHeadingId(explicitId ?? generateHeadingId(text)),
          text,
          level: heading.depth,
        });
      }
      if ('tokens' in token && Array.isArray(token.tokens)) {
        visit(token.tokens);
      }
    }
  };

  visit(parser.lexer(markdown));
  return headings;
}
