// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

"use strict";

const assert = require("assert");
const fs = require("fs");
const os = require("os");
const path = require("path");
const { spawnSync } = require("child_process");
const { resolvePolicySHA, preserveActionScripts, prepareProjectPlugins } = require("./project-plugins");

function fixture() {
  const temporary = fs.mkdtempSync(path.join(os.tmpdir(), "ocr-policy-test-"));
  const workspace = path.join(temporary, "repo & literal %VALUE% $NAME");
  fs.mkdirSync(path.join(workspace, ".opencodereview"), { recursive: true });
  const git = (...args) => {
    const result = spawnSync("git", ["-C", workspace, ...args], { encoding: "utf8" });
    assert.strictEqual(result.status, 0, result.stderr);
    return result.stdout.trim();
  };
  const write = (name, content) => fs.writeFileSync(path.join(workspace, name), content);
  git("init");
  git("config", "user.name", "Policy Test");
  git("config", "user.email", "policy@example.invalid");
  git("config", "core.autocrlf", "false");
  write(".opencodereview/plugins.json", '{"schema_version":1,"plugins":[]}\n');
  write(".opencodereview/plugins.lock.json", '{}\n');
  write(".opencodereview/rule.json", '{"rules":[{"path":"**/*","rule":"review.md"}]}\n');
  write("review.md", "Review carefully.\n");
  const commit = () => {
    git("add", ".");
    git("-c", "commit.gpgsign=false", "commit", "--no-verify", "-m", "policy");
    return git("rev-parse", "HEAD");
  };
  const args = { workspace, temporary, policySHA: commit(), ocrBinary: process.execPath };
  const calls = [];
  let identity = "a".repeat(64);
  let failure = "";
  args.run = (command, argv, options) => {
    if (command === "git") return spawnSync(command, argv, options);
    assert.strictEqual(command, args.ocrBinary);
    calls.push({ argv, options });
    assert.strictEqual(options.env.HOME, options.env.USERPROFILE);
    assert.ok(options.env.HOME.startsWith(path.join(temporary, "ocr-project-home-")));
    assert.deepStrictEqual(argv.slice(-2), ["--repo", fs.realpathSync(workspace)]);
    if (argv[1] === failure) return { status: 1, stderr: "locked plugin unavailable or unauthorized" };
    return { status: 0, stdout: argv[1] === "preflight" ? JSON.stringify(identity) : "" };
  };
  return {
    args, calls, git, write, commit,
    identity: value => { identity = value; },
    failure: value => { failure = value; },
    close: () => fs.rmSync(temporary, { recursive: true, force: true }),
  };
}

