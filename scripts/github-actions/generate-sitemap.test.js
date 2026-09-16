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
  parseStaticPaths,
  parseSlugUnion,
  collectPaths,
  defaultBaseUrl,
  escapeXml,
  absoluteUrl,
  buildSitemap,
  writeEntryPoints,
  parseArgs,
  main,
} = require(path.join(__dirname, "generate-sitemap.js"));

const REPO_ROOT = path.join(__dirname, "..", "..");

function fixtureRepo({
  docs = "| 'quickstart'\n  | 'faq';\n",
  blog = "| 'hello';\n",
  cname = null,
} = {}) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "generate-sitemap-"));
  fs.mkdirSync(path.join(root, "pages/src/content/docs"), { recursive: true });
  fs.mkdirSync(path.join(root, "pages/src/content/blog"), { recursive: true });
  fs.writeFileSync(
    path.join(root, "pages/src/App.tsx"),
    [
      "<Routes location={displayLocation}>",
      '  <Route path="/" element={<LandingPage />} />',
      '  <Route path="/features" element={<LandingPage />} />',
      '  <Route path="/docs/:slug" element={<DocsPage />} />',
      '  <Route path="*" element={<NotFoundPage />} />',
      "</Routes>",
    ].join("\n")
  );
  fs.writeFileSync(
    path.join(root, "pages/src/content/docs/index.ts"),
    `export type DocSlug =\n  ${docs}`
  );
  fs.writeFileSync(
    path.join(root, "pages/src/content/blog/index.ts"),
    `export type BlogSlug =\n  ${blog}`
  );
  if (cname) {
    fs.mkdirSync(path.join(root, "pages/public"), { recursive: true });
    fs.writeFileSync(path.join(root, "pages/public/CNAME"), `${cname}\n`);
  }
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

function withTempDir(name, fn) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), name));
  try {
    return fn(dir);
  } finally {
    fs.rmSync(dir, { recursive: true, force: true });
  }
}

function testParseStaticPaths() {
  const source = [
    "<Routes location={displayLocation}>",
    '  <Route path="/" element={<LandingPage />} />',
    '  <Route path="/docs" element={<DocsPage />} />',
    '  <Route path="/docs/:slug" element={<DocsPage />} />',
    '  <Route path="*" element={<NotFoundPage />} />',
    "</Routes>",
  ].join("\n");
  assert.deepStrictEqual(parseStaticPaths(source), ["/", "/docs"]);
}

