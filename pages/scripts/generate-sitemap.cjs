// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

'use strict';

const fs = require('fs');
const path = require('path');
const { SITE, sitePaths } = require('./site-config.cjs');

function buildSitemap() {
  const urls = sitePaths()
    .map(p => `  <url>\n    <loc>${SITE}${p}</loc>\n  </url>`)
    .join('\n');
  return `<?xml version="1.0" encoding="UTF-8"?>\n<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">\n${urls}\n</urlset>\n`;
}

if (require.main === module) {
  const out = path.resolve(__dirname, '../public/sitemap.xml');
  fs.writeFileSync(out, buildSitemap());
  console.log(`[generate-sitemap] wrote ${sitePaths().length} URLs to ${out}`);
}

module.exports = { buildSitemap };
