// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { describe, expect, it } from 'vitest';
import { extractHeadings } from './extractHeadings';
import { parseExplicitHeadingId } from './headingId';

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
});
