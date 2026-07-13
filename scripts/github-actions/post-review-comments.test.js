#!/usr/bin/env node
"use strict";

// Unit tests for scripts/github-actions/post-review-comments.js.
//
// Run via: node scripts/github-actions/post-review-comments.test.js
// (also wired as `npm run test:github-actions`).
//
// These tests drive runPostReviewComments directly with an injected mock
// github/core/fs, replacing the previous approach of regex-extracting the
// inline script from workflow YAML.

const assert = require("assert");
const path = require("path");
const { runPostReviewComments, safeFence, fencedBlock, lineSpan, sameCommentSpan, overlapsHistory, resolveThreshold, DEFAULT_OVERLAP_THRESHOLD, newCommentId, getPostedCommentIds, computeRetryDelayMs, formatWarnings } = require(path.join(__dirname, "post-review-comments.js"));

// Make all retry/pacing delays effectively zero so tests run fast.
// computeRetryDelayMs reads OCR_RETRY_MAX_DELAY / OCR_RETRY_BASE_DELAY via
// parseNonNegInt; "1" keeps the cap/base at 1ms so any transient/rate-limit
// backoff sleep is effectively instant.
process.env.OCR_MAX_RETRIES = "0";
process.env.OCR_SUCCESS_DELAY = "0";
process.env.OCR_FAILURE_DELAY = "0";
process.env.OCR_LOW_REMAINING_SPACING = "0";
process.env.OCR_LOW_REMAINING_THRESHOLD = "0";
process.env.OCR_RETRY_MAX_DELAY = "1";
process.env.OCR_RETRY_BASE_DELAY = "1";
process.env.OCR_READ_SUCCESS_DELAY = "0";
process.env.OCR_READ_LOW_REMAINING_SPACING = "0";

const context = {
  repo: { owner: "owner", repo: "repo" },
  issue: { number: 123 },
  eventName: "pull_request_target",
  payload: { pull_request: { head: { sha: "head-sha" } } },
};

function mockFs(resultText, stderrText) {
  return {
    readFileSync(file) {
      if (file === "/tmp/ocr-result.json") return resultText;
      if (file === "/tmp/ocr-stderr.log") return stderrText;
      throw new Error(`unexpected read: ${file}`);
    },
  };
}

function makeErr(message, status, headers) {
  const e = new Error(message);
  if (status != null) e.status = status;
  if (headers) e.response = { headers };
  return e;
}

// Identity key for a single inline review comment, used to drive per-comment
// error injection. Two comments on the same path but different lines get
// different keys, so one can fail (e.g. 422 line-unresolvable) while another on
// the same file succeeds. Mirrors the (path, line range) identity the bot uses
// for incremental dedup and the idempotency check.
function commentKey(rc) {
  if (!rc) return "?";
  return `${rc.path}|${rc.start_line != null ? rc.start_line : "-"}|${rc.line != null ? rc.line : "-"}`;
}

// Temporarily override env vars for a single (sync or async) test body, always
// restoring originals afterwards. Used for retry/quota tests that need a
// different OCR_MAX_RETRIES / OCR_LOW_REMAINING_THRESHOLD than the fast default.
async function withEnv(env, fn) {
  const saved = {};
  for (const k of Object.keys(env)) {
    saved[k] = process.env[k];
    process.env[k] = env[k];
  }
  try {
    return await fn();
  } finally {
    for (const k of Object.keys(env)) {
      if (saved[k] === undefined) delete process.env[k];
      else process.env[k] = saved[k];
    }
  }
}

function makeGithub(opts = {}) {
  const createReviewCalls = [];
  const issueComments = [];
  const updatedComments = [];
  const listCommentsCalls = [];
  const listReviewCommentsCalls = [];
  const listReviewsCalls = [];
  // Interleaved log of write operations (createReview / createComment /
  // updateComment) in call order, so tests can assert positioning invariants
  // such as "summary created before review" without timing the calls.
  const ops = [];
  // Per-comment attempt counter, keyed by commentKey, so perCommentError can be
  // attempt-aware (e.g. "429 on attempt 0, succeed on attempt 1").
  const perCommentAttempts = new Map();

  function successRemaining() {
    return opts.successRemaining != null ? String(opts.successRemaining) : "5000";
  }

  // Inline comment objects recorded in the BATCH createReview call (index 0)
  // only, so tests can simulate "this comment already landed on the server"
  // without predicting the random IDs from newCommentId(). Scoped to the batch
  // call so batch-level landing (echoPosted) stays disjoint from per-comment
  // landing (landedKeys, which reads per-comment calls at index >= 1).
  function batchPostedComments() {
    const out = [];
    const call = createReviewCalls[0];
    if (call) {
      for (const c of call.comments || []) {
        const m = /<!--\s*(ocr-\d+-\d+-[a-f0-9]+)\s*-->/.exec(c.body || "");
        if (m) {
          out.push({
            path: c.path,
            body: c.body,
            side: c.side || "RIGHT",
            start_line: c.start_line,
            line: c.line,
          });
        }
      }
    }
    return out;
  }

  return {
    createReviewCalls,
    issueComments,
    updatedComments,
    listCommentsCalls,
    listReviewCommentsCalls,
    listReviewsCalls,
    ops,
    rest: {
      users: {
        getAuthenticated: async () => ({ data: { login: "github-actions[bot]" } }),
      },
      pulls: {
        get: async () => ({ data: { head: { sha: "head-sha" } } }),
        createReview: async (params) => {
          createReviewCalls.push(params);
          ops.push({ type: "createReview", params });
          const callIdx = createReviewCalls.length - 1;
          const successRes = () => ({ data: {}, headers: { "x-ratelimit-remaining": successRemaining() } });
          if (callIdx === 0) {
            // Batch call. bulkErrorSpec (rich: with headers) takes precedence
            // over the legacy bulkError/bulkErrorStatus pair.
            if (opts.bulkErrorSpec) {
              throw makeErr(opts.bulkErrorSpec.message, opts.bulkErrorSpec.status, opts.bulkErrorSpec.headers);
            }
            if (opts.bulkError) {
              throw makeErr(opts.bulkError, opts.bulkErrorStatus, opts.bulkHeaders);
            }
            return successRes();
          }
          // Per-comment call (index >= 1). perCommentError(rc, attempt) lets a
          // test fail some comments and not others (partial failure), and be
          // attempt-aware (retry-then-succeed). Falls back to the legacy
          // individualError (applies to all per-comment calls) for older tests.
          if (typeof opts.perCommentError === "function") {
            const rc = params.comments && params.comments[0];
            const key = commentKey(rc);
            const attempt = perCommentAttempts.get(key) || 0;
            perCommentAttempts.set(key, attempt + 1);
            const spec = opts.perCommentError(rc, attempt);
            if (spec) throw makeErr(spec.message, spec.status, spec.headers);
            return successRes();
          }
          if (opts.individualError) {
            throw makeErr(opts.individualError, opts.individualErrorStatus, opts.individualHeaders);
          }
          return successRes();
        },
        listReviews: async (params) => {
          listReviewsCalls.push(params);
          // Consume a queued sequence of read errors (e.g. a transient 429 on
          // the read itself) before falling through to the normal response, so
          // withRetry's rate-limit backoff on reads can be exercised.
          if (opts.listReviewsErrorSeq && opts.listReviewsErrorSeq.length) {
            const spec = opts.listReviewsErrorSeq.shift();
            throw makeErr(spec.message, spec.status, spec.headers);
          }
          if (opts.listReviewsThrow) {
            throw makeErr("listReviews unavailable", 503);
          }
          // Simulate the batch review having landed on the server even though
          // createReview threw: echo the batch call's body (which carries the
          // REVIEW_TAG) as an existing review's body so findExistingBatchReview
          // matches it.
          if (opts.batchLanded && createReviewCalls[0]) {
            return { data: [{ id: 999, body: createReviewCalls[0].body || "" }] };
          }
          return { data: opts.reviews || [] };
        },
        listReviewComments: async (params) => {
          listReviewCommentsCalls.push(params);
          if (opts.listReviewCommentsThrow) {
            throw makeErr(opts.listReviewCommentsError || "read api unavailable", 503);
          }
          // Build the visible comment set from two disjoint, deduped sources:
          //   - echoPosted: comments carried by the BATCH call (index 0) that
          //     "already landed" — drives the batch-level getPostedCommentIds.
          //   - landedKeys: per-comment calls (index >= 1) that landed despite
          //     a 5xx/network error — drives per-comment isCommentAlreadyPosted.
          // Deduping by embedded comment id keeps them composable.
          if (opts.echoPosted || opts.landedKeys) {
            const byId = new Map();
            const add = (c) => {
              const m = /<!--\s*(ocr-\d+-\d+-[a-f0-9]+)\s*-->/.exec(c.body || "");
              const k = m ? m[1] : `${c.path}|${c.start_line != null ? c.start_line : "-"}|${c.line != null ? c.line : "-"}|${c.body}`;
              if (!byId.has(k)) byId.set(k, c);
            };
            if (opts.echoPosted) {
              const posted = batchPostedComments();
              const n = opts.postedCount != null ? opts.postedCount : posted.length;
              for (const c of posted.slice(0, n)) add(c);
            }
            if (opts.landedKeys) {
              for (let i = 1; i < createReviewCalls.length; i++) {
                const rc = createReviewCalls[i].comments && createReviewCalls[i].comments[0];
                if (rc && opts.landedKeys.has(commentKey(rc))) {
                  add({ path: rc.path, body: rc.body, side: rc.side || "RIGHT", start_line: rc.start_line, line: rc.line });
                }
              }
            }
            return { data: [...byId.values()] };
          }
          return { data: opts.history || [] };
        },
      },
      issues: {
        listComments: async (params) => {
          listCommentsCalls.push(params);
          return { data: opts.existingSummary || [] };
        },
        createComment: async (params) => {
          issueComments.push(params);
          ops.push({ type: "createComment", params });
          return { data: { id: 1000 + issueComments.length, html_url: `http://ex/c${issueComments.length}` } };
        },
        updateComment: async (params) => {
          updatedComments.push(params);
          ops.push({ type: "updateComment", params });
          return { data: { id: params.comment_id, html_url: `http://ex/u${updatedComments.length}` } };
        },
      },
    },
  };
}

