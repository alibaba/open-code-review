// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import React from 'react';
import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/react';
import { ThemedMonochromeIcon } from './ThemedMonochromeIcon';

const fakeSvgUrl = 'data:image/svg+xml,fake-monochrome-asset';

// The monochrome brand/social assets have literal white fills baked in.
// Rendering them through a plain <img> makes them invisible over the white
// documentation surfaces the light theme introduces (#1383), so the affected
// elements must go through the mask-based recoloring instead. These tests pin
// that contract for the component itself; Navbar and Footer consume it.
describe('ThemedMonochromeIcon', () => {
  it('paints the asset through a mask using the theme text token', () => {
    const { container } = render(
      <ThemedMonochromeIcon src={fakeSvgUrl} alt="Open Code Review" width={192} height={24} />,
    );
    const el = container.firstElementChild as HTMLElement;
    expect(el.tagName).toBe('SPAN');
    expect(el.style.backgroundColor).toBe('var(--text-strong)');
    expect(el.style.maskImage).toContain('fake-monochrome-asset');
    expect(el.style.maskSize).toBe('contain');
    expect(el.style.maskRepeat).toBe('no-repeat');
  });

  it('exposes the accessible name for meaningful icons', () => {
    const { container } = render(
      <ThemedMonochromeIcon src={fakeSvgUrl} alt="Open Code Review" width={192} height={24} />,
    );
    const el = container.firstElementChild as HTMLElement;
    expect(el.getAttribute('role')).toBe('img');
    expect(el.getAttribute('aria-label')).toBe('Open Code Review');
  });

  it('hides decorative icons from the accessibility tree', () => {
    const { container } = render(
      <ThemedMonochromeIcon src={fakeSvgUrl} alt="" width={18} height={18} />,
    );
    const el = container.firstElementChild as HTMLElement;
    expect(el.getAttribute('aria-hidden')).toBe('true');
    expect(el.getAttribute('aria-label')).toBeNull();
  });

  it('never renders the raw asset as an <img>', () => {
    const { container } = render(
      <ThemedMonochromeIcon src={fakeSvgUrl} alt="Open Code Review" width={192} height={24} />,
    );
    expect(container.querySelector('img')).toBeNull();
  });

  it('lets callers extend the treatment without overriding the paint token', () => {
    const { container } = render(
      <ThemedMonochromeIcon
        src={fakeSvgUrl}
        alt=""
        width={18}
        height={18}
        style={{ display: 'block', backgroundColor: '#ff0000' } as React.CSSProperties}
      />,
    );
    const el = container.firstElementChild as HTMLElement;
    // The spread order keeps the theme token authoritative over caller styles.
    expect(el.style.backgroundColor).toBe('var(--text-strong)');
    expect(el.style.display).toBe('block');
  });
});
