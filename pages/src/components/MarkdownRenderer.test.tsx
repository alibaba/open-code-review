// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { LanguageProvider } from '../i18n';
import MarkdownRenderer from './MarkdownRenderer';

// The headings are Russian on purpose: explicit heading IDs exist for text that
// cannot produce a usable ASCII slug on its own. Held in constants rather than
// inline, because a marker comment on the JSX line would render as heading text.
const H2 = 'Что делает навык'; // allow-non-english: fixture heading that cannot produce an ASCII slug
const H4 = 'Публикация'; // allow-non-english: fixture heading that cannot produce an ASCII slug
const CONTENT = `## ${H2} {#what-the-skill-does}\n\n#### ${H4} {#service-account}`;

describe('MarkdownRenderer heading IDs', () => {
  it('renders an explicit heading ID without displaying its marker', () => {
    render(
      <LanguageProvider>
        <MarkdownRenderer content={CONTENT} />
      </LanguageProvider>,
    );

    const heading = screen.getByRole('heading', { name: H2 });
    expect(heading.getAttribute('id')).toBe('what-the-skill-does');
    expect(heading.textContent).not.toContain('{#what-the-skill-does}');
    expect(screen.getByRole('heading', { name: H4, level: 4 }).getAttribute('id')).toBe('service-account');
  });
});
