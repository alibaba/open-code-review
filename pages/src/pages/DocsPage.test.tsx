// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import React from 'react';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, act, waitFor, cleanup } from '@testing-library/react';
import { MemoryRouter, Routes, Route, useNavigate, useLocation } from 'react-router-dom';
import { LanguageProvider, useTranslation } from '../i18n';
import { en } from '../i18n/en';
import { zh } from '../i18n/zh';
import type { Language } from '../i18n/types';
import DocsPage from './DocsPage';

/* Deferred loads let each test resolve or reject a language bundle at a
 * chosen moment. The mock mirrors the real loader's contract: successful
 * loads stay cached for the session, failures do not. Translation
 * availability is controllable per language so partial-locale fallback can
 * be exercised with fixtures. */
const h = vi.hoisted(() => ({
  pending: {} as Record<string, { resolve: (docs: Record<string, string>) => void; reject: (err: unknown) => void }[]>,
  loadCalls: [] as string[],
  cache: {} as Record<string, Record<string, string>>,
  inFlight: {} as Record<string, Promise<Record<string, string>> | undefined>,
  available: {} as Record<string, string[]>,
}));

vi.mock('../content/docs', () => ({
  loadLanguageContent: (lang: string) => {
    if (h.cache[lang]) return Promise.resolve(h.cache[lang]);
    if (h.inFlight[lang]) return h.inFlight[lang];
    h.loadCalls.push(lang);
    const promise = new Promise<Record<string, string>>((resolve, reject) => {
      (h.pending[lang] ??= []).push({
        resolve: (docs: Record<string, string>) => {
          h.cache[lang] = docs;
          h.inFlight[lang] = undefined;
          resolve(docs);
        },
        reject: (error: unknown) => {
          h.inFlight[lang] = undefined;
          reject(error);
        },
      });
    });
    h.inFlight[lang] = promise;
    return promise;
  },
  hasTranslation: (slug: string, lang: string) => (h.available[lang] ?? []).includes(slug),
  isPartialLocale: (lang: string) => (h.available[lang] ?? []).length < (h.available.en ?? []).length,
  getDocContent: (slug: string, loaded: { docs: Record<string, string>; english?: Record<string, string> }) =>
    loaded.docs[slug] ?? loaded.english?.[slug] ?? '',
  getDocTitle: (slug: string, loaded: { docs: Record<string, string>; english?: Record<string, string> }) => {
    const body = loaded.docs[slug] ?? loaded.english?.[slug] ?? '';
    return body ? `Title: ${body}` : slug;
  },
  searchDocs: (query: string, loaded: { docs: Record<string, string>; english?: Record<string, string> }) => {
    const q = query.trim().toLowerCase();
    if (!q) return [];
    const out: { slug: string; title: string; snippet: string }[] = [];
    const seen = new Set<string>();
    for (const source of [loaded.docs, loaded.english]) {
      if (!source) continue;
      for (const slug of Object.keys(source)) {
        if (seen.has(slug)) continue;
        seen.add(slug);
        if ((source[slug] ?? '').toLowerCase().includes(q)) {
          out.push({ slug, title: slug, snippet: '...' });
        }
      }
    }
    return out;
  },
}));

vi.mock('../components/Navbar', () => ({ default: () => <nav data-testid="navbar" /> }));
vi.mock('../components/Footer', () => ({ default: () => <footer data-testid="footer" /> }));
/* Renders headings and links with the IDs and hrefs used by the real renderer so
 * fragment navigation tests exercise the delegated click path. */
vi.mock('../components/MarkdownRenderer', () => ({
  default: ({ content }: { content: string }) => {
    const lines = content.split('\n');
    return (
      <div data-testid="markdown" data-content={content}>
        {lines
          .filter(line => line.startsWith('## '))
          .map(line => {
            const text = line.slice(3).trim();
            return <h2 key={text} id={text.toLowerCase().replace(/[^a-z0-9]+/g, '-')}>{text}</h2>;
          })}
        {lines
          .filter(line => line.startsWith('['))
          .map(line => {
            const match = line.match(/^\[([^\]]+)\]\(([^)]+)\)$/);
            if (!match) return null;
            return <a key={line} href={match[2]}>{match[1]}</a>;
          })}
      </div>
    );
  },
}));
vi.mock('../hooks/useResponsive', () => ({ useResponsive: () => ({ isMobile: false }) }));

