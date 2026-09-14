#!/usr/bin/env node

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

"use strict";

// Plain Node + assert, matching the other contract tests and the Node 14 floor.
const assert = require("assert").strict;
const fs = require("fs");
const os = require("os");
const path = require("path");
const { ARTIFACT, BOT_NAME, BOT_EMAIL, autofix, runGit } = require("./autofix-provider-presets");

const REMOTE_URL = "https://github.com/example/providers.git";
let passed = 0;
let failed = 0;

function test(name, fn) {
  const cleanups = [];
  let failure;
  try {
    fn({ after: (cleanup) => cleanups.push(cleanup) });
  } catch (error) {
    failure = error;
  } finally {
    for (const cleanup of cleanups) {
      try { cleanup(); } catch (error) { failure = failure || error; }
    }
  }
  if (failure) {
    failed += 1;
    console.error(`not ok - ${name}: ${failure.stack}`);
  } else {
    passed += 1;
    console.log(`ok - ${name}`);
  }
}

function fixture(t) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "ocr-provider-autofix-"));
  t.after(() => {
    assert.equal(path.dirname(path.resolve(root)), path.resolve(os.tmpdir()));
    assert.ok(path.basename(root).startsWith("ocr-provider-autofix-"));
    fs.rmSync(root, { recursive: true, force: true });
  });
  const remote = path.join(root, "remote.git");
  const cwd = path.join(root, "checkout");
  const baseEnv = { ...process.env, GIT_TERMINAL_PROMPT: "0" };
  delete baseEnv.GH_TOKEN;
  delete baseEnv.GITHUB_TOKEN;
  const local = (args, directory = cwd) => runGit(args, { cwd: directory, env: baseEnv });
  fs.mkdirSync(cwd);
  local(["init", "--bare", remote], root);
  local(["init", "--initial-branch=feature"]);
  local(["config", "user.name", "Fixture contributor"]);
  local(["config", "user.email", "fixture@example.invalid"]);
  local(["config", "commit.gpgSign", "false"]);
  local(["config", "core.autocrlf", "false"]);
  local(["config", "core.hooksPath", "/dev/null"]);
  fs.mkdirSync(path.dirname(path.join(cwd, ARTIFACT)), { recursive: true });
  fs.writeFileSync(path.join(cwd, ARTIFACT), "old presets\n");
  fs.writeFileSync(path.join(cwd, path.dirname(ARTIFACT), "providers.ts"), "// Provider facade\n");
  fs.writeFileSync(path.join(cwd, "registry.txt"), "provider source\n");
  local(["add", "."]);
  local(["commit", "-m", "Initial source"]);
  local(["push", remote, "HEAD:refs/heads/feature"]);
  const head = local(["rev-parse", "HEAD"]);
  local(["checkout", "--detach", head]);
  const env = {
    ...baseEnv,
    GITHUB_EVENT_NAME: "pull_request",
    GITHUB_REPOSITORY: "example/providers",
    GITHUB_ACTOR: "contributor",
    OCR_HEAD_REPOSITORY: "example/providers",
    OCR_HEAD_REF: "feature",
    OCR_BASE_REF: "main",
    OCR_HEAD_SHA: head,
    GH_TOKEN: "test-token-only",
  };
  let beforePush = () => {};
  let afterPush = () => {};
  let unreachable = false;
  const calls = [];
  const git = (args, options) => {
    calls.push(args);
    if (unreachable && args.includes("ls-remote")) throw new Error("Network unavailable");
    assert.equal(options.env.GH_TOKEN, undefined);
    assert.equal(options.env.GITHUB_TOKEN, undefined);
    const push = args.includes("push");
    if (push) {
      assert.ok(args.includes(`--force-with-lease=refs/heads/feature:${env.OCR_HEAD_SHA}`));
      assert.ok(!args.includes("--force"));
      assert.equal(options.env.GIT_CONFIG_KEY_0, `http.${REMOTE_URL}.extraheader`);
      beforePush();
    } else {
      assert.equal(options.env.GIT_CONFIG_VALUE_0, baseEnv.GIT_CONFIG_VALUE_0);
    }
    const output = runGit(args.map((arg) => arg === REMOTE_URL ? remote : arg), options);
    if (push) afterPush();
    return output;
  };
  return {
    cwd, remote, head, env, local, calls,
    write: () => fs.writeFileSync(path.join(cwd, ARTIFACT), "fresh presets\n"),
    run: (overrides = {}) => autofix({ cwd, env: { ...env, ...overrides }, git }),
    remoteHead: () => local(["rev-parse", "refs/heads/feature"], remote),
    beforePush: (fn) => { beforePush = fn; },
    afterPush: (fn) => { afterPush = fn; },
    failRemoteLookups: () => { unreachable = true; },
  };
}

