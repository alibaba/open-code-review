#!/usr/bin/env node

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

"use strict";

const fs = require("fs");
const path = require("path");
const { execFileSync } = require("child_process");

const ARTIFACT = "extensions/vscode/src/shared/providers.generated.ts";
const REGENERATE = "go generate ./internal/llm";
const BOT_NAME = "github-actions[bot]";
const BOT_EMAIL = "41898282+github-actions[bot]@users.noreply.github.com";

function runGit(args, options) {
  return execFileSync("git", args, {
    ...options,
    encoding: "utf8",
    stdio: ["ignore", "pipe", "pipe"],
  }).trimEnd();
}

// CI-only generation: discard the old output first so a missing go:generate
// directive cannot leave a stale committed artifact looking up to date.
function regenerate({ cwd = process.cwd(), env = process.env, exec = execFileSync } = {}) {
  const generatedPath = path.join(cwd, ARTIFACT);
  const generationEnv = { ...env };
  delete generationEnv.GH_TOKEN;
  delete generationEnv.GITHUB_TOKEN;
  fs.rmSync(generatedPath, { force: true });
  exec("go", ["generate", "./internal/llm"], {
    cwd, env: generationEnv, stdio: "inherit",
  });
  if (!fs.existsSync(generatedPath) || !fs.lstatSync(generatedPath).isFile()) {
    throw new Error(
      `Provider generation did not produce a regular artifact. ` +
      `Ensure the generator from #1221 is present and run '${REGENERATE}'.`,
    );
  }
}

