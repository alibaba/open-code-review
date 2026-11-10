// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import React from 'react';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import DocsPage from './DocsPage';

const mocks = vi.hoisted(() => ({
  loadDocPage: vi.fn(),
  searchDocs: vi.fn(async () => []),
}));

vi.mock('../content/docs', () => mocks);
vi.mock('../i18n', () => ({
  useTranslation: () => ({ language: 'en', t: (key: string) => key }),
}));
vi.mock('../hooks/useResponsive', () => ({
  useResponsive: () => ({ isMobile: true }),
}));
vi.mock('../components/Navbar', () => ({ default: () => <div data-testid="navbar" /> }));
vi.mock('../components/Footer', () => ({ default: () => <div data-testid="footer" /> }));
vi.mock('../components/MarkdownRenderer', () => ({
  default: ({ content }: { content: string }) => (
    <>
      <h2 id="target-heading">{content}</h2>
      {content === 'Linked content' && <a href="../installation/#target-heading">Installation section</a>}
    </>
  ),
}));

function LocationProbe() {
  const location = useLocation();
  return <div data-testid="location">{location.pathname}{location.hash}</div>;
}

function renderPage(entry: string) {
  return render(
    <MemoryRouter initialEntries={[entry]}>
      <LocationProbe />
      <Routes>
        <Route path="/docs/:slug" element={<DocsPage />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe('DocsPage asynchronous navigation', () => {
  beforeEach(() => {
    mocks.loadDocPage.mockReset();
    mocks.searchDocs.mockClear();
    vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => (
      window.setTimeout(() => callback(performance.now()), 0)
    ));
    vi.stubGlobal('cancelAnimationFrame', (id: number) => window.clearTimeout(id));
    vi.stubGlobal('scrollTo', vi.fn());
    Element.prototype.scrollIntoView = vi.fn();
  });

  it('scrolls a deep link only after its asynchronous content renders', async () => {
    mocks.loadDocPage.mockResolvedValue({
      slug: 'quickstart',
      title: 'QuickStart',
      content: 'Loaded content',
    });

    renderPage('/docs/quickstart#target-heading');
    expect(screen.getByRole('status')).toHaveTextContent('docs.loading');
    expect(await screen.findByRole('heading', { name: 'QuickStart' })).toBeInTheDocument();
    await waitFor(() => expect(Element.prototype.scrollIntoView).toHaveBeenCalled());
    expect(screen.getByTestId('location')).toHaveTextContent('/docs/quickstart#target-heading');
  });

  it('loads the next document after client-side navigation', async () => {
    mocks.loadDocPage.mockImplementation(async (slug: string) => ({
      slug,
      title: slug === 'quickstart' ? 'QuickStart' : 'Installation',
      content: `${slug} content`,
    }));

    renderPage('/docs/quickstart');
    expect(await screen.findByRole('heading', { name: 'QuickStart' })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /docs.sidebar.installation/ }));
    await waitFor(() => expect(screen.getByTestId('location')).toHaveTextContent('/docs/installation'));
    expect(await screen.findByRole('heading', { name: 'Installation' })).toBeInTheDocument();
    expect(mocks.loadDocPage).toHaveBeenLastCalledWith('installation', 'en');
  });

  it('retains cross-document fragments until lazy content renders', async () => {
    mocks.loadDocPage.mockImplementation(async (slug: string) => ({
      slug,
      title: slug === 'quickstart' ? 'QuickStart' : 'Installation',
      content: slug === 'quickstart' ? 'Linked content' : 'Installation content',
    }));

    renderPage('/docs/quickstart');
    fireEvent.click(await screen.findByRole('link', { name: 'Installation section' }));
    await waitFor(() => (
      expect(screen.getByTestId('location')).toHaveTextContent('/docs/installation#target-heading')
    ));
    expect(await screen.findByRole('heading', { name: 'Installation' })).toBeInTheDocument();
    await waitFor(() => expect(Element.prototype.scrollIntoView).toHaveBeenCalled());
  });
});
