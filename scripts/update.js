#!/usr/bin/env node

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

"use strict";

const fs = require("fs");
const path = require("path");
const https = require("https");
const { spawnSync } = require("child_process");

const {
  resolveNativeBinary,
  IS_WINDOWS,
  BINARY_FILENAME,
  STATE_DIR,
  STAGED_BIN_DIR,
  VERSION_JSON_PATH,
} = require("./platform");
const { loadPackageJson } = require("./install.js");
const { SEMVER_RE, parseVersionOutput, semverGt } = require("./version");

const tsFile = path.join(STATE_DIR, "last-update-check");
const lockFile = path.join(STATE_DIR, "update.lock");
const hintFile = path.join(STATE_DIR, "update-available");

const DEFAULT_REGISTRY = "https://registry.npmjs.org";

function touchTimestamp() {
  fs.mkdirSync(STATE_DIR, { recursive: true });
  const now = new Date();
  try {
    fs.utimesSync(tsFile, now, now);
  } catch (_) {
    fs.writeFileSync(tsFile, now.toISOString());
  }
}

function acquireLock() {
  fs.mkdirSync(STATE_DIR, { recursive: true });
  try {
    fs.writeFileSync(lockFile, String(process.pid), { flag: "wx" });
    return true;
  } catch (e) {
    if (e.code !== "EEXIST") return false;
    try {
      const pid = parseInt(fs.readFileSync(lockFile, "utf8").trim(), 10);
      process.kill(pid, 0);
      return false;
    } catch (_) {
      try {
        fs.unlinkSync(lockFile);
        fs.writeFileSync(lockFile, String(process.pid), { flag: "wx" });
        return true;
      } catch (_2) {
        return false;
      }
    }
  }
}

function releaseLock() {
  try {
    fs.unlinkSync(lockFile);
  } catch (_) {}
}

function getInstalledVersion(binPath) {
  try {
    const result = spawnSync(binPath, ["version"], {
      encoding: "utf8",
      timeout: 3000,
    });
    return parseVersionOutput(result.stdout);
  } catch (_) {
    return null;
  }
}

function fetchLatestVersion(pkg) {
  const registry = (pkg.publishConfig && pkg.publishConfig.registry) || DEFAULT_REGISTRY;
  const pkgName = pkg.name;
  if (!pkgName) return Promise.resolve(null);
  const encodedName = pkgName.replace(/\//g, "%2F");
  const url = `${registry.replace(/\/$/, "")}/${encodedName}/latest`;
  if (!url.startsWith("https://")) return Promise.resolve(null);

  return new Promise((resolve) => {
    const options = {
      headers: { "User-Agent": "ocr-updater", Accept: "application/json" },
      timeout: 15000,
    };
    const req = https
      .get(url, options, (res) => {
        if (res.statusCode !== 200) {
          res.resume();
          resolve(null);
          return;
        }
        let data = "";
        res.on("data", (chunk) => (data += chunk));
        res.on("end", () => {
          try {
            const json = JSON.parse(data);
            resolve(json.version || null);
          } catch (_) {
            resolve(null);
          }
        });
        res.on("error", () => resolve(null));
      })
      .on("error", () => resolve(null));
    req.on("timeout", () => {
      req.destroy();
      resolve(null);
    });
  });
}

function writeHint(latestVersion, pkgName) {
  try {
    fs.writeFileSync(hintFile, JSON.stringify({ version: latestVersion, pkg: pkgName }));
  } catch (_) {}
}

function removeHint() {
  try {
    fs.unlinkSync(hintFile);
  } catch (_) {}
}

function writeVersionJson(version) {
  const meta = {
    version,
    stagedAt: new Date().toISOString(),
    platform: `${process.platform}-${process.arch}`,
  };
  fs.writeFileSync(VERSION_JSON_PATH, JSON.stringify(meta, null, 2));
}

function stageBinary(srcPath) {
  try {
    fs.mkdirSync(STAGED_BIN_DIR, { recursive: true });
    const dest = path.join(STAGED_BIN_DIR, BINARY_FILENAME);
    const tmp = dest + ".tmp";
    fs.copyFileSync(srcPath, tmp);
    if (!IS_WINDOWS) {
      fs.chmodSync(tmp, 0o755);
    }
    fs.renameSync(tmp, dest);

    writeVersionJson(getInstalledVersion(dest) || "unknown");
  } catch (_) {}
}

async function main() {
  touchTimestamp();

  if (!acquireLock()) return;

  try {
    const resolved = resolveNativeBinary();
    if (!resolved) return;

    const installedVersion = getInstalledVersion(resolved.path);
    if (!installedVersion) return;

    const pkg = loadPackageJson();
    const latestVersion = await fetchLatestVersion(pkg);
    if (!latestVersion) return;

    if (!SEMVER_RE.test(latestVersion)) return;

    if (!semverGt(latestVersion, installedVersion)) {
      removeHint();
      return;
    }

    // Stage current binary before npm i -g to cover the gap window
    if (!resolved.fromStaged) {
      stageBinary(resolved.path);
    }

    const pkgName = pkg.name;
    const result = spawnSync("npm", ["i", "-g", `${pkgName}@${latestVersion}`], {
      encoding: "utf8",
      timeout: 120000,
      shell: IS_WINDOWS,
    });

    if (result.status === 0) {
      removeHint();
      // Refresh staged binary with the newly installed version
      const newResolved = resolveNativeBinary();
      if (newResolved && !newResolved.fromStaged) {
        stageBinary(newResolved.path);
      }
    } else {
      writeHint(latestVersion, pkgName);
    }
  } catch (_) {
  } finally {
    releaseLock();
  }
}

main().catch(() => {});