// Generation runs in the preceding workflow step, without a persisted token.
// This helper may commit only the generated artifact to the event's PR branch.
function autofix({ cwd = process.cwd(), env = process.env, git = runGit } = {}) {
  if (
    env.GITHUB_EVENT_NAME !== "pull_request" ||
    !env.GITHUB_REPOSITORY ||
    env.OCR_HEAD_REPOSITORY !== env.GITHUB_REPOSITORY ||
    env.GITHUB_ACTOR === "dependabot[bot]"
  ) {
    return "skipped";
  }
  if (env.GITHUB_SERVER_URL && env.GITHUB_SERVER_URL !== "https://github.com") {
    throw new Error("This helper targets the public GitHub.com repository; refusing another server.");
  }

  const repository = env.GITHUB_REPOSITORY;
  const branch = env.OCR_HEAD_REF;
  const expected = env.OCR_HEAD_SHA;
  if (
    !/^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/.test(repository) ||
    !branch || !env.OCR_BASE_REF || branch === env.OCR_BASE_REF ||
    !/^[0-9a-f]{40}$/.test(expected || "")
  ) {
    throw new Error("Missing or invalid pull request head metadata.");
  }

  const cleanEnv = { ...env, GIT_TERMINAL_PROMPT: "0" };
  delete cleanEnv.GH_TOKEN;
  delete cleanEnv.GITHUB_TOKEN;
  const command = (args, commandEnv = cleanEnv) => git(args, { cwd, env: commandEnv });
  const ref = `refs/heads/${branch}`;
  // Use the event's repository, not a remote that generation could have changed.
  const remote = `https://github.com/${repository}.git`;
  command(["check-ref-format", ref]);
  if (command(["rev-parse", "HEAD"]) !== expected) {
    throw new Error("Checkout does not match the pull request head; refusing to commit.");
  }

  const remoteHead = () => command(["ls-remote", "--heads", remote, ref]).split(/\s/)[0];
  if (remoteHead() !== expected) return "superseded";

  const changed = new Set([
    ...command(["diff", "--name-only", "-z", "HEAD"]).split("\0"),
    ...command(["ls-files", "--others", "--exclude-standard", "-z"]).split("\0"),
  ].filter(Boolean));
  if ([...changed].some((file) => file !== ARTIFACT)) {
    throw new Error("Generation changed files outside the provider artifact; refusing to commit.");
  }
  const generatedPath = path.join(cwd, ARTIFACT);
  if (!fs.existsSync(generatedPath)) {
    throw new Error(`The generated provider artifact is missing. Ensure the generator is present and run '${REGENERATE}'.`);
  }
  if (!fs.lstatSync(generatedPath).isFile()) {
    throw new Error("The generated provider artifact must be a regular file.");
  }
  // Ignored, untracked output is invisible to both commands above. It still
  // needs to be committed, including when the PR deleted and ignored it.
  if (!command(["ls-files", "--cached", "-z", "--", ARTIFACT]).split("\0").includes(ARTIFACT)) {
    changed.add(ARTIFACT);
  }
  if (changed.size === 0) return "unchanged";
  if (!env.GH_TOKEN) throw new Error("A repository-scoped GH_TOKEN is required to push the fix.");

  // Force only this allowlisted generated path, never other ignored files.
  command(["add", "--force", "--", ARTIFACT]);
  const staged = command(["diff", "--cached", "--name-only", "-z"]).split("\0").filter(Boolean);
  if (staged.length !== 1 || staged[0] !== ARTIFACT) {
    throw new Error("The index must contain only the generated provider artifact.");
  }
  if (!command(["ls-files", "--stage", "--", ARTIFACT]).startsWith("100644 ")) {
    throw new Error("The generated provider artifact must have ordinary file permissions.");
  }
  command([
    "-c", "core.hooksPath=/dev/null",
    "-c", `user.name=${BOT_NAME}`,
    "-c", `user.email=${BOT_EMAIL}`,
    "commit", "--no-gpg-sign", "-m", "chore(vscode): regenerate provider presets", "--only", "--", ARTIFACT,
  ]);
  const commit = command(["rev-parse", "HEAD"]);
  if (command(["rev-parse", `${commit}^`]) !== expected) {
    throw new Error("The fix must be a direct child of the expected PR head; refusing to push.");
  }
  const auth = Buffer.from(`x-access-token:${env.GH_TOKEN}`).toString("base64");
  try {
    command([
      "-c", "core.hooksPath=/dev/null", "-c", "credential.helper=",
      "push", `--force-with-lease=${ref}:${expected}`, remote, `HEAD:${ref}`,
    ], {
      ...cleanEnv,
      GIT_CONFIG_COUNT: "1",
      GIT_CONFIG_KEY_0: `http.${remote}.extraheader`,
      GIT_CONFIG_VALUE_0: `AUTHORIZATION: basic ${auth}`,
    });
  } catch {
    // The explicit lease also rejects branch deletion or rewind. Since the
    // fix's parent is exactly expected, an accepted update is a fast-forward.
    // A connection may fail after acceptance; verify instead of retrying.
    let accepted = false;
    try {
      accepted = remoteHead() === commit;
    } catch {
      // Verification needs the network too. Keep the recovery instructions
      // below when the server cannot confirm whether it accepted the push.
    }
    if (accepted) return "pushed";
    throw new Error(
      `Could not push the autofix (permissions, branch rules, or a concurrent update). ` +
      `Check the latest branch state, run '${REGENERATE}', and commit ${ARTIFACT}.`,
    );
  }
  let verifiedHead;
  try {
    verifiedHead = remoteHead();
  } catch {
    throw new Error("Git reported a successful push, but the branch state could not be verified. Check the latest PR head and CI before rerunning.");
  }
  if (verifiedHead !== commit) {
    throw new Error("The PR branch changed during push verification; check its latest CI run.");
  }
  return "pushed";
}

if (require.main === module) {
  try {
    if (process.argv.length === 3 && process.argv[2] === "--generate") {
      regenerate();
      console.log("Provider preset regeneration completed.");
    } else if (process.argv.length === 2) {
      console.log(`Provider preset autofix: ${autofix()}.`);
    } else {
      throw new Error("Usage: autofix-provider-presets.js [--generate]");
    }
  } catch (error) {
    const message = error.message.replace(/%/g, "%25").replace(/\r/g, "%0D").replace(/\n/g, "%0A");
    console.error(`::error::${message}`);
    process.exitCode = 1;
  }
}

module.exports = { ARTIFACT, BOT_NAME, BOT_EMAIL, autofix, regenerate, runGit };
