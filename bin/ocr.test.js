// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

"use strict";

const assert = require("assert");
const os = require("os");

const { launcherExitCode } = require("./ocr");

// Normal exits pass the child's status through unchanged.
assert.strictEqual(launcherExitCode({ status: 0 }), 0);
assert.strictEqual(launcherExitCode({ status: 1 }), 1);
assert.strictEqual(launcherExitCode({ status: 2 }), 2);

// Spawn failures (binary missing, etc.) keep the historical fallback of 1.
assert.strictEqual(launcherExitCode({ status: null, error: new Error("enoent") }), 1);

// A child killed by a signal has status = null; it must exit non-zero,
// conventionally 128 + signo, so pipelines cannot read a signal death
// as a clean run.
assert.strictEqual(launcherExitCode({ status: null, signal: "SIGTERM" }), 128 + os.constants.signals.SIGTERM);
assert.strictEqual(launcherExitCode({ status: null, signal: "SIGKILL" }), 128 + os.constants.signals.SIGKILL);
assert.strictEqual(launcherExitCode({ status: null, signal: "SIGINT" }), 128 + os.constants.signals.SIGINT);

// An unrecognized signal name still exits non-zero rather than 0.
assert.strictEqual(launcherExitCode({ status: null, signal: "SIGFAKE" }) > 0, true);

// A signal result never lets a null status fall through to the error branch's 0.
assert.strictEqual(
  launcherExitCode({ status: null, signal: "SIGTERM", error: undefined }),
  128 + os.constants.signals.SIGTERM
);

console.log("ocr launcher exit-code tests passed");
