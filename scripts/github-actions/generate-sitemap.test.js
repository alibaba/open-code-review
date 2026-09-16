#!/usr/bin/env node

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

"use strict";

const assert = require("assert");
const fs = require("fs");
const os = require("os");
const path = require("path");
const {
  DEFAULT_BASE_URL,
  STATIC_PATHS,
  parseSlugUnion,
  collectPaths,
  escapeXml,
  absoluteUrl,
  buildSitemap,
  parseArgs,
  main,
} = require(path.join(__dirname, "generate-sitemap.js"));

const REPO_ROOT = path.join(__dirname, "..", "..");

function fixtureRepo({ docs = "| 'quickstart'\n  | 'faq';\n", blog = "| 'hello';\n" } = {}) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "generate-sitemap-"));
  fs.mkdirSync(path.join(root, "pages/src/content/docs"), { recursive: true });
  fs.mkdirSync(path.join(root, "pages/src/content/blog"), { recursive: true });
  fs.writeFileSync(
    path.join(root, "pages/src/content/docs/index.ts"),
    `export type DocSlug =\n  ${docs}`
  );
  fs.writeFileSync(
    path.join(root, "pages/src/content/blog/index.ts"),
    `export type BlogSlug =\n  ${blog}`
  );
  return root;
}

function withFixtureRepo(options, fn) {
  const root = fixtureRepo(options);
  try {
    return fn(root);
  } finally {
    fs.rmSync(root, { recursive: true, force: true });
  }
}

function testParseSlugUnionSingleQuotes() {
  const source = [
    "export type DocSlug =",
    "  | 'quickstart'",
    "  | 'cli-reference';",
    "",
    "type LocalizedDocs = Partial<Record<DocSlug, string>>;",
  ].join("\n");
  assert.deepStrictEqual(parseSlugUnion(source, "DocSlug"), [
    "quickstart",
    "cli-reference",
  ]);
}

function testParseSlugUnionDoubleQuotesAndCompactForm() {
  assert.deepStrictEqual(
    parseSlugUnion('export type BlogSlug = "a" | "b";\nconst x = 1;', "BlogSlug"),
    ["a", "b"]
  );
}

function testParseSlugUnionMissingType() {
  assert.throws(
    () => parseSlugUnion("export type Other = 'a';", "DocSlug"),
    /no "export type DocSlug" found/
  );
}

function testParseSlugUnionWithoutSlugs() {
  assert.throws(
    () => parseSlugUnion("export type DocSlug = ;", "DocSlug"),
    /lists no slugs/
  );
}

function testCollectPathsCombinesStaticDocsAndBlog() {
  withFixtureRepo({}, (root) => {
    const paths = collectPaths(root);
    assert.deepStrictEqual(paths.slice(0, STATIC_PATHS.length), STATIC_PATHS);
    assert.deepStrictEqual(paths.slice(STATIC_PATHS.length), [
      "/docs/quickstart",
      "/docs/faq",
      "/blog/hello",
    ]);
  });
}

function testCollectPathsFromRealRepo() {
  const paths = collectPaths(REPO_ROOT);
  assert.ok(paths.includes("/"), "root route missing");
  assert.ok(paths.includes("/docs/quickstart"), "docs route missing");
  assert.ok(paths.includes("/docs/cli-reference"), "docs route missing");
  assert.ok(paths.includes("/blog/introducing-ocr-blog"), "blog route missing");
  const docsCount = paths.filter((p) => p.startsWith("/docs/")).length;
  const blogCount = paths.filter((p) => p.startsWith("/blog/")).length;
  assert.ok(docsCount >= 10, `expected the full docs set, got ${docsCount}`);
  assert.ok(blogCount >= 2, `expected the full blog set, got ${blogCount}`);
}

function testAbsoluteUrl() {
  assert.strictEqual(absoluteUrl("https://example.com", "/"), "https://example.com/");
  assert.strictEqual(
    absoluteUrl("https://example.com/", "/docs/faq"),
    "https://example.com/docs/faq"
  );
  assert.strictEqual(
    absoluteUrl("https://example.com///", "/docs/faq"),
    "https://example.com/docs/faq"
  );
}

function testEscapeXml() {
  assert.strictEqual(escapeXml("a&b<c>d\"e'f"), "a&amp;b&lt;c&gt;d&quot;e&apos;f");
}

