// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import React from 'react';
import { describe, it, expect, beforeAll, beforeEach, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Routes, Route, useLocation } from 'react-router-dom';
import DocsPage from './DocsPage';
import { LanguageProvider } from '../i18n';

vi.mock('../content/docs', () => ({
  getDocContent: () => '## Quickstart\n',
  getDocTitle: () => 'Quickstart',
  searchDocs: () => [],
}));

const LocationProbe: React.FC = () => {
  const { pathname } = useLocation();
  return <div data-testid="pathname">{pathname}</div>;
};

function renderDocs(initialEntry: string) {
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <LanguageProvider>
        <LocationProbe />
        <Routes>
          <Route path="/docs/:slug" element={<DocsPage />} />
        </Routes>
      </LanguageProvider>
    </MemoryRouter>,
  );
}

function installLocalStorageMock() {
  let store: Record<string, string> = {};
  Object.defineProperty(window, 'localStorage', {
    configurable: true,
    value: {
      getItem: (key: string) => store[key] ?? null,
      setItem: (key: string, value: string) => {
        store[key] = String(value);
      },
      removeItem: (key: string) => {
        delete store[key];
      },
      clear: () => {
        store = {};
      },
      key: (index: number) => Object.keys(store)[index] ?? null,
      get length() {
        return Object.keys(store).length;
      },
    } as Storage,
  });
}

describe('DocsPage', () => {
  beforeAll(() => {
    if (!('IntersectionObserver' in window)) {
      class IntersectionObserverStub {
        observe() {}
        unobserve() {}
        disconnect() {}
        takeRecords() {
          return [];
        }
      }
      (window as unknown as { IntersectionObserver: unknown }).IntersectionObserver = IntersectionObserverStub;
    }
  });

  beforeEach(() => {
    installLocalStorageMock();
    window.localStorage.clear();
    window.scrollTo = () => {};
  });

  it('redirects an unknown doc slug to the canonical quickstart URL', async () => {
    renderDocs('/docs/troubleshooting/#timeout-errorsuttggggg');

    await waitFor(() => {
      expect(screen.getByTestId('pathname').textContent).toBe('/docs/quickstart');
    });
  });

  it('keeps a known doc slug on its own URL', async () => {
    renderDocs('/docs/installation');

    await screen.findByRole('heading', { level: 1 });
    expect(screen.getByTestId('pathname').textContent).toBe('/docs/installation');
  });
});
