#!/usr/bin/env node

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

"use strict";

// Exercise the actual workflow steps with shallow merge checkouts and local remotes.
const assert = require("assert").strict;
const fs = require("fs");
const os = require("os");
const path = require("path");
const { execFileSync, spawnSync } = require("child_process");
const { pathToFileURL } = require("url");
const ARTIFACT = "extensions/vscode/src/shared/providers.generated.ts";
const INPUT = "internal/llm/providers.go";
const REMOTE = "https://github.com/example/providers.git";
const workflow = fs.readFileSync(path.resolve(__dirname, "../../.github/workflows/ci.yml"), "utf8");
const job = workflow.split("\n  test:\n")[1].split("\n  windows:\n")[0];
const bash = process.env.BASH || (process.platform === "win32"
  ? path.resolve(execFileSync("git", ["--exec-path"], { encoding: "utf8" }).trim(), "../../../bin/bash.exe")
  : "/bin/bash");
let passed = 0;

function shellStep(id) {
  const lines = job.split(`        id: provider-presets-${id}\n`)[1]
    .split("        run: ")[1].split(/\r?\n/);
  const first = lines.shift();
  if (first !== "|") return first;
  const body = [];
  for (const line of lines) {
    if (line.trim() && !line.startsWith("          ")) break;
    body.push(line.slice(10));
  }
  assert.ok(body.length > 1, `missing shell body: ${id}`);
  return body.join("\n");
}

function test(name, fn) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "ocr-preset-ci-"));
  try {
    fn(root);
    passed++;
    console.log(`ok - ${name}`);
  } finally {
    assert.equal(path.dirname(path.resolve(root)), path.resolve(os.tmpdir()));
    assert.ok(path.basename(root).startsWith("ocr-preset-ci-"));
    fs.rmSync(root, { recursive: true, force: true });
  }
}

