#!/usr/bin/env node
"use strict";

const { spawnSync, spawn } = require("child_process");
const path = require("path");
const fs = require("fs");
const os = require("os");

const { resolveNativeBinary } = require("../scripts/platform");

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

process.exit(result.status ?? (result.error ? 1 : 0));
