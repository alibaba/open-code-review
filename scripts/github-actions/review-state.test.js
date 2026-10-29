// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

"use strict";

const assert = require("assert");
const fs = require("fs");
const os = require("os");
const path = require("path");
const { saveReviewState, postReviewState } = require("./review-state");

async function main() {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "ocr-state-test-"));
  const context = { repo: { owner: "owner", repo: "repo" }, runId: 123 };
  const statePath = path.join(dir, "saved.json");
  const resultPath = path.join(dir, "result.json");
  const stderrPath = path.join(dir, "stderr.log");
  const headSha = "1".repeat(40);
  const originalJob = process.env.GITHUB_JOB;
  const originalToken = process.env.OCR_LLM_TOKEN;
  const calls = [];
  const outputs = {};
  const core = { info() {}, warning() {}, setOutput: (key, value) => { outputs[key] = value; } };
  const github = { rest: {
    pulls: { get: async (params) => {
      assert.strictEqual(params.pull_number, 7);
      return { data: { head: { sha: headSha } } };
    } },
    issues: { createComment: async (params) => {
      calls.push(params);
      return { data: { id: 1, html_url: "https://example.com/comment" } };
    } },
  } };
  try {
    fs.writeFileSync(resultPath, JSON.stringify({ comments: [], message: "Saved review findings" }));
    fs.writeFileSync(stderrPath, "review stderr");
    process.env.OCR_LLM_TOKEN = "must-not-be-saved";
    saveReviewState({ statePath, context, headSha, resultPath, stderrPath,
      options: { prNumber: 7, stickySummary: false, rangeMode: "checkpoint", rangeFrom: "old", rangeTo: headSha } });
    assert.strictEqual(calls.length, 0, "review mode must not publish");
    assert.ok(!fs.readFileSync(statePath, "utf8").includes(process.env.OCR_LLM_TOKEN));
    const saved = JSON.parse(fs.readFileSync(statePath, "utf8"));
    assert.strictEqual(saved.options.rangeMode, "checkpoint");
    assert.strictEqual(saved.stderr, "review stderr");
    await postReviewState({ statePath, github, context, core });
    assert.strictEqual(calls.length, 1);
    assert.strictEqual(calls[0].issue_number, 7);
    assert.match(calls[0].body, /Saved review findings/);
    assert.strictEqual(outputs.summary_comment_url, "https://example.com/comment");
    await assert.rejects(postReviewState({ statePath, github, context: { ...context, runId: 124 }, core }), /different/);
    await assert.rejects(postReviewState({ statePath, github, context: { ...context, repo: { owner: "other", repo: "repo" } }, core }), /different/);
    process.env.GITHUB_JOB = "another-job";
    await assert.rejects(postReviewState({ statePath, github, context, core }), /different/);
    if (originalJob === undefined) delete process.env.GITHUB_JOB;
    else process.env.GITHUB_JOB = originalJob;
    const reordered = { ...saved, identity: Object.fromEntries(Object.entries(saved.identity).reverse()), outputs: { range_mode: "checkpoint" } };
    fs.writeFileSync(statePath, JSON.stringify(reordered));
    await postReviewState({ statePath, github, context, core: { info() {}, warning() {} } });
    assert.strictEqual(calls.length, 2, "identity field order must not prevent posting without output support");
    for (const runId of [null, undefined]) {
      saveReviewState({ statePath, context: { ...context, runId }, headSha, resultPath, stderrPath,
        options: saved.options });
      assert.strictEqual(JSON.parse(fs.readFileSync(statePath, "utf8")).identity.run, "");
      await assert.rejects(postReviewState({ statePath, github, context, core }), /different/);
    }
    fs.writeFileSync(resultPath, "{");
    saveReviewState({ statePath, context, headSha, resultPath, stderrPath, options: saved.options });
    await postReviewState({ statePath, github, context, core });
    assert.strictEqual(calls.length, 3, "malformed results must reach the existing posting error handler");
    assert.match(calls[2].body, /review stderr/);
    fs.writeFileSync(statePath, JSON.stringify(saved));
    github.rest.pulls.get = async () => ({ data: { head: { sha: "2".repeat(40) } } });
    await assert.rejects(postReviewState({ statePath, github, context, core }), /head changed/);
    assert.strictEqual(calls.length, 3, "invalid states must not publish");
    fs.writeFileSync(statePath, "{");
    await assert.rejects(postReviewState({ statePath, github, context, core }), SyntaxError);
    console.log("Review state round trip, credentials exclusion, identity and stale-head tests passed.");
  } finally {
    if (originalToken === undefined) delete process.env.OCR_LLM_TOKEN;
    else process.env.OCR_LLM_TOKEN = originalToken;
    fs.rmSync(dir, { recursive: true, force: true });
  }
}
main().catch((error) => { console.error(error); process.exitCode = 1; });
