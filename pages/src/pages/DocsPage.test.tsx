// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import React from 'react';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, act, waitFor, cleanup } from '@testing-library/react';
import { MemoryRouter, Routes, Route } from 'react-router-dom';
import { LanguageProvider, useTranslation } from '../i18n';
import type { Language } from '../i18n/types';
import DocsPage from './DocsPage';

/* Deferred loads let each test resolve or reject a language bundle at a
 * chosen moment. The mock mirrors the real loader's contract: successful
 * loads stay cached for the session, failures do not. */
const h = vi.hoisted(() => ({
  pending: {} as Record<string, { resolve: (docs: Record<string, string>) => void; reject: (err: unknown) => void }[]>,
  loadCalls: [] as string[],
  cache: {} as Record<string, Record<string, string>>,
}));

vi.mock('../content/docs', () => ({
  loadLanguageContent: (lang: string) => {
    if (h.cache[lang]) return Promise.resolve(h.cache[lang]);
    h.loadCalls.push(lang);
    return new Promise((resolve, reject) => {
      (h.pending[lang] ??= []).push({
        resolve: (docs: Record<string, string>) => {
          h.cache[lang] = docs;
          resolve(docs);
        },
        reject,
      });
    });
  },
  hasTranslation: () => true,
  isPartialLocale: () => false,
  getDocContent: (slug: string, loaded: { docs: Record<string, string> }) => loaded.docs[slug] ?? '',
  getDocTitle: (slug: string, loaded: { docs: Record<string, string> }) =>
    loaded.docs[slug] ? `Title: ${loaded.docs[slug]}` : slug,
  searchDocs: () => [],
}));

vi.mock('../components/Navbar', () => ({ default: () => <nav data-testid="navbar" /> }));
vi.mock('../components/Footer', () => ({ default: () => <footer data-testid="footer" /> }));
vi.mock('../components/MarkdownRenderer', () => ({
  default: ({ content }: { content: string }) => <div data-testid="markdown" data-content={content} />,
}));
vi.mock('../hooks/useResponsive', () => ({ useResponsive: () => ({ isMobile: false }) }));

function SwitchLangButton({ to }: { to: Language }) {
  const { setLanguage } = useTranslation();
  return <button data-testid={`switch-${to}`} onClick={() => setLanguage(to)} />;
}

function renderDocsPage(initialLang: Language = 'en') {
  window.localStorage.setItem('ocr-lang', initialLang);
  return render(
    <MemoryRouter initialEntries={['/docs/quickstart']}>
      <LanguageProvider>
        <SwitchLangButton to="en" />
        <SwitchLangButton to="zh" />
        <Routes>
          <Route path="/docs/:slug" element={<DocsPage />} />
        </Routes>
      </LanguageProvider>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  window.localStorage.clear();
  window.scrollTo = () => {};
  h.loadCalls.length = 0;
  h.cache = {};
  for (const key of Object.keys(h.pending)) delete h.pending[key];
});

afterEach(() => {
  cleanup();
});

describe('DocsPage', () => {
  it('shows a loading state, then renders the loaded content', async () => {
    renderDocsPage('en');

    expect(screen.getByRole('status').textContent).toBe('Loading documentation...');
    expect(screen.queryByTestId('markdown')).toBeNull();

    await act(async () => {
      h.pending.en[0].resolve({ quickstart: 'quickstart body' });
    });

    expect(screen.getByTestId('markdown').getAttribute('data-content')).toBe('quickstart body');
    expect(screen.getByText('Title: quickstart body')).toBeTruthy();
  });

  it('ignores a stale response after a language switch', async () => {
    renderDocsPage('en');
    expect(h.loadCalls).toEqual(['en']);

    fireEvent.click(screen.getByTestId('switch-zh'));
    await act(async () => {
      h.pending.en[0].resolve({ quickstart: 'stale en body' });
    });

    // The stale English response must not render.
    expect(screen.queryByTestId('markdown')).toBeNull();
    expect(screen.getByRole('status')).toBeTruthy();

    await act(async () => {
      h.pending.zh[0].resolve({ quickstart: 'zh quickstart body' });
    });

    expect(screen.getByTestId('markdown').getAttribute('data-content')).toBe('zh quickstart body');
  });

  it('shows an error with a retry action and recovers without a reload', async () => {
    renderDocsPage('en');

    await act(async () => {
      h.pending.en[0].reject(new Error('chunk load failed'));
    });

    expect(screen.getByRole('alert').textContent).toContain('Failed to load documentation content.');

    fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
    expect(h.loadCalls.filter(c => c === 'en').length).toBe(2);

    await act(async () => {
      h.pending.en[1].resolve({ quickstart: 'recovered body' });
    });

    expect(screen.getByTestId('markdown').getAttribute('data-content')).toBe('recovered body');
  });

  it('navigating to another article in the same language reuses the loaded content', async () => {
    renderDocsPage('en');

    await act(async () => {
      h.pending.en[0].resolve({ quickstart: 'quickstart body', installation: 'installation body' });
    });

    // The sidebar entry is the first 'Installation' text in the DOM (the
    // prev/next pagination uses the same label).
    fireEvent.click(screen.getAllByText('Installation')[0]);

    await waitFor(() => {
      expect(screen.getByTestId('markdown').getAttribute('data-content')).toBe('installation body');
    });
    expect(h.loadCalls.filter(c => c === 'en').length).toBe(1);
  });

  it('reuses the cached language content after switching away and back', async () => {
    renderDocsPage('en');

    await act(async () => {
      h.pending.en[0].resolve({ quickstart: 'en body' });
    });

    fireEvent.click(screen.getByTestId('switch-zh'));
    await act(async () => {
      h.pending.zh[0].resolve({ quickstart: 'zh body' });
    });

    fireEvent.click(screen.getByTestId('switch-en'));

    // English resolves from the session cache without a fresh load.
    expect(h.loadCalls.filter(c => c === 'en').length).toBe(1);
    await waitFor(() => {
      expect(screen.getByTestId('markdown').getAttribute('data-content')).toBe('en body');
    });
  });
});