function mockCore() {
  const outputs = {};
  return {
    outputs,
    setOutput(name, value) { outputs[name] = value; },
    info() {},
  };
}

async function run({ result, stderr = "", opts = {}, githubOpts = {} }) {
  const resultText = typeof result === "string" ? result : JSON.stringify(result);
  const fs = mockFs(resultText, stderr);
  const github = makeGithub(githubOpts);
  const core = mockCore();
  const options = Object.assign({ stickySummary: true, incremental: false }, opts);
  await runPostReviewComments({
    github,
    context,
    core,
    fs,
    resultPath: "/tmp/ocr-result.json",
    stderrPath: "/tmp/ocr-stderr.log",
    ...options,
  });
  return { github, core, outputs: core.outputs };
}

// ---- Test cases (mirror PLAN §7) ----

async function testFailedInlineCommentsAreSummarized() {
  const result = {
    comments: [
      {
        path: "docs/no-line.md",
        content:
          "No-line content with a fenced block:\n\n```js\nconsole.log('still visible');\n```",
        existing_code: "",
        suggestion_code: "",
        start_line: 0,
        end_line: 0,
      },
      {
        path: "src/app.js",
        content: "Failed inline content must remain visible in the PR summary.",
        existing_code: "oldCall();",
        suggestion_code: "newCall();",
        start_line: 10,
        end_line: 10,
      },
    ],
    warnings: [],
  };

  const { github } = await run({
    result,
    githubOpts: {
      bulkError: 'Unprocessable Entity: "Line could not be resolved"',
      individualError: 'Unprocessable Entity: "Line could not be resolved"',
    },
    opts: { stickySummary: true },
  });

  assert.strictEqual(github.createReviewCalls.length, 2, "bulk + one per-comment attempt");
  assert.strictEqual(github.issueComments.length, 1, "summary anchor created (no existing)");
  assert.strictEqual(github.updatedComments.length, 1, "anchor finalized with the full body");
  const body = github.updatedComments[0].body;
  assert.match(body, /No-line content with a fenced block/);
  assert.match(body, /Failed inline content must remain visible/);
  assert.match(body, /Line could not be resolved/);
  // The no-line comment now carries the same reason line as a posting failure.
  assert.match(body, /GitHub could not post this as an inline comment: No line information provided/);
  // Posting statistics are merged into the leading summary header (the trailing
  // "📊 Posting Statistics" section is gone), so the merged stats must appear
  // BEFORE the per-comment renderings.
  assert.doesNotMatch(body, /Inline comments shown in summary/);
  assert.doesNotMatch(body, /📊 \*\*Posting Statistics:\*\*/);
  const statsIdx = body.indexOf("❌ Failed to post inline");
  const noLineIdx = body.indexOf("No-line content with a fenced block");
  const failedIdx = body.indexOf("Failed inline content must remain visible");
  assert.ok(statsIdx !== -1, "merged stats present in the header");
  assert.ok(statsIdx < noLineIdx, "merged stats rendered before no-line comment");
  assert.ok(statsIdx < failedIdx, "merged stats rendered before failed comment");
}

async function testWarningsListedAfterSummaryComments() {
  const result = {
    comments: [
      { path: "src/a.js", content: "Inline comment content.", start_line: 1, end_line: 1 },
      { path: "docs/no-line.md", content: "No-line comment content.", start_line: 0, end_line: 0 },
    ],
    warnings: [
      "file too large to review fully",
      { file: "assets/logo.png", message: "skipped binary asset", type: "binary_asset" },
    ],
  };

  const { github } = await run({ result, opts: { stickySummary: true } });

  assert.strictEqual(github.updatedComments.length, 1, "anchor finalized with the full body");
  const body = github.updatedComments[0].body;
  // Both the count line and the detailed list must be present...
  assert.match(body, /2 warning\(s\) occurred during review/);
  assert.match(body, /⚠️ \*\*Warnings:\*\*/);
  assert.match(body, /file too large to review fully/);
  // Object warnings surface file, type, and message.
  assert.match(body, /`assets\/logo\.png` \(`binary_asset`\): skipped binary asset/);
  // ...and the list must come AFTER the non-inline (no-line) review comment.
  const noLineIdx = body.indexOf("No-line comment content.");
  const warningsIdx = body.indexOf("⚠️ **Warnings:**");
  assert.ok(noLineIdx !== -1 && warningsIdx > noLineIdx, "warnings list placed after summary comments");
  // The pre-review anchor body must also surface the warning contents.
  assert.match(github.issueComments[0].body, /`assets\/logo\.png` \(`binary_asset`\): skipped binary asset/);
}

function testFormatWarnings() {
  assert.strictEqual(formatWarnings([]), "");
  assert.strictEqual(formatWarnings(null), "");
  assert.strictEqual(formatWarnings(undefined), "");
  // Plain string warnings.
  assert.match(formatWarnings(["a", "b"]), /⚠️ \*\*Warnings:\*\*/);
  assert.match(formatWarnings(["a", "b"]), /\n- a\n- b/);
  // Object warnings surface file, type, and message together.
  assert.match(
    formatWarnings([{ file: "internal/llm/resolver.go", message: "context deadline exceeded", type: "subtask_error" }]),
    /\n- `internal\/llm\/resolver\.go` \(`subtask_error`\): context deadline exceeded/
  );
  // Partial objects: only message.
  assert.match(formatWarnings([{ message: "boom" }]), /\n- boom/);
  // Partial objects: file + message, no type.
  assert.match(formatWarnings([{ file: "a.go", message: "m" }]), /\n- `a\.go`: m/);
  // Unknown object shapes degrade to a stable JSON stringification.
  assert.match(formatWarnings([{ code: 42 }]), /\n- \{"code":42\}/);
}

