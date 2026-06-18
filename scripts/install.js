#!/usr/bin/env node
"use strict";

const fs = require("fs");
const path = require("path");
const http = require("http");
const https = require("https");
const tls = require("tls");
const crypto = require("crypto");

const IS_WINDOWS = process.platform === "win32";
const BINARY_NAME = IS_WINDOWS ? "opencodereview.exe" : "opencodereview";

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

const stateDirPath = path.join(require("os").homedir(), ".opencodereview");

function getProxyUrl() {
  // 1. env vars take highest priority
  const proxy =
    process.env.HTTPS_PROXY ||
    process.env.HTTP_PROXY ||
    process.env.https_proxy ||
    process.env.http_proxy;
  if (proxy) {
    try {
      return new URL(proxy);
    } catch (_) {}
  }

  // 2. fallback: read from ~/.opencodereview/config.json
  try {
    const configPath = path.join(stateDirPath, "config.json");
    if (fs.existsSync(configPath)) {
      const config = JSON.parse(fs.readFileSync(configPath, "utf8"));
      if (config.proxy && config.proxy.url) {
        return new URL(config.proxy.url);
      }
    }
  } catch (_) {}

  return null;
}

class HttpsProxyAgent extends https.Agent {
  constructor(proxyUrl) {
    super({ keepAlive: true });
    this._proxyUrl = proxyUrl;
  }

  createConnection(options, callback) {
    const proxyPort = this._proxyUrl.port || 3128;
    const targetHost = options.hostname || options.host;
    const targetPort = options.port || 443;

    let done = false;
    const doneOnce = (err, socket) => {
      if (done) return;
      done = true;
      callback(err, socket);
    };

    const req = http.request({
      hostname: this._proxyUrl.hostname,
      port: proxyPort,
      method: "CONNECT",
      path: `${targetHost}:${targetPort}`,
      headers: this._proxyUrl.username
        ? {
            "Proxy-Authorization":
              "Basic " +
              Buffer.from(
                this._proxyUrl.username + ":" + (this._proxyUrl.password || "")
              ).toString("base64"),
          }
        : {},
    });

    req.setTimeout(30000, () => {
      req.destroy();
      doneOnce(new Error(`Proxy CONNECT ${targetHost}:${targetPort} timed out`));
    });

    req.on("connect", (res, socket) => {
      req.setTimeout(0); // clear timeout — CONNECT handshake is done
      if (res.statusCode !== 200) {
        socket.destroy();
        doneOnce(
          new Error(`Proxy CONNECT ${targetHost}:${targetPort} returned ${res.statusCode}`)
        );
        return;
      }
      const tlsOpts = {
        ...options,
        socket,
        servername: options.servername || targetHost,
      };
      const tlsSocket = tls.connect(tlsOpts, () =>
        doneOnce(null, tlsSocket)
      );
      tlsSocket.on("error", (err) => doneOnce(err));
    });

    req.on("error", (err) => doneOnce(err));
    req.end();
  }
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

  const proxyUrl = getProxyUrl();
  const options = proxyUrl
    ? { agent: new HttpsProxyAgent(proxyUrl) }
    : {};

  return new Promise((resolve, reject) => {
    https.get(url, options, (res) => {
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
    try {
      const checksumUrl = buildUrl(config.checksumPattern, vars);
      info("Verifying checksum...");
      const shaContent = await downloadText(checksumUrl);
      const actualSha = await computeChecksum(binaryDest);

      let verified = false;
      for (const line of shaContent.split("\n")) {
        const trimmed = line.trim();
        if (trimmed.includes(`-${os}-${arch}`)) {
          const expectedSha = trimmed.split(/\s+/)[0].toLowerCase();
          if (expectedSha) {
            if (actualSha !== expectedSha) {
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
        warn("No matching checksum entry found; skipping verification.");
      }
    } catch (e) {
      if (e.message.includes("mismatch")) {
        throw e;
      }
      warn(`Could not verify checksum: ${e.message}`);
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
