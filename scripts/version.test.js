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

assert.strictEqual(semverGt("1.8.7", "1.8.6"), true);
assert.strictEqual(semverGt("1.8.6", "1.8.6"), false);
assert.strictEqual(semverGt("1.8.5", "1.8.6"), false);
assert.strictEqual(semverGt("1.8.6", "1.8.6-beta.1"), true);

assert.strictEqual(shouldShowUpdateHint("1.8.7", "1.8.6"), true);
assert.strictEqual(shouldShowUpdateHint("1.8.6", "1.8.6"), false);
assert.strictEqual(shouldShowUpdateHint("1.8.5", "1.8.6"), false);
assert.strictEqual(shouldShowUpdateHint("not-a-version", "1.8.6"), false);
assert.strictEqual(shouldShowUpdateHint("1.8.7", null), true);

console.log("version tests passed");
