"use strict";

const path = require("path");
const fs = require("fs");

const IS_WINDOWS = process.platform === "win32";
const BINARY_FILENAME = IS_WINDOWS ? "opencodereview.exe" : "opencodereview";

const PLATFORM_PKG = {
  "darwin-arm64": "@alibaba-group/ocr-darwin-arm64",
  "darwin-x64": "@alibaba-group/ocr-darwin-x64",
  "linux-arm64": "@alibaba-group/ocr-linux-arm64",
  "linux-x64": "@alibaba-group/ocr-linux-x64",
  "win32-arm64": "@alibaba-group/ocr-win32-arm64",
  "win32-x64": "@alibaba-group/ocr-win32-x64",
};

function getPlatformPackageName() {
  const key = `${process.platform}-${process.arch}`;

  try {
    const parentPkg = JSON.parse(
      fs.readFileSync(path.join(__dirname, "..", "package.json"), "utf8")
    );
    const optDeps = parentPkg.optionalDependencies || {};
    for (const name of Object.keys(optDeps)) {
      if (name.endsWith(`-${key}`)) {
        return name;
      }
    }
  } catch (_) {}

  return PLATFORM_PKG[key] || null;
}

function resolveNativeBinary() {
  const pkgName = getPlatformPackageName();
  if (pkgName) {
    try {
      const pkgDir = path.dirname(require.resolve(`${pkgName}/package.json`));
      const binPath = path.join(pkgDir, "bin", BINARY_FILENAME);
      if (fs.existsSync(binPath)) {
        return { path: binPath, fromPlatformPkg: true };
      }
    } catch (err) {
      if (err.code !== "MODULE_NOT_FOUND") {
        console.warn(`[WARN] Unexpected error resolving ${pkgName}: ${err.message}`);
      }
    }
  }

  const legacyPath = path.join(__dirname, "..", "bin", BINARY_FILENAME);
  if (fs.existsSync(legacyPath)) {
    return { path: legacyPath, fromPlatformPkg: false };
  }

  return null;
}

module.exports = {
  IS_WINDOWS,
  BINARY_FILENAME,
  PLATFORM_PKG,
  getPlatformPackageName,
  resolveNativeBinary,
};
