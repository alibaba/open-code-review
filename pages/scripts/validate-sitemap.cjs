// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

'use strict';

const fs = require('fs');
const path = require('path');
const { XMLValidator, XMLParser } = require('fast-xml-parser');

// The site root every <loc> must live under — kept in sync with
// pages/public/sitemap.xml and the routes in webpack.config.cjs.
const SITE = 'https://open-codereview.ai';

function validateSitemap(xml) {
  const wellFormed = XMLValidator.validate(xml);
  if (wellFormed !== true) {
    return { ok: false, error: `malformed XML: ${wellFormed.err.msg} (line ${wellFormed.err.line})` };
  }
  const urlset = new XMLParser().parse(xml).urlset;
  const urls = urlset && urlset.url ? (Array.isArray(urlset.url) ? urlset.url : [urlset.url]) : [];
  if (urls.length === 0) {
    return { ok: false, error: '<urlset> must contain at least one <url>' };
  }
  for (const url of urls) {
    const loc = url.loc;
    if (typeof loc !== 'string' || !(loc === SITE || loc.startsWith(`${SITE}/`))) {
      return { ok: false, error: `<loc> must be a ${SITE} URL, got: ${JSON.stringify(loc)}` };
    }
  }
  return { ok: true, count: urls.length };
}

if (require.main === module) {
  const sitemapPath = process.argv[2]
    ? path.resolve(process.argv[2])
    : path.resolve(__dirname, '../public/sitemap.xml');
  const result = validateSitemap(fs.readFileSync(sitemapPath, 'utf8'));
  if (!result.ok) {
    console.error(`[validate-sitemap] ${result.error} in ${sitemapPath}`);
    process.exit(1);
  }
  console.log(`[validate-sitemap] OK: ${result.count} URLs in ${sitemapPath}`);
}

module.exports = { validateSitemap, SITE };
