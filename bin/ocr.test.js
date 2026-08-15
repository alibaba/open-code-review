#!/usr/bin/env node

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

// Unit tests for bin/ocr.js's computeExit().
//
// Run via: node bin/ocr.test.js

"use strict";

const assert = require("assert");
const { computeExit } = require("./ocr.js");

// Normal exit, no signal.
{
  const { code, message } = computeExit({ status: 0, signal: null, error: undefined });
  assert.strictEqual(code, 0);
  assert.strictEqual(message, null);
}

{
  const { code, message } = computeExit({ status: 2, signal: null, error: undefined });
  assert.strictEqual(code, 2);
  assert.strictEqual(message, null);
}

// spawnSync itself failed (e.g. ENOENT) -- no status, no signal, has error.
{
  const { code, message } = computeExit({ status: null, signal: null, error: new Error("ENOENT") });
  assert.strictEqual(code, 1);
  assert.strictEqual(message, null);
}

// Terminated by a signal: status is null and error is unset (this is the
// bug -- the old code fell through to 0, reporting a signal-killed run as
// a success).
{
  const { code, message } = computeExit({ status: null, signal: "SIGKILL", error: undefined });
  assert.strictEqual(code, 137); // 128 + 9
  assert.match(message, /terminated by signal SIGKILL/);
}

{
  const { code, message } = computeExit({ status: null, signal: "SIGTERM", error: undefined });
  assert.strictEqual(code, 143); // 128 + 15
  assert.match(message, /terminated by signal SIGTERM/);
}

console.log("ocr.test.js: all tests passed");
