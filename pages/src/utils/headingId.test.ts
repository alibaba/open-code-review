// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { describe, expect, it } from 'vitest';
import { extractHeadings } from './extractHeadings';
import { parseExplicitHeadingId } from './headingId';

// Russian on purpose: explicit heading IDs exist for text that cannot produce a
// usable ASCII slug on its own.
const HEADING = 'Что делает навк'; // allow-non-english: fixture heading that cannot produce an ASCII slug

describe('explicit heading IDs', () => {
  it('separates a trailing explicit ID from the visible heading text', () => {
    expect(parseExplicitHeadingId(`${HEADING} {#what-the-skill-does}`)).toEqual({
      text: HEADING,
      id: 'what-the-skill-does',
    });
  });

  it('leaves headings without an explicit ID unchanged', () => {
    expect(parseExplicitHeadingId('Configuration')).toEqual({ text: 'Configuration' });
  });

  it('uses the explicit ID in the table of contents without exposing its marker', () => {
    expect(extractHeadings(`## ${HEADING} {#what-the-skill-does}`)).toEqual([
      { id: 'what-the-skill-does', text: HEADING, level: 2 },
    ]);
  });
});
