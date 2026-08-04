import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { LanguageProvider } from '../i18n';
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
});