async function testErrorCommentUsesSafeFence() {
  const { github } = await run({
    result: "not json",
    stderr: "stderr includes a fence\n```js\nbroken();\n```",
    opts: { stickySummary: true },
  });

  assert.strictEqual(github.issueComments.length, 1);
  const body = github.issueComments[0].body;
  // stderr contains a 3-backtick fence, so safeFence must use 4 backticks.
  assert.match(body, /\n````\nstderr includes a fence/);
}

async function testStickyUpdatesExistingSummary() {
  const existing = [{ id: 42, body: "<!-- ocr-summary -->\nold summary", user: { login: "github-actions[bot]" } }];
  const result = { comments: [{ path: "src/a.js", content: "x", start_line: 1, end_line: 1 }], warnings: [] };

  const { github, outputs } = await run({
    result,
    githubOpts: { existingSummary: existing },
    opts: { stickySummary: true },
  });

  assert.strictEqual(github.updatedComments.length, 1, "existing summary updated");
  assert.strictEqual(github.issueComments.length, 0, "no new comment created");
  assert.strictEqual(github.updatedComments[0].comment_id, 42);
  assert.strictEqual(outputs.comments_inline, "1");
  assert.strictEqual(outputs.comments_skipped, "0");
  assert.strictEqual(outputs.summary_comment_url, "http://ex/u1");
}

// Non-sticky + batch fails (e.g. rate-limit) but the per-comment fallback then
// succeeds for every comment. The summary must still be posted as its own issue
// comment (the summary never rides in the review body anymore) and finalized
// with the success statistics.
async function testNonStickyFallbackAllSuccessStillPostsSummary() {
  const result = { comments: [{ path: "src/a.js", content: "comment A", start_line: 1, end_line: 1 }], warnings: [] };

  const { github, outputs } = await run({
    result,
    githubOpts: {
      // Batch fails (rate-limit)...
      bulkError: "rate limited",
      bulkErrorStatus: 429,
      // ...but the per-comment fallback succeeds (no individualError).
    },
    opts: { stickySummary: false },
  });

  // batch (call #1, failed) + one per-comment retry (call #2, succeeded).
  assert.strictEqual(github.createReviewCalls.length, 2, "batch + per-comment fallback");
  assert.strictEqual(github.issueComments.length, 1, "summary anchor posted as issue comment");
  assert.strictEqual(github.updatedComments.length, 1, "anchor finalized with the success stats");
  assert.strictEqual(outputs.comments_inline, "1");
  assert.strictEqual(outputs.comments_failed, "0");
}

async function testNonStickyCreatesNewCommentOnFallback() {
  const result = { comments: [{ path: "src/a.js", content: "Failed inline content.", start_line: 10, end_line: 10 }], warnings: [] };

  const { github } = await run({
    result,
    githubOpts: {
      bulkError: 'Unprocessable Entity: "Line could not be resolved"',
      individualError: 'Unprocessable Entity: "Line could not be resolved"',
    },
    opts: { stickySummary: false },
  });

  assert.strictEqual(github.issueComments.length, 1, "anchor summary comment created");
  assert.strictEqual(github.updatedComments.length, 1, "anchor finalized with full body");
  assert.match(github.updatedComments[0].body, /Failed inline content/);
}

async function testNoCommentsStickyUpdate() {
  const existing = [{ id: 7, body: "<!-- ocr-summary -->\nold good", user: { login: "github-actions[bot]" } }];
  const result = { comments: [], message: "All clear." };

  const { github } = await run({
    result,
    githubOpts: { existingSummary: existing },
    opts: { stickySummary: true },
  });

  assert.strictEqual(github.updatedComments.length, 1);
  assert.strictEqual(github.issueComments.length, 0);
  assert.match(github.updatedComments[0].body, /All clear\./);
}

async function testIncrementalSkipsOverlapping() {
  const history = [{ path: "src/a.js", line: 10, start_line: 10, side: "RIGHT", user: { login: "github-actions[bot]" } }];
  const result = {
    comments: [
      { path: "src/a.js", content: "overlap", start_line: 10, end_line: 10 },
      { path: "src/b.js", content: "new", start_line: 5, end_line: 5 },
    ],
    warnings: [],
  };

  const { github, outputs } = await run({
    result,
    githubOpts: { history },
    opts: { stickySummary: true, incremental: true },
  });

  assert.strictEqual(github.createReviewCalls.length, 1, "one batch review");
  const sent = github.createReviewCalls[0].comments;
  assert.strictEqual(sent.length, 1, "only non-overlapping comment sent");
  assert.strictEqual(sent[0].path, "src/b.js");
  assert.strictEqual(outputs.comments_skipped, "1");
  assert.strictEqual(outputs.comments_inline, "1");
}

async function testIncrementalAllOverlapPostsNoReview() {
  const history = [{ path: "src/a.js", line: 10, start_line: 10, side: "RIGHT", user: { login: "github-actions[bot]" } }];
  const result = { comments: [{ path: "src/a.js", content: "overlap", start_line: 10, end_line: 10 }], warnings: [] };

  const { github, outputs } = await run({
    result,
    githubOpts: { history },
    opts: { stickySummary: true, incremental: true },
  });

  assert.strictEqual(github.createReviewCalls.length, 0, "no review posted");
  assert.strictEqual(github.issueComments.length, 1, "summary anchor created");
  assert.strictEqual(github.updatedComments.length, 1, "anchor finalized with status body");
  assert.match(github.updatedComments[0].body, /nothing new was posted/);
  assert.strictEqual(outputs.comments_skipped, "1");
  assert.strictEqual(outputs.comments_inline, "0");
}

// Multi-line IoU dedup end-to-end at the default threshold (0.6). History
// covers [8,10]; of the three new multi-line comments, the identical span
// (IoU 1.0) is skipped while the low-IoU one (0.5) and a different file are
// posted. Also verifies a single-line comment is NOT suppressed by a prior
// multi-line block on an overlapping line.
async function testIncrementalMultiLineIoUDefaultThreshold() {
  const history = [{ path: "src/a.js", line: 10, start_line: 8, side: "RIGHT", user: { login: "github-actions[bot]" } }];
  const result = {
    comments: [
      { path: "src/a.js", content: "identical", start_line: 8, end_line: 10 }, // IoU 1.0 -> skipped
      { path: "src/a.js", content: "low-iou", start_line: 9, end_line: 11 }, // IoU 0.5 -> posted
      { path: "src/a.js", content: "single", start_line: 9, end_line: 9 }, // single vs multi -> posted
      { path: "src/b.js", content: "new", start_line: 1, end_line: 3 }, // other file -> posted
    ],
    warnings: [],
  };

  const { github, outputs } = await run({
    result,
    githubOpts: { history },
    opts: { stickySummary: true, incremental: true },
  });

  assert.strictEqual(github.createReviewCalls.length, 1, "one batch review");
  const sent = github.createReviewCalls[0].comments;
  assert.strictEqual(sent.length, 3, "identical multi-line span skipped, rest posted");
  const aJsLow = sent.find((c) => c.path === "src/a.js" && c.start_line === 9 && c.line === 11);
  const aJsSingle = sent.find((c) => c.path === "src/a.js" && c.line === 9 && c.start_line == null);
  assert.ok(aJsLow, "low-IoU multi-line comment was posted");
  assert.ok(aJsSingle, "single-line comment was not suppressed by multi-line history");
  assert.strictEqual(outputs.comments_skipped, "1");
  assert.strictEqual(outputs.comments_inline, "3");
}

// Threshold propagation: lowering incrementalOverlapThreshold to 0.4 makes the
// previously low-IoU span (0.5) now overlap, so it is skipped. Exercises the
// runPostReviewComments -> overlapsHistory wiring end-to-end.
async function testIncrementalOverlapThresholdPropagated() {
  const history = [{ path: "src/a.js", line: 10, start_line: 8, side: "RIGHT", user: { login: "github-actions[bot]" } }];
  const result = {
    comments: [{ path: "src/a.js", content: "low-iou", start_line: 9, end_line: 11 }], // IoU 0.5
    warnings: [],
  };

  const { github, outputs } = await run({
    result,
    githubOpts: { history },
    opts: { stickySummary: true, incremental: true, incrementalOverlapThreshold: 0.4 },
  });

  assert.strictEqual(github.createReviewCalls.length, 0, "no review posted (0.5 > 0.4 now overlaps)");
  assert.strictEqual(outputs.comments_skipped, "1");
  assert.strictEqual(outputs.comments_inline, "0");
}

