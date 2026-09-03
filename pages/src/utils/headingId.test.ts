// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { describe, expect, it } from 'vitest';
import { extractHeadings } from './extractHeadings';
import { createHeadingIdResolver, extractHeadingInfo, parseExplicitHeadingId } from './headingId';

describe('explicit heading IDs', () => {
  it('separates a trailing explicit ID from the visible heading text', () => {
    expect(parseExplicitHeadingId('Что делает навк {#what-the-skill-does}')).toEqual({
      text: 'Что делает навк',
      id: 'what-the-skill-does',
    });
  });

  it('leaves headings without an explicit ID unchanged', () => {
    expect(parseExplicitHeadingId('Configuration')).toEqual({ text: 'Configuration' });
  });

  it('uses the explicit ID in the table of contents without exposing its marker', () => {
    expect(extractHeadings('## Что делает навк {#what-the-skill-does}')).toEqual([
      { id: 'what-the-skill-does', text: 'Что делает навк', level: 2 },
    ]);
  });

  it('assigns unique IDs to repeated headings', () => {
    expect(extractHeadings('## Schema\n\n## Schema\n\n### Output\n\n### Output')).toEqual([
      { id: 'schema', text: 'Schema', level: 2 },
      { id: 'schema-2', text: 'Schema', level: 2 },
      { id: 'output', text: 'Output', level: 3 },
      { id: 'output-2', text: 'Output', level: 3 },
    ]);
  });

  it('avoids collisions between generated suffixes and later headings', () => {
    const resolveHeadingId = createHeadingIdResolver();
    expect(resolveHeadingId('schema')).toBe('schema');
    expect(resolveHeadingId('schema-2')).toBe('schema-2');
    expect(resolveHeadingId('schema')).toBe('schema-3');
  });

  it('keeps TOC IDs aligned when hidden heading levels repeat a title', () => {
    expect(extractHeadings('#### Schema\n\n## Schema')).toEqual([
      { id: 'schema-2', text: 'Schema', level: 2 },
    ]);
  });

  it('parses setext headings and tilde-fenced code like the Markdown renderer', () => {
    expect(extractHeadingInfo('Title\n=====\n\n## Schema\n\n~~~\n## Fake\n~~~\n\n## Schema')).toEqual([
      { id: 'title', text: 'Title', level: 1 },
      { id: 'schema', text: 'Schema', level: 2 },
      { id: 'schema-2', text: 'Schema', level: 2 },
    ]);
  });
});
