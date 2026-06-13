#!/usr/bin/env node
"use strict";
process.stderr.write("[ocr-bin] workspace path check\n");

const { spawnSync, spawn } = require("child_process");
const path = require("path");
const fs = require("fs");
const os = require("os");

const IS_WINDOWS = process.platform === "win32";
const binaryPath = path.join(__dirname, IS_WINDOWS ? "opencodereview.exe" : "opencodereview");

if (!process.env.OCR_NO_UPDATE) {
  const stateDir = path.join(os.homedir(), ".opencodereview");
  const tsFile = path.join(stateDir, "last-update-check");
  const cooldownMs =
    (parseInt(process.env.OCR_UPDATE_INTERVAL, 10) || 30) * 60 * 1000;

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