// ---- Idempotency tests (prevent duplicate review posts on retry) ----

// Batch createReview fails with 5xx but the batch actually landed on the
// server. The retry must post ONLY the comments that are missing, not all of
// them (which would create duplicates).
async function testBatchLandedRetriesOnlyMissingComments() {
  const result = {
    comments: [
      { path: "src/a.js", content: "comment A", start_line: 1, end_line: 1 },
      { path: "src/b.js", content: "comment B", start_line: 2, end_line: 2 },
      { path: "src/c.js", content: "comment C", start_line: 3, end_line: 3 },
    ],
    warnings: [],
  };

  const { github, outputs } = await run({
    result,
    githubOpts: {
      // Batch createReview fails with 5xx ...
      bulkError: "Bad Gateway",
      bulkErrorStatus: 502,
      // ... but the batch actually landed on the server (listReviews echoes
      // the batch call's REVIEW_TAG-tagged body back as an existing review).
      batchLanded: true,
      // 2 of the 3 inline comments are already posted (echoed from the batch
      // call's comment bodies via listReviewComments).
      echoPosted: true,
      postedCount: 2,
    },
    opts: { stickySummary: true },
  });

  // batch (call #1) + only the 1 missing comment retried (call #2). NOT 3
  // per-comment calls -> no duplicates.
  assert.strictEqual(github.createReviewCalls.length, 2, "batch + only the missing comment retried");
  assert.strictEqual(github.createReviewCalls[1].comments.length, 1, "exactly one comment retried");
  assert.strictEqual(github.createReviewCalls[1].comments[0].path, "src/c.js", "the missing comment is retried");
  assert.strictEqual(outputs.comments_inline, "3", "2 already-posted + 1 retried = 3 successes");
  assert.strictEqual(outputs.comments_failed, "0");
}

// Per-comment createReview fails with 5xx but the comment already landed on
// the server. It must be treated as a success (no retry, no duplicate).
async function testPerComment5xxAlreadyPostedTreatedAsSuccess() {
  const result = { comments: [{ path: "src/a.js", content: "comment A", start_line: 1, end_line: 1 }], warnings: [] };

  const { github, outputs } = await run({
    result,
    githubOpts: {
      bulkError: "Bad Gateway",
      bulkErrorStatus: 502,
      individualError: "Bad Gateway",
      individualErrorStatus: 502,
      // The comment is already on the server (echoed from the batch call's
      // comment body via listReviewComments).
      echoPosted: true,
    },
    opts: { stickySummary: true },
  });

  // batch (call #1) + one per-comment attempt (call #2) that 5xx'd. The
  // idempotency check finds the comment already posted -> no retry.
  assert.strictEqual(github.createReviewCalls.length, 2, "no retry after already-posted detection");
  assert.strictEqual(outputs.comments_inline, "1", "already-posted counted as success");
  assert.strictEqual(outputs.comments_failed, "0");
}

// Per-comment createReview fails with 5xx and the read API is unavailable, so
// the idempotency check cannot tell whether the comment landed. The retry must
// be SKIPPED (to avoid a duplicate) and the comment recorded as failed.
async function testPerComment5xxIdempotencyUnavailableSkipsRetry() {
  const result = { comments: [{ path: "src/a.js", content: "comment A", start_line: 1, end_line: 1 }], warnings: [] };

  const { github, outputs } = await run({
    result,
    githubOpts: {
      bulkError: "Bad Gateway",
      bulkErrorStatus: 502,
      individualError: "Bad Gateway",
      individualErrorStatus: 502,
      // Read API unavailable -> isCommentAlreadyPosted returns null (unknown).
      listReviewCommentsThrow: true,
    },
    opts: { stickySummary: true },
  });

  // batch (call #1) + one per-comment attempt (call #2). No retry despite 5xx
  // (unknown -> skip to avoid duplicate).
  assert.strictEqual(github.createReviewCalls.length, 2, "no retry when idempotency check is unavailable");
  assert.strictEqual(outputs.comments_failed, "1", "recorded as failed, not retried");
  // The uncertainty is surfaced in the finalized summary.
  assert.strictEqual(github.issueComments.length, 1, "anchor created");
  assert.strictEqual(github.updatedComments.length, 1, "anchor finalized");
  assert.match(github.updatedComments[0].body, /idempotency check unavailable/);
}

// A summary comment already exists (e.g. a previous attempt within the run
// posted it). The anchor phase must reuse it (no duplicate created) and the
// finalize phase must refresh it in place with the final body.
async function testSummaryDoesNotDuplicateWhenAlreadyPosted() {
  // context.runId/runAttempt are unset -> RUN_TAG = "0-1" -> SUMMARY_TAG =
  // "<!-- ocr-summary-run:0-1 -->". A real summary carries both the persistent
  // SUMMARY_MARKER and the per-run SUMMARY_TAG.
  const existing = [
    { id: 5, body: "<!-- ocr-summary -->\n<!-- ocr-summary-run:0-1 -->\nold summary", user: { login: "github-actions[bot]" } },
  ];
  const result = { comments: [{ path: "src/a.js", content: "x", start_line: 1, end_line: 1 }], warnings: [] };

  const { github, outputs } = await run({
    result,
    githubOpts: { existingSummary: existing },
    opts: { stickySummary: true },
  });

  // Batch review posted normally; the existing summary is reused and refreshed,
  // never duplicated.
  assert.strictEqual(github.createReviewCalls.length, 1, "batch review posted");
  assert.strictEqual(github.issueComments.length, 0, "no duplicate summary created");
  assert.strictEqual(github.updatedComments.length, 1, "existing summary refreshed in place");
  assert.strictEqual(github.updatedComments[0].comment_id, 5, "the existing comment is the one updated");
  assert.strictEqual(outputs.comments_inline, "1");
  assert.strictEqual(outputs.summary_comment_url, "http://ex/u1");
  assert.match(github.updatedComments[0].body, /Successfully posted inline: 1 comment/, "final body reflects the run outcome");
}

// Cold-start ordering: on the first review on a PR, the summary issue comment
// must be created BEFORE the batch review so its timeline position is above the
// review (GitHub orders issue comments oldest-first). It is then finalized
// (updated in place) after the review lands. This is the core fix for the
// "summary sandwiched between review blocks" defect on sticky PRs.
async function testSummaryAnchorCreatedBeforeReviewColdStart() {
  const result = { comments: [{ path: "src/a.js", content: "x", start_line: 1, end_line: 1 }], warnings: [] };

  const { github } = await run({
    result,
    githubOpts: { existingSummary: [] }, // cold start: no existing summary
    opts: { stickySummary: true },
  });

  const types = github.ops.map((o) => o.type);
  const anchorIdx = types.indexOf("createComment");
  const reviewIdx = types.indexOf("createReview");
  const finalizeIdx = types.lastIndexOf("updateComment");
  assert.notStrictEqual(anchorIdx, -1, "summary anchor created");
  assert.notStrictEqual(reviewIdx, -1, "batch review posted");
  assert.notStrictEqual(finalizeIdx, -1, "summary finalized");
  assert.ok(anchorIdx < reviewIdx, "summary anchor created BEFORE the review (cold-start positioning)");
  assert.ok(reviewIdx < finalizeIdx, "summary finalized AFTER the review");
  // The anchor body is a pre-review placeholder; the final body carries stats.
  assert.match(github.issueComments[0].body, /Posting review comments/);
  assert.match(github.updatedComments[0].body, /Successfully posted inline: 1 comment/);
}

