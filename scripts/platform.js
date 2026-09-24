// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

"use strict";

const path = require("path");
const fs = require("fs");
const os = require("os");

const IS_WINDOWS = process.platform === "win32";
const BINARY_FILENAME = IS_WINDOWS ? "opencodereview.exe" : "opencodereview";

const STATE_DIR = path.join(os.homedir(), ".opencodereview");
const STAGED_BIN_DIR = path.join(STATE_DIR, "staged");
const VERSION_JSON_PATH = path.join(STAGED_BIN_DIR, "version.json");

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

function resolveStagedBinary() {
  try {
    const raw = fs.readFileSync(VERSION_JSON_PATH, "utf8");
    const meta = JSON.parse(raw);
    if (!meta.version || !meta.stagedAt || !meta.platform) {
      return null;
    }

    const currentPlatform = `${process.platform}-${process.arch}`;
    if (meta.platform !== currentPlatform) {
      return null;
    }

    const binPath = path.join(STAGED_BIN_DIR, BINARY_FILENAME);
    let stat;
    try {
      stat = fs.statSync(binPath);
    } catch (_) {
      return null;
    }
    if (!stat.isFile() || stat.size === 0) {
      return null;
    }

    return {
      path: binPath,
      fromPlatformPkg: false,
      fromStaged: true,
      stagedVersion: meta.version,
    };
  } catch (_) {
    return null;
  }
}

function resolveNativeBinary() {
  const pkgName = getPlatformPackageName();
  if (pkgName) {
    try {
      const pkgDir = path.dirname(require.resolve(`${pkgName}/package.json`));
      const binPath = path.join(pkgDir, "bin", BINARY_FILENAME);
      if (fs.existsSync(binPath)) {
        return { path: binPath, fromPlatformPkg: true, fromStaged: false };
      }
    } catch (err) {
      if (err.code !== "MODULE_NOT_FOUND") {
        console.warn(`[WARN] Unexpected error resolving ${pkgName}: ${err.message}`);
      }
    }
  }

  const staged = resolveStagedBinary();
  if (staged) {
    return staged;
  }

  const legacyPath = path.join(__dirname, "..", "bin", BINARY_FILENAME);
  if (fs.existsSync(legacyPath)) {
    return { path: legacyPath, fromPlatformPkg: false, fromStaged: false };
  }

  return null;
}

module.exports = {
  IS_WINDOWS,
  BINARY_FILENAME,
  PLATFORM_PKG,
  STATE_DIR,
  STAGED_BIN_DIR,
  VERSION_JSON_PATH,
  getPlatformPackageName,
  resolveStagedBinary,
  resolveNativeBinary,
};
