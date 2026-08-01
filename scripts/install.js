#!/usr/bin/env node
"use strict";

const fs = require("fs");
const path = require("path");
const https = require("https");
const crypto = require("crypto");

const { IS_WINDOWS, BINARY_FILENAME: BINARY_NAME, resolveNativeBinary } = require("./platform");

const packageRoot = path.join(__dirname, "..");
const binDir = path.join(packageRoot, "bin");
const binaryDest = path.join(binDir, BINARY_NAME);

function info(msg) {
  console.log(`[INFO]  ${msg}`);
}

function warn(msg) {
  console.warn(`[WARN]  ${msg}`);
}

function error(msg) {
  console.error(`[ERROR] ${msg}`);
}

function detectPlatform() {
  let os = process.platform;
  let arch = process.arch;

  switch (arch) {
    case "x64":
      arch = "amd64";
      break;
    case "arm64":
      arch = "arm64";
      break;
    default:
      throw new Error(
        `Unsupported architecture: ${arch}. Supported: amd64 (x64), arm64`
      );
  }

  switch (os) {
    case "linux":
    case "darwin":
      break;
    case "win32":
      os = "windows";
      break;
    default:
      throw new Error(
        `Unsupported operating system: ${os}. Supported: linux, darwin, win32`
      );
  }

  return { os, arch };
}

function loadPackageJson() {
  const pkg = JSON.parse(fs.readFileSync(path.join(packageRoot, "package.json"), "utf8"));
  if (!pkg.version) {
    throw new Error("Missing version field in package.json");
  }
  if (!pkg.ocrConfig || !pkg.ocrConfig.urlPattern) {
    throw new Error("Missing ocrConfig.urlPattern in package.json");
  }
  return pkg;
}

function resolveVersion(pkg) {
  const envVersion = process.env.OCR_VERSION;
  if (envVersion) {
    const v = envVersion.startsWith("v") ? envVersion.slice(1) : envVersion;
    info(`Using pinned version from OCR_VERSION: ${v}`);
    return v;
  }

  info(`Using version from package.json: ${pkg.version}`);
  return pkg.version;
}

function buildUrl(pattern, vars) {
  return pattern
    .replace(/\{version\}/g, vars.version)
    .replace(/\{os\}/g, vars.os)
    .replace(/\{arch\}/g, vars.arch);
}

function download(url, maxRedirects = 10) {
  if (!url.startsWith("https")) {
    return Promise.reject(new Error(`Refusing non-HTTPS download: ${url}`));
  }
  if (maxRedirects <= 0) {
    return Promise.reject(new Error(`Too many redirects fetching ${url}`));
  }
  return new Promise((resolve, reject) => {
    https.get(url, (res) => {
      if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
        res.resume();
        download(res.headers.location, maxRedirects - 1).then(resolve).catch(reject);
        return;
      }
      if (res.statusCode !== 200) {
        res.resume();
        reject(new Error(`HTTP ${res.statusCode} fetching ${url}`));
        return;
      }
      resolve(res);
    }).on("error", reject);
  });
}

async function downloadText(url) {
  const res = await download(url);
  return new Promise((resolve, reject) => {
    let data = "";
    res.on("data", (chunk) => (data += chunk));
    res.on("end", () => resolve(data));
    res.on("error", reject);
  });
}

async function downloadBinary(url, destPath) {
  const res = await download(url);
  return new Promise((resolve, reject) => {
    const fileStream = fs.createWriteStream(destPath);
    res.on("error", (err) => {
      fileStream.destroy();
      fs.unlink(destPath, () => {});
      reject(err);
    });
    res.pipe(fileStream);
    fileStream.on("finish", () => fileStream.close(() => resolve()));
    fileStream.on("error", (err) => {
      fs.unlink(destPath, () => {});
      reject(err);
    });
  });
}

function computeChecksum(filePath) {
  return new Promise((resolve, reject) => {
    const hash = crypto.createHash("sha256");
    const stream = fs.createReadStream(filePath);
    stream.on("data", (chunk) => hash.update(chunk));
    stream.on("end", () => resolve(hash.digest("hex")));
    stream.on("error", reject);
  });
}

async function main() {
  info("OpenCodeReview Installer");
  info("=========================");

  const existing = resolveNativeBinary();
  if (existing && existing.fromPlatformPkg) {
    info("Binary provided by platform package, skipping download.");
    info(`  ${existing.path}`);
    return;
  }

  const { os, arch } = detectPlatform();
  info(`Detected platform: ${os}/${arch}`);

  const pkg = loadPackageJson();
  const version = resolveVersion(pkg);
  const config = pkg.ocrConfig;

  if (!fs.existsSync(binDir)) {
    fs.mkdirSync(binDir, { recursive: true });
  }

  if (!IS_WINDOWS) {
    const jsWrapper = path.join(binDir, "ocr.js");
    if (fs.existsSync(jsWrapper)) {
      try {
        fs.chmodSync(jsWrapper, 0o755);
      } catch (e) {
        warn(`Could not make ocr.js executable: ${e.message}`);
      }
    }
  }

  const vars = { version, os, arch };
  let downloadUrl = buildUrl(config.urlPattern, vars);
  if (IS_WINDOWS) {
    downloadUrl += ".exe";
  }
  info(`Downloading ${downloadUrl} ...`);

  await downloadBinary(downloadUrl, binaryDest);
  if (!IS_WINDOWS) {
    fs.chmodSync(binaryDest, 0o755);
  }

  if (config.checksumPattern) {
    const checksumUrl = buildUrl(config.checksumPattern, vars);
    info("Verifying checksum...");
    let shaContent;
    try {
      shaContent = await downloadText(checksumUrl);
    } catch (e) {
      try { fs.unlinkSync(binaryDest); } catch (_) {}
      throw new Error(`Failed to download checksum from ${checksumUrl}: ${e.message}`);
    }
    let actualSha;
    try {
      actualSha = await computeChecksum(binaryDest);
    } catch (e) {
      try { fs.unlinkSync(binaryDest); } catch (_) {}
      throw new Error(`Failed to compute checksum for ${binaryDest}: ${e.message}`);
    }

    let verified = false;
    for (const line of shaContent.split("\n")) {
      const trimmed = line.trim();
      if (trimmed.includes(`-${os}-${arch}`)) {
        const expectedSha = trimmed.split(/\s+/)[0].toLowerCase();
        if (expectedSha) {
          if (actualSha !== expectedSha) {
            try { fs.unlinkSync(binaryDest); } catch (_) {}
            throw new Error(
              `Checksum mismatch! Expected: ${expectedSha}, Got: ${actualSha}`
            );
          }
          info("Checksum verified.");
          verified = true;
          break;
        }
      }
    }
    if (!verified) {
      try { fs.unlinkSync(binaryDest); } catch (_) {}
      throw new Error(
        `No matching checksum entry for ${os}-${arch} in ${checksumUrl}`
      );
    }
  }

  info(`Installed: ${binaryDest}`);
  info("");
  info("OpenCodeReview is ready!");
  info("");
  info("Quick start:");
  info("  ocr version             Show version info");
  info("  ocr config set          Configure your LLM provider");
  info("  ocr review              Start a code review");
}

if (require.main === module) {
  main().catch((err) => {
    error(err.message);
    process.exit(1);
  });
} else {
  module.exports = {
    IS_WINDOWS,
    BINARY_NAME,
    detectPlatform,
    loadPackageJson,
    buildUrl,
    download,
    downloadText,
    downloadBinary,
    computeChecksum,
  };
}