// Cold start + non-sticky: the per-run summary is also anchored before the
// review (non-sticky still creates a fresh comment each run, but within the run
// it must lead the review for a natural reading order).
async function testSummaryAnchorCreatedBeforeReviewNonSticky() {
  const result = { comments: [{ path: "src/a.js", content: "x", start_line: 1, end_line: 1 }], warnings: [] };

  const { github } = await run({
    result,
    githubOpts: { existingSummary: [] },
    opts: { stickySummary: false },
  });

  const types = github.ops.map((o) => o.type);
  assert.ok(types.indexOf("createComment") < types.indexOf("createReview"), "anchor before review");
  assert.ok(types.indexOf("createReview") < types.lastIndexOf("updateComment"), "finalize after review");
}

function testNewCommentIdFormat() {
  const id = newCommentId("12-3");
  // Format: ocr-<runId>-<attempt>-<16 hex chars> (crypto.randomBytes(8)).
  assert.match(id, /^ocr-12-3-[a-f0-9]{16}$/, "id format is ocr-<run>-<hex>");
  // Random -> two calls produce distinct IDs (so two comments that share
  // path/line/content still get different IDs and the check never mistakes
  // one for the other).
  assert.notStrictEqual(newCommentId("1-1"), newCommentId("1-1"), "IDs are random per call");
}

async function testGetPostedCommentIdsExtractsEmbeddedIds() {
  const github = {
    rest: {
      pulls: {
        listReviewComments: async () => ({
          data: [
            { body: "<!-- ocr-0-1-aaaa0000bbbb1111 -->\ncontent a" },
            { body: "no id here" },
            { body: "<!-- ocr-0-1-cccc2222dddd3333 -->\ncontent c" },
            // User content that mentions the bare id string must NOT match:
            // the regex is anchored to <!-- ... --> wrappers, defending against
            // false positives in the idempotency check.
            { body: "see ocr-0-1-aaaa0000bbbb1111 somewhere" },
          ],
          headers: {},
        }),
      },
    },
  };
  const ids = await getPostedCommentIds({ github, owner: "o", repo: "r", prNumber: 1, log: () => {} });
  assert.strictEqual(ids.size, 2, "only IDs inside HTML comment wrappers are extracted");
  assert.ok(ids.has("ocr-0-1-aaaa0000bbbb1111"));
  assert.ok(ids.has("ocr-0-1-cccc2222dddd3333"));
  assert.ok(!ids.has("ocr-0-1-zzzz0000"), "non-hex tokens do not match");
}

// ---- computeRetryDelayMs unit tests ----
//
// The rate-limit retry strategy is a pure function of the error (status +
// response headers) and attempt number. The integration tests below cap every
// delay to ~1ms via OCR_RETRY_MAX_DELAY=1, so they cannot assert that specific
// headers are honored; these unit tests pin down each branch of the strategy
// directly. They run under realistic cap/base values (overridden locally) so
// the returned delayMs is meaningful.

function testComputeRetryDelayMs() {
  // Use realistic cap/base so delayMs reflects the strategy rather than the
  // 1ms test-harness cap. Restored at the end.
  const realCap = process.env.OCR_RETRY_MAX_DELAY;
  const realBase = process.env.OCR_RETRY_BASE_DELAY;
  process.env.OCR_RETRY_MAX_DELAY = "300000";
  process.env.OCR_RETRY_BASE_DELAY = "60000";
  try {
    // Non-error / non-retryable -> null (no retry).
    assert.strictEqual(computeRetryDelayMs(null, 0), null);
    assert.strictEqual(computeRetryDelayMs(makeErr("validation", 422), 0), null);

    // 429 honoring retry-after (seconds form): delay = secs * 1000.
    let r = computeRetryDelayMs(makeErr("rate", 429, { "retry-after": "5" }), 0);
    assert.strictEqual(r.source, "retry-after");
    assert.strictEqual(r.delayMs, 5000);

    // 429 honoring retry-after (HTTP-date form): source tagged accordingly,
    // delay ~ the time until the given date.
    const dateMs = Date.now() + 5000;
    r = computeRetryDelayMs(makeErr("rate", 429, { "retry-after": new Date(dateMs).toUTCString() }), 0);
    assert.strictEqual(r.source, "retry-after (HTTP-date)");
    assert.ok(r.delayMs > 0 && r.delayMs <= 5000, "HTTP-date retry-after within 5s window");

    // 429 with primary limit exhausted (remaining=0): wait until reset epoch.
    const reset = Math.floor(Date.now() / 1000) + 10;
    r = computeRetryDelayMs(makeErr("rate", 429, { "x-ratelimit-remaining": "0", "x-ratelimit-reset": String(reset) }), 0);
    assert.strictEqual(r.source, "x-ratelimit-reset");
    assert.strictEqual(r.delayMs, 10000);

    // remaining > 0 must NOT trigger the reset branch even with a reset header.
    r = computeRetryDelayMs(makeErr("rate", 429, { "x-ratelimit-remaining": "1", "x-ratelimit-reset": String(reset) }), 0);
    assert.strictEqual(r.source, "exponential-backoff");

    // 429 with no hint: exponential backoff, base*2^attempt + 0..999 jitter.
    r = computeRetryDelayMs(makeErr("rate", 429), 0);
    assert.strictEqual(r.source, "exponential-backoff");
    assert.ok(r.delayMs >= 60000 && r.delayMs <= 60999, "attempt 0 backoff = 60000 + jitter");
    r = computeRetryDelayMs(makeErr("rate", 429), 2);
    assert.ok(r.delayMs >= 240000 && r.delayMs <= 240999, "attempt 2 backoff = 240000 + jitter");

    // 403 is a rate-limit ONLY when the message mentions rate limit/abuse/secondary.
    assert.ok(computeRetryDelayMs(makeErr("rate limit exceeded", 403), 0) != null, "403 + 'rate limit' retryable");
    assert.ok(computeRetryDelayMs(makeErr("abuse detection", 403), 0) != null, "403 + 'abuse' retryable");
    assert.ok(computeRetryDelayMs(makeErr("secondary rate", 403), 0) != null, "403 + 'secondary' retryable");
    assert.strictEqual(computeRetryDelayMs(makeErr("forbidden", 403), 0), null, "plain 403 not retryable");

    // 5xx transient: shorter base (2000ms) than rate-limit, grows with attempt.
    r = computeRetryDelayMs(makeErr("Bad Gateway", 502), 0);
    assert.strictEqual(r.source, "transient-backoff");
    assert.ok(r.delayMs >= 2000 && r.delayMs <= 2999, "502 attempt 0 = 2000 + jitter");
    // 408 timeout is also treated as transient.
    assert.strictEqual(computeRetryDelayMs(makeErr("timeout", 408), 0).source, "transient-backoff");

    // Cap: a huge retry-after is clamped to OCR_RETRY_MAX_DELAY.
    r = computeRetryDelayMs(makeErr("rate", 429, { "retry-after": "1000000" }), 0);
    assert.strictEqual(r.delayMs, 300000, "capped to 300000ms");
    assert.match(r.detail, /CAPPED/, "capping is surfaced in detail");
  } finally {
    if (realCap === undefined) delete process.env.OCR_RETRY_MAX_DELAY;
    else process.env.OCR_RETRY_MAX_DELAY = realCap;
    if (realBase === undefined) delete process.env.OCR_RETRY_BASE_DELAY;
    else process.env.OCR_RETRY_BASE_DELAY = realBase;
  }
}

// ---- Cross-scenario integration tests ----
//
// rate-limit × partial-invalid-content × landed-on-server intersect on the
// per-comment fallback loop, where EACH comment can independently succeed,
// fail with a non-retryable 4xx, retry on 429, or be recovered (or not) via
// the idempotency check after a 5xx/network error. The mock's perCommentError
// (comment-keyed, attempt-aware) + landedKeys/echoPosted drive these.

