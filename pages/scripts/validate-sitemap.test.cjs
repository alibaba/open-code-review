// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

'use strict';

const { validateSitemap, SITE } = require('./validate-sitemap.cjs');

const VALID = `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>${SITE}/</loc></url>
  <url><loc>${SITE}/docs/quickstart</loc></url>
</urlset>`;

describe('validate-sitemap', () => {
  it('accepts a well-formed sitemap with site URLs', () => {
    expect(validateSitemap(VALID)).toEqual({ ok: true, count: 2 });
  });

  it('rejects malformed XML', () => {
    const result = validateSitemap('<urlset><url><loc>broken');
    expect(result.ok).toBe(false);
    expect(result.error).toMatch(/malformed XML/);
  });

  it('rejects a missing <urlset> root', () => {
    const result = validateSitemap('<?xml version="1.0"?><foo/>');
    expect(result.ok).toBe(false);
    expect(result.error).toMatch(/at least one/);
  });

  it('rejects an empty <urlset>', () => {
    const result = validateSitemap('<?xml version="1.0"?><urlset></urlset>');
    expect(result.ok).toBe(false);
    expect(result.error).toMatch(/at least one/);
  });

  it('rejects <loc> URLs outside the site', () => {
    const bad = VALID.replace(`${SITE}/docs/quickstart`, 'https://evil.example/x');
    const result = validateSitemap(bad);
    expect(result.ok).toBe(false);
    expect(result.error).toMatch(SITE);
  });
});
