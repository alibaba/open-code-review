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
const html = readFileSync(
  join(dirname(fileURLToPath(import.meta.url)), '..', 'index.html'),
  'utf8'
);

const inlineStyle = html.match(/<style>([\s\S]*?)<\/style>/)?.[1] ?? '';

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
      name: 'matches the runtime foreground colour so nothing shifts',
      assert: () => expect(inlineStyle).toMatch(/color:\s*#ffffff/),
    },
    {
      name: 'declares a dark color-scheme for the pre-stylesheet paint',
      assert: () =>
        expect(html).toMatch(/<meta\s+name="color-scheme"\s+content="dark"\s*\/?>/),
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
        const style = html.indexOf('<style>');
        const link = html.search(/<link[^>]*rel="stylesheet"/);
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
