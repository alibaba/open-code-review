#!/usr/bin/env node

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

"use strict";

const { spawnSync, spawn } = require("child_process");
const path = require("path");
const fs = require("fs");
const os = require("os");

const { resolveNativeBinary } = require("../scripts/platform");

// Determines the exit code for a spawnSync() result against the native
// binary. `result.status` is null when the process was terminated by a
// signal rather than exiting normally -- e.g. OOM-killed -- and in that
// case `result.error` is also unset, since no spawn error occurred. Exit
// with 128 + signal number, the same convention shells use, instead of
// falling through to 0 and reporting a signal-killed run as a success.
// Exported for testing.
function computeExit(result) {
  if (result.signal) {
    // os.constants.signals has no entry for signal names Node doesn't
    // recognize on this platform; fall back to SIGHUP's number (1) so we
    // still report a signal-related failure instead of throwing.
    return {
      code: 128 + (os.constants.signals[result.signal] || 1),
      message: `[ERROR] OpenCodeReview binary was terminated by signal ${result.signal}`,
    };
  }
  if (result.error) {
    return {
      code: result.status ?? 1,
      message: `[ERROR] Failed to run OpenCodeReview binary: ${result.error.message}`,
    };
  }
  return { code: result.status ?? 0, message: null };
}

function main() {
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
    if (hint.version && hint.pkg) {
      console.error(
        `\x1b[33m[ocr] A new version (v${hint.version}) is available. Run to update:\x1b[0m\n` +
        `\x1b[33m  npm i -g ${hint.pkg}@${hint.version}\x1b[0m\n`
      );
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

  const { code, message } = computeExit(result);
  if (message) console.error(message);
  process.exit(code);
}

if (require.main === module) {
  main();
}

module.exports = { computeExit };