function fixture(root, options = {}) {
  const source = path.join(root, "source");
  const cwd = path.join(root, "checkout");
  const remote = path.join(root, "remote.git");
  const env = { ...process.env, LC_ALL: "C", GIT_CONFIG_NOSYSTEM: "1",
    GIT_CONFIG_GLOBAL: path.join(root, "gitconfig"), GIT_TERMINAL_PROMPT: "0" };
  for (const key of ["GH_TOKEN", "GITHUB_TOKEN", "GIT_CONFIG_COUNT", "GIT_CONFIG_KEY_0", "GIT_CONFIG_VALUE_0"]) delete env[key];
  fs.mkdirSync(source);
  let active = source;
  const git = (args, directory = active) => execFileSync("git", args, { cwd: directory, env, encoding: "utf8", stdio: ["ignore", "pipe", "pipe"] }).trim();
  const write = (file, text) => {
    fs.mkdirSync(path.dirname(path.join(active, file)), { recursive: true });
    fs.writeFileSync(path.join(active, file), text);
  };
  for (const [key, value] of Object.entries({ "user.name": "Fixture contributor", "user.email": "fixture@example.invalid",
    "commit.gpgSign": "false", "core.autocrlf": "false", "core.hooksPath": "/dev/null" })) git(["config", "--global", key, value]);
  git(["init", "--bare", remote]);
  git(["init", "-b", "feature"]);
  write(ARTIFACT, "old presets\n");
  write("extensions/vscode/src/shared/providers.ts", "// Provider facade\n");
  write(INPUT, "old presets\n");
  git(["add", "."]);
  git(["commit", "-m", "Initial source"]);
  const base = git(["rev-parse", "HEAD"]);
  for (const file of options.paths || [INPUT]) write(file, options.text || "fresh presets\n");
  if (options.deleted) {
    git(["rm", ARTIFACT]);
    write(".gitignore", `${ARTIFACT}\n`);
  }
  if (options.renamed) git(["mv", ARTIFACT, `${ARTIFACT}.old`]);
  git(["add", "."]);
  git(["commit", "--allow-empty", "-m", "PR head"]);
  const head = git(["rev-parse", "HEAD"]);
  git(["checkout", "-b", "main", base]);
  for (const file of options.basePaths || ["base-only.txt"]) write(file, "Base branch change\n");
  git(["add", "."]);
  git(["commit", "-m", "Advance base branch"]);
  git(["merge", "--no-ff", "feature", "-m", "Test merge"]);
  const merge = git(["rev-parse", "HEAD"]);
  git(["push", remote, `${head}:refs/heads/feature`, "HEAD:refs/heads/ci-fixture"]);
  git(["clone", "--depth", "2", "--branch", "ci-fixture", pathToFileURL(remote).href, cwd]);
  active = cwd;
  git(["config", `url.${pathToFileURL(remote).href}.insteadOf`, REMOTE]);
  Object.assign(env, { HEAD_SHA: head, HEAD_REF: "feature", GITHUB_REPOSITORY: "example/providers",
    GITHUB_SERVER_URL: "https://github.com", GITHUB_WORKSPACE: cwd, RUNNER_TEMP: root.replace(/\\/g, "/"),
    GENERATED: ARTIFACT, LOCAL_REMOTE: remote.replace(/\\/g, "/"), PUSH_CALLS: path.join(root, "pushes") });

  function run(id, overrides = {}) {
    const script = path.join(root, `${id}.sh`);
    const setup = `
go() {
  test "$*" = 'generate ./internal/llm' || return 9
  test -z "\${GITHUB_TOKEN:-}" || return 9
  test ! -e "$GENERATED" || return 9
  case "\${GENERATOR_MODE:-normal}" in
    noop) return 0;;
    failure) return 1;;
    directory) mkdir -p "$GENERATED"; return;;
  esac
  cp ${INPUT} "$GENERATED"
  case "\${GENERATOR_MODE:-normal}" in
    tracked) printf 'unexpected edit\\n' >> ${INPUT};;
    untracked) printf 'unexpected file\\n' > extra.txt;;
  esac
}
git() {
  if [[ "$1" == worktree && "$2" == add && "\${WORKTREE_MODE:-}" == failure ]]; then
    return 1
  fi
  for arg in "$@"; do
    if [[ "$arg" == ls-remote && "\${HEAD_QUERY_MODE:-}" == failure ]]; then
      return 1
    fi
    if [[ "$arg" == push ]]; then
      echo push >> "$PUSH_CALLS"
      test -z "\${GITHUB_TOKEN:-}" || return 9
      case "\${PUSH_MODE:-normal}" in
        advance|rewind) command git --git-dir="$LOCAL_REMOTE" update-ref refs/heads/feature "$NEXT_HEAD";;
        delete) command git --git-dir="$LOCAL_REMOTE" update-ref -d refs/heads/feature;;
        reject) return 1;;
        accepted-error) command git "$@"; return 1;;
      esac
    fi
  done
  command git "$@"
}
`;
    fs.writeFileSync(script, setup + shellStep(id));
    const output = path.join(root, `${id}.output`);
    fs.writeFileSync(output, "");
    return spawnSync(bash, ["--noprofile", "--norc", script.replace(/\\/g, "/")], {
      cwd: ["scope", "prepare", "cleanup"].includes(id) ? cwd : active,
      env: { ...env, GITHUB_OUTPUT: output, ...(id === "push" ? { GITHUB_TOKEN: "test-token-only" } : {}), ...overrides },
      encoding: "utf8", timeout: 30000,
    });
  }
  function output(id) { return fs.readFileSync(path.join(root, `${id}.output`), "utf8").trim(); }
  function prepare() {
    const result = run("prepare");
    assert.equal(result.status, 0, result.stderr);
    active = output("prepare").slice("path=".length);
    assert.equal(path.dirname(path.resolve(active)), path.resolve(root));
    env.PROVIDER_WORKTREE = active;
  }
  function regenerate(mode = "normal") {
    if (active === cwd) prepare();
    const result = run("generate", { GENERATOR_MODE: mode });
    return result.status === 0 ? run("validate") : result;
  }
  function generate() {
    const result = regenerate();
    assert.equal(result.status, 0, result.stderr);
  }
  function commitAndPush(overrides = {}) {
    const result = run("commit");
    return result.status === 0 && output("commit") === "created=true" ? run("push", overrides) : result;
  }
  function nextHead() {
    const next = git(["commit-tree", `${head}^{tree}`, "-p", head, "-m", "Concurrent update"]);
    git(["push", remote, `${next}:refs/heads/race-fixture`]);
    return next;
  }
  return { cwd, remote, env, head, base, merge, git, write, run, output, prepare, regenerate, generate, commitAndPush, nextHead,
    remoteHead: () => git(["rev-parse", "refs/heads/feature"], remote),
    pushes: () => fs.existsSync(env.PUSH_CALLS) ? fs.readFileSync(env.PUSH_CALLS, "utf8").trim().split("\n").length : 0 };
}

