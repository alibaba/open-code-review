// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

'use strict';

const path = require('path');
const { extractDocSlugs, extractBlogSlugs } = require('./extract-slugs.cjs');

const SITE = 'https://open-codereview.ai';

const TOP_LEVEL_ROUTES = ['features', 'benchmark', 'quickstart'];
const DOC_INDEX = 'docs';
const BLOG_INDEX = 'blog';
const STATIC_ROUTES = [...TOP_LEVEL_ROUTES, DOC_INDEX, BLOG_INDEX];

const pagesRoot = path.resolve(__dirname, '..');

function docSlugs() {
  return extractDocSlugs(pagesRoot);
}

function blogSlugs() {
  return extractBlogSlugs(pagesRoot);
}

function sitePaths() {
  const paths = ['/', ...TOP_LEVEL_ROUTES.map(r => `/${r}`), `/${DOC_INDEX}`];
  for (const slug of docSlugs()) paths.push(`/${DOC_INDEX}/${slug}`);
  paths.push(`/${BLOG_INDEX}`);
  for (const slug of blogSlugs()) paths.push(`/${BLOG_INDEX}/${slug}`);
  return paths;
}

module.exports = { SITE, STATIC_ROUTES, docSlugs, blogSlugs, sitePaths };
