// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

"use strict";

const fs = require("fs");
const path = require("path");

const DEFAULT_BASE_URL = "https://open-codereview.ai";
const APP_ROUTES_FILE = "pages/src/App.tsx";
const CNAME_FILE = "pages/public/CNAME";

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

function parseStaticPaths(source) {
  const paths = [];
  for (const match of source.matchAll(/<Route\s+path="([^"]+)"/g)) {
    const urlPath = match[1];
    if (urlPath === "*" || urlPath.includes(":")) continue;
    paths.push(urlPath);
  }
  if (!paths.includes("/")) {
    throw new Error(`generate-sitemap: no routes found in ${APP_ROUTES_FILE}`);
  }
  return paths;
}

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
  const appRoutes = fs.readFileSync(path.join(repoRoot, APP_ROUTES_FILE), "utf8");
  const paths = parseStaticPaths(appRoutes);
  for (const route of CONTENT_ROUTES) {
    const source = fs.readFileSync(path.join(repoRoot, route.file), "utf8");
    for (const slug of parseSlugUnion(source, route.typeName)) {
      paths.push(route.urlPrefix + slug);
    }
  }
  return paths;
}

function defaultBaseUrl(repoRoot) {
  let cname = "";
  try {
    cname = fs.readFileSync(path.join(repoRoot, CNAME_FILE), "utf8").trim();
  } catch {
    return DEFAULT_BASE_URL;
  }
  return cname ? `https://${cname}` : DEFAULT_BASE_URL;
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

function writeEntryPoints(siteDir, paths) {
  const html = fs.readFileSync(path.join(siteDir, "index.html"), "utf8");
  const written = [];
  for (const urlPath of paths) {
    if (urlPath === "/") continue;
    const target = path.join(siteDir, urlPath, "index.html");
    fs.mkdirSync(path.dirname(target), { recursive: true });
    fs.writeFileSync(target, html);
    written.push(target);
  }
  return written;
}

function parseArgs(argv) {
  const options = {};
  const valued = ["--out", "--base-url", "--repo-root", "--site-dir"];
  for (let i = 0; i < argv.length; i++) {
    const flag = argv[i];
    if (!valued.includes(flag)) {
      throw new Error(`generate-sitemap: unknown argument "${flag}"`);
    }
    const value = argv[++i];
    if (value === undefined || value.startsWith("--")) {
      throw new Error(`generate-sitemap: ${flag} requires a value`);
    }
    if (flag === "--out") options.out = value;
    else if (flag === "--base-url") options.baseUrl = value;
    else if (flag === "--repo-root") options.repoRoot = value;
    else options.siteDir = value;
  }
  return options;
}

function main(argv = process.argv.slice(2), env = process.env) {
  const options = parseArgs(argv);
  const repoRoot = options.repoRoot || env.OCR_REPO_ROOT || process.cwd();
  const baseUrl = options.baseUrl || env.SITE_URL || defaultBaseUrl(repoRoot);
  const out = options.out || env.SITEMAP_OUT || path.join("_site", "sitemap.xml");
  const siteDir = options.siteDir || env.SITE_DIR || "";

  const paths = Array.from(new Set(collectPaths(repoRoot)));
  fs.mkdirSync(path.dirname(out), { recursive: true });
  fs.writeFileSync(out, buildSitemap(paths, { baseUrl }));
  console.log(`Wrote sitemap with ${paths.length} URLs to ${out}`);
  if (siteDir) {
    const entries = writeEntryPoints(siteDir, paths);
    console.log(`Wrote ${entries.length} static entry points under ${siteDir}`);
  }
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
  APP_ROUTES_FILE,
  CNAME_FILE,
  CONTENT_ROUTES,
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
};
