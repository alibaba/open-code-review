#!/usr/bin/env node

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

"use strict";

const { spawnSync, spawn } = require("child_process");
const path = require("path");
const fs = require("fs");
const os = require("os");

const { resolveNativeBinary } = require("../scripts/platform");
const { version: packageVersion } = require("../package.json");
const { shouldShowUpdateHint } = require("../scripts/version");

// Maps a child process result to the launcher exit code.
// A child killed by a signal has status = null, which must not fall through
// to a clean 0 — pipelines gating on the exit code would read an OOM kill
// (or any signal death) as success. Conventional 128+signo is used instead.
function launcherExitCode(result) {
  if (result.signal) {
    return 128 + (os.constants.signals[result.signal] ?? 1);
  }
  return result.status ?? (result.error ? 1 : 0);
}

module.exports = { launcherExitCode };

if (require.main !== module) {
  // Required as a module (tests); the launcher body below must not run.
  return;
}

const resolved = resolveNativeBinary();
if (!resolved) {
  console.error(
    "[ERROR] OpenCodeReview binary not found. Run: npm install -g @alibaba-group/open-code-review"
  );
  process.exit(1);
}
const binaryPath = resolved.path;

const hintFile = path.join(os.homedir(), ".opencodereview", "update-available");
try {
  const hint = JSON.parse(fs.readFileSync(hintFile, "utf8"));
  if (hint.pkg && shouldShowUpdateHint(hint.version, packageVersion)) {
    console.error(
      `\x1b[33m[ocr] A new version (v${hint.version}) is available. Run to update:\x1b[0m\n` +
      `\x1b[33m  npm i -g ${hint.pkg}@${hint.version}\x1b[0m\n`
    );
  } else {
    fs.unlinkSync(hintFile);
  }
} catch (_) {}

if (!process.env.OCR_NO_UPDATE) {
  const stateDir = path.join(os.homedir(), ".opencodereview");
  const tsFile = path.join(stateDir, "last-update-check");
  const cooldownMs =
    (parseInt(process.env.OCR_UPDATE_INTERVAL, 10) || 18) * 60 * 1000;

  let shouldCheck = true;
  try {
    const mt = fs.statSync(tsFile).mtimeMs;
    if (Date.now() - mt < cooldownMs) shouldCheck = false;
  } catch (_) {}

  if (shouldCheck) {
    const updateScript = path.join(__dirname, "..", "scripts", "update.js");
    const child = spawn(process.execPath, [updateScript], {
      detached: true,
      stdio: "ignore",
      env: Object.assign({}, process.env, { OCR_NO_UPDATE: "1" }),
    });
    child.unref();
  }
}

const result = spawnSync(binaryPath, process.argv.slice(2), {
  stdio: "inherit",
  env: process.env,
});

process.exit(launcherExitCode(result));