async function main() {
  const sha = "a".repeat(40);
  let fetched = 0;
  const github = { rest: { pulls: { get: async args => {
    fetched++;
    assert.deepStrictEqual(args, { owner: "owner", repo: "repo", pull_number: 7 });
    return { data: { base: { sha } } };
  } } } };
  const context = { repo: { owner: "owner", repo: "repo" }, payload: {} };
  assert.strictEqual(await resolvePolicySHA({ github, context, prNumber: "7" }), sha);
  assert.strictEqual(fetched, 1);
  context.payload.pull_request = { number: 7, base: { sha }, head: { sha: "b".repeat(40) } };
  assert.strictEqual(await resolvePolicySHA({ github, context, prNumber: "7" }), sha);
  assert.strictEqual(await resolvePolicySHA({ github, context, policySHA: "C".repeat(40) }), "c".repeat(40));
  assert.strictEqual(fetched, 1);
  await assert.rejects(resolvePolicySHA({ github, context, policySHA: "main" }), {
    message: "policy_sha must be a full 40-character commit SHA",
  });
  context.payload.pull_request.number = 8;
  assert.strictEqual(await resolvePolicySHA({ github, context, prNumber: "7" }), sha);
  assert.strictEqual(fetched, 2, "an explicit PR override must resolve that PR's base");

  const emptyContext = { ...context, payload: {} };
  const resolutionError = { message: "Could not resolve a valid base commit SHA for PR #7" };
  for (const data of [undefined, {}, { base: {} }, { base: { sha: "invalid" } }, { base: { sha: 123 } }]) {
    const invalidAPI = { rest: { pulls: { get: async () => ({ data }) } } };
    await assert.rejects(resolvePolicySHA({ github: invalidAPI, context: emptyContext, prNumber: "7" }), resolutionError);
  }
  const invalidPayload = { ...context, payload: { pull_request: { number: 7, base: { sha: "invalid" } } } };
  await assert.rejects(resolvePolicySHA({ github, context: invalidPayload, prNumber: "7" }), resolutionError);
  assert.strictEqual(fetched, 2, "invalid explicit or payload SHAs must not fall back to an API lookup");
  const apiFailure = new Error("API request failed");
  const failingAPI = { rest: { pulls: { get: async () => { throw apiFailure; } } } };
  await assert.rejects(resolvePolicySHA({ github: failingAPI, context: emptyContext, prNumber: "7" }), error => error === apiFailure);

  const f = fixture();
  try {
    assert.throws(() => prepareProjectPlugins({ ...f.args, ocrBinary: undefined }), /absolute OCR native binary/);
    assert.throws(() => prepareProjectPlugins({ ...f.args, ocrBinary: "ocr.cmd" }), /absolute OCR native binary/);
    const bin = path.join(f.args.temporary, "bin & literal %VALUE%");
    fs.mkdirSync(bin);
    const native = path.join(bin, path.basename(process.execPath));
    fs.copyFileSync(process.execPath, native);
    fs.chmodSync(native, 0o755);
    const preload = path.join(bin, "cli.cjs");
    fs.writeFileSync(preload, `
const fs = require("fs");
const path = require("path");
const args = process.argv.slice(2);
if (args[0] === "sync") fs.writeFileSync(path.join(process.env.HOME, "installed"), JSON.stringify(args));
if (args[0] === "preflight") process.stdout.write(JSON.stringify("a".repeat(64)));
process.exit(0);
`);
    const originalOptions = process.env.NODE_OPTIONS;
    try {
      process.env.NODE_OPTIONS = `--require ${JSON.stringify(preload)}`;
      const result = prepareProjectPlugins({ ...f.args, ocrBinary: native, run: undefined });
      const args = JSON.parse(fs.readFileSync(path.join(result.home, "installed"), "utf8"));
      assert.deepStrictEqual(args.slice(0, 2), ["sync", "--frozen"]);
      assert.strictEqual(args[args.length - 1], fs.realpathSync(f.args.workspace));
    } finally {
      if (originalOptions === undefined) delete process.env.NODE_OPTIONS;
      else process.env.NODE_OPTIONS = originalOptions;
    }
    const localAction = path.join(f.args.temporary, "local-action");
    const relative = "scripts/github-actions";
    fs.mkdirSync(path.join(localAction, relative), { recursive: true });
    for (const name of ["project-plugins.js", "post-review-comments.js"]) {
      fs.copyFileSync(path.join(__dirname, name), path.join(localAction, relative, name));
    }
    const saved = preserveActionScripts(localAction, f.args.temporary);
    fs.rmSync(localAction, { recursive: true });
    assert.strictEqual(typeof require(path.join(saved, relative, "project-plugins.js")).prepareProjectPlugins, "function");
    assert.ok(Object.keys(require(path.join(saved, relative, "post-review-comments.js"))).length > 0);
    const first = prepareProjectPlugins(f.args);
    assert.deepStrictEqual(f.calls.map(c => c.argv[1]), ["sync", "preflight"]);
    assert.ok(f.calls[0].argv.includes("--frozen"));
    assert.ok(f.calls[1].argv.includes("--fingerprint"));
    assert.strictEqual(f.calls[0].options.env.HOME, first.home);
    f.calls.length = 0;
    const second = prepareProjectPlugins({ ...f.args, allowExec: true });
    assert.deepStrictEqual(f.calls.map(c => c.argv[1]), ["sync", "trust", "preflight"]);
    assert.notStrictEqual(first.home, second.home);
    assert.strictEqual(first.fingerprint, second.fingerprint, "identity must survive fresh runners");
    f.identity("b".repeat(64));
    assert.notStrictEqual(prepareProjectPlugins(f.args).fingerprint, first.fingerprint);
    f.identity("a".repeat(64));
    f.write("review.md", "Changed referenced rule.\n");
    f.args.policySHA = f.commit();
    assert.notStrictEqual(prepareProjectPlugins(f.args).fingerprint, first.fingerprint);

    for (const phase of ["sync", "trust", "preflight"]) {
      f.failure(phase);
      f.calls.length = 0;
      assert.throws(() => prepareProjectPlugins({ ...f.args, allowExec: true }), /unavailable or unauthorized/);
      assert.strictEqual(f.calls.at(-1).argv[1], phase, "failed preflight must stop the job");
      assert.ok(!fs.existsSync(f.calls[0].options.env.HOME), "failed installs must remove their private home");
    }
    f.failure("");
    f.identity("invalid");
    assert.throws(() => prepareProjectPlugins(f.args), /verified project plugin fingerprint/);
    f.identity("a".repeat(64));

    const beforeConversion = prepareProjectPlugins(f.args).fingerprint;
    f.git("config", "core.autocrlf", "true");
    for (const name of [".opencodereview/plugins.json", ".opencodereview/plugins.lock.json", ".opencodereview/rule.json", "review.md"]) {
      const file = path.join(f.args.workspace, name);
      fs.writeFileSync(file, fs.readFileSync(file, "utf8").replace(/\r?\n/g, "\r\n"));
    }
    assert.strictEqual(prepareProjectPlugins(f.args).fingerprint, beforeConversion, "checkout EOL conversion must preserve policy identity");

    f.calls.length = 0;
    assert.throws(() => prepareProjectPlugins({ ...f.args, policySHA: "d".repeat(40) }), /trusted commit/);
    assert.throws(() => prepareProjectPlugins({ ...f.args, rule: "../outside.json" }), /inside the trusted checkout/);
    f.write("extra.json", "{}");
    assert.throws(() => prepareProjectPlugins({ ...f.args, rule: "extra.json" }), /not committed/);
    f.write("review.md", "PR head rule\n");
    assert.throws(() => prepareProjectPlugins(f.args), /differs from the trusted commit/);
    assert.strictEqual(f.calls.length, 0, "untrusted policy must fail before invoking OCR");
    f.git("checkout", "--", "review.md");
    fs.unlinkSync(path.join(f.args.workspace, ".opencodereview/plugins.lock.json"));
    f.args.policySHA = f.commit();
    assert.throws(() => prepareProjectPlugins(f.args), /not committed/);
  } finally {
    f.close();
  }
  console.log("project plugin policy tests passed");
}

main().catch(error => { console.error(error); process.exitCode = 1; });