function SwitchLangButton({ to }: { to: Language }) {
  const { setLanguage } = useTranslation();
  return <button data-testid={`switch-${to}`} onClick={() => setLanguage(to)} />;
}

const ALL_TEST_SLUGS = ['quickstart', 'installation'];

function resetFixtures() {
  h.loadCalls.length = 0;
  h.cache = {};
  h.inFlight = {};
  h.available = { en: [...ALL_TEST_SLUGS], zh: [...ALL_TEST_SLUGS], ja: [...ALL_TEST_SLUGS] };
  for (const key of Object.keys(h.pending)) delete h.pending[key];
}

/* Back/forward buttons driven through the router's history stack, standing
 * in for browser navigation. */
function NavHistoryButtons() {
  const navigate = useNavigate();
  return (
    <>
      <button data-testid="go-back" onClick={() => navigate(-1)} />
      <button data-testid="go-forward" onClick={() => navigate(1)} />
    </>
  );
}

function CurrentLocation() {
  const { pathname, hash } = useLocation();
  return <div data-testid="location">{pathname}{hash}</div>;
}

function renderDocsPage(initialLang: Language = 'en', initialEntry = '/docs/quickstart') {
  window.localStorage.setItem('ocr-lang', initialLang);
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <LanguageProvider>
        <SwitchLangButton to="en" />
        <SwitchLangButton to="zh" />
        <SwitchLangButton to="ja" />
        <NavHistoryButtons />
        <CurrentLocation />
        <Routes>
          <Route path="/docs/:slug" element={<DocsPage />} />
        </Routes>
      </LanguageProvider>
    </MemoryRouter>,
  );
}

function openSearch() {
  fireEvent.keyDown(window, { key: 'k', ctrlKey: true });
}

function typeQuery(value: string) {
  fireEvent.change(screen.getByRole('textbox'), { target: { value } });
}

function mockReload(): ReturnType<typeof vi.fn> {
  const reloadSpy = vi.fn();
  vi.spyOn(window, 'location', 'get').mockReturnValue({
    reload: reloadSpy,
    href: 'http://localhost/',
  } as unknown as Location);
  return reloadSpy;
}

const scrollIntoView = vi.fn();

