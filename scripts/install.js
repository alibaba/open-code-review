#!/usr/bin/env node

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

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

// Default PATHEXT on Windows when the variable is unset/empty.
const DEFAULT_PATHEXT = ".COM;.EXE;.BAT;.CMD;.VBS;.VBE;.JS;.JSE;.WSF;.WSH;.MSC";

// File names a shell may resolve for the `ocr` command. POSIX executes the
// plain name only; Windows probes the PATHEXT variants in the same order
// cmd.exe would, plus the bare name last for POSIX-like shells (Git Bash).
function ocrCandidateNames(isWindows, pathExtEnv) {
  if (!isWindows) {
    return ["ocr"];
  }
  const raw = pathExtEnv && String(pathExtEnv).trim() ? pathExtEnv : DEFAULT_PATHEXT;
  const names = String(raw)
    .split(";")
    .map((e) => e.trim().toLowerCase())
    .filter((e) => e.startsWith("."))
    .map((e) => "ocr" + e);
  names.push("ocr");
  return Array.from(new Set(names));
}

// Path comparisons must be case-insensitive on Windows: realpath output and
// env-derived paths (npm_config_prefix, shim targets) can legitimately
// differ in casing (drive letter, profile dir, junctions).
function pathsEqual(a, b, isWindows) {
  return isWindows ? a.toLowerCase() === b.toLowerCase() : a === b;
}

function pathHasPrefix(p, prefix, isWindows) {
  return isWindows
    ? p.toLowerCase().startsWith(prefix.toLowerCase())
    : p.startsWith(prefix);
}

// Walk PATH in order and return every entry that contains an `ocr` executable
// as { dir, name, path }. Within one directory only the first matching
// variant is reported, mirroring how a shell resolves that directory.
// candidateNames defaults to ocrCandidateNames(isWindows).
function findOcrOnPath(pathEnv, isWindows, pathSep, candidateNames) {
  const names = candidateNames || ocrCandidateNames(isWindows);
  const found = [];
  const seenDirs = new Set();
  for (const dir of String(pathEnv || "").split(pathSep || (isWindows ? ";" : ":"))) {
    if (!dir) {
      continue;
    }
    let realDir = dir;
    try {
      realDir = fs.realpathSync(dir);
    } catch (_) {
      // Leave the dir as written; a stat failure below skips it anyway.
    }
    const key = isWindows ? realDir.toLowerCase() : realDir;
    if (seenDirs.has(key)) {
      continue;
    }
    seenDirs.add(key);
    for (const name of names) {
      const candidate = path.join(dir, name);
      try {
        const st = fs.statSync(candidate); // follows symlinks
        if (!st.isFile()) {
          continue;
        }
        if (!isWindows && (st.mode & 0o111) === 0) {
          continue; // not executable on POSIX
        }
        found.push({ dir, name, path: candidate });
        break; // first matching variant in this directory wins
      } catch (_) {
        // Missing or unreadable entry: try the next variant.
      }
    }
  }
  return found;
}

// True when a resolved PATH entry belongs to *this* installation: either a
// symlink to bin/ocr.js (npm/yarn/pnpm on POSIX) or a package-manager shim
// (npm .cmd/.ps1 on Windows, wrapper scripts elsewhere) that launches it.
function isOurCommand(filePath, packageRoot, pkgName, isWindows) {
  const ourJs = path.join(packageRoot, "bin", "ocr.js");
  let real = filePath;
  try {
    real = fs.realpathSync(filePath);
  } catch (_) {
    // Keep the raw path; the comparison below simply won't match.
  }
  try {
    if (pathsEqual(real, fs.realpathSync(ourJs), isWindows)) {
      return true;
    }
  } catch (_) {
    // bin/ocr.js missing: fall through to the shim content check.
  }
  try {
    const st = fs.statSync(filePath);
    if (st.isFile() && st.size < 4096) {
      // Shims embed the script they launch; compare with normalized slashes
      // and casing (the embedded path follows the installer's casing).
      const content = fs.readFileSync(filePath, "utf8").replace(/\\/g, "/").toLowerCase();
      if (content.includes(`node_modules/${pkgName}/bin/ocr.js`.toLowerCase())) {
        return true;
      }
    }
  } catch (_) {
    // Unreadable candidate: treat as foreign.
  }
  return false;
}

// For a global install npm links `ocr` into <prefix>/bin (POSIX) or <prefix>
// (Windows). Returns that directory when packageRoot lives under the global
// prefix, otherwise null (a local install does not expect a PATH command).
function globalBinDir(packageRoot, isWindows, env) {
  const prefix = env && env.npm_config_prefix;
  if (!prefix) {
    return null;
  }
  const globalRoot = isWindows
    ? path.join(prefix, "node_modules")
    : path.join(prefix, "lib", "node_modules");
  if (!pathHasPrefix(packageRoot, globalRoot + path.sep, isWindows)) {
    return null;
  }
  return isWindows ? prefix : path.join(prefix, "bin");
}

