// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { cleanup, render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { LanguageProvider } from '../i18n';
import { en } from '../i18n/en';
import { zh } from '../i18n/zh';
import { ja } from '../i18n/ja';
import { ko } from '../i18n/ko';
import { ru } from '../i18n/ru';
import ProvidersSection from './ProvidersSection';

function renderProviders() {
  return render(
    <MemoryRouter>
      <LanguageProvider>
        <ProvidersSection />
      </LanguageProvider>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  localStorage.setItem('ocr-lang', 'en');
});

afterEach(() => {
  cleanup();
  localStorage.clear();
});

describe('ProvidersSection', () => {
  it('exposes one named region and one accessible list of model brands', () => {
    renderProviders();

    expect(screen.getByRole('region', { name: 'Built-in support for leading model providers' })).toBeInTheDocument();
    const list = screen.getByRole('list', { name: 'Supported model brands' });
    const names = within(list).getAllByRole('listitem').map((item) => item.textContent);
    expect(names).toEqual(['Anthropic', 'OpenAI', 'Gemini', 'Qwen', 'DeepSeek', 'Kimi', 'Z.ai', 'MiniMax', 'Mistral', 'xAI']);
    expect(screen.getAllByRole('list')).toHaveLength(1);
    expect(screen.getAllByRole('listitem')).toHaveLength(10);
    expect(screen.queryAllByRole('img')).toHaveLength(0);
  });

  it('hides the identical loop copy from assistive technology', () => {
    renderProviders();

    const lists = screen.getAllByRole('list', { hidden: true });
    expect(lists).toHaveLength(2);
    expect(lists[1]).toHaveAttribute('aria-hidden', 'true');
    expect(lists[1].textContent).toBe(lists[0].textContent);
    expect(lists[1].querySelector('a, button, [tabindex]')).toBeNull();
  });

  it('links to the provider configuration guide', () => {
    renderProviders();

    expect(screen.getByRole('link', { name: 'Explore supported providers' })).toHaveAttribute('href', '/docs/configuration');
  });

  it('does not render a separate animation control', () => {
    renderProviders();

    expect(screen.queryByRole('button')).not.toBeInTheDocument();
  });

  it('keeps the documentation link keyboard accessible', async () => {
    const user = userEvent.setup();
    renderProviders();

    await user.tab();
    expect(screen.getByRole('link')).toHaveFocus();
  });

  it.each(Object.entries({ en, zh, ja, ko, ru }))('renders translated copy for %s', (language, translations) => {
    localStorage.setItem('ocr-lang', language);
    renderProviders();

    expect(screen.getByRole('heading', { level: 2 })).toHaveTextContent(translations['providers.title']);
    expect(screen.getByText(translations['providers.description'])).toBeInTheDocument();
    expect(screen.getByRole('list', { name: translations['providers.listLabel'] })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: translations['providers.explore'] })).toBeInTheDocument();
  });
});