test("unchanged presets do not create a commit", (t) => {
  const f = fixture(t);
  assert.equal(f.run(), "unchanged");
  assert.equal(f.remoteHead(), f.head);
  assert.ok(!f.calls.some((args) => args.includes("commit") || args.includes("push")));
});

test("pushes only the regenerated artifact with the standard bot identity", (t) => {
  const f = fixture(t);
  f.write();
  assert.equal(f.run(), "pushed");
  const commit = f.remoteHead();
  assert.equal(f.local(["show", "-s", "--format=%P", commit]), f.head);
  assert.equal(f.local(["show", "-s", "--format=%an <%ae>", commit]), `${BOT_NAME} <${BOT_EMAIL}>`);
  assert.equal(f.local(["diff", "--name-only", f.head, commit]), ARTIFACT);
  assert.equal(f.local(["show", `${commit}:${ARTIFACT}`]), "fresh presets");
  assert.equal(f.local(["status", "--porcelain"]), "");
  assert.ok(!f.local(["config", "--local", "--list"]).includes("test-token-only"));
});

test("restores an artifact deleted by the PR, including its untracked replacement", (t) => {
  const f = fixture(t);
  f.local(["rm", "--", ARTIFACT]);
  f.local(["commit", "-m", "Remove artifact"]);
  f.local(["push", f.remote, "HEAD:refs/heads/feature"]);
  f.env.OCR_HEAD_SHA = f.local(["rev-parse", "HEAD"]);
  f.write();
  assert.equal(f.run(), "pushed");
  assert.equal(f.local(["show", `${f.remoteHead()}:${ARTIFACT}`]), "fresh presets");
});

for (const overrides of [
  { GITHUB_EVENT_NAME: "push" },
  { OCR_HEAD_REPOSITORY: "fork/providers" },
  { GITHUB_ACTOR: "dependabot[bot]" },
]) {
  test(`skips an ineligible event: ${JSON.stringify(overrides)}`, () => {
    assert.equal(autofix({
      env: { GITHUB_EVENT_NAME: "pull_request", GITHUB_REPOSITORY: "example/providers", OCR_HEAD_REPOSITORY: "example/providers", ...overrides },
      git: () => { throw new Error("Git must not run"); },
    }), "skipped");
  });
}

test("rejects a wrong checkout or the base branch before committing", (t) => {
  const f = fixture(t);
  f.write();
  assert.throws(() => f.run({ OCR_HEAD_SHA: "0".repeat(40) }), /Checkout does not match/);
  assert.throws(() => f.run({ OCR_HEAD_REF: "main" }), /invalid pull request/);
  assert.throws(() => f.run({ OCR_HEAD_REF: "invalid..branch" }));
  assert.equal(f.remoteHead(), f.head);
});

test("rejects another GitHub server before making a request", (t) => {
  const f = fixture(t);
  assert.throws(() => f.run({ GITHUB_SERVER_URL: "https://github.example.test" }), /refusing another server/);
  assert.equal(f.calls.length, 0);
});

test("rejects a missing artifact or missing push token", (t) => {
  const f = fixture(t);
  fs.unlinkSync(path.join(f.cwd, ARTIFACT));
  assert.throws(() => f.run(), /artifact is missing.*go generate/);
  f.write();
  assert.throws(() => f.run({ GH_TOKEN: "" }), /GH_TOKEN is required/);
  assert.equal(f.remoteHead(), f.head);
});

test("rejects executable permissions on the generated artifact", (t) => {
  const f = fixture(t);
  f.write();
  fs.chmodSync(path.join(f.cwd, ARTIFACT), 0o755);
  f.local(["add", "--", ARTIFACT]);
  f.local(["update-index", "--chmod=+x", ARTIFACT]);
  assert.throws(() => f.run(), /ordinary file permissions/);
  assert.equal(f.remoteHead(), f.head);
});

