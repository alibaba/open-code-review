// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import React from 'react';

export interface ThemedMonochromeIconProps {
  /** The monochrome SVG asset, imported as a URL (Vite asset import). */
  src: string;
  /** Accessible name; pass an empty string only for decorative icons. */
  alt: string;
  width: number | string;
  height: number | string;
  style?: React.CSSProperties;
}

// The monochrome brand/social assets ship with literal #FFFFFF fills baked
// into the SVG. Rendered through <img>, those pixels never follow the theme,
// so the logo disappears over the white documentation surfaces the light
// palette introduces (#1383). This component recolors by masking instead of
// drawing: the SVG becomes the mask shape and the paint comes from
// var(--text-strong), the same token the surrounding nav/footer text uses.
//
// The treatment is deliberately scoped to this component. Multicolor and
// provider assets keep their <img> rendering, and the terminal/install
// surfaces stay dark on purpose — never invert images globally.
export const ThemedMonochromeIcon: React.FC<ThemedMonochromeIconProps> = ({
  src,
  alt,
  width,
  height,
  style,
}) => {
  const maskStyle: React.CSSProperties = {
    ...style,
    width,
    height,
    display: style?.display ?? 'inline-block',
    // Re-asserted after the spread: a literal color here is exactly how the
    // logo disappears over the light documentation surfaces (#1383).
    backgroundColor: 'var(--text-strong)',
    maskImage: `url(${src})`,
    WebkitMaskImage: `url(${src})`,
    maskSize: 'contain',
    WebkitMaskSize: 'contain',
    maskRepeat: 'no-repeat',
    WebkitMaskRepeat: 'no-repeat',
    maskPosition: 'center',
    WebkitMaskPosition: 'center',
  };
  if (alt === '') {
    // Decorative: hidden from the accessibility tree, mirroring alt="".
    return <span aria-hidden="true" style={maskStyle} />;
  }
  return <span role="img" aria-label={alt} style={maskStyle} />;
};
