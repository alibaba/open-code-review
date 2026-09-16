// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

"use strict";

const fs = require("fs");
const path = require("path");

const DEFAULT_BASE_URL = "https://open-codereview.ai";

const STATIC_PATHS = [
  "/",
  "/features",
  "/benchmark",
  "/quickstart",
  "/docs",
  "/blog",
];

const CONTENT_ROUTES = [
  {
    file: "pages/src/content/docs/index.ts",
    typeName: "DocSlug",
    urlPrefix: "/docs/",
  },
  {
    file: "pages/src/content/blog/index.ts",
    typeName: "BlogSlug",
    urlPrefix: "/blog/",
  },
];

function parseSlugUnion(source, typeName) {
  const match = new RegExp(`export type ${typeName}\\s*=([^;]*);`).exec(source);
  if (!match) {
    throw new Error(`generate-sitemap: no "export type ${typeName}" found`);
  }
  const slugs = Array.from(match[1].matchAll(/'([^']+)'|"([^"]+)"/g), (m) => m[1] || m[2]);
  if (slugs.length === 0) {
    throw new Error(`generate-sitemap: "export type ${typeName}" lists no slugs`);
  }
  return slugs;
}

function collectPaths(repoRoot) {
  const paths = STATIC_PATHS.slice();
  for (const route of CONTENT_ROUTES) {
    const source = fs.readFileSync(path.join(repoRoot, route.file), "utf8");
    for (const slug of parseSlugUnion(source, route.typeName)) {
      paths.push(route.urlPrefix + slug);
    }
  }
  return paths;
}

function escapeXml(text) {
  return String(text)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&apos;");
}

function absoluteUrl(baseUrl, urlPath) {
  const base = String(baseUrl).replace(/\/+$/, "");
  return urlPath === "/" ? `${base}/` : `${base}${urlPath}`;
}

function buildSitemap(paths, { baseUrl = DEFAULT_BASE_URL } = {}) {
  const entries = Array.from(new Set(paths), (urlPath) => {
    const loc = escapeXml(absoluteUrl(baseUrl, urlPath));
    return `  <url>\n    <loc>${loc}</loc>\n  </url>`;
  });
  return (
    '<?xml version="1.0" encoding="UTF-8"?>\n' +
    '<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">\n' +
    `${entries.join("\n")}\n` +
    "</urlset>\n"
  );
}

function parseArgs(argv) {
  const options = {};
  for (let i = 0; i < argv.length; i++) {
    const flag = argv[i];
    const value = argv[i + 1];
    if (flag !== "--out" && flag !== "--base-url" && flag !== "--repo-root") {
      throw new Error(`generate-sitemap: unknown argument "${flag}"`);
    }
    if (value === undefined) {
      throw new Error(`generate-sitemap: ${flag} requires a value`);
    }
    i++;
    if (flag === "--out") options.out = value;
    else if (flag === "--base-url") options.baseUrl = value;
    else options.repoRoot = value;
  }
  return options;
}

function main(argv = process.argv.slice(2), env = process.env) {
  const options = parseArgs(argv);
  const repoRoot = options.repoRoot || env.OCR_REPO_ROOT || process.cwd();
  const baseUrl = options.baseUrl || env.SITE_URL || DEFAULT_BASE_URL;
  const out = options.out || env.SITEMAP_OUT || path.join("_site", "sitemap.xml");

  const paths = collectPaths(repoRoot);
  fs.mkdirSync(path.dirname(out), { recursive: true });
  fs.writeFileSync(out, buildSitemap(paths, { baseUrl }));
  console.log(`Wrote sitemap with ${new Set(paths).size} URLs to ${out}`);
  return 0;
}

if (require.main === module) {
  try {
    process.exit(main());
  } catch (err) {
    console.error(err.message);
    process.exit(1);
  }
}

module.exports = {
  DEFAULT_BASE_URL,
  STATIC_PATHS,
  CONTENT_ROUTES,
  parseSlugUnion,
  collectPaths,
  escapeXml,
  absoluteUrl,
  buildSitemap,
  parseArgs,
  main,
};
