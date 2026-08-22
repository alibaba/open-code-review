// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { LanguageProvider } from '../i18n';
import HeroSection from './HeroSection';

function renderHero() {
  render(
    <MemoryRouter>
      <LanguageProvider>
        <HeroSection />
      </LanguageProvider>
    </MemoryRouter>,
  );
}

// The panel is found through the id that `aria-controls` already points at, so
// the test leans on the accessibility wiring instead of a test-only hook.
const panel = () => document.getElementById('install-more-panel');
const trigger = () => screen.getByRole('button', { name: /More|MacPorts/i });

describe('HeroSection install channels', () => {
  it('starts on the first channel with the panel closed', () => {
    renderHero();
    expect(screen.getByText('npm i -g @alibaba-group/open-code-review')).toBeTruthy();
    expect(panel()).toBeNull();
  });

  it('picking an overflow channel swaps the command and closes the panel', async () => {
    const user = userEvent.setup();
    renderHero();

    await user.click(trigger());
    expect(panel()).not.toBeNull();

    await user.click(screen.getByRole('button', { name: /MacPorts/i }));

    expect(screen.getByText('sudo port install open-code-review')).toBeTruthy();
    expect(panel()).toBeNull();
  });

  it('closes when a primary tab is clicked', async () => {
    const user = userEvent.setup();
    renderHero();

    await user.click(trigger());
    await user.click(screen.getByRole('button', { name: /Homebrew/i }));

    expect(screen.getByText('brew install open-code-review')).toBeTruthy();
    expect(panel()).toBeNull();
  });

  // Keyboard activation dispatches `click` with no preceding `mousedown`, so
  // this does not go through the same path as the test above.
  it('closes when a primary tab is activated by keyboard', async () => {
    const user = userEvent.setup();
    renderHero();

    await user.click(trigger());
    screen.getByRole('button', { name: /Homebrew/i }).focus();
    await user.keyboard('{Enter}');

    expect(screen.getByText('brew install open-code-review')).toBeTruthy();
    expect(panel()).toBeNull();
  });

  it('closes on Escape and on an outside click', async () => {
    const user = userEvent.setup();
    renderHero();

    await user.click(trigger());
    await user.keyboard('{Escape}');
    expect(panel()).toBeNull();

    await user.click(trigger());
    await user.click(document.body);
    expect(panel()).toBeNull();
  });

  it('reflects the selected overflow channel on the trigger', async () => {
    const user = userEvent.setup();
    renderHero();
    expect(screen.getByRole('button', { name: /^More$/i })).toBeTruthy();

    await user.click(trigger());
    await user.click(screen.getByRole('button', { name: /MacPorts/i }));

    const collapsed = screen.getByRole('button', { name: /MacPorts/i });
    expect(collapsed.getAttribute('aria-expanded')).toBe('false');
    expect(screen.queryByRole('button', { name: /^More$/i })).toBeNull();
  });
});