// P0-1: batch rate-limit (429) triggers the per-comment fallback, where SOME
// comments succeed and SOME fail with 422 (invalid content, e.g. line gone).
// Verifies success/failed counts split correctly and ONLY the failed comment
// is surfaced in the summary (successful inline comments are not duplicated
// into the summary).
async function testBatchRateLimitWithPartialInvalidContent() {
  const result = {
    comments: [
      { path: "src/a.js", content: "valid A", start_line: 1, end_line: 1 },
      { path: "src/b.js", content: "invalid B (line gone)", start_line: 99, end_line: 99 },
      { path: "src/c.js", content: "valid C", start_line: 3, end_line: 3 },
    ],
    warnings: [],
  };

  const { github, outputs } = await run({
    result,
    githubOpts: {
      bulkErrorSpec: { message: "rate limited", status: 429, headers: { "retry-after": "1" } },
      perCommentError: (rc) => {
        // b.js is invalid (422); a.js and c.js succeed.
        if (commentKey(rc) === "src/b.js|-|99") {
          return { status: 422, message: 'Unprocessable Entity: "Line could not be resolved"' };
        }
        return null;
      },
    },
    opts: { stickySummary: true },
  });

  // batch (429) + 3 per-comment calls (a ok, b 422, c ok).
  assert.strictEqual(github.createReviewCalls.length, 4, "batch + 3 per-comment attempts");
  assert.strictEqual(outputs.comments_inline, "2", "a and c posted");
  assert.strictEqual(outputs.comments_failed, "1", "b failed (invalid content)");
  // Fix B: a pure 429 never reached the server, so the idempotency reads must
  // be skipped entirely (no listReviews / listReviewComments).
  assert.strictEqual(github.listReviewsCalls.length, 0, "429 batch skips listReviews idempotency read");
  assert.strictEqual(github.listReviewCommentsCalls.length, 0, "no per-comment idempotency reads (422 non-retryable, successes need none)");
  // Summary surfaces ONLY the failed comment (in the finalized body).
  assert.strictEqual(github.issueComments.length, 1, "anchor created");
  assert.strictEqual(github.updatedComments.length, 1, "anchor finalized");
  const body = github.updatedComments[0].body;
  assert.match(body, /invalid B/, "failed comment content appears in summary");
  assert.doesNotMatch(body, /valid A/, "successful comment not duplicated into summary");
  assert.doesNotMatch(body, /valid C/, "successful comment not duplicated into summary");
}

// P0-2: per-comment rate-limit with retries. One comment recovers after a
// retry (429 then success); another stays rate-limited until retries are
// exhausted. Requires OCR_MAX_RETRIES >= 1 (overridden locally).
async function testPerCommentRateLimitRetryThenSuccessAndExhausted() {
  const result = {
    comments: [
      { path: "src/a.js", content: "recovers after retry", start_line: 1, end_line: 1 },
      { path: "src/b.js", content: "always rate limited", start_line: 2, end_line: 2 },
    ],
    warnings: [],
  };

  return withEnv({ OCR_MAX_RETRIES: "1" }, async () => {
    const { github, outputs } = await run({
      result,
      githubOpts: {
        bulkErrorSpec: { message: "rate limited", status: 429 },
        perCommentError: (rc, attempt) => {
          if (commentKey(rc) === "src/a.js|-|1") {
            // a.js: 429 on attempt 0, success on attempt 1.
            return attempt === 0 ? { status: 429, message: "rate limited" } : null;
          }
          // b.js: always 429 -> retries exhausted -> failed.
          return { status: 429, message: "rate limited" };
        },
      },
      opts: { stickySummary: true },
    });

    // batch + a(2 attempts: 429 then ok) + b(2 attempts: 429, 429 exhausted).
    assert.strictEqual(github.createReviewCalls.length, 5, "batch + a(2) + b(2)");
    assert.strictEqual(outputs.comments_inline, "1", "a recovered via retry");
    assert.strictEqual(outputs.comments_failed, "1", "b exhausted all retries");
  });
}

// P0-3: batch 5xx but the batch LANDED on the server. The batch-level
// idempotency check finds some comments already posted; the MISSING ones are
// retried per-comment, where one fails with 422 (invalid content). Verifies
// batch-level dedup and per-comment failure compose without double-counting.
async function testBatchLandedWithPerCommentPartialInvalid() {
  const result = {
    comments: [
      { path: "src/a.js", content: "already landed A", start_line: 1, end_line: 1 },
      { path: "src/b.js", content: "already landed B", start_line: 2, end_line: 2 },
      { path: "src/c.js", content: "invalid C", start_line: 99, end_line: 99 },
    ],
    warnings: [],
  };

  const { github, outputs } = await run({
    result,
    githubOpts: {
      bulkError: "Bad Gateway",
      bulkErrorStatus: 502,
      batchLanded: true,
      echoPosted: true,
      postedCount: 2, // a and b already on the server
      perCommentError: (rc) => {
        if (commentKey(rc) === "src/c.js|-|99") {
          return { status: 422, message: 'Unprocessable Entity: "Line could not be resolved"' };
        }
        return null;
      },
    },
    opts: { stickySummary: true },
  });

  // batch (502, landed) + only the 1 missing comment (c) retried, which 422s.
  assert.strictEqual(github.createReviewCalls.length, 2, "batch + only missing c retried");
  assert.strictEqual(outputs.comments_inline, "2", "a,b recovered via batch-landing; c failed");
  assert.strictEqual(outputs.comments_failed, "1", "c invalid content");
}

// P0-4: the full four-state mix under a landed batch. Combines batch-level
// landing with per-comment: success, 422-invalid, 5xx-landed (recovered via
// idempotency), and 5xx-NOT-landed (failed). This is the most entangled
// intersection of all three scenarios.
async function testBatchLandedWithPerCommentMixedStates() {
  const result = {
    comments: [
      { path: "src/a.js", content: "batch-landed A", start_line: 1, end_line: 1 },
      { path: "src/b.js", content: "success B", start_line: 2, end_line: 2 },
      { path: "src/c.js", content: "invalid C", start_line: 99, end_line: 99 },
      { path: "src/d.js", content: "5xx landed D", start_line: 4, end_line: 4 },
      { path: "src/e.js", content: "5xx not landed E", start_line: 5, end_line: 5 },
    ],
    warnings: [],
  };

  const landedKeys = new Set(["src/d.js|-|4"]);
  const { outputs } = await run({
    result,
    githubOpts: {
      bulkError: "Bad Gateway",
      bulkErrorStatus: 502,
      batchLanded: true,
      echoPosted: true,
      postedCount: 1, // only a batch-landed
      landedKeys, // d lands despite its per-comment 502
      perCommentError: (rc) => {
        const key = commentKey(rc);
        if (key === "src/c.js|-|99") return { status: 422, message: "Line could not be resolved" };
        if (key === "src/d.js|-|4") return { status: 502, message: "Bad Gateway" };
        if (key === "src/e.js|-|5") return { status: 502, message: "Bad Gateway" };
        return null; // b succeeds
      },
    },
    opts: { stickySummary: true },
  });

  // a(batch-landed) + b(success) + d(5xx-landed) = 3 successes;
  // c(422) + e(5xx-not-landed) = 2 failures.
  assert.strictEqual(outputs.comments_inline, "3", "a+b+d succeed across three different recovery paths");
  assert.strictEqual(outputs.comments_failed, "2", "c(422) + e(5xx not landed) fail");
}

// P1: a network-layer error (no HTTP status) is treated as "maybe reached the
// server", so the idempotency check runs. A comment that landed is recovered;
// one that did not is recorded as failed (no blind retry that would duplicate).
async function testNetworkErrorLandedRecoveredAndNotLandedFailed() {
  const result = {
    comments: [
      { path: "src/a.js", content: "net landed", start_line: 1, end_line: 1 },
      { path: "src/b.js", content: "net not landed", start_line: 2, end_line: 2 },
    ],
    warnings: [],
  };

  const landedKeys = new Set(["src/a.js|-|1"]);
  const { outputs } = await run({
    result,
    githubOpts: {
      bulkError: "Bad Gateway",
      bulkErrorStatus: 502,
      landedKeys,
      // status omitted -> typeof status !== "number" && status == null ->
      // maybeReachedServer=true -> idempotency check decides.
      perCommentError: () => ({ message: "ECONNRESET" }),
    },
    opts: { stickySummary: true },
  });

  assert.strictEqual(outputs.comments_inline, "1", "a recovered (landed) via idempotency check");
  assert.strictEqual(outputs.comments_failed, "1", "b not landed -> failed, no blind retry");
}

