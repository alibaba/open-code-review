// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import githubIcon from '../assets/icons/icon-github.svg';
import { LanguageProvider } from '../i18n';
import { en } from '../i18n/en';
import { zh } from '../i18n/zh';
import { ja } from '../i18n/ja';
import { ko } from '../i18n/ko';
import { ru } from '../i18n/ru';
import { vi as viLang } from '../i18n/vi';
import Footer from './Footer';

function renderFooter() {
  return render(
    <LanguageProvider>
      <Footer />
    </LanguageProvider>,
  );
}

afterEach(() => {
  cleanup();
  localStorage.clear();
  vi.unstubAllGlobals();
});

describe.each([375, 1440])('Footer at %dpx', (width) => {
  it.each(Object.entries({ en, zh, ja, ko, ru, vi: viLang }))('links to the contact profile in %s', (language, translations) => {
    vi.stubGlobal('innerWidth', width);
    localStorage.setItem('ocr-lang', language);
    renderFooter();

    const footer = within(screen.getByRole('contentinfo'));
    const contact = footer.getByRole('link', { name: translations['footer.contact'] });
    expect(contact).toBeVisible();
    expect(contact).toHaveAttribute('href', 'https://github.com/lizhengfeng101');
    expect(contact).toHaveAttribute('target', '_blank');
    expect(contact).toHaveAttribute('rel', 'noopener noreferrer');
    expect(within(contact).getByRole('presentation')).toHaveAttribute('src', githubIcon);
    expect(footer.getByRole('link', { name: translations['footer.brand'] }))
      .toHaveAttribute('href', 'https://github.com/alibaba/open-code-review');
  });
});

it('keeps the contact link keyboard accessible', async () => {
  localStorage.setItem('ocr-lang', 'en');
  const user = userEvent.setup();
  renderFooter();

  await user.tab();
  await user.tab();
  expect(screen.getByRole('link', { name: 'Contact us' })).toHaveFocus();
  await user.tab();
  expect(screen.getByRole('button', { name: 'English' })).toHaveFocus();
});

it('updates the contact label when switching languages', async () => {
  localStorage.setItem('ocr-lang', 'zh');
  const user = userEvent.setup();
  renderFooter();

  expect(screen.getByRole('link', { name: zh['footer.contact'] })).toBeVisible();
  await user.click(screen.getByRole('button'));
  await user.click(screen.getByRole('button', { name: 'English' }));

  expect(screen.getByRole('link', { name: 'Contact us' }))
    .toHaveAttribute('href', 'https://github.com/lizhengfeng101');
});