test("workflow reuses the test job and passes the token only to the push step", () => {
  assert.ok(!workflow.includes("  provider-presets-autofix:"));
  assert.ok(job.includes("github.event_name == 'pull_request'"));
  assert.ok(job.includes("github.event.pull_request.head.repo.full_name == github.repository"));
  assert.ok(job.includes("github.actor != 'dependabot[bot]'"));
  assert.ok(job.includes("fetch-depth: 2"));
  assert.ok(job.includes("persist-credentials: false"));
  assert.ok(!/^\s+ref:/m.test(job));
  assert.equal((workflow.match(/contents: write/g) || []).length, 1);
  assert.ok(!job.split("      - name: Push the provider catalog")[0].includes("GITHUB_TOKEN:"));
  assert.ok(job.includes("if: steps.provider-presets-commit.outputs.created == 'true'"));
  assert.ok(job.includes("if: always() && steps.provider-presets-prepare.outputs.path != ''"));
});

for (const file of [INPUT, "internal/llm/protocol.go", "internal/llm/gen/main.go", "go.mod", "go.sum", ARTIFACT]) {
  test(`regenerates for a PR change to ${file}`, (root) => {
    const f = fixture(root, { paths: [file] });
    assert.equal(f.git(["rev-parse", "--is-shallow-repository"]), "true");
    assert.equal(f.run("scope").status, 0);
    assert.equal(f.output("scope"), "needed=true");
  });
}

test("unrelated PR changes skip autofix even when the base changed providers", (root) => {
  const f = fixture(root, { paths: ["cmd/unrelated.go"], basePaths: [INPUT] });
  assert.equal(f.run("scope").status, 0);
  assert.equal(f.output("scope"), "");
  assert.equal(f.pushes(), 0);
});

test("catalog renames still trigger regeneration", (root) => {
  const f = fixture(root, { paths: [], renamed: true });
  assert.equal(f.run("scope").status, 0);
  assert.equal(f.output("scope"), "needed=true");
});

test("missing merge parents fail instead of silently skipping the path check", (root) => {
  const f = fixture(root);
  f.git(["checkout", "--detach", f.head]);
  assert.notEqual(f.run("scope").status, 0);
  assert.equal(f.output("scope"), "");
});

test("commits only the PR-head catalog and leaves the merge checkout unchanged", (root) => {
  const f = fixture(root);
  f.generate();
  assert.equal(f.output("validate"), "changed=true");
  const result = f.commitAndPush();
  assert.equal(result.status, 0, result.stderr);
  const head = f.remoteHead();
  assert.equal(f.git(["show", "-s", "--format=%P", head]), f.head);
  assert.equal(f.git(["diff", "--name-only", f.head, head]), ARTIFACT);
  assert.equal(f.git(["show", `${head}:${ARTIFACT}`]), "fresh presets");
  assert.equal(f.git(["show", "-s", "--format=%an <%ae>", head]), "github-actions[bot] <41898282+github-actions[bot]@users.noreply.github.com>");
  assert.ok(!f.git(["config", "--local", "--list"]).includes("test-token-only"));
  assert.equal(f.git(["rev-parse", "HEAD"], f.cwd), f.merge);
  assert.equal(f.git(["status", "--porcelain"], f.cwd), "");
  assert.equal(f.run("cleanup").status, 0);
  assert.ok(!fs.existsSync(f.env.PROVIDER_WORKTREE));
});

test("unchanged generation does not request a commit", (root) => {
  const f = fixture(root, { text: "old presets\n" });
  f.generate();
  assert.equal(f.output("validate"), "");
  assert.equal(f.pushes(), 0);
});