function testParseStaticPathsRejectsRouteListWithoutRoot() {
  assert.throws(
    () => parseStaticPaths('<Route path="/docs/:slug" element={<DocsPage />} />'),
    /no routes found in pages\/src\/App\.tsx/
  );
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

function testCollectPathsCombinesAppRoutesAndContent() {
  withFixtureRepo({}, (root) => {
    assert.deepStrictEqual(collectPaths(root), [
      "/",
      "/features",
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
  const appRoutes = parseStaticPaths(
    fs.readFileSync(path.join(REPO_ROOT, "pages/src/App.tsx"), "utf8")
  );
  for (const route of appRoutes) {
    assert.ok(paths.includes(route), `static route ${route} missing from the sitemap`);
  }
  assert.ok(paths.includes("/blog"), "blog index route missing");
}

function testDefaultBaseUrlReadsCname() {
  withFixtureRepo({ cname: "example.com" }, (root) => {
    assert.strictEqual(defaultBaseUrl(root), "https://example.com");
  });
}

function testDefaultBaseUrlFallsBackWithoutCname() {
  withFixtureRepo({}, (root) => {
    assert.strictEqual(defaultBaseUrl(root), DEFAULT_BASE_URL);
  });
}

function testRealCnameAndRobotsTxtAgree() {
  const baseUrl = defaultBaseUrl(REPO_ROOT);
  const robots = fs.readFileSync(path.join(REPO_ROOT, "pages/public/robots.txt"), "utf8");
  assert.ok(robots.includes(`Sitemap: ${baseUrl}/sitemap.xml`));
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

function testWriteEntryPointsCopiesShellToNestedPaths() {
  withTempDir("generate-sitemap-site-", (siteDir) => {
    fs.writeFileSync(path.join(siteDir, "index.html"), "<html>shell</html>");
    const written = writeEntryPoints(siteDir, ["/", "/features", "/docs/quickstart"]);
    assert.deepStrictEqual(written, [
      path.join(siteDir, "features", "index.html"),
      path.join(siteDir, "docs", "quickstart", "index.html"),
    ]);
    assert.strictEqual(
      fs.readFileSync(path.join(siteDir, "features/index.html"), "utf8"),
      "<html>shell</html>"
    );
    assert.strictEqual(
      fs.readFileSync(path.join(siteDir, "docs/quickstart/index.html"), "utf8"),
      "<html>shell</html>"
    );
  });
}

function testParseArgs() {
  assert.deepStrictEqual(parseArgs([]), {});
  assert.deepStrictEqual(
    parseArgs([
      "--out",
      "sitemap.xml",
      "--base-url",
      "https://x.dev",
      "--repo-root",
      "/repo",
      "--site-dir",
      "_site",
    ]),
    { out: "sitemap.xml", baseUrl: "https://x.dev", repoRoot: "/repo", siteDir: "_site" }
  );
}

function testParseArgsRejectsUnknownFlagsAndMissingValues() {
  assert.throws(() => parseArgs(["--nope"]), /unknown argument "--nope"/);
  assert.throws(() => parseArgs(["--out"]), /--out requires a value/);
  assert.throws(() => parseArgs(["--base-url"]), /--base-url requires a value/);
  assert.throws(() => parseArgs(["--repo-root"]), /--repo-root requires a value/);
  assert.throws(() => parseArgs(["--site-dir"]), /--site-dir requires a value/);
}

function testParseArgsRejectsFlagUsedAsValue() {
  assert.throws(
    () => parseArgs(["--base-url", "--out", "sitemap.xml"]),
    /--base-url requires a value/
  );
  assert.throws(() => parseArgs(["--out", "--repo-root", "/repo"]), /--out requires a value/);
}

function testMainWritesSitemapToRequestedPath() {
  withFixtureRepo({}, (root) => {
    withTempDir("generate-sitemap-out-", (outDir) => {
      const out = path.join(outDir, "nested", "sitemap.xml");
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
    });
  });
}

function testMainWritesEntryPointsWhenSiteDirIsGiven() {
  withFixtureRepo({}, (root) => {
    withTempDir("generate-sitemap-site-", (siteDir) => {
      fs.writeFileSync(path.join(siteDir, "index.html"), "<html>shell</html>");
      main(
        [
          "--repo-root",
          root,
          "--out",
          path.join(siteDir, "sitemap.xml"),
          "--site-dir",
          siteDir,
          "--base-url",
          "https://example.com",
        ],
        {}
      );
      assert.ok(fs.existsSync(path.join(siteDir, "features/index.html")));
      assert.ok(fs.existsSync(path.join(siteDir, "docs/quickstart/index.html")));
      assert.ok(fs.existsSync(path.join(siteDir, "blog/hello/index.html")));
      assert.ok(!fs.existsSync(path.join(siteDir, "index.html/index.html")));
    });
  });
}

function testMainReadsEnvironmentOverrides() {
  withFixtureRepo({ cname: "example.org" }, (root) => {
    withTempDir("generate-sitemap-env-", (outDir) => {
      const out = path.join(outDir, "sitemap.xml");
      main([], { OCR_REPO_ROOT: root, SITEMAP_OUT: out });
      const xml = fs.readFileSync(out, "utf8");
      assert.ok(xml.includes("<loc>https://example.org/</loc>"));
      assert.ok(xml.includes("<loc>https://example.org/docs/faq</loc>"));
    });
  });
}

function testMainSiteUrlOverrideBeatsCname() {
  withFixtureRepo({ cname: "example.org" }, (root) => {
    withTempDir("generate-sitemap-env-", (outDir) => {
      const out = path.join(outDir, "sitemap.xml");
      main(["--repo-root", root], { SITEMAP_OUT: out, SITE_URL: "https://example.net/base/" });
      const xml = fs.readFileSync(out, "utf8");
      assert.ok(xml.includes("<loc>https://example.net/base/</loc>"));
      assert.ok(xml.includes("<loc>https://example.net/base/docs/faq</loc>"));
    });
  });
}

function runAll() {
  testParseStaticPaths();
  testParseStaticPathsRejectsRouteListWithoutRoot();
  testParseSlugUnionSingleQuotes();
  testParseSlugUnionDoubleQuotesAndCompactForm();
  testParseSlugUnionMissingType();
  testParseSlugUnionWithoutSlugs();
  testCollectPathsCombinesAppRoutesAndContent();
  testCollectPathsFromRealRepo();
  testDefaultBaseUrlReadsCname();
  testDefaultBaseUrlFallsBackWithoutCname();
  testRealCnameAndRobotsTxtAgree();
  testAbsoluteUrl();
  testEscapeXml();
  testBuildSitemap();
  testBuildSitemapDefaultsToProductionHostAndDedupes();
  testBuildSitemapEscapesQueryAndAmpersands();
  testWriteEntryPointsCopiesShellToNestedPaths();
  testParseArgs();
  testParseArgsRejectsUnknownFlagsAndMissingValues();
  testParseArgsRejectsFlagUsedAsValue();
  testMainWritesSitemapToRequestedPath();
  testMainWritesEntryPointsWhenSiteDirIsGiven();
  testMainReadsEnvironmentOverrides();
  testMainSiteUrlOverrideBeatsCname();
  console.log("All generate-sitemap tests passed.");
}

try {
  runAll();
} catch (err) {
  console.error(err);
  process.exit(1);
}
