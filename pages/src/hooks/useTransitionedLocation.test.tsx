// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import React, { Suspense } from 'react';
import { describe, it, expect } from 'vitest';
import { render, screen, fireEvent, act } from '@testing-library/react';
import { MemoryRouter, Routes, Route, useNavigate } from 'react-router-dom';
import { useTransitionedLocation } from './useTransitionedLocation';

/** A lazy component whose chunk resolves only when `resolveChunk` is called. */
function deferredLazy(): { LazyPage: React.LazyExoticComponent<React.FC>; resolveChunk: () => Promise<void> } {
  let resolve!: (m: { default: React.FC }) => void;
  const chunk = new Promise<{ default: React.FC }>((r) => {
    resolve = r;
  });
  const LazyPage = React.lazy(() => chunk);
  const resolveChunk = async () => {
    resolve({ default: () => <div>lazy page</div> });
    // Let React finish the resumed render.
    await act(async () => {
      await chunk;
    });
  };
  return { LazyPage, resolveChunk };
}

const HomePage: React.FC = () => {
  const navigate = useNavigate();
  return (
    <div>
      <span>home page</span>
      <button type="button" onClick={() => navigate('/lazy')}>
        go
      </button>
    </div>
  );
};

const Harness: React.FC<{ LazyPage: React.FC }> = ({ LazyPage }) => {
  const displayLocation = useTransitionedLocation();
  const navigate = useNavigate();
  return (
    <>
      <button type="button" onClick={() => navigate(-1)}>
        back
      </button>
      <Suspense fallback={<div data-testid="black-fallback" />}>
        <Routes location={displayLocation}>
          <Route path="/" element={<HomePage />} />
          <Route path="/lazy" element={<LazyPage />} />
        </Routes>
      </Suspense>
    </>
  );
};

describe('useTransitionedLocation', () => {
  it('keeps the previous page visible while a lazy route loads, instead of the Suspense fallback', async () => {
    const { LazyPage, resolveChunk } = deferredLazy();
    render(
      <MemoryRouter initialEntries={['/']}>
        <Harness LazyPage={LazyPage} />
      </MemoryRouter>,
    );
    expect(screen.getByText('home page')).toBeTruthy();

    fireEvent.click(screen.getByRole('button', { name: 'go' }));

    // The chunk has not resolved: the old page must still be on screen and
    // the black fallback must not have replaced it.
    expect(screen.getByText('home page')).toBeTruthy();
    expect(screen.queryByTestId('black-fallback')).toBeNull();

    await resolveChunk();
    expect(await screen.findByText('lazy page')).toBeTruthy();
    expect(screen.queryByText('home page')).toBeNull();
  });

  it('still shows the Suspense fallback on first paint, when there is no previous page', async () => {
    const { LazyPage, resolveChunk } = deferredLazy();
    render(
      <MemoryRouter initialEntries={['/lazy']}>
        <Harness LazyPage={LazyPage} />
      </MemoryRouter>,
    );
    // First load of a lazy route: nothing to keep on screen, fallback shows.
    expect(screen.getByTestId('black-fallback')).toBeTruthy();

    await resolveChunk();
    expect(await screen.findByText('lazy page')).toBeTruthy();
  });

  it('follows back/forward navigation', async () => {
    const { LazyPage, resolveChunk } = deferredLazy();
    render(
      <MemoryRouter initialEntries={['/']}>
        <Harness LazyPage={LazyPage} />
      </MemoryRouter>,
    );
    fireEvent.click(screen.getByRole('button', { name: 'go' }));
    await resolveChunk();
    expect(await screen.findByText('lazy page')).toBeTruthy();

    // Back to "/" — that route is not lazy, so it renders directly.
    fireEvent.click(screen.getByRole('button', { name: 'back' }));
    expect(await screen.findByText('home page')).toBeTruthy();
  });
});