for (const mode of ["noop", "failure", "directory", "tracked", "untracked"]) {
  test(`rejects unsafe generation and cleans up: ${mode}`, (root) => {
    const f = fixture(root);
    assert.notEqual(f.regenerate(mode).status, 0);
    assert.equal(f.remoteHead(), f.head);
    assert.equal(f.pushes(), 0);
    assert.equal(f.run("cleanup").status, 0);
    assert.equal(f.git(["status", "--porcelain"], f.cwd), "");
  });
}

test("rejects a merge checkout whose PR parent differs from the event head", (root) => {
  const f = fixture(root);
  const result = f.run("prepare", { HEAD_SHA: f.base });
  assert.notEqual(result.status, 0);
  assert.ok(result.stdout.includes("does not match the event PR head"));
  assert.equal(f.output("prepare"), "");
});

test("cleans up failed worktree preparation without publishing its path", (root) => {
  const f = fixture(root);
  const result = f.run("prepare", { WORKTREE_MODE: "failure" });
  assert.notEqual(result.status, 0);
  assert.ok(result.stdout.includes("Could not prepare"));
  assert.equal(f.output("prepare"), "");
  assert.equal(fs.readdirSync(root).filter((name) => name.startsWith("provider-presets.")).length, 0);
});

test("restores a deleted and ignored catalog without scripts in the PR branch", (root) => {
  const f = fixture(root, { paths: [], deleted: true });
  assert.equal(f.run("scope").status, 0);
  assert.equal(f.output("scope"), "needed=true");
  f.generate();
  assert.equal(f.commitAndPush().status, 0);
  assert.equal(f.git(["diff", "--name-only", f.head, f.remoteHead()]), ARTIFACT);
});

test("refuses unrelated staged files before committing", (root) => {
  const f = fixture(root);
  f.generate();
  f.write(INPUT, "another edit\n");
  f.git(["add", INPUT]);
  assert.notEqual(f.run("commit").status, 0);
  assert.equal(f.pushes(), 0);
});

test("skips a PR branch that advanced before the commit step", (root) => {
  const f = fixture(root);
  f.generate();
  const next = f.nextHead();
  f.git(["update-ref", "refs/heads/feature", next], f.remote);
  const result = f.commitAndPush();
  assert.equal(result.status, 0, result.stderr);
  assert.ok(result.stdout.includes("outdated autofix"));
  assert.equal(f.remoteHead(), next);
  assert.equal(f.pushes(), 0);
});

test("reports an unavailable remote head before committing or pushing", (root) => {
  const f = fixture(root);
  f.generate();
  const result = f.run("commit", { HEAD_QUERY_MODE: "failure" });
  assert.notEqual(result.status, 0);
  assert.ok(result.stdout.includes("Could not read the current PR head"));
  assert.equal(f.output("commit"), "");
  assert.equal(f.remoteHead(), f.head);
  assert.equal(f.pushes(), 0);
});

for (const mode of ["advance", "rewind", "delete", "reject", "accepted-error"]) {
  test(`does not retry or rebase after push-time ${mode}`, (root) => {
    const f = fixture(root);
    f.generate();
    const next = mode === "rewind" ? f.base : f.nextHead();
    const result = f.commitAndPush({ PUSH_MODE: mode, NEXT_HEAD: next });
    assert.notEqual(result.status, 0);
    assert.ok(result.stdout.includes("Could not confirm the autofix push"));
    assert.equal(f.pushes(), 1);
    if (mode === "delete") assert.equal(f.git(["ls-remote", "--heads", f.remote, "refs/heads/feature"]), "");
    else if (mode === "advance" || mode === "rewind") assert.equal(f.remoteHead(), next);
    else if (mode === "reject") assert.equal(f.remoteHead(), f.head);
    else assert.equal(f.git(["show", "-s", "--format=%P", f.remoteHead()]), f.head);
  });
}

test("creating a commit needs no token, while pushing requires one", (root) => {
  const f = fixture(root);
  f.generate();
  assert.equal(f.run("commit").status, 0);
  assert.equal(f.output("commit"), "created=true");
  assert.notEqual(f.run("push", { GITHUB_TOKEN: "" }).status, 0);
  assert.equal(f.remoteHead(), f.head);
  assert.equal(f.pushes(), 0);
});

console.log(`All ${passed} provider autofix workflow tests passed.`);