beforeEach(() => {
  window.localStorage.clear();
  window.scrollTo = () => {};
  resetFixtures();
  if (!window.requestAnimationFrame) {
    window.requestAnimationFrame = ((cb: FrameRequestCallback) =>
      setTimeout(() => cb(performance.now()), 0)) as unknown as typeof requestAnimationFrame;
    window.cancelAnimationFrame = clearTimeout as unknown as typeof cancelAnimationFrame;
  }
  Element.prototype.scrollIntoView = scrollIntoView;
  class IntersectionObserverStub {
    observe() {}
    unobserve() {}
    disconnect() {}
    takeRecords() { return []; }
  }
  (window as unknown as { IntersectionObserver: unknown }).IntersectionObserver = IntersectionObserverStub;
  scrollIntoView.mockClear();
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

  it('ignores intermediate responses after rapid language switches', async () => {
    renderDocsPage('en');
    expect(h.loadCalls).toEqual(['en']);

    fireEvent.click(screen.getByTestId('switch-zh'));
    fireEvent.click(screen.getByTestId('switch-ja'));

    await act(async () => {
      h.pending.en[0].resolve({ quickstart: 'stale en body' });
      h.pending.zh[0].resolve({ quickstart: 'stale zh body' });
    });

    // Neither stale response may render; the page is still loading Japanese.
    expect(screen.queryByTestId('markdown')).toBeNull();
    expect(screen.getByRole('status')).toBeTruthy();

    await act(async () => {
      h.pending.ja[0].resolve({ quickstart: 'ja quickstart body' });
    });

    expect(screen.getByTestId('markdown').getAttribute('data-content')).toBe('ja quickstart body');
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

  it('keeps transient main-language failures retryable in place', async () => {
    renderDocsPage('en');

    await act(async () => {
      h.pending.en[0].reject(new Error('temporary network failure'));
    });

    expect(screen.getByRole('button', { name: en['docs.content.retry'] })).toBeTruthy();
    expect(screen.queryByRole('button', { name: en['error.reload'] })).toBeNull();

    fireEvent.click(screen.getByRole('button', { name: en['docs.content.retry'] }));
    await act(async () => {
      h.pending.en[1].resolve({ quickstart: 'recovered body' });
    });

    expect(screen.getByTestId('markdown').getAttribute('data-content')).toBe('recovered body');
  });

  it('offers guarded page reload after repeated stale main-language chunk failures', async () => {
    const reloadSpy = mockReload();
    renderDocsPage('en');

    await act(async () => {
      h.pending.en[0].reject(new Error('Loading chunk 7 failed.'));
    });
    expect(screen.queryByRole('button', { name: en['error.reload'] })).toBeNull();

    fireEvent.click(screen.getByRole('button', { name: en['docs.content.retry'] }));
    await act(async () => {
      h.pending.en[1].reject(new Error('Loading chunk 7 failed.'));
    });

    expect(screen.getByRole('button', { name: en['error.reload'] })).toBeTruthy();
    sessionStorage.setItem('ocr-chunk-reload-attempted', 'true');
    fireEvent.click(screen.getByRole('button', { name: en['error.reload'] }));
    expect(sessionStorage.getItem('ocr-chunk-reload-attempted')).toBeNull();
    expect(reloadSpy).toHaveBeenCalledTimes(1);
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

  /* ─── Search content states ─── */

  it('search shows loading while content is pending, then results after arrival', async () => {
    renderDocsPage('en');
    openSearch();
    typeQuery('body');

    // The search must not claim "no results" before its content arrives.
    expect(screen.getByText('Searching...')).toBeTruthy();
    expect(screen.queryByText('No results found')).toBeNull();

    await act(async () => {
      h.pending.en[0].resolve({ quickstart: 'quickstart body' });
    });

    // The query is re-evaluated against the arrived content.
    expect(screen.getByRole('button', { name: /quickstart/ })).toBeTruthy();
    expect(screen.queryByText('Searching...')).toBeNull();
    expect(screen.queryByText('No results found')).toBeNull();
  });

  it('search shows an error with retry when the language content fails, then recovers', async () => {
    renderDocsPage('en');

    await act(async () => {
      h.pending.en[0].reject(new Error('chunk load failed'));
    });

    openSearch();
    typeQuery('body');

    expect(screen.getByText('Search content failed to load.')).toBeTruthy();
    expect(screen.queryByText('No results found')).toBeNull();

    // Both the page and the modal expose a retry; either recovers the search.
    fireEvent.click(screen.getAllByRole('button', { name: 'Retry' })[0]);
    expect(h.loadCalls.filter(c => c === 'en').length).toBe(2);

    await act(async () => {
      h.pending.en[1].resolve({ quickstart: 'quickstart body' });
    });

    expect(screen.getByRole('button', { name: /quickstart/ })).toBeTruthy();
  });

  it('search follows language switches with an open query', async () => {
    renderDocsPage('en');

    await act(async () => {
      h.pending.en[0].resolve({ quickstart: 'en match' });
    });

    openSearch();
    typeQuery('match');
    expect(screen.getByRole('button', { name: /quickstart/ })).toBeTruthy();

    fireEvent.click(screen.getByTestId('switch-zh'));
    // The pending language shows a loading state (in the new locale), not
    // stale results or a misleading "no results".
    expect(screen.getByText(zh['docs.search.loading'])).toBeTruthy();
    expect(screen.queryByRole('button', { name: /quickstart/ })).toBeNull();

    await act(async () => {
      h.pending.zh[0].resolve({ quickstart: 'zh match' });
    });

    expect(screen.getByRole('button', { name: /quickstart/ })).toBeTruthy();
    expect(screen.queryByText(zh['docs.search.loading'])).toBeNull();
  });

  /* ─── English fallback for partial locales ─── */

  it('renders a missing article through the English fallback for a partial locale', async () => {
    h.available.zh = ['quickstart'];
    renderDocsPage('zh', '/docs/installation');

    await act(async () => {
      h.pending.zh[0].resolve({ quickstart: 'zh quickstart' });
    });

    // The article is missing in the partial locale, so the page waits on the
    // English fallback rather than rendering an empty body.
    expect(screen.getByRole('status').textContent).toBe(zh['docs.content.loading']);
    expect(h.loadCalls).toEqual(['zh', 'en']);

    await act(async () => {
      h.pending.en[0].resolve({ installation: 'en installation body' });
    });

    expect(screen.getByTestId('markdown').getAttribute('data-content')).toBe('en installation body');
  });

  it('exposes retry when the English fallback fails for a missing article', async () => {
    h.available.zh = ['quickstart'];
    renderDocsPage('zh', '/docs/installation');

    await act(async () => {
      h.pending.zh[0].resolve({ quickstart: 'zh quickstart' });
    });
    await act(async () => {
      h.pending.en[0].reject(new Error('en chunk load failed'));
    });

    expect(screen.getByRole('alert').textContent).toContain(zh['docs.content.loadError']);
    expect(screen.queryByTestId('markdown')).toBeNull();

    fireEvent.click(screen.getByRole('button', { name: zh['docs.content.retry'] }));
    expect(h.loadCalls.filter(c => c === 'en').length).toBe(2);

    await act(async () => {
      h.pending.en[1].resolve({ installation: 'en installation body' });
    });

    expect(screen.getByTestId('markdown').getAttribute('data-content')).toBe('en installation body');
  });

  it('offers guarded page reload after repeated stale English fallback failures', async () => {
    const reloadSpy = mockReload();
    h.available.zh = ['quickstart'];
    renderDocsPage('zh', '/docs/installation');

    await act(async () => {
      h.pending.zh[0].resolve({ quickstart: 'zh quickstart' });
    });
    await act(async () => {
      h.pending.en[0].reject(new Error('Failed to fetch dynamically imported module: /en.js'));
    });

    expect(screen.queryByRole('button', { name: zh['error.reload'] })).toBeNull();
    fireEvent.click(screen.getByRole('button', { name: zh['docs.content.retry'] }));

    await act(async () => {
      h.pending.en[1].reject(new Error('Failed to fetch dynamically imported module: /en.js'));
    });

    expect(screen.getByRole('button', { name: zh['error.reload'] })).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: zh['error.reload'] }));
    expect(reloadSpy).toHaveBeenCalledTimes(1);
  });
  it('uses cached English content after a failed fallback and language round trip', async () => {
    h.available.zh = ['quickstart'];
    renderDocsPage('zh', '/docs/installation');

    await act(async () => {
      h.pending.zh[0].resolve({ quickstart: 'zh quickstart' });
    });
    expect(h.loadCalls).toEqual(['zh', 'en']);

    await act(async () => {
      h.pending.en[0].reject(new Error('en fallback failed'));
    });
    expect(screen.getByRole('alert').textContent).toContain(zh['docs.content.loadError']);
    expect(screen.getByRole('button', { name: zh['docs.content.retry'] })).toBeTruthy();
    expect(screen.queryByTestId('markdown')).toBeNull();

    fireEvent.click(screen.getByTestId('switch-en'));
    expect(h.loadCalls).toEqual(['zh', 'en', 'en']);
    await act(async () => {
      h.pending.en[1].resolve({ installation: 'en installation body' });
    });
    expect(screen.getByTestId('markdown').getAttribute('data-content')).toBe('en installation body');
    expect(screen.queryByRole('alert')).toBeNull();

    fireEvent.click(screen.getByTestId('switch-zh'));
    await waitFor(() => {
      expect(screen.getByTestId('markdown').getAttribute('data-content')).toBe('en installation body');
    });

    expect(screen.getByTestId('location').textContent).toBe('/docs/installation');
    expect(screen.queryByRole('alert')).toBeNull();
    expect(screen.queryByRole('status')).toBeNull();
    expect(screen.queryByRole('button', { name: zh['docs.content.retry'] })).toBeNull();
    expect(h.loadCalls).toEqual(['zh', 'en', 'en']);
  });

  it('reattaches the shared fallback response after navigating to another missing article', async () => {
    h.available.zh = [];
    renderDocsPage('zh', '/docs/installation');

    await act(async () => {
      h.pending.zh[0].resolve({});
    });
    expect(h.loadCalls).toEqual(['zh', 'en']);

    fireEvent.click(screen.getAllByText(zh['docs.sidebar.configuration'])[0]);
    expect(h.loadCalls.filter(c => c === 'en').length).toBe(1);

    await act(async () => {
      h.pending.en[0].resolve({ configuration: 'current configuration body' });
    });
    await waitFor(() => {
      expect(screen.getByTestId('markdown').getAttribute('data-content')).toBe('current configuration body');
    });
  });
  it('reattaches one delayed English bundle after rapid search changes', async () => {
    h.available.zh = ['quickstart'];
    renderDocsPage('zh', '/docs/quickstart');

    await act(async () => {
      h.pending.zh[0].resolve({ quickstart: 'zh quickstart' });
    });

    openSearch();
    typeQuery('installation');
    typeQuery('configuration');
    expect(h.loadCalls).toEqual(['zh', 'en']);

    await act(async () => {
      h.pending.en[0].resolve({
        installation: 'installation result',
        configuration: 'configuration result',
      });
    });

    expect(screen.getByRole('button', { name: /configuration/ })).toBeTruthy();
    expect(screen.queryByText(zh['docs.search.loading'])).toBeNull();
  });

  it('completes the search after a trailing-space edit while the fallback is in flight', async () => {
    h.available.zh = ['quickstart'];
    renderDocsPage('zh', '/docs/quickstart');

    await act(async () => {
      h.pending.zh[0].resolve({ quickstart: 'zh quickstart' });
    });

    openSearch();
    typeQuery('english-only');
    typeQuery('english-only ');
    expect(screen.getByText(zh['docs.search.loading'])).toBeTruthy();

    await act(async () => {
      h.pending.en[0].resolve({ installation: 'english-only body' });
    });

    expect(screen.getByRole('button', { name: /installation/ })).toBeTruthy();
    expect(screen.queryByText(zh['docs.search.loading'])).toBeNull();
    expect(h.loadCalls).toEqual(['zh', 'en']);
  });

  it('keeps the retry attempt through query edits and still offers reload after a second failure', async () => {
    h.available.zh = ['quickstart'];
    renderDocsPage('zh', '/docs/quickstart');

    await act(async () => {
      h.pending.zh[0].resolve({ quickstart: 'zh quickstart' });
    });

    openSearch();
    typeQuery('english-only');
    await act(async () => {
      h.pending.en[0].reject(new Error('Failed to fetch dynamically imported module: /en.js'));
    });
    expect(screen.getByText(zh['docs.search.loadError'])).toBeTruthy();
    expect(screen.queryByRole('button', { name: zh['error.reload'] })).toBeNull();

    fireEvent.click(screen.getByRole('button', { name: zh['docs.content.retry'] }));
    typeQuery('english-only extra');

    await act(async () => {
      h.pending.en[1].reject(new Error('Failed to fetch dynamically imported module: /en.js'));
    });

    expect(screen.getByText(zh['docs.search.loadError'])).toBeTruthy();
    expect(screen.getByRole('button', { name: zh['error.reload'] })).toBeTruthy();
    expect(h.loadCalls.filter(c => c === 'en').length).toBe(2);
  });

  it('reattaches the in-flight fallback after the query is cleared and re-entered', async () => {
    h.available.zh = ['quickstart'];
    renderDocsPage('zh', '/docs/quickstart');

    await act(async () => {
      h.pending.zh[0].resolve({ quickstart: 'zh quickstart' });
    });

    openSearch();
    typeQuery('english-only');
    typeQuery('');
    typeQuery('english');
    expect(h.loadCalls.filter(c => c === 'en').length).toBe(1);

    await act(async () => {
      h.pending.en[0].resolve({ installation: 'english body' });
    });

    expect(screen.getByRole('button', { name: /installation/ })).toBeTruthy();
    expect(screen.queryByText(zh['docs.search.loading'])).toBeNull();
  });

  it('keeps the retry attempt through article changes and still offers reload after a second failure', async () => {
    h.available.zh = [];
    renderDocsPage('zh', '/docs/installation');

    await act(async () => {
      h.pending.zh[0].resolve({});
    });
    await act(async () => {
      h.pending.en[0].reject(new Error('Failed to fetch dynamically imported module: /en.js'));
    });
    expect(screen.getByRole('alert')).toBeTruthy();
    expect(screen.queryByRole('button', { name: zh['error.reload'] })).toBeNull();

    fireEvent.click(screen.getByRole('button', { name: zh['docs.content.retry'] }));
    fireEvent.click(screen.getAllByText(zh['docs.sidebar.configuration'])[0]);

    await act(async () => {
      h.pending.en[1].reject(new Error('Failed to fetch dynamically imported module: /en.js'));
    });

    expect(screen.getByRole('alert')).toBeTruthy();
    expect(screen.getByRole('button', { name: zh['error.reload'] })).toBeTruthy();
    expect(h.loadCalls.filter(c => c === 'en').length).toBe(2);
  });

  it('reattaches a delayed English bundle after leaving and returning to an article', async () => {
    h.available.zh = ['quickstart'];
    renderDocsPage('zh', '/docs/installation');

    await act(async () => {
      h.pending.zh[0].resolve({ quickstart: 'zh quickstart' });
    });
    expect(h.loadCalls).toEqual(['zh', 'en']);

    fireEvent.click(screen.getAllByText(zh['docs.sidebar.quickstart'])[0]);
    fireEvent.click(screen.getAllByText(zh['docs.sidebar.installation'])[0]);

    await act(async () => {
      h.pending.en[0].resolve({ installation: 'en installation body' });
    });

    await waitFor(() => {
      expect(screen.getByTestId('markdown').getAttribute('data-content')).toBe('en installation body');
    });
    expect(h.loadCalls.filter(c => c === 'en').length).toBe(1);
    expect(screen.queryByRole('button', { name: zh['docs.content.retry'] })).toBeNull();
  });

  it('shows the current fallback error and retries after changing articles', async () => {
    h.available.zh = [];
    renderDocsPage('zh', '/docs/installation');

    await act(async () => {
      h.pending.zh[0].resolve({});
    });
    fireEvent.click(screen.getAllByText(zh['docs.sidebar.configuration'])[0]);

    await act(async () => {
      h.pending.en[0].reject(new Error('en fallback failed'));
    });
    expect(screen.getByRole('alert')).toBeTruthy();

    fireEvent.click(screen.getByRole('button', { name: zh['docs.content.retry'] }));
    await act(async () => {
      h.pending.en[1].resolve({ configuration: 'current configuration body' });
    });

    await waitFor(() => {
      expect(screen.getByTestId('markdown').getAttribute('data-content')).toBe('current configuration body');
    });
  });

  it('keeps the fragment and scrolls after a fallback Retry succeeds', async () => {
    h.available.zh = ['quickstart'];
    renderDocsPage('zh', '/docs/quickstart');

    await act(async () => {
      h.pending.zh[0].resolve({
        quickstart: '[Installation](../installation/#intro)',
      });
    });

    fireEvent.click(screen.getByRole('link', { name: 'Installation' }));
    expect(screen.getByTestId('location').textContent).toBe('/docs/installation#intro');

    await act(async () => {
      h.pending.en[0].reject(new Error('en fallback failed'));
    });
    fireEvent.click(screen.getByRole('button', { name: zh['docs.content.retry'] }));

    await act(async () => {
      h.pending.en[1].resolve({ installation: '## Intro\n\ninstallation body' });
    });

    await waitFor(() => {
      expect(scrollIntoView).toHaveBeenCalledTimes(1);
    });
    expect(screen.getByTestId('location').textContent).toBe('/docs/installation#intro');
  });
  it('preserves delayed internal fragments through fallback navigation and history', async () => {
    h.available.zh = ['quickstart'];
    renderDocsPage('zh', '/docs/quickstart');

    await act(async () => {
      h.pending.zh[0].resolve({
        quickstart: '[Installation](../installation/#intro)',
      });
    });

    fireEvent.click(screen.getByRole('link', { name: 'Installation' }));
    expect(screen.getByTestId('location').textContent).toBe('/docs/installation#intro');
    expect(h.loadCalls).toEqual(['zh', 'en']);

    await new Promise(resolve => setTimeout(resolve, 20));
    expect(scrollIntoView).not.toHaveBeenCalled();

    await act(async () => {
      h.pending.en[0].resolve({ installation: '## Intro\n\ninstallation body' });
    });

    await waitFor(() => {
      expect(scrollIntoView).toHaveBeenCalledTimes(1);
    });

    fireEvent.click(screen.getByTestId('go-back'));
    await waitFor(() => {
      expect(screen.getByTestId('location').textContent).toBe('/docs/quickstart');
    });

    fireEvent.click(screen.getByTestId('go-forward'));
    await waitFor(() => {
      expect(screen.getByTestId('location').textContent).toBe('/docs/installation#intro');
    });
    await waitFor(() => {
      expect(scrollIntoView).toHaveBeenCalledTimes(2);
    });
  });
  it('search in a partial locale loads the English fallback and covers English entries', async () => {
    h.available.zh = ['quickstart'];
    renderDocsPage('zh', '/docs/quickstart');

    await act(async () => {
      h.pending.zh[0].resolve({ quickstart: 'zh quickstart' });
    });

    openSearch();
    typeQuery('english-only');

    // The query needs English coverage for the partial locale, so the search
    // reports loading until the fallback arrives.
    expect(screen.getByText(zh['docs.search.loading'])).toBeTruthy();
    expect(h.loadCalls).toEqual(['zh', 'en']);

    await act(async () => {
      h.pending.en[0].resolve({ installation: 'english-only body' });
    });

    expect(screen.getByRole('button', { name: /installation/ })).toBeTruthy();
    expect(screen.queryByText(zh['docs.search.loading'])).toBeNull();
  });

  it('does not request English for fully translated pages without a query', async () => {
    h.available.zh = ['quickstart', 'installation'];
    renderDocsPage('zh', '/docs/quickstart');

    await act(async () => {
      h.pending.zh[0].resolve({ quickstart: 'zh quickstart' });
    });

    // Give any speculative fallback effect a chance to fire.
    await act(async () => {});

    expect(h.loadCalls).toEqual(['zh']);
    expect(screen.getByTestId('markdown').getAttribute('data-content')).toBe('zh quickstart');
  });

  /* ─── Asynchronous navigation ─── */

  it('scrolls to the URL fragment only after the content renders', async () => {
    renderDocsPage('en', '/docs/quickstart#intro');

    expect(scrollIntoView).not.toHaveBeenCalled();

    await act(async () => {
      h.pending.en[0].resolve({ quickstart: '## Intro\n\nintro body' });
    });

    await waitFor(() => {
      expect(scrollIntoView).toHaveBeenCalledTimes(1);
    });
  });

  it('updates the page TOC after the content renders', async () => {
    renderDocsPage('en');

    await act(async () => {
      h.pending.en[0].resolve({ quickstart: '## Intro\n\nintro body\n\n## Details\n\ndetails body' });
    });

    expect(screen.getByRole('button', { name: 'Intro' })).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Details' })).toBeTruthy();
  });

  it('browser back and forward re-render content without reloading the language', async () => {
    renderDocsPage('en');

    await act(async () => {
      h.pending.en[0].resolve({ quickstart: 'quickstart body', installation: 'installation body' });
    });
    expect(screen.getByTestId('markdown').getAttribute('data-content')).toBe('quickstart body');

    fireEvent.click(screen.getAllByText('Installation')[0]);
    await waitFor(() => {
      expect(screen.getByTestId('markdown').getAttribute('data-content')).toBe('installation body');
    });

    fireEvent.click(screen.getByTestId('go-back'));
    await waitFor(() => {
      expect(screen.getByTestId('markdown').getAttribute('data-content')).toBe('quickstart body');
    });

    fireEvent.click(screen.getByTestId('go-forward'));
    await waitFor(() => {
      expect(screen.getByTestId('markdown').getAttribute('data-content')).toBe('installation body');
    });

    expect(h.loadCalls.filter(c => c === 'en').length).toBe(1);
  });
});
