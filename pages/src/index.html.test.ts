// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, it, expect } from 'vitest';

// The critical-CSS block in index.html is the whole fix for the initial white
// flash: every rule in src/styles/index.css ships inside the deferred JS chunks,
// so anything not inlined here has no effect until the last chunk executes.
// Nothing else in the build depends on the block, which makes it easy to delete
// by accident during an unrelated <head> edit — hence these assertions.
// Resolved from this file rather than process.cwd() so the test does not depend
// on where vitest was invoked from. Deliberately not `new URL('../index.html',
// import.meta.url)`: Vite rewrites that exact pattern into an asset URL, which
// is no longer a file:// path by the time it reaches fileURLToPath.
const here = dirname(fileURLToPath(import.meta.url));
const html = readFileSync(join(here, '..', 'index.html'), 'utf8');
const indexCss = readFileSync(join(here, 'styles', 'index.css'), 'utf8');

// Comments explain this block and name the tags they discuss, so structural
// assertions must not see them — otherwise a tag mentioned in prose reads as a
// tag in the document.
const markup = html.replace(/<!--[\s\S]*?-->/g, '').replace(/<!--/g, '');

const inlineStyle = markup.match(/<style>([\s\S]*?)<\/style>/)?.[1] ?? '';

// Everything before <noscript>: the copies inside it are deliberately
// render-blocking and would otherwise look like the bug this guards against.
const headBeforeNoscript = markup.slice(0, markup.indexOf('<noscript>'));

/** background-color / color out of a `body { ... }` block. */
function bodyColors(css: string): { bg?: string; fg?: string } {
  const block = css.match(/(^|\s)body\s*{([^}]*)}/)?.[2] ?? '';
  return {
    bg: block.match(/background-color:\s*(#[0-9a-fA-F]{3,8})/)?.[1]?.toLowerCase(),
    // (?<!-) so `background-color` does not match as `color`.
    fg: block.match(/(?<!-)\bcolor:\s*(#[0-9a-fA-F]{3,8})/)?.[1]?.toLowerCase(),
  };
}

describe('index.html critical CSS', () => {
  const cases: { name: string; assert: () => void }[] = [
    {
      name: 'inlines a <style> block',
      assert: () => expect(inlineStyle.trim()).not.toBe(''),
    },
    {
      name: 'paints the dark background on html and body',
      // html too, not just body: it is the element the browser has already
      // resolved when the first paint happens.
      assert: () => expect(inlineStyle).toMatch(/html,\s*body\s*{[^}]*background-color:\s*#000000/),
    },
    {
      // The colours are duplicated from index.css by necessity — index.css is not
      // available at first paint. Asserting they still match is the only thing
      // stopping a later edit to index.css from reintroducing a visible shift.
      name: 'keeps the inline colours in sync with index.css',
      assert: () => {
        const css = bodyColors(indexCss);
        expect(css.bg).toBeDefined();
        expect(css.fg).toBeDefined();
        expect(bodyColors(inlineStyle)).toEqual(css);
      },
    },
    {
      // Critical CSS is pointless while the browser is still blocked on two CDN
      // round-trips, so the links must not be render-blocking.
      name: 'loads both CDN stylesheets without blocking first paint',
      assert: () => {
        const links = headBeforeNoscript.match(/<link[^>]*rel="stylesheet"[^>]*>/g) ?? [];
        expect(links).toHaveLength(2);
        for (const link of links) {
          expect(link).toMatch(/media="print"/);
          expect(link).toMatch(/onload="this\.media='all'"/);
        }
      },
    },
    {
      // The onload swap cannot run with scripting off, so without these the
      // no-JS experience would lose fonts and icons entirely.
      name: 'falls back to blocking stylesheets when scripting is off',
      assert: () => {
        const noscript = markup.match(/<noscript>([\s\S]*?)<\/noscript>/)?.[1] ?? '';
        expect(noscript).toContain('fonts.googleapis.com');
        expect(noscript).toContain('font-awesome');
        expect(noscript).not.toMatch(/media="print"/);
      },
    },
    {
      name: 'shows the boot indicator only while #root is empty',
      // :empty is what makes React's first render remove it, with no JS cleanup.
      assert: () => expect(inlineStyle).toMatch(/#root:empty::(after|before)/),
    },
    {
      name: 'stops the indicator animating under prefers-reduced-motion',
      assert: () => {
        expect(inlineStyle).toMatch(/@media\s*\(prefers-reduced-motion:\s*reduce\)/);
        const reduced = inlineStyle.slice(
          inlineStyle.indexOf('prefers-reduced-motion')
        );
        expect(reduced).toMatch(/animation:\s*none/);
      },
    },
    {
      name: 'comes before the first external stylesheet',
      // Critical CSS that loses this race is not critical CSS.
      assert: () => {
        const style = markup.indexOf('<style>');
        const link = headBeforeNoscript.search(/<link[^>]*rel="stylesheet"/);
        expect(style).toBeGreaterThan(-1);
        expect(link).toBeGreaterThan(-1);
        expect(style).toBeLessThan(link);
      },
    },
  ];

  for (const c of cases) {
    it(c.name, c.assert);
  }
});