// Classify how `ocr` resolves in the installing shell right after install.
//
// Why this exists: shells cache command locations (the bash/zsh hash table).
// If the session ever ran a different executable named `ocr` before this
// install, the *current* terminal keeps running that cached location even
// though the real `ocr` now sits earlier on PATH; only new terminals resolve
// correctly. The installer cannot clear the parent shell's cache, so it must
// detect the situation and tell the user (see printPathResolutionNotice).
//
// Returns one of:
//   { status: "ok", resolved }                 resolves here, no lookalikes
//   { status: "stale-hash-risk", resolved,
//     stale: [paths] }                         resolves here, but another ocr
//                                              exists that open shells may have
//                                              cached
//   { status: "shadowed", resolved,
//     ours: [paths] }                          a different executable wins in
//                                              every terminal, though ours is
//                                              also on PATH
//   { status: "not-on-path", binDir,
//     occupiedBy }                             global install, but this
//                                              installation's `ocr` is not
//                                              resolvable; occupiedBy is the
//                                              foreign executable holding the
//                                              name, or null
//   { status: "unknown" }                      not resolvable; local install
function checkOcrCommand(opts) {
  const { packageRoot, pkgName, pathEnv, isWindows, env } = opts;
  const pathSep = opts.pathSep || (isWindows ? ";" : ":");
  const names = ocrCandidateNames(isWindows, env && env.PATHEXT);
  const found = findOcrOnPath(pathEnv, isWindows, pathSep, names);
  // Classify each entry once; isOurCommand performs filesystem I/O.
  const classified = found.map((f) => ({
    dir: f.dir,
    name: f.name,
    path: f.path,
    ours: isOurCommand(f.path, packageRoot, pkgName, isWindows),
  }));
  const ours = classified.filter((f) => f.ours);
  if (ours.length === 0) {
    // This installation has no resolvable `ocr` on PATH. For global installs
    // we know where the command should live, so report that — including who
    // currently occupies the command name, if anyone.
    const binDir = globalBinDir(packageRoot, isWindows, env);
    if (!binDir) {
      return { status: "unknown" };
    }
    return {
      status: "not-on-path",
      binDir,
      occupiedBy: classified.length > 0 ? classified[0].path : null,
    };
  }
  const foreign = classified.filter((f) => !f.ours);
  const first = classified[0];
  if (first.ours) {
    if (foreign.length > 0) {
      return {
        status: "stale-hash-risk",
        resolved: first.path,
        stale: foreign.map((f) => f.path),
      };
    }
    return { status: "ok", resolved: first.path };
  }
  return {
    status: "shadowed",
    resolved: first.path,
    ours: ours.map((f) => f.path),
  };
}

// Render the classification from checkOcrCommand as install-time notices.
function printPathResolutionNotice() {
  let result;
  try {
    result = checkOcrCommand({
      packageRoot,
      pkgName: loadPackageJson().name,
      pathEnv: process.env.PATH || "",
      isWindows: IS_WINDOWS,
      env: process.env,
    });
  } catch (e) {
    warn(`Could not verify how the ocr command resolves: ${e.message}`);
    return;
  }
  switch (result.status) {
    case "ok":
      break;
    case "stale-hash-risk":
      warn(`Another executable named 'ocr' exists on your PATH: ${result.stale.join(", ")}.`);
      warn("Terminals opened before this install may have cached that location and keep");
      warn("running it instead of OpenCodeReview (new terminals are not affected).");
      warn("If 'ocr' misbehaves in your current terminal, refresh the shell's command cache:");
      warn("  bash/zsh: hash -r    (zsh also: rehash)    or simply open a new terminal");
      warn("Verify with: ocr version   (expected output: open-code-review vX.Y.Z ...)");
      break;
    case "shadowed": {
      const oursNote = result.ours.length > 0 ? ` (${result.ours[0]})` : "";
      warn(`'ocr' resolves to ${result.resolved}, which takes precedence over the`);
      warn(`OpenCodeReview you just installed${oursNote}. Commands like 'ocr config`);
      warn("provider' will run that other program in every terminal. Remove or rename the");
      warn("file above, or move OpenCodeReview's bin directory earlier in your PATH.");
      warn("Verify with: ocr version   (expected output: open-code-review vX.Y.Z ...)");
      break;
    }
    case "not-on-path":
      if (result.occupiedBy) {
        warn(`'ocr' resolves to ${result.occupiedBy}, but that is a different program: the`);
        warn(`ocr you just installed is not resolvable on PATH. It should live in`);
        warn(`${result.binDir}; add that directory to your PATH (earlier than the entry`);
        warn("above), then open a new terminal.");
      } else {
        warn(`'ocr' was not found on your PATH. Add ${result.binDir} to your PATH (then open`);
        warn("a new terminal), or run the binary from that directory directly.");
      }
      break;
    default:
      break; // local install without a PATH entry: nothing to verify
  }
}

async function main() {
  info("OpenCodeReview Installer");
  info("=========================");

  const existing = resolveNativeBinary();
  if (existing && existing.fromPlatformPkg) {
    info("Binary provided by platform package, skipping download.");
    info(`  ${existing.path}`);
    printPathResolutionNotice();
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
  printPathResolutionNotice();
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
    ocrCandidateNames,
    pathsEqual,
    pathHasPrefix,
    findOcrOnPath,
    isOurCommand,
    globalBinDir,
    checkOcrCommand,
  };
}
