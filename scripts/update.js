#!/usr/bin/env node
"use strict";

const fs = require("fs");
const path = require("path");
const os = require("os");
const https = require("https");
const { spawnSync } = require("child_process");

const { resolveNativeBinary } = require("./platform");
const { loadPackageJson } = require("./install.js");

const stateDir = path.join(os.homedir(), ".opencodereview");
const tsFile = path.join(stateDir, "last-update-check");
const lockFile = path.join(stateDir, "update.lock");
const hintFile = path.join(stateDir, "update-available");

const DEFAULT_REGISTRY = "https://registry.npmjs.org";

function touchTimestamp() {
  fs.mkdirSync(stateDir, { recursive: true });
  const now = new Date();
  try {
    fs.utimesSync(tsFile, now, now);
  } catch (_) {
    fs.writeFileSync(tsFile, now.toISOString());
  }
}

function acquireLock() {
  fs.mkdirSync(stateDir, { recursive: true });
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
    const match = (result.stdout || "").match(/v(\d+\.\d+(?:\.\d+)?)/);
    return match ? match[1] : null;
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

const SEMVER_RE = /^\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$/;

function semverGt(a, b) {
  const pa = a.replace(/-.*$/, "").split(".").map(Number);
  const pb = b.replace(/-.*$/, "").split(".").map(Number);
  for (let i = 0; i < 3; i++) {
    if ((pa[i] || 0) > (pb[i] || 0)) return true;
    if ((pa[i] || 0) < (pb[i] || 0)) return false;
  }
  const aPre = a.includes("-");
  const bPre = b.includes("-");
  if (bPre && !aPre) return true;
  return false;
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

    const pkgName = pkg.name;
    const IS_WINDOWS = process.platform === "win32";
    const result = spawnSync("npm", ["i", "-g", `${pkgName}@${latestVersion}`], {
      encoding: "utf8",
      timeout: 120000,
      shell: IS_WINDOWS,
    });

    if (result.status === 0) {
      removeHint();
    } else {
      writeHint(latestVersion, pkgName);
    }
  } catch (_) {
  } finally {
    releaseLock();
  }
}

main().catch(() => {});
