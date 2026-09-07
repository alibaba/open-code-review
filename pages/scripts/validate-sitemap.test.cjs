// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

'use strict';

const { validateSitemap, SITE } = require('./validate-sitemap.cjs');
const { buildSitemap } = require('./generate-sitemap.cjs');
const { sitePaths } = require('./site-config.cjs');

describe('validate-sitemap', () => {
  it('accepts the generated sitemap', () => {
    expect(validateSitemap(buildSitemap())).toEqual({ ok: true, count: sitePaths().length });
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
    const bad = buildSitemap().replace(`${SITE}/docs/quickstart`, 'https://evil.example/x');
    const result = validateSitemap(bad);
    expect(result.ok).toBe(false);
    expect(result.error).toMatch(SITE);
  });

  it('rejects a sitemap missing a site route', () => {
    const missing = buildSitemap().replace(/  <url>\n    <loc>https:\/\/open-codereview\.ai\/docs\/faq<\/loc>\n  <\/url>\n/, '');
    const result = validateSitemap(missing);
    expect(result.ok).toBe(false);
    expect(result.error).toMatch(/missing \/docs\/faq/);
  });

  it('rejects a sitemap listing an unknown route', () => {
    const extra = buildSitemap().replace(
      '</urlset>',
      '  <url>\n    <loc>https://open-codereview.ai/docs/does-not-exist</loc>\n  </url>\n</urlset>'
    );
    const result = validateSitemap(extra);
    expect(result.ok).toBe(false);
    expect(result.error).toMatch(/unexpected \/docs\/does-not-exist/);
  });
});
