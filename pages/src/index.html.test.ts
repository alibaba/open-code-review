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
// tag in the document. Split on comment boundaries and keep only non-comment
// segments (avoids CodeQL's incomplete-multi-character-sanitization rule).
const markup = html.split(/<!--[\s\S]*?-->/).join('');

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

/** --bg-page / --text-strong out of a theme block of index.css (":root" or
 *  ":root[data-theme='light']"). The body rule itself uses var(), so the sync
 *  check compares the values the variables resolve to. */
function themeTokens(css: string, block: RegExp): { bg?: string; fg?: string } {
  const body = css.match(block)?.[1] ?? '';
  return {
    bg: body.match(/--bg-page:\s*(#[0-9a-fA-F]{3,8})/)?.[1]?.toLowerCase(),
    fg: body.match(/--text-strong:\s*(#[0-9a-fA-F]{3,8})/)?.[1]?.toLowerCase(),
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
      // The body rule in index.css uses var(--bg-page)/var(--text-strong), so the
      // sync is against the values those tokens resolve to in each theme.
      name: 'keeps the inline colours in sync with index.css',
      assert: () => {
        const dark = themeTokens(indexCss, /:root\s*{([^}]*)}/);
        expect(dark.bg).toBeDefined();
        expect(dark.fg).toBeDefined();
        expect(bodyColors(inlineStyle)).toEqual(dark);
      },
    },
    {
      name: 'paints the stored light theme on html and body',
      assert: () => {
        const light = inlineStyle.match(
          /html\[data-theme='light'\],\s*html\[data-theme='light'\]\s*body\s*{([^}]*)}/
        )?.[1] ?? '';
        expect(light).not.toBe('');
        const cssLight = themeTokens(indexCss, /:root\[data-theme='light'\]\s*{([^}]*)}/);
        expect(cssLight.bg).toBeDefined();
        expect(cssLight.fg).toBeDefined();
        expect(light.match(/background-color:\s*(#[0-9a-fA-F]{3,8})/)?.[1]?.toLowerCase()).toBe(cssLight.bg);
        expect(light.match(/(?<!-)\bcolor:\s*(#[0-9a-fA-F]{3,8})/)?.[1]?.toLowerCase()).toBe(cssLight.fg);
      },
    },
    {
      // The theme must be known before the critical CSS paints, or a stored
      // light theme flashes dark for a frame. The bootstrap script has to run
      // ahead of the <style> block and only ever sets a valid theme value.
      // The script body is sliced with indexOf rather than a script-tag regex
      // (a regex here trips CodeQL's HTML-sanitization checks).
      name: 'sets the stored theme before first paint',
      assert: () => {
        const open = headBeforeNoscript.indexOf('<script>');
        const close = headBeforeNoscript.indexOf('</script>');
        expect(open).toBeGreaterThanOrEqual(0);
        const script = open >= 0 && close > open ? headBeforeNoscript.slice(open, close) : '';
        expect(script).toContain("localStorage.getItem('ocr-theme')");
        expect(script).toContain('document.documentElement.dataset.theme');
        expect(markup.indexOf('<script>')).toBeLessThan(markup.indexOf('<style>'));
      },
    },
    {
      // With no stored choice the OS preference must decide before first
      // paint too, or a first visit by a light-mode user paints dark and
      // swaps only after React mounts — the same flash the stored-choice
      // path guards against, one frame later.
      name: 'consults the OS color scheme when no theme is stored',
      assert: () => {
        const open = headBeforeNoscript.indexOf('<script>');
        const close = headBeforeNoscript.indexOf('</script>');
        expect(open).toBeGreaterThanOrEqual(0);
        const script = open >= 0 && close > open ? headBeforeNoscript.slice(open, close) : '';
        expect(script).toContain('matchMedia');
        expect(script).toContain('prefers-color-scheme: light');
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
