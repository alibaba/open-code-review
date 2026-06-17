#!/usr/bin/env node
"use strict";

const fs = require("fs");
const path = require("path");
const os = require("os");
const https = require("https");
const { spawnSync } = require("child_process");

const {
  IS_WINDOWS,
  BINARY_NAME,
  detectPlatform,
  loadPackageJson,
  buildUrl,
  downloadText,
  downloadBinary,
  computeChecksum,
} = require("./install.js");
const packageRoot = path.join(__dirname, "..");
const binDir = path.join(packageRoot, "bin");
const binaryPath = path.join(binDir, BINARY_NAME);
const stateDir = path.join(os.homedir(), ".opencodereview");
const tsFile = path.join(stateDir, "last-update-check");
const lockFile = path.join(stateDir, "update.lock");

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

function getInstalledVersion() {
  try {
    const result = spawnSync(binaryPath, ["version"], {
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

function semverGt(a, b) {
  const pa = a.split(".").map(Number);
  const pb = b.split(".").map(Number);
  for (let i = 0; i < 3; i++) {
    if ((pa[i] || 0) > (pb[i] || 0)) return true;
    if ((pa[i] || 0) < (pb[i] || 0)) return false;
  }
  return false;
}

function cleanupTemp() {
  try {
    const files = fs.readdirSync(binDir);
    for (const f of files) {
      if (f.startsWith(".opencodereview.tmp.")) {
        fs.unlinkSync(path.join(binDir, f));
      }
    }
  } catch (_) {}
}

async function main() {
  touchTimestamp();

  if (!acquireLock()) return;

  cleanupTemp();

  try {
    const { resolveNativeBinary } = require("./platform");
    const resolved = resolveNativeBinary();
    if (resolved && resolved.fromPlatformPkg) {
      info("Binary managed by platform package, skipping auto-update.");
      return;
    }
    const installedVersion = getInstalledVersion();
    if (!installedVersion) return;

    const pkg = loadPackageJson();
    const latestVersion = await fetchLatestVersion(pkg);
    if (!latestVersion) return;

    if (!semverGt(latestVersion, installedVersion)) return;

    const { os: platform, arch } = detectPlatform();
    const config = pkg.ocrConfig;

    const vars = { version: latestVersion, os: platform, arch };
    let downloadUrl = buildUrl(config.urlPattern, vars);
    if (IS_WINDOWS) {
      downloadUrl += ".exe";
    }

    const tempPath = path.join(binDir, `.opencodereview.tmp.${process.pid}`);
    await downloadBinary(downloadUrl, tempPath);
    if (!IS_WINDOWS) {
      fs.chmodSync(tempPath, 0o755);
    }

    if (config.checksumPattern) {
      const checksumUrl = buildUrl(config.checksumPattern, vars);
      let shaContent;
      try {
        shaContent = await downloadText(checksumUrl);
      } catch (_) {
        fs.unlinkSync(tempPath);
        return;
      }
      let actualSha;
      try {
        actualSha = await computeChecksum(tempPath);
      } catch (_) {
        fs.unlinkSync(tempPath);
        return;
      }

      let verified = false;
      for (const line of shaContent.split("\n")) {
        const trimmed = line.trim();
        if (trimmed.includes(`-${platform}-${arch}`)) {
          const expectedSha = trimmed.split(/\s+/)[0].toLowerCase();
          if (expectedSha && actualSha !== expectedSha) {
            fs.unlinkSync(tempPath);
            return;
          }
          verified = true;
          break;
        }
      }
      if (!verified) {
        fs.unlinkSync(tempPath);
        return;
      }
    }

    if (IS_WINDOWS) {
      const oldPath = binaryPath + ".old";
      try { fs.unlinkSync(oldPath); } catch (_) {}
      try {
        fs.renameSync(binaryPath, oldPath);
      } catch (e) {
        if (fs.existsSync(binaryPath)) {
          throw e;
        }
      }
      try {
        fs.renameSync(tempPath, binaryPath);
      } catch (e) {
        try { fs.renameSync(oldPath, binaryPath); } catch (_) {}
        throw e;
      }
      try { fs.unlinkSync(oldPath); } catch (_) {}
    } else {
      fs.renameSync(tempPath, binaryPath);
    }
  } catch (_) {
    cleanupTemp();
  } finally {
    releaseLock();
  }
}

main().catch(() => {});
