// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

'use strict';

const fs = require('fs');
const os = require('os');
const path = require('path');
const { extractUnionMembers, extractDocSlugs, extractBlogSlugs } = require('./extract-slugs.cjs');

const PAGES_ROOT = path.resolve(__dirname, '..');

describe('extract-slugs', () => {
  it('parses every DocSlug member from the registry union', () => {
    expect(extractDocSlugs(PAGES_ROOT)).toEqual([
      'quickstart',
      'installation',
      'configuration',
      'cli-reference',
      'review-rules',
      'architecture',
      'tools',
      'mcp',
      'viewer',
      'telemetry',
      'agent-skill',
      'claude-code',
      'cicd',
      'delegate',
      'contributing',
      'faq',
    ]);
  });

  it('parses every BlogSlug member from the registry union', () => {
    expect(extractBlogSlugs(PAGES_ROOT)).toEqual([
      'introducing-ocr-blog',
      'oss-two-month-retrospective',
    ]);
  });

  it('throws when the union is reformatted into a shape it cannot parse', () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'extract-slugs-'));
    try {
      // A one-line union (the likeliest prettier reformat) must fail loudly
      // instead of silently yielding no routes.
      const oneLine = path.join(dir, 'one-line.ts');
      fs.writeFileSync(oneLine, "export type DocSlug = 'a' | 'b';\n");
      expect(() => extractUnionMembers(oneLine, 'DocSlug')).toThrow(/no `export type DocSlug =` union/);

      // A first member without the leading `|` is also unparseable.
      const noPipe = path.join(dir, 'no-pipe.ts');
      fs.writeFileSync(noPipe, "export type DocSlug =\n  'quickstart';\n");
      expect(() => extractUnionMembers(noPipe, 'DocSlug')).toThrow(/no parseable members/);
    } finally {
      fs.rmSync(dir, { recursive: true, force: true });
    }
  });
});
