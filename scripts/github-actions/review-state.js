// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

"use strict";

const fs = require("fs");
const path = require("path");
const os = require("os");
const { runPostReviewComments } = require("./post-review-comments");

function identity(context) {
  return {
    repository: `${context.repo.owner}/${context.repo.repo}`,
    run: String(context.runId != null ? context.runId : ""),
    attempt: String(process.env.GITHUB_RUN_ATTEMPT || ""),
    job: String(process.env.GITHUB_JOB || ""),
  };
}

function saveReviewState({ statePath, context, headSha, resultPath, stderrPath, options, outputs = {} }) {
  const result = fs.readFileSync(resultPath, "utf8");
  const state = {
    version: 1,
    identity: identity(context),
    headSha,
    options,
    outputs,
    result,
    stderr: fs.readFileSync(stderrPath, "utf8"),
  };
  // No tokens or model configuration are persisted. An atomic replacement
  // prevents a partial write from looking like a completed review.
  fs.mkdirSync(path.dirname(statePath), { recursive: true });
  const pending = `${statePath}.${process.pid}.tmp`;
  fs.writeFileSync(pending, JSON.stringify(state), { mode: 0o600 });
  fs.renameSync(pending, statePath);
}

async function postReviewState({ statePath, github, context, core }) {
  const state = JSON.parse(fs.readFileSync(statePath, "utf8"));
  const expectedIdentity = identity(context);
  if (state.version !== 1 || Object.keys(expectedIdentity).some((key) => state.identity?.[key] !== expectedIdentity[key])) {
    throw new Error("Saved review belongs to a different repository, run, attempt or job");
  }
  if (!state.headSha || !Number.isSafeInteger(state.options?.prNumber) || state.options.prNumber < 1) {
    throw new Error("Saved review has no valid pull request identity");
  }
  const pr = await github.rest.pulls.get({
    owner: context.repo.owner,
    repo: context.repo.repo,
    pull_number: state.options.prNumber,
  });
  if (pr.data.head.sha !== state.headSha) {
    throw new Error("Pull request head changed after the saved review; run a new review");
  }
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "ocr-post-"));
  try {
    const resultPath = path.join(dir, "result.json");
    const stderrPath = path.join(dir, "stderr.log");
    fs.writeFileSync(resultPath, state.result, { mode: 0o600 });
    fs.writeFileSync(stderrPath, state.stderr, { mode: 0o600 });
    await runPostReviewComments({ ...state.options, github, context, core, fs, resultPath, stderrPath });
    if (core && typeof core.setOutput === "function") {
      for (const [key, value] of Object.entries(state.outputs || {})) core.setOutput(key, value);
    }
  } finally {
    fs.rmSync(dir, { recursive: true, force: true });
  }
}

module.exports = { saveReviewState, postReviewState };