test("rejects unrelated staged, unstaged, and untracked changes", (t) => {
  const f = fixture(t);
  f.write();
  fs.writeFileSync(path.join(f.cwd, "registry.txt"), "unexpected edit\n");
  assert.throws(() => f.run(), /outside the provider artifact/);
  f.local(["add", "registry.txt"]);
  assert.throws(() => f.run(), /outside the provider artifact/);
  f.local(["restore", "--staged", "--worktree", "registry.txt"]);
  fs.writeFileSync(path.join(f.cwd, "unexpected.txt"), "unexpected file\n");
  assert.throws(() => f.run(), /outside the provider artifact/);
  assert.equal(f.remoteHead(), f.head);
});

test("skips a run whose source branch has already advanced", (t) => {
  const f = fixture(t);
  f.local(["commit", "--allow-empty", "-m", "Concurrent update"]);
  const newer = f.local(["rev-parse", "HEAD"]);
  f.local(["push", f.remote, "HEAD:refs/heads/feature"]);
  f.local(["checkout", "--detach", f.head]);
  f.write();
  assert.equal(f.run(), "superseded");
  assert.equal(f.remoteHead(), newer);
});

test("never overwrites a concurrent update that arrives during push", (t) => {
  const f = fixture(t);
  f.write();
  let newer;
  f.beforePush(() => {
    const tree = f.local(["rev-parse", `${f.head}^{tree}`]);
    newer = f.local(["commit-tree", tree, "-p", f.head, "-m", "Concurrent update"]);
    f.local(["push", f.remote, `${newer}:refs/heads/feature`]);
  });
  assert.throws(() => f.run(), /Could not push the autofix/);
  assert.equal(f.remoteHead(), newer);
});

test("never recreates a branch deleted during the run", (t) => {
  const f = fixture(t);
  f.write();
  f.beforePush(() => f.local(["update-ref", "-d", "refs/heads/feature"], f.remote));
  assert.throws(() => f.run(), /Could not push the autofix/);
  assert.equal(f.local(["ls-remote", "--heads", f.remote, "refs/heads/feature"]), "");
});

test("never restores commits that the contributor removed by rewinding the branch", (t) => {
  const f = fixture(t);
  f.local(["commit", "--allow-empty", "-m", "Contributor update"]);
  f.env.OCR_HEAD_SHA = f.local(["rev-parse", "HEAD"]);
  f.local(["push", f.remote, "HEAD:refs/heads/feature"]);
  f.write();
  f.beforePush(() => f.local(["update-ref", "refs/heads/feature", f.head, f.env.OCR_HEAD_SHA], f.remote));
  assert.throws(() => f.run(), /Could not push the autofix/);
  assert.equal(f.remoteHead(), f.head);
});

test("reports a permission failure without retrying or exposing a token", (t) => {
  const f = fixture(t);
  f.write();
  f.beforePush(() => { throw new Error("Push denied"); });
  assert.throws(() => f.run(), (error) => /Could not push/.test(error.message) && !error.message.includes("test-token-only"));
  assert.equal(f.calls.filter((args) => args.includes("push")).length, 1);
  assert.equal(f.remoteHead(), f.head);
});

test("recognizes a successful server update after a connection error", (t) => {
  const f = fixture(t);
  f.write();
  f.afterPush(() => { throw new Error("Connection reset after acceptance"); });
  assert.equal(f.run(), "pushed");
  assert.equal(f.calls.filter((args) => args.includes("push")).length, 1);
});

test("keeps recovery instructions when push and its verification both lose the network", (t) => {
  const f = fixture(t);
  f.write();
  f.beforePush(() => {
    f.failRemoteLookups();
    throw new Error("Network unavailable");
  });
  assert.throws(() => f.run(), /Could not push the autofix.*go generate/);
  assert.equal(f.remoteHead(), f.head);
});

test("reports an accepted push whose final verification loses the network", (t) => {
  const f = fixture(t);
  f.write();
  f.afterPush(() => f.failRemoteLookups());
  assert.throws(() => f.run(), /successful push.*Check the latest PR head and CI/);
  assert.notEqual(f.remoteHead(), f.head);
  assert.equal(f.calls.filter((args) => args.includes("push")).length, 1);
});

if (failed) {
  console.error(`${failed} provider autofix test(s) failed.`);
  process.exitCode = 1;
} else {
  console.log(`All ${passed} provider autofix tests passed.`);
}