// P1: the batch-level idempotency check itself throws (listReviews
// unavailable). The code degrades to the original fallback (retry ALL
// comments, accepting duplicate risk) rather than aborting.
async function testBatchIdempotencyCheckFailureDegradesToFullRetry() {
  const result = {
    comments: [
      { path: "src/a.js", content: "A", start_line: 1, end_line: 1 },
      { path: "src/b.js", content: "B", start_line: 2, end_line: 2 },
    ],
    warnings: [],
  };

  const { github, outputs } = await run({
    result,
    githubOpts: {
      bulkError: "Bad Gateway",
      bulkErrorStatus: 502,
      listReviewsThrow: true, // findExistingBatchReview fails -> degrade
      perCommentError: () => null, // all per-comment succeed
    },
    opts: { stickySummary: true },
  });

  // Degrade retries ALL (no filtering) -> batch + 2 per-comment.
  assert.strictEqual(github.createReviewCalls.length, 3, "degraded to full retry of all comments");
  assert.strictEqual(outputs.comments_inline, "2");
  assert.strictEqual(outputs.comments_failed, "0");
}

// P1 (smoke): low remaining quota on a per-comment success triggers the
// proactive throttle branch. We cannot spy on the internal sleep, so this
// only verifies the branch executes without breaking the flow or counts.
async function testLowQuotaProactiveThrottleDoesNotBreakFlow() {
  return withEnv({ OCR_LOW_REMAINING_THRESHOLD: "3" }, async () => {
    const result = { comments: [{ path: "src/a.js", content: "A", start_line: 1, end_line: 1 }], warnings: [] };
    const { outputs } = await run({
      result,
      githubOpts: {
        bulkError: "rate limited",
        bulkErrorStatus: 429, // force the per-comment fallback path
        successRemaining: 2, // <= threshold -> low-quota branch
        perCommentError: () => null,
      },
      opts: { stickySummary: true },
    });
    assert.strictEqual(outputs.comments_inline, "1", "low-quota throttle does not impede success");
  });
}

// Fix B (focused): a pure rate-limit (429) on the batch means the request never
// reached the server, so the batch did not land. The idempotency reads
// (listReviews / listReviewComments) must be SKIPPED entirely — querying would
// be pointless and would pressure the API during an ongoing rate-limit episode.
// The batch rate-limit cooldown still runs before the per-comment retry.
async function testBatchRateLimitSkipsIdempotencyReads() {
  const result = {
    comments: [{ path: "src/a.js", content: "A", start_line: 1, end_line: 1 }],
    warnings: [],
  };

  const { github, outputs } = await run({
    result,
    githubOpts: {
      bulkErrorSpec: { message: "rate limited", status: 429, headers: { "retry-after": "1" } },
      perCommentError: () => null, // per-comment succeeds
    },
    opts: { stickySummary: true },
  });

  assert.strictEqual(github.listReviewsCalls.length, 0, "listReviews not called (429 never reached server)");
  assert.strictEqual(github.listReviewCommentsCalls.length, 0, "listReviewComments not called");
  assert.strictEqual(github.createReviewCalls.length, 2, "batch + 1 per-comment");
  assert.strictEqual(outputs.comments_inline, "1");
}

// Fix A + read self-protection: a 5xx batch MAY have landed, so the idempotency
// read runs — but only AFTER cooling down. The read itself can also hit a
// rate-limit; withRetry (wrapping readWithPacing) must back off and recover so
// the batch-landing detection still works. Requires OCR_MAX_RETRIES >= 1.
async function testBatchReadRateLimitRetriedViaWithRetry() {
  const result = {
    comments: [
      { path: "src/a.js", content: "A", start_line: 1, end_line: 1 },
      { path: "src/b.js", content: "B", start_line: 2, end_line: 2 },
    ],
    warnings: [],
  };

  return withEnv({ OCR_MAX_RETRIES: "1" }, async () => {
    const { github, outputs } = await run({
      result,
      githubOpts: {
        bulkError: "Bad Gateway",
        bulkErrorStatus: 502,
        batchLanded: true,
        echoPosted: true,
        postedCount: 2, // both comments already on the server
        // The idempotency read (listReviews) itself is rate-limited once, then
        // succeeds: withRetry must honor retry-after and recover.
        listReviewsErrorSeq: [
          { status: 429, message: "rate limited", headers: { "retry-after": "1" } },
        ],
      },
      opts: { stickySummary: true },
    });

    // listReviews: 1st call 429, 2nd call success -> read recovered via retry.
    assert.strictEqual(github.listReviewsCalls.length, 2, "read retried after its own 429");
    assert.strictEqual(outputs.comments_inline, "2", "both recovered as already-posted");
    assert.strictEqual(outputs.comments_failed, "0");
    assert.strictEqual(github.createReviewCalls.length, 1, "no per-comment retry (all already posted)");
  });
}

// ---- Pure helper unit tests ----

function testSafeFenceAndFencedBlock() {
  assert.strictEqual(safeFence("plain"), "```");
  // single backticks -> maxTicks=1 -> max(3, 2) = 3
  assert.strictEqual(safeFence("a `backtick` here"), "```");
  // 5 backticks -> maxTicks=5 -> 6
  assert.strictEqual(safeFence("`````"), "``````");
  const block = fencedBlock("```js\nx\n```");
  assert.ok(block.startsWith("````"));
  assert.ok(block.endsWith("````"));
}

function testLineSpan() {
  assert.deepStrictEqual(lineSpan({ line: 10, start_line: 5 }), { start: 5, end: 10, multiline: true });
  assert.deepStrictEqual(lineSpan({ line: 7 }), { start: 7, end: 7, multiline: false });
  assert.deepStrictEqual(lineSpan({ start_line: 3 }), { start: 3, end: 3, multiline: false });
  // start_line === line collapses to a single-line span.
  assert.deepStrictEqual(lineSpan({ line: 9, start_line: 9 }), { start: 9, end: 9, multiline: false });
  assert.strictEqual(lineSpan({}), null);
  // Invalid line numbers (0, negative, NaN) are dropped by num(); a span with
  // no usable line resolves to null.
  assert.strictEqual(lineSpan({ line: 0 }), null);
  assert.strictEqual(lineSpan({ line: -3 }), null);
  assert.strictEqual(lineSpan({ line: NaN }), null);
  // An invalid start_line but valid line degrades to a single-line span.
  assert.deepStrictEqual(lineSpan({ line: 5, start_line: 0 }), { start: 5, end: 5, multiline: false });
  assert.deepStrictEqual(lineSpan({ line: 5, start_line: -1 }), { start: 5, end: 5, multiline: false });
  // Reversed order (start_line > line) is normalized via min/max.
  assert.deepStrictEqual(lineSpan({ line: 3, start_line: 8 }), { start: 3, end: 8, multiline: true });
}

