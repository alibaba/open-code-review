// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

'use strict';

const fs = require('fs');
const path = require('path');
const { buildSitemap } = require('./generate-sitemap.cjs');
const { SITE, sitePaths } = require('./site-config.cjs');

describe('generate-sitemap', () => {
  it('emits one <url> per site path, rooted at SITE', () => {
    const xml = buildSitemap();
    const locs = [...xml.matchAll(/<loc>([^<]+)<\/loc>/g)].map(m => m[1]);
    expect(locs).toEqual(sitePaths().map(p => `${SITE}${p}`));
  });

  it('matches the committed public/sitemap.xml', () => {
    const committed = fs.readFileSync(path.resolve(__dirname, '../public/sitemap.xml'), 'utf8');
    expect(committed).toBe(buildSitemap());
  });
});
