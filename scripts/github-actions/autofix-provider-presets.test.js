#!/usr/bin/env node

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

"use strict";

// Execute the CI shell steps with a generator fixture and real local Git remotes.
const assert = require("assert").strict;
const fs = require("fs");
const os = require("os");
const path = require("path");
const { execFileSync, spawnSync } = require("child_process");
const { pathToFileURL } = require("url");
const ARTIFACT = "extensions/vscode/src/shared/providers.generated.ts";
const REMOTE = "https://github.com/example/providers.git";
const workflow = fs.readFileSync(path.resolve(__dirname, "../../.github/workflows/ci.yml"), "utf8");
const autofix = workflow.split("  provider-presets-autofix:\n")[1].split("\n  test:\n")[0];
const bash = process.env.BASH || (process.platform === "win32"
  ? path.resolve(execFileSync("git", ["--exec-path"], { encoding: "utf8" }).trim(), "../../../bin/bash.exe")
  : "/bin/bash");
let passed = 0;

function shellStep(id) {
  const lines = autofix.split(`        id: provider-presets-${id}\n`)[1]
    .split("        run: |\n")[1].split(/\r?\n/);
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

function fixture(root) {
  const cwd = path.join(root, "checkout");
  const remote = path.join(root, "remote.git");
  const env = { ...process.env, LC_ALL: "C", GIT_CONFIG_NOSYSTEM: "1",
    GIT_CONFIG_GLOBAL: path.join(root, "gitconfig"), GIT_TERMINAL_PROMPT: "0" };
  for (const key of ["GH_TOKEN", "GITHUB_TOKEN", "GIT_CONFIG_COUNT", "GIT_CONFIG_KEY_0", "GIT_CONFIG_VALUE_0"]) delete env[key];
  fs.mkdirSync(cwd);
  const git = (args, directory = cwd) => execFileSync("git", args, { cwd: directory, env, encoding: "utf8", stdio: ["ignore", "pipe", "pipe"] }).trim();
  const write = (file, text) => {
    fs.mkdirSync(path.dirname(path.join(cwd, file)), { recursive: true });
    fs.writeFileSync(path.join(cwd, file), text);
  };
  git(["init", "--bare", remote]);
  git(["init", "-b", "feature"]);
  for (const [key, value] of Object.entries({ "user.name": "Fixture contributor", "user.email": "fixture@example.invalid",
    "commit.gpgSign": "false", "core.autocrlf": "false", "core.hooksPath": "/dev/null" })) git(["config", key, value]);
  write(ARTIFACT, "old presets\n");
  write("extensions/vscode/src/shared/providers.ts", "// Provider facade\n");
  write("registry.txt", "fresh presets\n");
  git(["add", "."]);
  git(["commit", "-m", "Initial source"]);
  const base = git(["rev-parse", "HEAD"]);
  git(["commit", "--allow-empty", "-m", "PR head"]);
  const head = git(["rev-parse", "HEAD"]);
  git(["push", remote, "HEAD:refs/heads/feature"]);
  git(["config", `url.${pathToFileURL(remote).href}.insteadOf`, REMOTE]);
  git(["checkout", "--detach", head]);
  Object.assign(env, { HEAD_SHA: head, HEAD_REF: "feature", GITHUB_REPOSITORY: "example/providers",
    GITHUB_SERVER_URL: "https://github.com", GITHUB_WORKSPACE: cwd, GITHUB_OUTPUT: path.join(root, "output"),
    GENERATED: ARTIFACT, LOCAL_REMOTE: remote.replace(/\\/g, "/"), PUSH_CALLS: path.join(root, "pushes") });

  function run(id, overrides = {}) {
    const script = path.join(root, `${id}.sh`);
    const setup = `
go() {
  test "$*" = 'generate ./internal/llm' || return 9
  test -z "\${GH_TOKEN:-}" || return 9
  test ! -e "$GENERATED" || return 9
  case "\${GENERATOR_MODE:-normal}" in
    noop) return 0;;
    failure) return 1;;
    directory) mkdir -p "$GENERATED"; return;;
  esac
  cp registry.txt "$GENERATED"
  case "\${GENERATOR_MODE:-normal}" in
    tracked) printf 'unexpected edit\\n' >> registry.txt;;
    untracked) printf 'unexpected file\\n' > extra.txt;;
  esac
}
git() {
  for arg in "$@"; do
    if [[ "$arg" == push ]]; then
      echo push >> "$PUSH_CALLS"
      test -z "\${GH_TOKEN:-}" || return 9
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
    return spawnSync(bash, ["--noprofile", "--norc", script.replace(/\\/g, "/")], {
      cwd, env: { ...env, ...(id === "commit" ? { GH_TOKEN: "test-token-only" } : {}), ...overrides },
      encoding: "utf8", timeout: 30000,
    });
  }
  function generate(mode = "normal") {
    const result = run("generate", { GENERATOR_MODE: mode });
    assert.equal(result.status, 0, result.stderr);
  }
  function nextHead() {
    const next = git(["commit-tree", `${head}^{tree}`, "-p", head, "-m", "Concurrent update"]);
    git(["push", remote, `${next}:refs/heads/race-fixture`]);
    return next;
  }
  return { cwd, remote, env, head, base, git, write, run, generate, nextHead,
    remoteHead: () => git(["rev-parse", "refs/heads/feature"], remote),
    changed: () => fs.readFileSync(env.GITHUB_OUTPUT, "utf8").trim(),
    pushes: () => fs.existsSync(env.PUSH_CALLS) ? fs.readFileSync(env.PUSH_CALLS, "utf8").trim().split("\n").length : 0 };
}

test("workflow separates PR-head writes from merge-ref tests", () => {
  assert.ok(autofix.includes("github.event_name == 'pull_request'"));
  assert.ok(autofix.includes("github.event.pull_request.head.repo.full_name == github.repository"));
  assert.ok(autofix.includes("github.actor != 'dependabot[bot]'"));
  assert.ok(autofix.includes("ref: ${{ github.event.pull_request.head.sha }}"));
  assert.ok(autofix.includes("persist-credentials: false"));
  assert.ok(autofix.includes("if: steps.provider-presets-generate.outputs.changed == 'true'"));
  assert.ok(!autofix.split("      - name: Commit provider presets")[0].includes("GH_TOKEN:"));
  assert.equal((workflow.match(/contents: write/g) || []).length, 1);
  assert.ok(!workflow.split("\n  test:\n")[1].split("\n  windows:\n")[0].includes("ref:"));
});

test("commits exactly the regenerated catalog on the PR head with the standard bot identity", (root) => {
  const f = fixture(root);
  f.generate();
  assert.equal(f.changed(), "changed=true");
  const result = f.run("commit");
  assert.equal(result.status, 0, result.stderr);
  const head = f.remoteHead();
  assert.equal(f.git(["show", "-s", "--format=%P", head]), f.head);
  assert.equal(f.git(["diff", "--name-only", f.head, head]), ARTIFACT);
  assert.equal(f.git(["show", `${head}:${ARTIFACT}`]), "fresh presets");
  assert.equal(f.git(["show", "-s", "--format=%an <%ae>", head]), "github-actions[bot] <41898282+github-actions[bot]@users.noreply.github.com>");
  assert.ok(!f.git(["config", "--local", "--list"]).includes("test-token-only"));
});

test("unchanged generation does not request a commit", (root) => {
  const f = fixture(root);
  f.write("registry.txt", "old presets\n");
  f.git(["add", "registry.txt"]);
  f.git(["commit", "-m", "Matching catalog"]);
  f.env.HEAD_SHA = f.git(["rev-parse", "HEAD"]);
  f.generate();
  assert.equal(f.changed(), "changed=false");
  assert.equal(f.pushes(), 0);
});

for (const mode of ["noop", "failure", "directory", "tracked", "untracked"]) {
  test(`rejects unsafe generation: ${mode}`, (root) => {
    const f = fixture(root);
    assert.notEqual(f.run("generate", { GENERATOR_MODE: mode }).status, 0);
    assert.equal(f.remoteHead(), f.head);
    assert.equal(f.pushes(), 0);
  });
}

test("rejects a checkout that differs from the event head", (root) => {
  const f = fixture(root);
  f.git(["checkout", "--detach", f.base]);
  assert.notEqual(f.run("generate").status, 0);
  assert.equal(f.remoteHead(), f.head);
});

test("restores a deleted and ignored catalog without needing scripts in the PR branch", (root) => {
  const f = fixture(root);
  f.git(["rm", ARTIFACT]);
  f.write(".gitignore", `${ARTIFACT}\n`);
  f.git(["add", ".gitignore"]);
  f.git(["commit", "-m", "Delete and ignore catalog"]);
  f.env.HEAD_SHA = f.git(["rev-parse", "HEAD"]);
  f.git(["push", f.remote, "HEAD:refs/heads/feature"]);
  f.generate();
  assert.equal(f.run("commit").status, 0);
  assert.equal(f.git(["diff", "--name-only", f.env.HEAD_SHA, f.remoteHead()]), ARTIFACT);
});

test("refuses unrelated staged files before committing", (root) => {
  const f = fixture(root);
  f.generate();
  f.write("registry.txt", "another edit\n");
  f.git(["add", "registry.txt"]);
  assert.notEqual(f.run("commit").status, 0);
  assert.equal(f.pushes(), 0);
});

test("skips a PR branch that advanced before the commit step", (root) => {
  const f = fixture(root);
  f.generate();
  const next = f.nextHead();
  f.git(["update-ref", "refs/heads/feature", next], f.remote);
  const result = f.run("commit");
  assert.equal(result.status, 0, result.stderr);
  assert.ok(result.stdout.includes("outdated autofix"));
  assert.equal(f.remoteHead(), next);
  assert.equal(f.pushes(), 0);
});

for (const mode of ["advance", "rewind", "delete", "reject", "accepted-error"]) {
  test(`does not retry or rebase after push-time ${mode}`, (root) => {
    const f = fixture(root);
    f.generate();
    const next = mode === "rewind" ? f.base : f.nextHead();
    const result = f.run("commit", { PUSH_MODE: mode, NEXT_HEAD: next });
    assert.notEqual(result.status, 0);
    assert.ok(result.stdout.includes("Could not confirm the autofix push"));
    assert.equal(f.pushes(), 1);
    if (mode === "delete") assert.equal(f.git(["ls-remote", "--heads", f.remote, "refs/heads/feature"]), "");
    else if (mode === "advance" || mode === "rewind") assert.equal(f.remoteHead(), next);
    else if (mode === "reject") assert.equal(f.remoteHead(), f.head);
    else assert.equal(f.git(["show", "-s", "--format=%P", f.remoteHead()]), f.head);
  });
}

test("requires a push token before creating a commit", (root) => {
  const f = fixture(root);
  f.generate();
  assert.notEqual(f.run("commit", { GH_TOKEN: "" }).status, 0);
  assert.equal(f.git(["rev-parse", "HEAD"]), f.head);
  assert.equal(f.pushes(), 0);
});

console.log(`All ${passed} provider autofix workflow tests passed.`);
