// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import React from 'react';
import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { fireEvent } from '@testing-library/dom';
import { ThemeProvider, useTheme } from './ThemeProvider';

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

const ToggleProbe: React.FC = () => {
  const { theme, toggleTheme } = useTheme();
  return (
    <button type="button" onClick={toggleTheme}>
      {theme}
    </button>
  );
};

describe('ThemeProvider', () => {
  beforeEach(() => {
    installLocalStorageMock();
    window.localStorage.clear();
    delete document.documentElement.dataset.theme;
  });

  afterEach(() => {
    delete document.documentElement.dataset.theme;
  });

  it('defaults to dark when nothing is stored', () => {
    render(
      <ThemeProvider>
        <ToggleProbe />
      </ThemeProvider>
    );
    expect(screen.getByRole('button').textContent).toBe('dark');
  });

  it('restores a stored light theme', () => {
    window.localStorage.setItem('ocr-theme', 'light');
    render(
      <ThemeProvider>
        <ToggleProbe />
      </ThemeProvider>
    );
    expect(screen.getByRole('button').textContent).toBe('light');
  });

  it('toggles dark to light, persists the choice, and updates <html>', () => {
    render(
      <ThemeProvider>
        <ToggleProbe />
      </ThemeProvider>
    );
    expect(document.documentElement.dataset.theme).toBe('dark');

    fireEvent.click(screen.getByRole('button'));

    expect(screen.getByRole('button').textContent).toBe('light');
    expect(document.documentElement.dataset.theme).toBe('light');
    expect(window.localStorage.getItem('ocr-theme')).toBe('light');
  });

  it('toggles back to dark and persists it', () => {
    window.localStorage.setItem('ocr-theme', 'light');
    render(
      <ThemeProvider>
        <ToggleProbe />
      </ThemeProvider>
    );
    fireEvent.click(screen.getByRole('button'));
    expect(window.localStorage.getItem('ocr-theme')).toBe('dark');
    expect(document.documentElement.dataset.theme).toBe('dark');
  });
});