function testBuildSitemap() {
  const xml = buildSitemap(["/", "/docs/faq"], { baseUrl: "https://example.com/" });
  assert.strictEqual(
    xml,
    '<?xml version="1.0" encoding="UTF-8"?>\n' +
      '<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">\n' +
      "  <url>\n    <loc>https://example.com/</loc>\n  </url>\n" +
      "  <url>\n    <loc>https://example.com/docs/faq</loc>\n  </url>\n" +
      "</urlset>\n"
  );
}

function testBuildSitemapDefaultsToProductionHostAndDedupes() {
  const xml = buildSitemap(["/docs/faq", "/docs/faq"]);
  assert.strictEqual((xml.match(/<loc>/g) || []).length, 1);
  assert.ok(xml.includes(`<loc>${DEFAULT_BASE_URL}/docs/faq</loc>`));
}

function testBuildSitemapEscapesQueryAndAmpersands() {
  const xml = buildSitemap(["/search?a=1&b=2"], { baseUrl: "https://example.com" });
  assert.ok(xml.includes("<loc>https://example.com/search?a=1&amp;b=2</loc>"));
}

function testParseArgs() {
  assert.deepStrictEqual(parseArgs([]), {});
  assert.deepStrictEqual(
    parseArgs(["--out", "sitemap.xml", "--base-url", "https://x.dev", "--repo-root", "/repo"]),
    { out: "sitemap.xml", baseUrl: "https://x.dev", repoRoot: "/repo" }
  );
}

function testParseArgsRejectsUnusableInput() {
  assert.throws(() => parseArgs(["--nope"]), /unknown argument "--nope"/);
  assert.throws(() => parseArgs(["--out"]), /--out requires a value/);
  assert.throws(() => parseArgs(["--base-url"]), /--base-url requires a value/);
  assert.throws(() => parseArgs(["--repo-root"]), /--repo-root requires a value/);
}

function testMainWritesSitemapToRequestedPath() {
  withFixtureRepo({}, (root) => {
    const outDir = fs.mkdtempSync(path.join(os.tmpdir(), "generate-sitemap-out-"));
    const out = path.join(outDir, "nested", "sitemap.xml");
    try {
      const code = main(
        ["--repo-root", root, "--out", out, "--base-url", "https://example.com"],
        {}
      );
      assert.strictEqual(code, 0);
      const xml = fs.readFileSync(out, "utf8");
      assert.ok(xml.startsWith('<?xml version="1.0" encoding="UTF-8"?>'));
      assert.ok(xml.includes("<loc>https://example.com/docs/quickstart</loc>"));
      assert.ok(xml.includes("<loc>https://example.com/blog/hello</loc>"));
      assert.ok(xml.endsWith("</urlset>\n"));
    } finally {
      fs.rmSync(outDir, { recursive: true, force: true });
    }
  });
}

function testMainReadsEnvironmentOverrides() {
  withFixtureRepo({}, (root) => {
    const outDir = fs.mkdtempSync(path.join(os.tmpdir(), "generate-sitemap-env-"));
    const out = path.join(outDir, "sitemap.xml");
    try {
      main([], {
        OCR_REPO_ROOT: root,
        SITEMAP_OUT: out,
        SITE_URL: "https://example.org/base/",
      });
      const xml = fs.readFileSync(out, "utf8");
      assert.ok(xml.includes("<loc>https://example.org/base/</loc>"));
      assert.ok(xml.includes("<loc>https://example.org/base/docs/faq</loc>"));
    } finally {
      fs.rmSync(outDir, { recursive: true, force: true });
    }
  });
}

function runAll() {
  testParseSlugUnionSingleQuotes();
  testParseSlugUnionDoubleQuotesAndCompactForm();
  testParseSlugUnionMissingType();
  testParseSlugUnionWithoutSlugs();
  testCollectPathsCombinesStaticDocsAndBlog();
  testCollectPathsFromRealRepo();
  testAbsoluteUrl();
  testEscapeXml();
  testBuildSitemap();
  testBuildSitemapDefaultsToProductionHostAndDedupes();
  testBuildSitemapEscapesQueryAndAmpersands();
  testParseArgs();
  testParseArgsRejectsUnusableInput();
  testMainWritesSitemapToRequestedPath();
  testMainReadsEnvironmentOverrides();
  console.log("All generate-sitemap tests passed.");
}

try {
  runAll();
} catch (err) {
  console.error(err);
  process.exit(1);
}
