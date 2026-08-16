// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import React from 'react';
import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { MemoryRouter, Routes, Route } from 'react-router-dom';
import NotFoundPage from './NotFoundPage';
import { LanguageProvider } from '../i18n';
import { en } from '../i18n/en';
import { zh } from '../i18n/zh';
import { ja } from '../i18n/ja';
import { ru } from '../i18n/ru';
import type { Language, TranslationKeys } from '../i18n/types';

const translations: Record<Language, TranslationKeys> = { en, zh, ja, ru };

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

function renderNotFound(language: Language) {
  if (language !== 'en') {
    window.localStorage.setItem('ocr-lang', language);
  }

  return render(
    <MemoryRouter initialEntries={['/missing']}>
      <LanguageProvider>
        <Routes>
          <Route path="/" element={<div>home page</div>} />
          <Route path="*" element={<NotFoundPage />} />
        </Routes>
      </LanguageProvider>
    </MemoryRouter>,
  );
}

describe('NotFoundPage', () => {
  beforeEach(() => {
    installLocalStorageMock();
    window.localStorage.clear();
    window.scrollTo = () => {};
  });

  afterEach(() => {
    window.localStorage.clear();
  });

  it('renders the not-found message and returns home', () => {
    renderNotFound('en');

    expect(screen.getByRole('heading', { name: 'Page not found' })).toBeTruthy();
    expect(
      screen.getByText('The page you are looking for does not exist or has moved.'),
    ).toBeTruthy();

    fireEvent.click(screen.getByRole('button', { name: 'Back to Home' }));
    expect(screen.getByText('home page')).toBeTruthy();
  });

  it.each(
    (Object.keys(translations) as Language[]).map((language) => [language, translations[language]] as const),
  )('renders the %s translation', (language, table) => {
    renderNotFound(language);
    expect(screen.getByRole('heading', { name: table['notFound.title'] })).toBeTruthy();
  });
});
