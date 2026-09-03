// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { LanguageProvider } from '../i18n';
import { extractHeadings } from '../utils/extractHeadings';
import MarkdownRenderer from './MarkdownRenderer';

describe('MarkdownRenderer heading IDs', () => {
  it('renders an explicit heading ID without displaying its marker', () => {
    render(
      <LanguageProvider>
        <MarkdownRenderer content={'## Что делает навык {#what-the-skill-does}\n\n#### Публикация {#service-account}'} />
      </LanguageProvider>,
    );

    const heading = screen.getByRole('heading', { name: 'Что делает навык' });
    expect(heading.getAttribute('id')).toBe('what-the-skill-does');
    expect(heading.textContent).not.toContain('{#what-the-skill-does}');
    expect(screen.getByRole('heading', { name: 'Публикация', level: 4 }).getAttribute('id')).toBe('service-account');
  });

  it('renders unique IDs for repeated headings', () => {
    render(
      <LanguageProvider>
        <MarkdownRenderer content={'## Schema\n\n## Schema\n\n### Output\n\n### Output'} />
      </LanguageProvider>,
    );

    expect(screen.getAllByRole('heading', { name: 'Schema' }).map((heading) => heading.id)).toEqual([
      'schema',
      'schema-2',
    ]);
    expect(screen.getAllByRole('heading', { name: 'Output' }).map((heading) => heading.id)).toEqual([
      'output',
      'output-2',
    ]);
  });

  it('keeps rendered heading IDs aligned with the TOC extraction', () => {
    const content = [
      'Setext title',
      '=============',
      '',
      '## Schema',
      '',
      '~~~',
      '## Fake heading inside a code block',
      '~~~',
      '',
      '## Schema',
      '### Output',
      '### Output',
    ].join('\n');
    const { container } = render(
      <LanguageProvider>
        <MarkdownRenderer content={content} />
      </LanguageProvider>,
    );

    const renderedTocHeadingIds = Array.from(container.querySelectorAll('h2, h3')).map((heading) => heading.id);
    expect(renderedTocHeadingIds).toEqual(extractHeadings(content).map(({ id }) => id));
  });
});
