// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

'use strict';

const fs = require('fs');
const path = require('path');

function extractUnionMembers(sourcePath, typeName) {
  const source = fs.readFileSync(sourcePath, 'utf8');
  const match = source.match(new RegExp(`export type ${typeName} =\\n([\\s\\S]*?);`));
  if (!match) {
    throw new Error(`[extract-slugs] no \`export type ${typeName} =\` union found in ${sourcePath}`);
  }
  const members = [];
  for (const line of match[1].split('\n')) {
    const m = line.match(/^\s*\|\s*'([^']+)'\s*$/);
    if (m) members.push(m[1]);
  }
  if (members.length === 0) {
    throw new Error(`[extract-slugs] \`${typeName}\` union has no parseable members in ${sourcePath}`);
  }
  return members;
}

/** All doc slugs — the route segment of every /docs/<slug> page. */
function extractDocSlugs(pagesRoot) {
  return extractUnionMembers(path.join(pagesRoot, 'src/content/docs/index.ts'), 'DocSlug');
}

/** All blog slugs — the route segment of every /blog/<slug> page. */
function extractBlogSlugs(pagesRoot) {
  return extractUnionMembers(path.join(pagesRoot, 'src/content/blog/index.ts'), 'BlogSlug');
}

module.exports = { extractUnionMembers, extractDocSlugs, extractBlogSlugs };