function testSameCommentSpan() {
  const sl = (n) => ({ start: n, end: n, multiline: false });
  const ml = (a, b) => ({ start: a, end: b, multiline: true });
  // Rule 1: single vs multi never match.
  assert.strictEqual(sameCommentSpan(sl(9), ml(8, 10), 0.6), false);
  assert.strictEqual(sameCommentSpan(ml(8, 10), sl(9), 0.6), false);
  // Rule 2: single-line, same line matches; different line does not.
  assert.strictEqual(sameCommentSpan(sl(9), sl(9), 0.6), true);
  assert.strictEqual(sameCommentSpan(sl(9), sl(10), 0.6), false);
  // Rule 3: multi-line IoU. [8,10] vs [9,11] => overlap 2 / union 4 = 0.5.
  assert.strictEqual(sameCommentSpan(ml(8, 10), ml(9, 11), 0.6), false);
  assert.strictEqual(sameCommentSpan(ml(8, 10), ml(9, 11), 0.4), true);
  // [8,10] vs [8,9] => overlap 2 / union 3 ~= 0.67.
  assert.strictEqual(sameCommentSpan(ml(8, 10), ml(8, 9), 0.6), true);
  // Identical spans => IoU 1.
  assert.strictEqual(sameCommentSpan(ml(8, 10), ml(8, 10), 0.6), true);
  // Disjoint multi-line spans never match.
  assert.strictEqual(sameCommentSpan(ml(1, 3), ml(8, 10), 0.6), false);
  // IoU comparison is strict: exactly at the threshold is NOT a match.
  // [8,10] vs [9,11] => IoU 0.5; threshold 0.5 => 0.5 > 0.5 is false.
  assert.strictEqual(sameCommentSpan(ml(8, 10), ml(9, 11), 0.5), false);
  // Single-line matching (rule 2) ignores threshold entirely: same line still
  // matches even at threshold = 1.
  assert.strictEqual(sameCommentSpan(sl(9), sl(9), 1), true);
  // threshold = 1 is unreachable for multi-line under strict >: even identical
  // spans (IoU 1) do not satisfy 1 > 1, so nothing ever matches. Locks the
  // strict-> semantics.
  assert.strictEqual(sameCommentSpan(ml(8, 10), ml(8, 10), 1), false);
}

function testResolveThreshold() {
  // Valid values in (0, 1] pass through unchanged.
  assert.strictEqual(resolveThreshold(0.6), 0.6);
  assert.strictEqual(resolveThreshold(0.5), 0.5);
  assert.strictEqual(resolveThreshold(1), 1);
  // Numeric strings are accepted (mirrors parseFloat(action input)).
  assert.strictEqual(resolveThreshold("0.4"), 0.4);
  // Out-of-range values fall back to the default.
  assert.strictEqual(resolveThreshold(0), DEFAULT_OVERLAP_THRESHOLD);
  assert.strictEqual(resolveThreshold(-0.5), DEFAULT_OVERLAP_THRESHOLD);
  assert.strictEqual(resolveThreshold(1.5), DEFAULT_OVERLAP_THRESHOLD);
  // Non-numeric / missing values fall back to the default.
  assert.strictEqual(resolveThreshold(NaN), DEFAULT_OVERLAP_THRESHOLD);
  assert.strictEqual(resolveThreshold("abc"), DEFAULT_OVERLAP_THRESHOLD);
  assert.strictEqual(resolveThreshold(undefined), DEFAULT_OVERLAP_THRESHOLD);
  assert.strictEqual(resolveThreshold(null), DEFAULT_OVERLAP_THRESHOLD);
}

function testOverlapsHistory() {
  // Rule 2: single-line, same line => overlap; different line => no overlap.
  const sl = [{ path: "a.js", line: 9, side: "RIGHT" }];
  assert.strictEqual(overlapsHistory({ path: "a.js", line: 9, start_line: 9, side: "RIGHT" }, sl), true);
  assert.strictEqual(overlapsHistory({ path: "a.js", line: 20, start_line: 20, side: "RIGHT" }, sl), false);
  // Rule 1: single-line vs multi-line never overlap.
  const ml = [{ path: "a.js", line: 10, start_line: 8, side: "RIGHT" }];
  assert.strictEqual(overlapsHistory({ path: "a.js", line: 9, start_line: 9, side: "RIGHT" }, ml), false);
  // Rule 3: multi-line IoU vs default threshold 0.6.
  assert.strictEqual(overlapsHistory({ path: "a.js", line: 10, start_line: 8, side: "RIGHT" }, ml), true);
  assert.strictEqual(overlapsHistory({ path: "a.js", line: 11, start_line: 9, side: "RIGHT" }, ml), false);
  assert.strictEqual(overlapsHistory({ path: "a.js", line: 9, start_line: 8, side: "RIGHT" }, ml), true);
  // Threshold argument lowers the bar (IoU 0.5 > 0.4).
  assert.strictEqual(overlapsHistory({ path: "a.js", line: 11, start_line: 9, side: "RIGHT" }, ml, 0.4), true);
  // Different path and LEFT-side history are still ignored.
  assert.strictEqual(overlapsHistory({ path: "b.js", line: 10, start_line: 8, side: "RIGHT" }, ml), false);
  const leftHist = [{ path: "a.js", line: 10, start_line: 8, side: "LEFT" }];
  assert.strictEqual(overlapsHistory({ path: "a.js", line: 10, start_line: 8, side: "RIGHT" }, leftHist), false);
  // An unresolvable current comment (no usable line) never overlaps.
  assert.strictEqual(overlapsHistory({ path: "a.js", side: "RIGHT" }, ml), false);
  // Unresolvable history entries are skipped, not fatal: a later valid entry
  // on the same path can still match.
  const mixedHist = [
    { path: "a.js", side: "RIGHT" }, // no line info -> lineSpan null
    { path: "a.js", line: 9, side: "RIGHT" }, // single-line 9
  ];
  assert.strictEqual(overlapsHistory({ path: "a.js", line: 9, start_line: 9, side: "RIGHT" }, mixedHist), true);
  // Any-of semantics: multiple history entries, a match on any one wins.
  const multiHist = [
    { path: "a.js", line: 5, start_line: 5, side: "RIGHT" }, // no match
    { path: "a.js", line: 10, start_line: 8, side: "RIGHT" }, // matches [8,10]
  ];
  assert.strictEqual(overlapsHistory({ path: "a.js", line: 10, start_line: 8, side: "RIGHT" }, multiHist), true);
  // A history entry with no side field still participates (falsy side bypasses
  // the RIGHT-only guard).
  const noSideHist = [{ path: "a.js", line: 9 }];
  assert.strictEqual(overlapsHistory({ path: "a.js", line: 9, start_line: 9, side: "RIGHT" }, noSideHist), true);
  // An invalid threshold falls back to the default (IoU 0.5 < 0.6 -> no match).
  assert.strictEqual(overlapsHistory({ path: "a.js", line: 11, start_line: 9, side: "RIGHT" }, ml, "garbage"), false);
}

async function main() {
  await testFailedInlineCommentsAreSummarized();
  await testWarningsListedAfterSummaryComments();
  await testErrorCommentUsesSafeFence();
  await testStickyUpdatesExistingSummary();
  await testNonStickyCreatesNewCommentOnFallback();
  await testNonStickyFallbackAllSuccessStillPostsSummary();
  await testNoCommentsStickyUpdate();
  await testIncrementalSkipsOverlapping();
  await testIncrementalAllOverlapPostsNoReview();
  await testIncrementalMultiLineIoUDefaultThreshold();
  await testIncrementalOverlapThresholdPropagated();
  // Idempotency
  await testBatchLandedRetriesOnlyMissingComments();
  await testPerComment5xxAlreadyPostedTreatedAsSuccess();
  await testPerComment5xxIdempotencyUnavailableSkipsRetry();
  await testSummaryDoesNotDuplicateWhenAlreadyPosted();
  await testSummaryAnchorCreatedBeforeReviewColdStart();
  await testSummaryAnchorCreatedBeforeReviewNonSticky();
  await testGetPostedCommentIdsExtractsEmbeddedIds();
  // Rate-limit strategy (pure function)
  testComputeRetryDelayMs();
  // Cross-scenario: rate-limit x partial-invalid x landed
  await testBatchRateLimitWithPartialInvalidContent();
  await testPerCommentRateLimitRetryThenSuccessAndExhausted();
  await testBatchLandedWithPerCommentPartialInvalid();
  await testBatchLandedWithPerCommentMixedStates();
  await testNetworkErrorLandedRecoveredAndNotLandedFailed();
  await testBatchIdempotencyCheckFailureDegradesToFullRetry();
  await testLowQuotaProactiveThrottleDoesNotBreakFlow();
  await testBatchRateLimitSkipsIdempotencyReads();
  await testBatchReadRateLimitRetriedViaWithRetry();
  // Pure helpers
  testSafeFenceAndFencedBlock();
  testFormatWarnings();
  testLineSpan();
  testSameCommentSpan();
  testResolveThreshold();
  testOverlapsHistory();
  testNewCommentIdFormat();
  console.log("All post-review-comments tests passed.");
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
