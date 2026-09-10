// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

"use strict";

const assert = require("assert");
const os = require("os");
const { EventEmitter } = require("events");

const { launcherExitCode, installSignalHandlers } = require("./ocr");

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

// POSIX signals are relayed to the child and all installed handlers are
// removable once the child has settled.
{
  const signalTarget = new EventEmitter();
  const relayed = [];
  const remove = installSignalHandlers(
    { kill: (signal) => relayed.push(signal) },
    "linux",
    signalTarget
  );
  signalTarget.emit("SIGINT");
  signalTarget.emit("SIGTERM");
  assert.deepStrictEqual(relayed, ["SIGINT", "SIGTERM"]);
  remove();
  assert.strictEqual(signalTarget.listenerCount("SIGINT"), 0);
  assert.strictEqual(signalTarget.listenerCount("SIGTERM"), 0);
}

// On Windows, console Ctrl+C already reaches both attached processes. The
// launcher keeps itself alive but must not turn SIGINT into TerminateProcess.
{
  const signalTarget = new EventEmitter();
  const relayed = [];
  const remove = installSignalHandlers(
    { kill: (signal) => relayed.push(signal) },
    "win32",
    signalTarget
  );
  signalTarget.emit("SIGINT");
  assert.deepStrictEqual(relayed, []);
  assert.strictEqual(signalTarget.listenerCount("SIGTERM"), 0);
  remove();
  assert.strictEqual(signalTarget.listenerCount("SIGINT"), 0);
}

console.log("ocr launcher signal policy tests passed");

const { spawn } = require("child_process");
const path = require("path");
const fs = require("fs");
const {
  BINARY_FILENAME,
  getPlatformPackageName,
} = require("../scripts/platform");

function createLauncherFixture() {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "ocr-launcher-test-"));
  const packageName = getPlatformPackageName();
  assert.ok(packageName, `unsupported test platform: ${process.platform}-${process.arch}`);

  for (const relativePath of [
    "bin/ocr.js",
    "scripts/platform.js",
    "scripts/version.js",
    "package.json",
  ]) {
    const destination = path.join(root, relativePath);
    fs.mkdirSync(path.dirname(destination), { recursive: true });
    fs.copyFileSync(path.join(__dirname, "..", relativePath), destination);
  }

  const packageDir = path.join(root, "node_modules", ...packageName.split("/"));
  const binaryPath = path.join(packageDir, "bin", BINARY_FILENAME);
  fs.mkdirSync(path.dirname(binaryPath), { recursive: true });
  fs.writeFileSync(
    path.join(packageDir, "package.json"),
    JSON.stringify({ name: packageName, version: "0.0.0" })
  );

  if (process.platform === "win32") {
    fs.copyFileSync(process.execPath, binaryPath);
  } else {
    fs.symlinkSync(process.execPath, binaryPath);
  }

  const home = path.join(root, "home");
  fs.mkdirSync(home);
  return {
    root,
    home,
    binaryPath,
    launcher: path.join(root, "bin", "ocr.js"),
    nodePath: path.join(root, "node_modules"),
  };
}

function writeTarget(fixture, name, source) {
  const target = path.join(fixture.root, name);
  fs.writeFileSync(target, source);
  return target;
}

function runLauncher(fixture, args, signal, readyToken) {
  return new Promise((resolve, reject) => {
    const nodePath = process.env.NODE_PATH
      ? `${fixture.nodePath}${path.delimiter}${process.env.NODE_PATH}`
      : fixture.nodePath;
    const launcher = spawn(process.execPath, [fixture.launcher, ...args], {
      stdio: ["ignore", "pipe", "pipe"],
      env: {
        ...process.env,
        HOME: fixture.home,
        USERPROFILE: fixture.home,
        NODE_PATH: nodePath,
        OCR_NO_UPDATE: "1",
      },
    });

    let output = "";
    let signalSent = !signal;
    let finished = false;

    function onData(chunk) {
      output += chunk.toString();
      if (!signalSent && output.includes(readyToken)) {
        signalSent = true;
        launcher.kill(signal);
      }
    }

    launcher.stdout.on("data", onData);
    launcher.stderr.on("data", onData);

    const guard = setTimeout(() => {
      if (finished) return;
      finished = true;
      launcher.kill("SIGKILL");
      reject(new Error(`launcher test timed out; output: ${output}`));
    }, 8000);

    launcher.on("error", (err) => {
      if (finished) return;
      finished = true;
      clearTimeout(guard);
      reject(err);
    });
    launcher.on("close", (code, receivedSignal) => {
      if (finished) return;
      finished = true;
      clearTimeout(guard);
      resolve({ code, signal: receivedSignal, output, signalSent });
    });
  });
}

async function testExitCodePropagation(fixture) {
  const target = writeTarget(fixture, "exit-23.js", "process.exit(23);\n");
  const result = await runLauncher(fixture, [target]);
  assert.strictEqual(result.code, 23, "launcher must propagate a normal exit code");
}

async function testGracefulSignalRelay(fixture, signal) {
  const ready = `TARGET_READY_${signal}`;
  const received = `TARGET_RECEIVED_${signal}`;
  const target = writeTarget(
    fixture,
    `relay-${signal}.js`,
    `process.on(${JSON.stringify(signal)}, () => {\n` +
      `  console.log(${JSON.stringify(received)});\n` +
      `  process.exit(7);\n` +
      `});\n` +
      `console.log(${JSON.stringify(ready)});\n` +
      `setTimeout(() => process.exit(3), 5000);\n`
  );
  const result = await runLauncher(fixture, [target], signal, ready);

  assert.ok(result.signalSent, `${signal}: target never reported ready`);
  assert.ok(
    result.output.includes(received),
    `${signal}: launcher did not relay the signal; output: ${result.output}`
  );
  assert.strictEqual(result.code, 7, `${signal}: child exit code was not propagated`);
}

async function testSignalDeathExitCode(fixture) {
  const ready = "TARGET_READY_FOR_SIGTERM";
  const target = writeTarget(
    fixture,
    "signal-death.js",
    `console.log(${JSON.stringify(ready)});\n` +
      `setTimeout(() => process.exit(3), 5000);\n`
  );
  const result = await runLauncher(fixture, [target], "SIGTERM", ready);

  assert.strictEqual(
    result.code,
    128 + os.constants.signals.SIGTERM,
    "launcher must map child signal death to 128 + signo"
  );
}

async function testSpawnFailure(fixture) {
  fs.unlinkSync(fixture.binaryPath);
  fs.mkdirSync(fixture.binaryPath);
  const result = await runLauncher(fixture, []);

  assert.strictEqual(result.code, 1, `spawn failure should exit 1; ${result.output}`);
  assert.ok(
    result.output.includes("Failed to start"),
    `spawn failure should be reported; output: ${result.output}`
  );
}

(async () => {
  const fixture = createLauncherFixture();
  try {
    await testExitCodePropagation(fixture);
    if (process.platform === "win32") {
      console.log("skipping POSIX signal relay cases on win32");
    } else {
      await testGracefulSignalRelay(fixture, "SIGINT");
      await testGracefulSignalRelay(fixture, "SIGTERM");
      await testSignalDeathExitCode(fixture);
    }
    await testSpawnFailure(fixture);
    console.log("ocr launcher integration tests passed");
  } finally {
    fs.rmSync(fixture.root, { recursive: true, force: true });
  }
})().catch((err) => {
  console.error(err.stack || err.message);
  process.exitCode = 1;
});
