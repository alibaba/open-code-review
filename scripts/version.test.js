"use strict";

const assert = require("assert");

const {
  parseVersionOutput,
  semverGt,
  shouldShowUpdateHint,
} = require("./version");

assert.strictEqual(
  parseVersionOutput("open-code-review v1.8.6 (1b193db35) darwin/arm64"),
  "1.8.6"
);
assert.strictEqual(parseVersionOutput("version unavailable"), null);
assert.strictEqual(
  parseVersionOutput("open-code-review v1.8.6-beta.1+darwin.arm64"),
  "1.8.6-beta.1+darwin.arm64"
);

assert.strictEqual(semverGt("1.8.7", "1.8.6"), true);
assert.strictEqual(semverGt("1.8.6", "1.8.6"), false);
assert.strictEqual(semverGt("1.8.5", "1.8.6"), false);
assert.strictEqual(semverGt("1.8.6", "1.8.6-beta.1"), true);
assert.strictEqual(semverGt("1.8.6+build.2", "1.8.6+build.1"), false);
assert.strictEqual(semverGt("1.8.6", "1.8.6-beta.1+build.1"), true);
assert.strictEqual(semverGt("not-a-version", "1.8.6"), false);

assert.strictEqual(shouldShowUpdateHint("1.8.7", "1.8.6"), true);
assert.strictEqual(shouldShowUpdateHint("1.8.6", "1.8.6"), false);
assert.strictEqual(shouldShowUpdateHint("1.8.5", "1.8.6"), false);
assert.strictEqual(shouldShowUpdateHint("not-a-version", "1.8.6"), false);
assert.strictEqual(shouldShowUpdateHint("1.8.7", null), false);
assert.strictEqual(shouldShowUpdateHint("1.8.7", "not-a-version"), false);
assert.strictEqual(shouldShowUpdateHint("1.8.6+new", "1.8.6+old"), false);

console.log("version tests passed");
