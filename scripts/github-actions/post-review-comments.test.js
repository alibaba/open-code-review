#!/usr/bin/env node

// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

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
const { runPostReviewComments, safeFence, fencedBlock, lineSpan, sameCommentSpan, overlapsHistory, resolveThreshold, DEFAULT_OVERLAP_THRESHOLD, newCommentId, getPostedCommentIds, computeRetryDelayMs, formatWarnings, resolveBatchSize, sortToSendDeterministically, chunkArray, buildRunTags, DEFAULT_BATCH_SIZE, buildBadge, buildBadgeImage, SEVERITY_BADGE_COLOR, sanitizeMetadata, buildPolicy, routeComment, formatComment, formatCommentMarkdown, NO_ROUTING, CATEGORIES, SEVERITIES, SEVERITY_RANK, parseDiffHunkRanges, classifyCommentAgainstDiff, describeCommentLocation, isLineResolutionFailure, getPrDiffHunks, SUMMARY_MARKER, buildCheckpointMarker, parseCheckpointMarker, validateCheckpointPayload, readCheckpointComment, resolveCheckpointRange, isCheckpointAuthorOurs, preserveCheckpointMarker } = require(path.join(__dirname, "post-review-comments.js"));

// REVIEW_TAG as the production code builds it for this test's hardcoded run
// identity (context.runId=undefined -> 0, runAttempt=undefined -> 1). Used as
// the primary discriminator between batch createReview calls (body ===
// REVIEW_TAG) and per-comment fallback calls (body === ""). Reconstructed via
// the exported buildRunTags rather than hardcoded so it tracks any future tag
// format change. `length > 1` is NOT a safe discriminator once N=1 batches
// exist (a single-comment batch collides with the per-comment shape).
const REVIEW_TAG = buildRunTags(undefined, undefined).REVIEW_TAG;

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

function makeErr(message, status, headers, data) {
  const e = new Error(message);
  if (status != null) e.status = status;
  if (headers || data) e.response = {};
  if (headers) e.response.headers = headers;
  // Real Octokit puts the parsed response body here, and it is the only source
  // of the errors[] strings GitHub actually returns. Tests that omit it exercise
  // a shape production never sees.
  if (data) e.response.data = data;
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
  const listFilesCalls = [];
  const getPullCalls = [];
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

  // Inline comment objects recorded in BATCH createReview calls only, so tests
  // can simulate "this comment already landed on the server" without predicting
  // the random IDs from newCommentId(). Under multi-batch (N < toSend.length)
  // there are several batch calls (body === REVIEW_TAG), so this scans ALL batch
  // calls — not just createReviewCalls[0]. Scoped to batch calls so batch-level
  // landing (echoPosted) stays disjoint from per-comment landing (landedKeys,
  // which reads per-comment calls with body === "").
  function batchPostedComments() {
    const out = [];
    for (const call of createReviewCalls) {
      if ((call.body || "") !== REVIEW_TAG) continue;
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

  // Count of batch createReview calls issued so far (body === REVIEW_TAG), so a
  // per-batch-index error spec can target e.g. "fail batch #2 but not #1".
  function batchCallCount() {
    let n = 0;
    for (const call of createReviewCalls) {
      if ((call.body || "") === REVIEW_TAG) n++;
    }
    return n;
  }

  return {
    createReviewCalls,
    issueComments,
    updatedComments,
    listCommentsCalls,
    listReviewCommentsCalls,
    listReviewsCalls,
    listFilesCalls,
    getPullCalls,
    ops,
    rest: {
      users: {
        getAuthenticated: async () => ({ data: { login: "github-actions[bot]" } }),
      },
      pulls: {
        get: async (params) => {
          getPullCalls.push(params);
          return { data: { head: { sha: opts.headSha || "head-sha" } } };
        },
        createReview: async (params) => {
          createReviewCalls.push(params);
          ops.push({ type: "createReview", params });
          const callIdx = createReviewCalls.length - 1;
          const successRes = () => ({ data: {}, headers: { "x-ratelimit-remaining": successRemaining() } });
          // Discriminate batch vs per-comment by body, NOT callIdx. Under
          // multi-batch (N < toSend.length) several batch calls precede the
          // per-comment fallbacks, so callIdx === 0 is unsound. Batch calls
          // carry body === REVIEW_TAG; per-comment fallback calls use body === "".
          // (comments.length > 1 is NOT a safe discriminator once N=1 batches
          // exist — a single-comment batch collides with per-comment shape.)
          const isBatch = (params.body || "") === REVIEW_TAG;
          if (isBatch) {
            const batchIdx = batchCallCount() - 1;
            // Per-batch error spec takes precedence (lets a test fail batch #2
            // but not #1); then the legacy bulkError/bulkErrorSpec apply to all
            // batches uniformly.
            if (typeof opts.batchErrorSpec === "function") {
              const spec = opts.batchErrorSpec(batchIdx);
              if (spec) throw makeErr(spec.message, spec.status, spec.headers, spec.data);
            } else if (Array.isArray(opts.batchErrorSpec)) {
              const spec = opts.batchErrorSpec[batchIdx];
              if (spec) throw makeErr(spec.message, spec.status, spec.headers, spec.data);
            }
            if (opts.bulkErrorSpec) {
              throw makeErr(
                opts.bulkErrorSpec.message,
                opts.bulkErrorSpec.status,
                opts.bulkErrorSpec.headers,
                opts.bulkErrorSpec.data
              );
            }
            if (opts.bulkError) {
              throw makeErr(opts.bulkError, opts.bulkErrorStatus, opts.bulkHeaders);
            }
            return successRes();
          }
          // Per-comment call. perCommentError(rc, attempt) lets a test fail some
          // comments and not others (partial failure), and be attempt-aware
          // (retry-then-succeed). Falls back to the legacy individualError
          // (applies to all per-comment calls) for older tests.
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
        listFiles: async (params) => {
          listFilesCalls.push(params);
          if (opts.listFilesThrow) {
            throw makeErr(opts.listFilesError || "listFiles unavailable", opts.listFilesStatus || 503);
          }
          // Honor page/per_page so tests can exercise the multi-page walk and
          // the >MAX_PAGES truncation guard, not just a single short page.
          const all = opts.files || [];
          const perPage = params.per_page || 100;
          const page = params.page || 1;
          return { data: all.slice((page - 1) * perPage, page * perPage) };
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
          // echoBatchIdx: echo ONLY the comments carried by batch call #N
          // (0-based among batch calls). Needed to simulate "the SECONDARY
          // filtered batch landed but its response was lost" without also
          // marking the primary batch's comments as posted — echoPosted scans
          // ALL batch calls, and the two calls share comment IDs.
          if (opts.echoBatchIdx != null) {
            let seen = -1;
            for (const call of createReviewCalls) {
              if ((call.body || "") !== REVIEW_TAG) continue;
              seen++;
              if (seen !== opts.echoBatchIdx) continue;
              // postedCount echoes only the FIRST N of that batch's comments, so
              // a test can simulate a partially-landed review: the reconciler
              // must re-send exactly the comments the server never received.
              const carried = call.comments || [];
              const n = opts.postedCount != null ? opts.postedCount : carried.length;
              return {
                data: carried.slice(0, n).map((c) => ({
                  path: c.path,
                  body: c.body,
                  side: c.side || "RIGHT",
                  start_line: c.start_line,
                  line: c.line,
                })),
              };
            }
            return { data: [] };
          }
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
  const logs = [];
  return {
    outputs,
    logs,
    setOutput(name, value) { outputs[name] = value; },
    info(message) { logs.push(message); },
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

// P1: the batch-level review lookup itself throws. Because the failed write may
// have landed, retrying all comments would duplicate them. Finalize visibly
// instead: no retry, each unverified item accounted as failed.
async function testBatchIdempotencyCheckFailureStopsVisibly() {
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
      perCommentError: () => null, // would succeed—and duplicate—if retried
    },
    opts: { stickySummary: true },
  });

  assert.strictEqual(github.createReviewCalls.length, 1, "an ambiguous landed batch must not be retried");
  assert.strictEqual(outputs.comments_inline, "0");
  assert.strictEqual(outputs.comments_failed, "2");
  assert.strictEqual(github.updatedComments.length, 1, "the summary must still finalize");
  assert.strictEqual(github.updatedComments[0].body.split("Could not verify whether").length - 1, 2);
  assert.strictEqual(github.updatedComments[0].body.includes("A"), true);
  assert.strictEqual(github.updatedComments[0].body.includes("B"), true);
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

// ---- Batching tests (issue #479) ----

// Build N synthetic inline-commentable comments with deterministic, distinct
// (path, line) identities so partitioning/sorting is observable. Line numbers
// increase with the index so the deterministic sort (path → start_line →
// end_line → origIndex) reproduces the input order for same-path entries.
function makeComments(n) {
  const out = [];
  for (let i = 0; i < n; i++) {
    out.push({ path: `src/file${i}.js`, content: `comment ${i}`, start_line: i + 1, end_line: i + 1 });
  }
  return out;
}

// Pure-helper: resolveBatchSize clamps invalid/missing values to the default
// and passes valid positives through (B1/A2).
function testResolveBatchSize() {
  assert.strictEqual(resolveBatchSize(1), 1, "minimum valid size");
  assert.strictEqual(resolveBatchSize(50), 50);
  assert.strictEqual(resolveBatchSize(1000), 1000);
  // Invalid: 0, negative, NaN, non-numeric, missing -> default.
  assert.strictEqual(resolveBatchSize(0), DEFAULT_BATCH_SIZE);
  assert.strictEqual(resolveBatchSize(-5), DEFAULT_BATCH_SIZE);
  assert.strictEqual(resolveBatchSize(NaN), DEFAULT_BATCH_SIZE);
  assert.strictEqual(resolveBatchSize("garbage"), DEFAULT_BATCH_SIZE);
  assert.strictEqual(resolveBatchSize(""), DEFAULT_BATCH_SIZE);
  assert.strictEqual(resolveBatchSize(undefined), DEFAULT_BATCH_SIZE);
  assert.strictEqual(resolveBatchSize(null), DEFAULT_BATCH_SIZE);
  // Numeric strings parse (mirrors parseInt of the action input).
  assert.strictEqual(resolveBatchSize("20"), 20);
}

// Pure-helper: chunkArray partitions into contiguous slices; the last slice is
// the remainder (B1/AS2/AS3).
function testChunkArray() {
  assert.deepStrictEqual(chunkArray([], 5), []);
  assert.deepStrictEqual(chunkArray([1], 5), [[1]]);
  // Exact multiple: last slice is full-sized.
  assert.deepStrictEqual(chunkArray([1, 2, 3, 4], 2), [[1, 2], [3, 4]]);
  // Remainder: last slice is the leftover.
  assert.deepStrictEqual(chunkArray([1, 2, 3, 4, 5], 2), [[1, 2], [3, 4], [5]]);
  // 71 @ 20 -> [20,20,20,11] (the canonical acceptance scenario AS3).
  const chunks = chunkArray(makeComments(71).map((_, i) => i), 20);
  assert.deepStrictEqual(chunks.map((c) => c.length), [20, 20, 20, 11]);
}

// Pure-helper: sortToSendDeterministically is stable and does not mutate the
// input (B2/AS4).
function testSortToSendDeterministically() {
  const items = [
    { comment: { path: "b.js", start_line: 5, end_line: 5 } },
    { comment: { path: "a.js", start_line: 10, end_line: 10 } },
    { comment: { path: "a.js", start_line: 3, end_line: 3 } },
    { comment: { path: "a.js", start_line: 3, end_line: 7 } },
  ];
  const snapshot = items.map((i) => i.comment);
  const sorted = sortToSendDeterministically(items);
  // Input not mutated.
  assert.deepStrictEqual(items.map((i) => i.comment), snapshot, "input array not mutated");
  // Order: a.js:3-3, a.js:3-7, a.js:10-10, b.js:5-5.
  assert.strictEqual(sorted[0].comment.path, "a.js");
  assert.strictEqual(sorted[0].comment.start_line, 3);
  assert.strictEqual(sorted[0].comment.end_line, 3);
  assert.strictEqual(sorted[1].comment.start_line, 3);
  assert.strictEqual(sorted[1].comment.end_line, 7);
  assert.strictEqual(sorted[2].comment.start_line, 10);
  assert.strictEqual(sorted[3].comment.path, "b.js");
  // Determinism: identical input -> identical output across runs.
  const sorted2 = sortToSendDeterministically(items);
  assert.strictEqual(JSON.stringify(sorted2), JSON.stringify(sorted), "deterministic across runs");
}

// AS1/AS2/AS3/AS4: 71 comments @ N=20 -> exactly 4 batch createReview calls
// with comment counts [20,20,20,11]; all comment bodies present; deterministic
// across two runs.
async function testBatchPartitioningDeterministic() {
  const result = { comments: makeComments(71), warnings: [] };
  const run1 = await run({ result, opts: { reviewCommentBatchSize: 20 } });
  const batchCalls = run1.github.createReviewCalls.filter((c) => (c.body || "") === REVIEW_TAG);
  assert.strictEqual(batchCalls.length, 4, "ceil(71/20) = 4 batches");
  assert.deepStrictEqual(
    batchCalls.map((c) => c.comments.length),
    [20, 20, 20, 11],
    "partition sizes [20,20,20,11]"
  );
  // Every comment body (with its fence) appears exactly once across batches.
  const allBodies = batchCalls.flatMap((c) => c.comments.map((rc) => rc.body));
  assert.strictEqual(allBodies.length, 71, "all 71 comments present");
  // AS4: a second run produces byte-identical batch composition (the random
  // fence IDs differ, but the partition — which path/line ends up in which
  // batch — is identical).
  const run2 = await run({ result, opts: { reviewCommentBatchSize: 20 } });
  const batchCalls2 = run2.github.createReviewCalls.filter((c) => (c.body || "") === REVIEW_TAG);
  const paths1 = batchCalls.flatMap((c) => c.comments.map((rc) => rc.path));
  const paths2 = batchCalls2.flatMap((c) => c.comments.map((rc) => rc.path));
  assert.deepStrictEqual(paths2, paths1, "deterministic partition across runs");
  // Telemetry reflects the partition.
  assert.strictEqual(run1.outputs.batches_total, "4");
  assert.strictEqual(run1.outputs.batches_attempted, "4");
  assert.strictEqual(run1.outputs.batches_succeeded, "4");
  assert.strictEqual(run1.outputs.comments_inline, "71");
}

// AS2 edge: N=1 -> one createReview call per comment, each carrying exactly 1.
async function testBatchSizeOnePerComment() {
  const result = { comments: makeComments(3), warnings: [] };
  const { github, outputs } = await run({ result, opts: { reviewCommentBatchSize: 1 } });
  const batchCalls = github.createReviewCalls.filter((c) => (c.body || "") === REVIEW_TAG);
  assert.strictEqual(batchCalls.length, 3, "N=1 -> 3 single-comment batches");
  for (const c of batchCalls) {
    assert.strictEqual(c.comments.length, 1, "each batch carries exactly one comment");
  }
  assert.strictEqual(outputs.batches_total, "3");
  assert.strictEqual(outputs.comments_inline, "3");
}

// AS2 edge: N >= toSend.length -> a single batch (no regression vs the previous
// all-in-one behavior).
async function testBatchSizeLargerThanToSend() {
  const result = { comments: makeComments(3), warnings: [] };
  const { github, outputs } = await run({ result, opts: { reviewCommentBatchSize: 100 } });
  const batchCalls = github.createReviewCalls.filter((c) => (c.body || "") === REVIEW_TAG);
  assert.strictEqual(batchCalls.length, 1, "single batch when N >= toSend.length");
  assert.strictEqual(batchCalls[0].comments.length, 3);
  assert.strictEqual(outputs.batches_total, "1");
  assert.strictEqual(outputs.comments_inline, "3");
}

// AS5: 2 batches; batch #2 throws 5xx but landed on the server. Only batch #2's
// missing comments are retried; batch #1's comments are untouched (no
// double-post). Requires the per-batch error spec.
async function testBatchPartialSuccessReconcilesPerBatch() {
  const result = { comments: makeComments(4), warnings: [] };
  const { github, core, outputs } = await run({
    result,
    githubOpts: {
      // Fail ONLY batch #2 (index 1) with a 5xx; batch #1 (index 0) succeeds.
      batchErrorSpec: (batchIdx) =>
        batchIdx === 1 ? { status: 502, message: "Bad Gateway" } : null,
      // Batch #2's review landed despite the 5xx.
      batchLanded: true,
      // getPostedCommentIds returns a GLOBAL set across all reviews. Batch #1
      // succeeded, so its 2 comments (c0,c1) are genuinely on the server; 1 of
      // batch #2's (c2) also landed. batchPostedComments() scans ALL batch calls
      // in order [c0,c1,c2,c3], so postedCount=3 echoes [c0,c1,c2] -> batch #2's
      // chunk [c2,c3] filters to toRetry=[c3] (only the missing one).
      echoPosted: true,
      postedCount: 3,
    },
    opts: { reviewCommentBatchSize: 2 },
  });
  const batchCalls = github.createReviewCalls.filter((c) => (c.body || "") === REVIEW_TAG);
  assert.strictEqual(batchCalls.length, 2, "two batches issued");
  // Batch #1 (call 0) succeeded wholesale; batch #2 (call 1) failed.
  // Per-comment fallback calls carry body === "".
  const perCommentCalls = github.createReviewCalls.filter((c) => (c.body || "") !== REVIEW_TAG);
  // Only batch #2's missing comment (c3) is retried (not batch #1's, not c2).
  assert.strictEqual(perCommentCalls.length, 1, "only batch #2's missing comment retried");
  assert.strictEqual(perCommentCalls[0].comments.length, 1);
  // The retried comment belongs to batch #2 (c3), never batch #1 (c0/c1).
  const batch1Paths = new Set(batchCalls[0].comments.map((rc) => rc.path));
  assert.strictEqual(
    batch1Paths.has(perCommentCalls[0].comments[0].path),
    false,
    "retried comment is not from batch #1"
  );
  // All 4 end up posted (3 batch-landed + 1 retried), none failed.
  assert.strictEqual(outputs.comments_inline, "4");
  assert.strictEqual(outputs.comments_failed, "0");
  assert.strictEqual(outputs.batches_total, "2");
  assert.strictEqual(outputs.batches_reconciled, "1", "batch #2 reconciled");
  assert.ok(
    core.logs.some((message) => message.includes("may belong to an earlier batch")),
    "reconciliation log clarifies that the matched review may belong to an earlier batch"
  );
}

// B4: 71 comments, one batch partially fails irrecoverably ->
// comments_inline + comments_failed == 71 (exhaustive, mutually exclusive);
// batches_total == 4.
async function testBatchCountsExhaustive() {
  const result = { comments: makeComments(71), warnings: [] };
  const { outputs } = await run({
    result,
    githubOpts: {
      // Fail batch #4 (index 3, the 11-comment remainder) with a 5xx.
      batchErrorSpec: (batchIdx) =>
        batchIdx === 3 ? { status: 502, message: "Bad Gateway" } : null,
      // Batch #4 did NOT land, and its per-comment retries all fail with a
      // non-retryable 422 (line unresolvable) -> recorded as failed.
      batchLanded: false,
      perCommentError: () => ({ status: 422, message: "Line could not be resolved" }),
    },
    opts: { reviewCommentBatchSize: 20 },
  });
  const inline = parseInt(outputs.comments_inline, 10);
  const failed = parseInt(outputs.comments_failed, 10);
  assert.strictEqual(inline + failed, 71, "inline + failed == 71 (exhaustive)");
  assert.strictEqual(outputs.batches_total, "4");
  // Batches 1-3 (60 comments) all succeed; batch 4 (11) all fail.
  assert.strictEqual(inline, 60);
  assert.strictEqual(failed, 11);
}

// B6 (multi-batch): the idempotency read API is unavailable mid-sequence. The
// affected batch's comments are recorded as failed (NOT reposted, avoiding
// duplicates) and earlier/later batches are undisturbed. This is required
// because the existing single-batch testPerComment5xxIdempotencyUnavailableSkipsRetry
// does not exercise B6 across batch boundaries.
async function testBatchReconcileUnavailableStopsVisibly() {
  const result = { comments: makeComments(4), warnings: [] };
  const { github, outputs } = await run({
    result,
    githubOpts: {
      // Fail batch #2 (index 1) with a 5xx (may have reached the server).
      batchErrorSpec: (batchIdx) =>
        batchIdx === 1 ? { status: 502, message: "Bad Gateway" } : null,
      // The read API (listReviewComments) is unavailable -> the per-comment
      // idempotency check returns null (unknown) -> skip retry, record failed.
      listReviewCommentsThrow: true,
      // Per-comment fallback also 5xx's so the unavailable path is exercised.
      perCommentError: () => ({ status: 502, message: "Bad Gateway" }),
    },
    opts: { reviewCommentBatchSize: 2 },
  });
  const batchCalls = github.createReviewCalls.filter((c) => (c.body || "") === REVIEW_TAG);
  assert.strictEqual(batchCalls.length, 2, "two batches issued");
  // Batch #1 (indices 0,1) succeeded; batch #2 (indices 2,3) failed and could
  // not be reconciled -> its comments are recorded as failed, not retried.
  const perCommentCalls = github.createReviewCalls.filter((c) => (c.body || "") !== REVIEW_TAG);
  // Each of batch #2's 2 comments is attempted once via the per-comment
  // fallback, but the unavailable idempotency read returns null -> the retry
  // is skipped (break) and the comment recorded as failed. Exactly 2 attempts,
  // no blind retries that would duplicate.
  assert.strictEqual(
    perCommentCalls.length,
    2,
    "exactly one fallback attempt per batch #2 comment, no blind retry"
  );
  assert.strictEqual(outputs.comments_inline, "2", "batch #1's 2 comments posted");
  assert.strictEqual(outputs.comments_failed, "2", "batch #2's 2 comments recorded as failed");
  assert.strictEqual(outputs.batches_total, "2");
}

// A2: invalid batch sizes fall back to the default (50), producing a single
// batch for test-sized input.
async function testBatchSizeInvalidFallsBackToDefault() {
  for (const bad of [0, -5, "garbage"]) {
    const result = { comments: makeComments(3), warnings: [] };
    const { github, outputs } = await run({ result, opts: { reviewCommentBatchSize: bad } });
    const batchCalls = github.createReviewCalls.filter((c) => (c.body || "") === REVIEW_TAG);
    assert.strictEqual(batchCalls.length, 1, `invalid size ${JSON.stringify(bad)} -> single batch (default 50)`);
    assert.strictEqual(outputs.batches_total, "1");
  }
}

// B7: per-batch telemetry outputs are present and correct.
async function testBatchTelemetryOutputs() {
  const result = { comments: makeComments(5), warnings: [] };
  const { outputs } = await run({ result, opts: { reviewCommentBatchSize: 2 } });
  // ceil(5/2) = 3 batches.
  assert.strictEqual(outputs.batches_total, "3");
  assert.strictEqual(outputs.batches_attempted, "3");
  assert.strictEqual(outputs.batches_succeeded, "3");
  assert.strictEqual(outputs.batches_reconciled, "0");
  // Existing outputs unchanged.
  assert.strictEqual(outputs.comments_total, "5");
  assert.strictEqual(outputs.comments_inline, "5");
  assert.strictEqual(outputs.comments_failed, "0");
  // batch_summary is valid JSON with the documented shape.
  const summary = JSON.parse(outputs.batch_summary);
  assert.strictEqual(summary.total, 3);
  assert.strictEqual(summary.attempted, 3);
  assert.strictEqual(summary.succeeded, 3);
  assert.strictEqual(summary.reconciled, 0);
  assert.strictEqual(summary.batch_size, 2);
  assert.strictEqual(summary.inline, 5);
  assert.strictEqual(summary.failed, 0);
}

// ---- Badge + publication policy tests (#478) ----
//
// I6: buildBadge byte-matches the CLI's buildBadge degeneration
// (cmd/opencodereview/output.go:98-114). Each degeneration branch is pinned,
// plus control-char sanitization so a model-emitted newline cannot break the
// comment body layout.
function testBuildBadgeMatchesCliDegeneration() {
  // both present -> "[category · severity]" with a middot (U+00B7)
  assert.strictEqual(buildBadge({ category: "bug", severity: "high" }), "[bug · high]");
  // only category -> "[category]"
  assert.strictEqual(buildBadge({ category: "style", severity: "" }), "[style]");
  assert.strictEqual(buildBadge({ category: "style", severity: null }), "[style]");
  assert.strictEqual(buildBadge({ category: "style" }), "[style]");
  // only severity -> "[severity]"
  assert.strictEqual(buildBadge({ category: "", severity: "low" }), "[low]");
  assert.strictEqual(buildBadge({ category: null, severity: "low" }), "[low]");
  assert.strictEqual(buildBadge({ severity: "low" }), "[low]");
  // neither -> "" (no badge rendered)
  assert.strictEqual(buildBadge({}), "");
  assert.strictEqual(buildBadge({ category: "", severity: "" }), "");
  assert.strictEqual(buildBadge({ category: null, severity: null }), "");
  // missing comment object entirely
  assert.strictEqual(buildBadge(null), "");
  assert.strictEqual(buildBadge(undefined), "");
  // The separator is the U+00B7 middot (·), exactly matching the CLI's
  // fmt.Sprintf("[%s · %s]", ...). Pin the exact byte (not "." or "-" or "·"'s
  // decomposition) so a future edit that swaps the separator fails loudly.
  const both = buildBadge({ category: "bug", severity: "low" });
  assert.ok(both.includes("·"), "badge contains the U+00B7 middot");
  assert.strictEqual(both, "[bug · low]", "exact badge string for the common case");
  // control-char sanitization: the Action strips ALL control chars (including
  // \t and \n) from metadata — intentionally STRICTER than the CLI's
  // sanitizeTerminal (which keeps \t/\n), because a newline/tab would break
  // the Markdown comment body layout. Documented divergence from strict OC1
  // byte-parity; clean enum values match exactly across surfaces.
  assert.strictEqual(buildBadge({ category: "bu\ng", severity: "high" }), "[bug · high]");
  assert.strictEqual(buildBadge({ category: "bug", severity: "hi\tgh" }), "[bug · high]");
  assert.strictEqual(buildBadge({ category: "bug\r\n", severity: "high" }), "[bug · high]");
  // a value that is ALL control chars degenerates to "" (badge not rendered),
  // not a label of empty brackets.
  assert.strictEqual(buildBadge({ category: "\n\r\t", severity: "\n" }), "");
}

// #882: buildBadgeImage renders category+severity metadata as a single
// shields.io static badge in the reviewer-suggested format
// img.shields.io/badge/<category>-<severity>-<color> (color keyed off
// severity: low green, medium orange, high red, critical darkred), whose alt
// text is the exact plain-text badge content. Any non-empty metadata renders a
// badge — a single field gets a single segment, and an unknown severity falls
// back to the fixed category color rather than yielding "undefined".
function testBuildBadgeImage() {
  // both known -> single combined badge, severity picks the color
  assert.strictEqual(
    buildBadgeImage({ category: "bug", severity: "high" }),
    "![bug · high](https://img.shields.io/badge/bug-high-red)"
  );
  assert.strictEqual(
    buildBadgeImage({ category: "security", severity: "critical" }),
    "![security · critical](https://img.shields.io/badge/security-critical-darkred)"
  );
  assert.strictEqual(
    buildBadgeImage({ category: "performance", severity: "medium" }),
    "![performance · medium](https://img.shields.io/badge/performance-medium-orange)"
  );
  assert.strictEqual(
    buildBadgeImage({ category: "style", severity: "low" }),
    "![style · low](https://img.shields.io/badge/style-low-green)"
  );
  // only one field present -> single-segment badge in a fixed color
  assert.strictEqual(
    buildBadgeImage({ category: "documentation" }),
    "![documentation](https://img.shields.io/badge/documentation-blue)"
  );
  assert.strictEqual(
    buildBadgeImage({ severity: "critical" }),
    "![critical](https://img.shields.io/badge/critical-darkred)"
  );
  // neither -> "" (no badge line), matching buildBadge
  assert.strictEqual(buildBadgeImage({}), "");
  assert.strictEqual(buildBadgeImage(null), "");
  assert.strictEqual(buildBadgeImage(undefined), "");
  // unknown values -> badge still renders; a known severity picks its color,
  // an unknown severity falls back to the fixed category color
  assert.strictEqual(
    buildBadgeImage({ category: "weird", severity: "high" }),
    "![weird · high](https://img.shields.io/badge/weird-high-red)"
  );
  assert.strictEqual(
    buildBadgeImage({ category: "bug", severity: "extreme" }),
    "![bug · extreme](https://img.shields.io/badge/bug-extreme-blue)"
  );
  assert.strictEqual(
    buildBadgeImage({ category: "weird" }),
    "![weird](https://img.shields.io/badge/weird-blue)"
  );
  // case-insensitive enum match (metadata is normalized before comparison,
  // but the alt text preserves the sanitized original like buildBadge does)
  assert.strictEqual(
    buildBadgeImage({ category: "Bug", severity: "HIGH" }),
    "![Bug · HIGH](https://img.shields.io/badge/bug-high-red)"
  );
  // control chars are stripped before enum matching (sanitizeMetadata), so a
  // value that sanitizes to a known enum still renders as an image
  assert.strictEqual(
    buildBadgeImage({ category: "bu\ng", severity: "high" }),
    "![bug · high](https://img.shields.io/badge/bug-high-red)"
  );
  // Markdown-special characters in metadata are escaped in the alt text so a
  // stray "]" can't prematurely close the image's [...] span and a "\" can't
  // escape it (malformed model output at high temperature).
  assert.strictEqual(
    buildBadgeImage({ category: "x]y", severity: "high" }),
    "![x\\]y · high](https://img.shields.io/badge/x%5Dy-high-red)"
  );
  assert.strictEqual(
    buildBadgeImage({ category: "a[b", severity: "high" }),
    "![a\\[b · high](https://img.shields.io/badge/a%5Bb-high-red)"
  );
  assert.strictEqual(
    buildBadgeImage({ category: "bug", severity: "hi\\gh" }),
    "![bug · hi\\\\gh](https://img.shields.io/badge/bug-hi%5Cgh-blue)"
  );
}

// Drift guard for the SEVERITIES <-> SEVERITY_BADGE_COLOR invariant: every
// severity enum member must have an explicit color so it does not silently
// fall back to the fixed category color.
function testSeverityBadgeColorCoversSeverities() {
  for (const s of SEVERITIES) {
    assert.ok(
      Object.prototype.hasOwnProperty.call(SEVERITY_BADGE_COLOR, s),
      `SEVERITY_BADGE_COLOR is missing an entry for severity "${s}"`
    );
  }
  // A non-enum severity (high-temperature model output, per reviewer) still
  // renders via the fixed fallback color, never a "-undefined" URL.
  const out = buildBadgeImage({ category: "bug", severity: "blocker" });
  assert.strictEqual(out, "![bug · blocker](https://img.shields.io/badge/bug-blocker-blue)");
  assert.ok(!out.includes("undefined"), "no undefined interpolated into output");
}

function testSanitizeMetadataStripsControlChars() {
  assert.strictEqual(sanitizeMetadata("clean"), "clean");
  assert.strictEqual(sanitizeMetadata("a\nb"), "ab");
  assert.strictEqual(sanitizeMetadata("a\tb"), "ab");
  assert.strictEqual(sanitizeMetadata("a\rb"), "ab");
  assert.strictEqual(sanitizeMetadata("a\x00b"), "ab");
  assert.strictEqual(sanitizeMetadata("a\x7fb"), "ab");
  assert.strictEqual(sanitizeMetadata("\n\r\t"), "");
  // null/undefined/numbers degrade safely to their string form.
  assert.strictEqual(sanitizeMetadata(null), "");
  assert.strictEqual(sanitizeMetadata(undefined), "");
  assert.strictEqual(sanitizeMetadata(42), "42");
}

// I1: buildPolicy fails open on any malformed input — a bad policy never routes
// a finding, so no finding is ever silently dropped because the policy itself
// was broken. The NO_ROUTING sentinel is returned for every non-routing case.
function testBuildPolicyFailsOpenOnMalformed() {
  // empty / null inputs -> no routing
  assert.strictEqual(buildPolicy({}), NO_ROUTING);
  assert.strictEqual(buildPolicy({ severityThreshold: "", categories: "" }), NO_ROUTING);
  assert.strictEqual(buildPolicy({ severityThreshold: null, categories: null }), NO_ROUTING);
  assert.strictEqual(buildPolicy(undefined), NO_ROUTING);
  // unknown severity -> severity routing disabled (fail-open)
  assert.strictEqual(buildPolicy({ severityThreshold: "trivial" }), NO_ROUTING);
  assert.strictEqual(buildPolicy({ severityThreshold: "Criticals" }), NO_ROUTING);
  // garbage threshold -> no routing
  assert.strictEqual(buildPolicy({ severityThreshold: "garbage" }), NO_ROUTING);
  // all-unknown categories -> category routing disabled (fail-open)
  assert.strictEqual(buildPolicy({ categories: "unknown,also-unknown" }), NO_ROUTING);
  // a known threshold enables severity routing; the returned rank is correct.
  const lowP = buildPolicy({ severityThreshold: "low" });
  assert.strictEqual(lowP.routeBySeverity, true);
  assert.strictEqual(lowP.routeByCategory, false);
  assert.strictEqual(lowP.severityRank, SEVERITY_RANK.get("low"));
  // case-insensitivity
  const medP = buildPolicy({ severityThreshold: "MeDiUm" });
  assert.strictEqual(medP.routeBySeverity, true);
  assert.strictEqual(medP.severityRank, SEVERITY_RANK.get("medium"));
  // known categories enable category routing; unknown tokens dropped.
  const catP = buildPolicy({ categories: "Style, UNKNOWN, documentation" });
  assert.strictEqual(catP.routeByCategory, true);
  assert.strictEqual(catP.routeBySeverity, false);
  assert.ok(catP.categories.has("style"));
  assert.ok(catP.categories.has("documentation"));
  assert.ok(!catP.categories.has("unknown"));
  // whitespace-only threshold -> no routing
  assert.strictEqual(buildPolicy({ severityThreshold: "   " }), NO_ROUTING);
}

// I1: routeComment never routes a finding with unknown/malformed metadata — it
// falls through to the normal inline path (visible), never dropped. Boundary
// inclusivity ("at-or-below") is pinned so the threshold value itself routes.
function testRouteCommentUnknownMetadataNeverRouted() {
  const policy = buildPolicy({ severityThreshold: "medium", categories: "style,documentation" });
  // severity routing: medium threshold routes medium AND low (inclusive-at-or-below)
  assert.strictEqual(routeComment({ severity: "medium" }, policy).routed, true);
  assert.strictEqual(routeComment({ severity: "low" }, policy).routed, true);
  // severity strictly above the threshold stays inline
  assert.strictEqual(routeComment({ severity: "high" }, policy).routed, false);
  assert.strictEqual(routeComment({ severity: "critical" }, policy).routed, false);
  // unknown/empty severity is NEVER routed by severity (fail-open: I1)
  assert.strictEqual(routeComment({ severity: "" }, policy).routed, false);
  assert.strictEqual(routeComment({ severity: "trivial" }, policy).routed, false);
  assert.strictEqual(routeComment({ severity: null }, policy).routed, false);
  assert.strictEqual(routeComment({}, policy).routed, false);
  // category routing: a listed category routes regardless of severity
  assert.strictEqual(routeComment({ category: "style" }, policy).routed, true);
  assert.strictEqual(routeComment({ category: "documentation" }, policy).routed, true);
  // unlisted/unknown category is NEVER routed by category (fail-open: I1)
  assert.strictEqual(routeComment({ category: "bug" }, policy).routed, false);
  assert.strictEqual(routeComment({ category: "unknown" }, policy).routed, false);
  assert.strictEqual(routeComment({ category: "" }, policy).routed, false);
  assert.strictEqual(routeComment({ category: null }, policy).routed, false);
  // case-insensitive category matching
  assert.strictEqual(routeComment({ category: "STYLE" }, policy).routed, true);
  assert.strictEqual(routeComment({ category: "Documentation" }, policy).routed, true);
  // NO_ROUTING sentinel never routes anything
  assert.strictEqual(routeComment({ severity: "low", category: "style" }, NO_ROUTING).routed, false);
  assert.strictEqual(routeComment({ severity: "low", category: "style" }, null).routed, false);
  // routed result carries a reason string
  const r = routeComment({ severity: "low", category: "style" }, policy);
  assert.ok(r.routed);
  assert.ok(typeof r.reason === "string" && r.reason.length > 0);
  assert.match(r.reason, /severity low/);
  assert.match(r.reason, /category style/);
}

// Pins the "at-or-below" boundary explicitly (PLAN_VALIDATION Risk C): the
// threshold value itself routes, and the floor (low) routes low.
function testRouteSeverityBelowBoundaryInclusive() {
  const lowP = buildPolicy({ severityThreshold: "low" });
  // threshold = low routes ONLY low (the floor). critical/high/medium stay.
  assert.strictEqual(routeComment({ severity: "low" }, lowP).routed, true);
  assert.strictEqual(routeComment({ severity: "medium" }, lowP).routed, false);
  assert.strictEqual(routeComment({ severity: "high" }, lowP).routed, false);
  assert.strictEqual(routeComment({ severity: "critical" }, lowP).routed, false);
  const critP = buildPolicy({ severityThreshold: "critical" });
  // threshold = critical routes everything (all severities are at-or-below it).
  for (const sev of SEVERITIES) {
    assert.strictEqual(routeComment({ severity: sev }, critP).routed, true);
  }
}

// A finding that matches BOTH the severity and category conditions routes
// EXACTLY ONCE (no double-count): the severity branch short-circuits, the
// category branch is never reached, and the partition loop counts the finding
// in the routed bucket a single time.
async function testFindingMatchingBothConditionsRoutesOnce() {
  const result = {
    comments: [
      // matches BOTH severity (low <= low) AND category (style in list)
      { path: "src/both.js", content: "matches both", category: "style", severity: "low", start_line: 1, end_line: 1 },
      // matches only severity
      { path: "src/sev.js", content: "sev only", category: "bug", severity: "low", start_line: 2, end_line: 2 },
      // matches only category
      { path: "src/cat.js", content: "cat only", category: "documentation", severity: "critical", start_line: 3, end_line: 3 },
    ],
    warnings: [],
  };

  const { github, outputs } = await run({
    result,
    opts: { routeSeverityBelow: "low", routeCategories: "style,documentation" },
  });

  // All three route (none posted inline); routed count is exactly 3 (no
  // double-count from the "both" finding matching two conditions).
  assert.strictEqual(github.createReviewCalls.length, 0, "no inline posting — all routed");
  assert.strictEqual(outputs.comments_routed, "3", "each finding counted once even when matching both conditions");
  assert.strictEqual(outputs.comments_total, "3");
  assert.strictEqual(outputs.comments_inline, "0");
}

// I6 / additive behavior: formatComment prepends the badge AFTER the id HTML
// comment, so the idempotency regex (unanchored) still matches and the badge
// is the first VISIBLE line. No badge when category/severity are absent.
function testFormatCommentBadgePlacement() {
  // with id and badge: id HTML comment stays first, badge on the next line
  const withBadge = formatComment({ content: "body", category: "bug", severity: "high" }, "ocr-1-1-abcd");
  assert.ok(withBadge.startsWith("<!-- ocr-1-1-abcd -->\n"), "id HTML comment is the first bytes");
  assert.ok(
    withBadge.startsWith("<!-- ocr-1-1-abcd -->\n![bug · high](https://img.shields.io/badge/bug-high-red)\n"),
    "badge image follows id line"
  );
  assert.ok(withBadge.endsWith("body"), "content preserved at the end");
  // without id: badge is the first line
  const noId = formatComment({ content: "body", category: "style", severity: "low" });
  assert.ok(noId.startsWith("![style · low](https://img.shields.io/badge/style-low-green)\n"));
  // no metadata -> no badge line at all (byte-identical to pre-change output)
  const noBadge = formatComment({ content: "body" }, "ocr-1-1-abcd");
  assert.strictEqual(noBadge, "<!-- ocr-1-1-abcd -->\nbody");
  // suggestion block still appends after the badge
  const withSuggestion = formatComment(
    { content: "c", category: "bug", severity: "high", existing_code: "old", suggestion_code: "new" },
    "ocr-1-1-abcd"
  );
  assert.match(withSuggestion, /!\[bug · high\]/);
  assert.match(withSuggestion, /\*\*Suggestion:\*\*/);
  assert.match(withSuggestion, /```suggestion/);
}

// I6: formatCommentMarkdown prepends the badge as a leading line before the
// path heading (PLAN_VALIDATION Risk A confirmed placement).
function testFormatCommentMarkdownBadgePlacement() {
  const md = formatCommentMarkdown({ path: "a.js", content: "body", category: "bug", severity: "high" });
  // badge image is the first line, before the heading
  assert.ok(md.startsWith("![bug · high](https://img.shields.io/badge/bug-high-red)\n"), "badge is the leading line");
  assert.match(md, /### 📄 `a.js`/);
  assert.match(md, /body/);
  // no metadata -> no badge line, heading is first (byte-identical to pre-change)
  const noBadge = formatCommentMarkdown({ path: "a.js", content: "body" });
  assert.ok(noBadge.startsWith("### 📄 `a.js`"));
  assert.ok(!noBadge.includes("·"));
}

// I3: with no routing input set, placement is identical to today (modulo the
// additive badge prefix, which is "" for findings without metadata). Exercises
// the full runPostReviewComments path with default empty policy.
async function testNoRoutingInputPreservesBehavior() {
  const result = {
    comments: [
      { path: "src/a.js", content: "inline content", start_line: 10, end_line: 10 },
      { path: "docs/no-line.md", content: "no-line content", start_line: 0, end_line: 0 },
    ],
    warnings: [],
  };

  const { github, outputs } = await run({ result });

  // The inline comment is posted via the batch review (not routed).
  assert.strictEqual(github.createReviewCalls.length, 1, "one batch review");
  const sent = github.createReviewCalls[0].comments;
  assert.strictEqual(sent.length, 1);
  assert.strictEqual(sent[0].path, "src/a.js");
  // no routed bucket (output defaults to 0)
  assert.strictEqual(outputs.comments_routed, "0");
  assert.strictEqual(outputs.comments_inline, "1");
  assert.strictEqual(outputs.comments_total, "2");
}

// I2 + OC4: severity routing moves at-or-below findings to the summary and
// counts reconcile to the total.
async function testRouteSeverityBelowRoutesToSummary() {
  const result = {
    comments: [
      { path: "src/critical.js", content: "critical finding", category: "bug", severity: "critical", start_line: 1, end_line: 1 },
      { path: "src/low.js", content: "low finding", category: "style", severity: "low", start_line: 2, end_line: 2 },
      { path: "docs/no-line.md", content: "no-line finding", category: "documentation", severity: "low", start_line: 0, end_line: 0 },
    ],
    warnings: [],
  };

  const { github, outputs } = await run({
    result,
    opts: { routeSeverityBelow: "low" },
  });

  // only the critical inline finding is posted (low is routed, no-line stays in summary)
  assert.strictEqual(github.createReviewCalls.length, 1, "one batch review");
  const sent = github.createReviewCalls[0].comments;
  assert.strictEqual(sent.length, 1, "only critical inline finding posted");
  assert.strictEqual(sent[0].path, "src/critical.js");
  // counts reconcile (I2): inline + routed + summary == total (skipped=failed=0)
  assert.strictEqual(outputs.comments_total, "3");
  assert.strictEqual(outputs.comments_inline, "1");
  assert.strictEqual(outputs.comments_routed, "1");
  assert.strictEqual(outputs.comments_failed, "0");
  assert.strictEqual(outputs.comments_skipped, "0");
  // routed finding rendered in the summary with its reason
  const body = github.updatedComments[0].body;
  assert.match(body, /📋 Routed to summary by policy: 1 comment\(s\)/);
  assert.match(body, /low finding/);
  assert.match(body, /Routed to summary \(severity low/);
  // no-line finding still rendered in summary too
  assert.match(body, /no-line finding/);
}

// OC4: comma-list category routing moves listed categories to the summary.
async function testRouteCategoriesRoutesToSummary() {
  const result = {
    comments: [
      { path: "src/bug.js", content: "bug finding", category: "bug", severity: "high", start_line: 1, end_line: 1 },
      { path: "src/style.js", content: "style finding", category: "style", severity: "low", start_line: 2, end_line: 2 },
      { path: "docs/doc.md", content: "doc finding", category: "documentation", severity: "low", start_line: 3, end_line: 3 },
    ],
    warnings: [],
  };

  const { github, outputs } = await run({
    result,
    opts: { routeCategories: "style,documentation" },
  });

  // only the bug finding is posted inline; style + documentation routed
  assert.strictEqual(github.createReviewCalls.length, 1);
  const sent = github.createReviewCalls[0].comments;
  assert.strictEqual(sent.length, 1);
  assert.strictEqual(sent[0].path, "src/bug.js");
  assert.strictEqual(outputs.comments_inline, "1");
  assert.strictEqual(outputs.comments_routed, "2");
  assert.strictEqual(outputs.comments_total, "3");
  const body = github.updatedComments[0].body;
  assert.match(body, /📋 Routed to summary by policy: 2 comment\(s\)/);
  assert.match(body, /style finding/);
  assert.match(body, /doc finding/);
}

// I4: routed findings never enter the createReview write path, so they cannot
// be double-posted on retry. Verified by inspecting createReviewCalls bodies.
async function testRoutedFindingsNeverCallCreateReview() {
  const result = {
    comments: [
      { path: "src/keep.js", content: "keep inline", category: "bug", severity: "critical", start_line: 1, end_line: 1 },
      { path: "src/route.js", content: "route me", category: "style", severity: "low", start_line: 2, end_line: 2 },
    ],
    warnings: [],
  };

  const { github, outputs } = await run({
    result,
    // Inject a batch error so the per-comment retry path runs; routed findings
    // must STILL not appear in any createReview call (batch or per-comment).
    githubOpts: {
      bulkErrorSpec: { message: "Bad Gateway", status: 502 },
      // No batchLanded / echoPosted -> full retry of toSend (the non-routed set).
    },
    opts: { routeCategories: "style" },
  });

  // Every createReview call must contain ONLY the kept finding's path, never
  // the routed one. This is the faithful proxy for "cannot be double-posted":
  // a finding absent from every write call cannot land twice.
  for (const call of github.createReviewCalls) {
    const paths = (call.comments || []).map((c) => c.path);
    assert.ok(!paths.includes("src/route.js"), `routed finding appeared in createReview call: ${JSON.stringify(paths)}`);
    assert.ok(paths.includes("src/keep.js"), `kept finding missing from createReview call: ${JSON.stringify(paths)}`);
  }
  assert.strictEqual(outputs.comments_routed, "1");
  // at least the batch + one per-comment retry happened
  assert.ok(github.createReviewCalls.length >= 2, "batch then per-comment retry ran");
}

// I2: accounting reconciles across mixed inputs —
// inline + summary + skipped + failed + routed == total.
async function testAccountingReconcilesToTotal() {
  const history = [{ path: "src/overlap.js", line: 5, start_line: 5, side: "RIGHT", user: { login: "github-actions[bot]" } }];
  const result = {
    comments: [
      // routed by severity (low)
      { path: "src/routed.js", content: "routed", category: "style", severity: "low", start_line: 1, end_line: 1 },
      // inline (posted successfully via per-comment retry)
      { path: "src/inline.js", content: "inline", category: "bug", severity: "high", start_line: 2, end_line: 2 },
      // skipped by incremental overlap
      { path: "src/overlap.js", content: "overlap", category: "bug", severity: "high", start_line: 5, end_line: 5 },
      // failed to post (422 non-retryable during per-comment retry)
      { path: "src/fail.js", content: "fail", category: "bug", severity: "high", start_line: 3, end_line: 3 },
      // no-line (summary)
      { path: "docs/noline.md", content: "no-line", category: "documentation", severity: "low", start_line: 0, end_line: 0 },
    ],
    warnings: [],
  };

  const { outputs } = await run({
    result,
    githubOpts: {
      history,
      // A 502 on the BATCH call forces the per-comment retry path, where
      // perCommentError actually fires (it only runs on per-comment calls).
      bulkErrorSpec: { message: "Bad Gateway", status: 502 },
      perCommentError: (rc) => {
        if (rc && rc.path === "src/fail.js") {
          return { message: "Line could not be resolved", status: 422 };
        }
        return null;
      },
    },
    opts: { incremental: true, routeSeverityBelow: "low" },
  });

  const total = Number(outputs.comments_total);
  const inline = Number(outputs.comments_inline);
  const summary = 1; // the no-line finding (always summary)
  const skipped = Number(outputs.comments_skipped);
  const routed = Number(outputs.comments_routed);
  const failed = Number(outputs.comments_failed);
  assert.strictEqual(inline + summary + skipped + routed + failed, total, "counts sum to total (I2)");
  assert.strictEqual(total, 5);
  assert.strictEqual(routed, 1, "low-severity valid-line finding routed");
  assert.strictEqual(inline, 1, "high-severity finding posted inline");
  assert.strictEqual(skipped, 1, "overlap skipped by incremental");
  assert.strictEqual(failed, 1, "fail.js failed to post");
}

// I1 / fail-open for the policy itself: malformed routing inputs degrade to
// no-routing, so the Action behaves exactly like today (no finding dropped
// because the policy string was garbage).
async function testMalformedRoutingPolicyFailsOpen() {
  const result = {
    comments: [
      { path: "src/a.js", content: "a", category: "bug", severity: "low", start_line: 1, end_line: 1 },
      { path: "src/b.js", content: "b", category: "style", severity: "high", start_line: 2, end_line: 2 },
    ],
    warnings: [],
  };

  // unknown severity threshold -> no routing; both stay inline
  const { outputs } = await run({
    result,
    opts: { routeSeverityBelow: "trivial" },
  });
  assert.strictEqual(outputs.comments_routed, "0");
  assert.strictEqual(outputs.comments_inline, "2");
  // all-unknown categories -> no routing
  const { outputs: o2 } = await run({
    result,
    opts: { routeCategories: "nonsense,garbage" },
  });
  assert.strictEqual(o2.comments_routed, "0");
  assert.strictEqual(o2.comments_inline, "2");
}

// I4: routed findings carry no id (formatComment called without an id arg),
// so even a hypothetical leak into a write path could not match the
// idempotency regex. Defense-in-depth check on the routed item shape.
async function testRoutedFindingsCarryNoIdempotencyId() {
  const result = {
    comments: [
      { path: "src/route.js", content: "route me", category: "style", severity: "low", start_line: 2, end_line: 2 },
    ],
    warnings: [],
  };

  const { github } = await run({
    result,
    opts: { routeCategories: "style" },
  });
  // No createReview call at all (the only finding was routed).
  assert.strictEqual(github.createReviewCalls.length, 0);
  // The routed body in the summary must NOT contain an ocr-... id comment.
  const body = github.updatedComments[0].body;
  assert.doesNotMatch(body, /ocr-\d+-\d+-[a-f0-9]+/, "routed summary body carries no idempotency id");
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
  await testBatchIdempotencyCheckFailureStopsVisibly();
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
  // Batching (issue #479) — pure helpers
  testResolveBatchSize();
  testChunkArray();
  testSortToSendDeterministically();
  // Batching (issue #479) — integration via mock
  await testBatchPartitioningDeterministic();
  await testBatchSizeOnePerComment();
  await testBatchSizeLargerThanToSend();
  await testBatchPartialSuccessReconcilesPerBatch();
  await testBatchCountsExhaustive();
  await testBatchReconcileUnavailableStopsVisibly();
  await testBatchSizeInvalidFallsBackToDefault();
  await testBatchTelemetryOutputs();
  // Badge + publication policy (#478)
  testBuildBadgeMatchesCliDegeneration();
  testBuildBadgeImage();
  testSeverityBadgeColorCoversSeverities();
  testSanitizeMetadataStripsControlChars();
  testBuildPolicyFailsOpenOnMalformed();
  testRouteCommentUnknownMetadataNeverRouted();
  testRouteSeverityBelowBoundaryInclusive();
  await testFindingMatchingBothConditionsRoutesOnce();
  testFormatCommentBadgePlacement();
  testFormatCommentMarkdownBadgePlacement();
  await testNoRoutingInputPreservesBehavior();
  await testRouteSeverityBelowRoutesToSummary();
  await testRouteCategoriesRoutesToSummary();
  await testRoutedFindingsNeverCallCreateReview();
  await testAccountingReconcilesToTotal();
  await testMalformedRoutingPolicyFailsOpen();
  await testRoutedFindingsCarryNoIdempotencyId();
  // Diff hunk parsing & 422 line-resolution fallback
  testParseDiffHunkRanges();
  testClassifyCommentAgainstDiff();
  testIsLineResolutionFailure();
  testDescribeCommentLocation();
  await testGetPrDiffHunks();
  await testGetPrDiffHunksTruncationIsIncomplete();
  await testGetPrDiffHunksPaginatesCompleteInventory();
  await testClippedPatchIsUnknown();
  await testMovedHeadMakesInventoryIncomplete();
  await testRunnerHeadDriftPreservesComments();
  await testEmptyDiffInventoryDoesNotCondemnComments();
  await testHttp422SecondaryFilteredBatchFallback();
  await testAllValidBatchSkipsSecondaryAndKeepsEveryComment();
  await testHttp422SecondarySuccessStillPostsUnknownComments();
  await testHttp422SecondaryFailureReconcilesInsteadOfDuplicating();
  await testPostedCommentReadFailureDoesNotUnwindTheRun();
  await testHttp422SecondaryFailureReconcilesPerCommentNotWholesale();
  await testHttp422SecondaryFailureFallsBackWhenNothingLanded();
  await testHttp422SecondaryRateLimitCoolsDownBeforeRetrying();
  await testMatchingWordingOnNon422SkipsFilteredBatch();
  await testNonLineResolution422SkipsFilteredBatch();
  await testUnknownDiffMetadataStillPostsComments();
  await testDiffFetchFailureDegradesToPerComment();
  await testAllCommentsFilteredOutAccounting();
  await testCrossHunkRangeIsFilteredOut();
  // Cross-push checkpoints (#476) — read path
  testCheckpointMarkerRoundTrip();
  testValidateCheckpointPayload();
  await testCheckpointTwelveReasonsAreDistinct();
  await testCheckpointSingleFieldMutationsFailClosed();
  await testCheckpointAuthorVerificationFailsClosed();
  await testCheckpointRejectsDifferentBotIdentity();
  await testCheckpointWidenOnly();
  await testCheckpointSameHeadRerun();
  await testCheckpointResolveShape();
  // Cross-push checkpoints (#476) — write path
  await testCheckpointAdvanceGateTable();
  await testCheckpointAdvanceRequiresFullSha();
  await testCheckpointCarryForwardOnEveryBodyPath();
  await testCheckpointAdvancesOnZeroFindings();
  await testCheckpointNeverAdvancesWithoutSticky();
  await testCheckpointCarryIsGatedLikeTheAdvance();
  await testCheckpointResolverUsesPreReadComment();
  await testCheckpointOptOutLeavesBodyUnchanged();
  await testCheckpointWriteThenReadRoundTrip();
  // Cross-push checkpoints (#476) — defect repairs
  await testCheckpointAuthorSurvivesUnavailableGetUser();
  testCheckpointBotTypeAloneEstablishesAuthorship();
  await testCheckpointRejectsUserToServerAppComment();
  await testCheckpointNoopLeavesSummaryUntouched();
  await testCheckpointNoopSurvivesUnreadableOcrOutput();
  await testCheckpointEventFullScopeTable();
  await testCheckpointCarryNeverErasesExistingMarker();
  await testCheckpointRescueSkipsAmbiguousMarkers();
  await testCheckpointAdvanceRequiresAFingerprint();
  await testSummaryShowsNarrowedRangeLabel();
  await testCheckpointReasonsMatchTheDocs();
  await testActionResolveScriptFallsBackInsteadOfThrowing();
  await testActionScriptFingerprintCoversRepoLocalRules();
  testActionResolveStepNeverFailsTheJob();
  testActionFingerprintsRepoLocalRuleFile();
  testActionFingerprintIncludesBackground();
  testActionRangeFromIsAStepOutput();
  testActionEmitsMachineReadableRangeOutputs();
  testActionPinsGithubScriptSha();
  await testActionUnresolvedVersionEmptiesTheFingerprint();
  await testActionFingerprintCoversLlmHeaderAxes();
  await testActionPinsAuthorToTheDefaultTokenApp();
  await testActionResolveStepDeclaresItsRefInputs();
  await testCheckpointMarkerMatchingIsStateless();
  console.log("All post-review-comments tests passed.");
}
function testParseDiffHunkRanges() {
  const patch = `@@ -10,3 +10,4 @@
 context line 10
-deleted line 11
+added line 11
+added line 12
 context line 13`;
  const ranges = parseDiffHunkRanges(patch);
  assert.deepStrictEqual(ranges, [{ start: 10, end: 13 }]);

  // Two hunks stay SEPARATE ranges: the gap between them is not commentable,
  // and a span straddling both is not a legal multi-line comment.
  const twoHunks = `@@ -1,3 +1,3 @@
 a
 b
 c
@@ -50,3 +50,3 @@
 x
 y
 z`;
  assert.deepStrictEqual(parseDiffHunkRanges(twoHunks), [
    { start: 1, end: 3 },
    { start: 50, end: 52 },
  ]);

  // A pure-deletion hunk has no RIGHT-side lines at all.
  assert.deepStrictEqual(parseDiffHunkRanges("@@ -5,2 +4,0 @@\n-gone\n-also gone"), []);

  // "\\ No newline at end of file" must not advance the line counter.
  const noNewline = `@@ -1,1 +1,2 @@
 kept
+added
\\ No newline at end of file`;
  assert.deepStrictEqual(parseDiffHunkRanges(noNewline), [{ start: 1, end: 2 }]);

  // A trailing newline in the patch string yields a bare "" after split; it is
  // not a diff body line and must not extend the range.
  assert.deepStrictEqual(parseDiffHunkRanges("@@ -1,1 +1,1 @@\n context\n"), [{ start: 1, end: 1 }]);

  assert.deepStrictEqual(parseDiffHunkRanges(""), []);
  assert.deepStrictEqual(parseDiffHunkRanges(null), []);
}

function testClassifyCommentAgainstDiff() {
  const diff = {
    complete: true,
    known: new Set(["foo.js", "binary.png"]),
    files: new Map([["foo.js", [{ start: 10, end: 12 }, { start: 50, end: 52 }]]]),
  };
  const at = (rc) => classifyCommentAgainstDiff({ reviewComment: rc }, diff);

  // Single line inside a hunk.
  assert.strictEqual(at({ path: "foo.js", line: 11 }), "valid");
  // Single line outside every hunk.
  assert.strictEqual(at({ path: "foo.js", line: 30 }), "invalid");
  // File not in the PR at all.
  assert.strictEqual(at({ path: "bar.js", line: 10 }), "invalid");

  // Multi-line span wholly inside ONE hunk.
  assert.strictEqual(at({ path: "foo.js", start_line: 10, line: 12 }), "valid");
  // Span straddling two hunks: both endpoints exist, but not in the same hunk.
  // A flat line-set would wrongly call this valid and 422 all over again.
  assert.strictEqual(at({ path: "foo.js", start_line: 11, line: 51 }), "invalid");
  // Reversed span.
  assert.strictEqual(at({ path: "foo.js", start_line: 52, line: 11 }), "invalid");
  // Span partially overhanging the end of a hunk.
  assert.strictEqual(at({ path: "foo.js", start_line: 11, line: 13 }), "invalid");

  // ---- "unknown" must never be reported as "invalid" ----
  // File is in the PR but GitHub omitted its patch (binary / oversized diff).
  assert.strictEqual(at({ path: "binary.png", line: 3 }), "unknown");
  // No line information to check.
  assert.strictEqual(at({ path: "foo.js", line: null }), "unknown");
  // LEFT-side comment: we only model RIGHT-side lines.
  assert.strictEqual(at({ path: "foo.js", line: 11, side: "LEFT" }), "unknown");
  // A truncated inventory proves nothing about an absent path.
  assert.strictEqual(
    classifyCommentAgainstDiff({ reviewComment: { path: "bar.js", line: 1 } }, { ...diff, complete: false }),
    "unknown"
  );
  // No inventory at all (the fetch failed).
  assert.strictEqual(classifyCommentAgainstDiff({ reviewComment: { path: "foo.js", line: 11 } }, null), "unknown");
}

function testIsLineResolutionFailure() {
  // VERBATIM capture from live GitHub: POST /repos/{o}/{r}/pulls/{n}/reviews
  // with two out-of-diff comments. This is the shape production actually sees,
  // and it pins that errors[] is an array of STRINGS (not {field} objects).
  //
  // On THIS shape the decisive wording arrives TWICE over independent paths:
  // Octokit's composed `error.message` ("<data.message>: <errors[] entries>")
  // and the raw `response.data.errors[]` strings. Either one alone is enough,
  // so the live fixture cannot tell them apart — deleting the error.message
  // source leaves it green. What it DOES prove is that `data.message`
  // ("Unprocessable Entity") is not one of them; that is asserted separately
  // below. The two fixtures after it pin each source on its own.
  const liveBody = {
    message: "Unprocessable Entity",
    errors: ["Line could not be resolved and Line could not be resolved"],
    documentation_url: "https://docs.github.com/rest/pulls/reviews#create-a-review-for-a-pull-request",
    status: "422",
  };
  const live = makeErr(
    'Unprocessable Entity: "Line could not be resolved" - https://docs.github.com/rest/pulls/reviews#create-a-review-for-a-pull-request',
    422,
    null,
    liveBody
  );
  assert.strictEqual(isLineResolutionFailure(live), true, "the real live 422 must activate the fallback");
  // The bare structured message is NOT sufficient on its own — proving the
  // errors[]/message path is what carries the decision.
  assert.strictEqual(isLineResolutionFailure({ message: liveBody.message }), false);

  // ---- each source pinned in isolation ----
  // (a) Composed message only, NO errors[] — the shape seen whenever a caller
  // re-wraps the error and the structured body is lost. error.message is then
  // the sole carrier.
  const messageOnly = makeErr(
    'Unprocessable Entity: "Line could not be resolved" - https://docs.github.com/rest/pulls/reviews#create-a-review-for-a-pull-request',
    422,
    null,
    { message: "Unprocessable Entity", documentation_url: "https://docs.github.com/rest", status: "422" }
  );
  assert.strictEqual(messageOnly.response.data.errors, undefined, "fixture (a) must carry no errors[]");
  assert.strictEqual(isLineResolutionFailure(messageOnly), true, "the composed error.message alone must activate the fallback");

  // (b) errors[] only — `error.message` is the non-matching bare status text,
  // so the decision can come from nowhere but the structured entries.
  const errorsOnly = makeErr("Unprocessable Entity", 422, null, {
    message: "Unprocessable Entity",
    errors: ["Line could not be resolved"],
  });
  assert.strictEqual(isLineResolutionFailure({ message: errorsOnly.message }), false, "fixture (b)'s message must not match on its own");
  assert.strictEqual(isLineResolutionFailure(errorsOnly), true, "response.data.errors[] alone must activate the fallback");
  // Other wordings observed live on the same endpoint.
  assert.strictEqual(isLineResolutionFailure({ message: "Start position could not be resolved" }), true);
  assert.strictEqual(isLineResolutionFailure({ message: "Path could not be resolved" }), true);
  // A real non-line 422 seen live (missing comment body) must fall through.
  assert.strictEqual(
    isLineResolutionFailure({
      message:
        "Variable $threads of type [DraftPullRequestReviewThread] was provided invalid value for 0.body (Expected value to not be null)",
    }),
    false
  );

  // GitHub's own wording for this failure.
  assert.strictEqual(isLineResolutionFailure({ message: "Validation Failed: line must be part of the diff" }), true);
  assert.strictEqual(isLineResolutionFailure({ message: "Line could not be resolved" }), true);
  // Structured {field} entries are NOT returned by createReview (it returns
  // plain strings), but other REST endpoints do return them, so the branch is
  // kept and covered here as defensive behavior rather than observed behavior.
  assert.strictEqual(
    isLineResolutionFailure({ message: "Validation Failed", response: { data: { errors: [{ field: "start_line", code: "invalid" }] } } }),
    true
  );
  assert.strictEqual(
    isLineResolutionFailure({ message: "Validation Failed", errors: [{ message: "pull_request_review_thread.line must be part of the diff" }] }),
    true
  );

  // 422 on this endpoint also means "the endpoint has been spammed" — that must
  // NOT be read as a line-resolution problem, or the fallback would re-send a
  // batch into a throttled endpoint.
  assert.strictEqual(isLineResolutionFailure({ message: "You have exceeded a secondary rate limit" }), false);
  assert.strictEqual(isLineResolutionFailure({ message: "Validation Failed: body is too long" }), false);
  assert.strictEqual(isLineResolutionFailure({ message: "Validation Failed" }), false);
  assert.strictEqual(isLineResolutionFailure({}), false);
  assert.strictEqual(isLineResolutionFailure(null), false);
}

function testDescribeCommentLocation() {
  assert.strictEqual(describeCommentLocation({ line: 42 }), "Line 42");
  // A range that failed on start_line must not be described by its (valid) end
  // line alone.
  assert.strictEqual(describeCommentLocation({ start_line: 40, line: 42 }), "Lines 40-42");
  assert.strictEqual(describeCommentLocation({ start_line: 42, line: 42 }), "Line 42");
  assert.strictEqual(describeCommentLocation({}), "Line n/a");
}

async function testGetPrDiffHunks() {
  const files = [
    { filename: "src/main.js", patch: "@@ -1,2 +1,2 @@\n context 1\n+added 2" },
    { filename: "assets/logo.png" }, // no patch (binary)
  ];
  const gh = makeGithub({ files });
  const diff = await getPrDiffHunks({ github: gh, owner: "owner", repo: "repo", prNumber: 123, log: () => {} });
  assert.strictEqual(diff.complete, true);
  assert.deepStrictEqual(diff.files.get("src/main.js"), [{ start: 1, end: 2 }]);
  // A patchless file is KNOWN (so it is not "not in the PR") but has no ranges.
  assert.strictEqual(diff.known.has("assets/logo.png"), true);
  assert.strictEqual(diff.files.has("assets/logo.png"), false);

  // Cache: a second call for the same run must not re-fetch.
  const cache = {};
  const gh2 = makeGithub({ files });
  await getPrDiffHunks({ github: gh2, owner: "o", repo: "r", prNumber: 1, log: () => {}, cache });
  const afterFirst = gh2.listFilesCalls.length;
  await getPrDiffHunks({ github: gh2, owner: "o", repo: "r", prNumber: 1, log: () => {}, cache });
  assert.strictEqual(gh2.listFilesCalls.length, afterFirst, "cached diff inventory must not re-fetch listFiles");
}

async function testGetPrDiffHunksPaginatesCompleteInventory() {
  const files = [];
  for (let i = 0; i < 101; i++) {
    files.push({ filename: `f${i}.js`, patch: "@@ -1,1 +1,1 @@\n line" });
  }
  const gh = makeGithub({ files });
  const diff = await getPrDiffHunks({
    github: gh,
    owner: "o",
    repo: "r",
    prNumber: 1,
    log: () => {},
  });
  assert.strictEqual(gh.listFilesCalls.length, 2, "101 files must require exactly two listFiles pages");
  assert.strictEqual(diff.complete, true, "a fully enumerated multi-page inventory is complete");
  assert.strictEqual(
    classifyCommentAgainstDiff({ reviewComment: { path: "f100.js", line: 1 } }, diff),
    "valid",
    "a path from page two must participate in classification"
  );
}

async function testClippedPatchIsUnknown() {
  const gh = makeGithub({
    files: [
      {
        filename: "src/clipped.js",
        // Header declares 100 RIGHT-side lines; the returned body carries only
        // two. The observed prefix cannot prove later lines out-of-diff.
        patch: "@@ -1,2 +1,100 @@\n context\n+added",
      },
    ],
  });
  const diff = await getPrDiffHunks({
    github: gh,
    owner: "o",
    repo: "r",
    prNumber: 1,
    log: () => {},
  });
  assert.strictEqual(diff.complete, true, "the file list itself is complete");
  assert.strictEqual(diff.known.has("src/clipped.js"), true);
  assert.strictEqual(diff.files.has("src/clipped.js"), false, "a clipped patch must not expose authoritative ranges");
  assert.strictEqual(
    classifyCommentAgainstDiff({ reviewComment: { path: "src/clipped.js", line: 50 } }, diff),
    "unknown",
    "a line hidden by patch clipping must not be condemned"
  );
}

async function testMovedHeadMakesInventoryIncomplete() {
  const gh = makeGithub({
    headSha: "new-head",
    files: [{ filename: "src/current.js", patch: "@@ -1,1 +1,1 @@\n line" }],
  });
  const diff = await getPrDiffHunks({
    github: gh,
    owner: "o",
    repo: "r",
    prNumber: 1,
    commitSha: "reviewed-head",
    log: () => {},
  });
  assert.strictEqual(gh.getPullCalls.length, 1, "commit-aware inventory must verify the current PR head");
  assert.strictEqual(diff.complete, false, "a current-head inventory cannot prove locations on an older reviewed commit");
  assert.strictEqual(
    classifyCommentAgainstDiff({ reviewComment: { path: "missing-on-current.js", line: 1 } }, diff),
    "unknown"
  );
}

async function testRunnerHeadDriftPreservesComments() {
  const gh = makeGithub({
    headSha: "new-head",
    files: [{ filename: "src/current.js", patch: "@@ -1,1 +1,1 @@\n current" }],
    batchErrorSpec: [{ message: "Line could not be resolved", status: 422 }],
  });
  const result = {
    comments: [
      // Valid on the event-time head but absent from the current-head inventory.
      { path: "src/event-head.js", content: "must survive head drift", start_line: 1, end_line: 1 },
    ],
  };

  await runPostReviewComments({
    github: gh,
    context, // context head is "head-sha"; mocked current head is "new-head"
    core: { setOutput() {} },
    fs: mockFs(JSON.stringify(result), ""),
    out: {},
  });

  assert.strictEqual(gh.getPullCalls.length, 1, "the runner must thread commitSha into inventory verification");
  assert.strictEqual(gh.createReviewCalls.length, 2, "head drift must preserve the comment via per-comment fallback");
  assert.strictEqual(gh.createReviewCalls[1].body, "");
  assert.strictEqual(gh.createReviewCalls[1].comments[0].path, "src/event-head.js");
  const summaryText = gh.updatedComments[0].body;
  assert.strictEqual(summaryText.includes("Successfully posted inline: 1 comment(s)"), true);
  assert.strictEqual(summaryText.includes("outside PR diff hunks"), false);
}

async function testGetPrDiffHunksTruncationIsIncomplete() {
  // 30 pages x 100 files is the GitHub listFiles ceiling; a PR at or past it
  // yields an inventory we must not treat as authoritative.
  const files = [];
  for (let i = 0; i < 3100; i++) files.push({ filename: `f${i}.js`, patch: "@@ -1,1 +1,1 @@\n a" });
  const gh = makeGithub({ files });
  const diff = await getPrDiffHunks({ github: gh, owner: "o", repo: "r", prNumber: 1, log: () => {} });
  assert.strictEqual(diff.complete, false, "a truncated file walk must report complete=false");
  // ...and an incomplete inventory must never condemn a comment.
  assert.strictEqual(classifyCommentAgainstDiff({ reviewComment: { path: "nope.js", line: 1 } }, diff), "unknown");
}

// REGRESSION: an EMPTY changed-file list is an anomaly, not proof that every
// commented path sits outside the diff. A PR that produced review comments has
// changed files by construction, so an empty listFiles response means the diff
// is unavailable (not yet materialized server-side, or a malformed body).
// Treating it as authoritative would classify EVERY comment "invalid" and
// discard the whole batch without a single posting attempt — the exact outcome
// the tri-state classification exists to prevent. This is the mirror of the
// >MAX_PAGES truncation guard: too many files and zero files are both
// "cannot judge".
async function testEmptyDiffInventoryDoesNotCondemnComments() {
  const gh = makeGithub({
    files: [], // listFiles returns an empty page
    batchErrorSpec: [{ message: "Line could not be resolved", status: 422 }],
  });
  const result = {
    comments: [
      { path: "src/a.js", content: "c1", start_line: 1, end_line: 1 },
      { path: "src/b.js", content: "c2", start_line: 2, end_line: 2 },
    ],
  };

  await runPostReviewComments({
    github: gh,
    context,
    core: { setOutput() {} },
    fs: mockFs(JSON.stringify(result), ""),
    out: {},
  });

  assert.strictEqual(gh.listFilesCalls.length > 0, true, "the 422 fallback must have consulted the diff inventory");
  // Batch (422) + one per-comment ATTEMPT each. Nothing may be routed to the
  // summary without ever being tried.
  assert.strictEqual(gh.createReviewCalls.length, 3, "every comment must still be attempted individually");
  const perComment = gh.createReviewCalls.filter((c) => (c.body || "") === "");
  assert.strictEqual(perComment.length, 2);
  assert.deepStrictEqual(
    perComment.map((c) => c.comments[0].path).sort(),
    ["src/a.js", "src/b.js"],
    "both comments must reach the per-comment loop"
  );

  const summaryText = gh.updatedComments[0].body;
  assert.strictEqual(summaryText.includes("Successfully posted inline: 2 comment(s)"), true);
  // buildSummaryBody omits the failed line entirely when the count is zero.
  assert.strictEqual(summaryText.includes("Failed to post inline"), false);
  assert.strictEqual(summaryText.includes("outside PR diff hunks"), false, "an empty inventory must condemn nothing");
}

async function testHttp422SecondaryFilteredBatchFallback() {
  const files = [
    { filename: "src/valid.js", patch: "@@ -1,2 +1,2 @@\n context 1\n+added 2" },
  ];
  const result = {
    comments: [
      { path: "src/valid.js", content: "valid comment", start_line: 2, end_line: 2, severity: "high", category: "bug" },
      { path: "src/invalid.js", content: "out of diff comment", start_line: 99, end_line: 99, severity: "high", category: "bug" },
    ],
  };

  const gh = makeGithub({
    files,
    batchErrorSpec: [
      {
        message: "Unprocessable Entity",
        status: 422,
        data: {
          message: "Unprocessable Entity",
          errors: ["Line could not be resolved"],
        },
      },
    ],
  });
  const core = { setOutput() {} };

  await runPostReviewComments({
    github: gh,
    context,
    core,
    fs: mockFs(JSON.stringify(result), ""),
    out: {},
  });

  // Call #0: initial batch (body === REVIEW_TAG, 2 comments) -> threw 422.
  // Call #1: secondary filtered batch (1 surviving comment) -> succeeded.
  // No per-comment calls: the whole point of the fallback.
  assert.strictEqual(gh.createReviewCalls.length, 2, "Expected 2 createReview calls (initial batch + secondary filtered batch)");
  const secondaryCall = gh.createReviewCalls[1];
  assert.strictEqual(secondaryCall.comments.length, 1, "Secondary batch should contain only 1 valid comment");
  assert.strictEqual(secondaryCall.comments[0].path, "src/valid.js");

  assert.strictEqual(gh.updatedComments.length, 1);
  const summaryText = gh.updatedComments[0].body;
  assert.strictEqual(summaryText.includes("Successfully posted inline: 1 comment(s)"), true);
  assert.strictEqual(summaryText.includes("Failed to post inline: 1 comment(s)"), true);
  assert.strictEqual(summaryText.includes("Line 99 could not be resolved"), true);
  assert.strictEqual(
    summaryText.split("out of diff comment").length - 1,
    1,
    "the filtered finding's original content must appear exactly once in the summary"
  );
}

// REGRESSION, two halves of one behavior:
//   * A batch in which classification removed NOTHING must not be re-sent: the
//     filtered payload would be byte-identical to the one GitHub just rejected,
//     so the resend is a guaranteed second 422 against an endpoint that may be
//     spam-throttling us. Reachable whenever our diff view disagrees with
//     GitHub's — commitSha is the head SHA captured at trigger time, while the
//     hunk inventory describes the PR's CURRENT diff.
//   * ...but skipping the resend must not DISCARD those comments. They are the
//     provably-valid ones; they belong in the per-comment loop, which is the
//     pre-existing behavior for anything the batch path cannot place.
async function testAllValidBatchSkipsSecondaryAndKeepsEveryComment() {
  const files = [{ filename: "src/valid.js", patch: "@@ -1,3 +1,3 @@\n a\n b\n c" }];
  // Every comment sits inside the hunk, so classification filters nothing.
  const result = {
    comments: [
      { path: "src/valid.js", content: "c1", start_line: 1, end_line: 1 },
      { path: "src/valid.js", content: "c2", start_line: 2, end_line: 2 },
    ],
  };
  const gh = makeGithub({
    files,
    batchErrorSpec: [{ message: "Line could not be resolved", status: 422 }],
  });

  await runPostReviewComments({
    github: gh,
    context,
    core: { setOutput() {} },
    fs: mockFs(JSON.stringify(result), ""),
    out: {},
  });

  const batches = gh.createReviewCalls.filter((c) => (c.body || "") === REVIEW_TAG);
  assert.strictEqual(batches.length, 1, "an unfiltered payload must not be re-sent to the endpoint that just rejected it");
  // Batch (422) + one per-comment attempt each.
  assert.strictEqual(gh.createReviewCalls.length, 3, "skipping the secondary batch must not discard the valid comments");
  const perComment = gh.createReviewCalls.filter((c) => (c.body || "") === "");
  assert.deepStrictEqual(perComment.map((c) => c.comments[0].line), [1, 2]);

  const summaryText = gh.updatedComments[0].body;
  assert.strictEqual(summaryText.includes("Successfully posted inline: 2 comment(s)"), true);
  // buildSummaryBody omits the failed line entirely when the count is zero.
  assert.strictEqual(summaryText.includes("Failed to post inline"), false);
}

// REGRESSION: a SUCCEEDING secondary batch must not swallow the "unknown"
// comments alongside it. The three verdicts have three different destinations
// in this scenario — valid -> the secondary batch, invalid -> the summary,
// unknown -> the per-comment loop — and only a fixture carrying all three at
// once can tell a correct hand-off from one that drops a bucket. The prior
// tests all exercised a FAILING secondary, so the success path's
// `toRetry = unknownItems` assignment was never pinned.
async function testHttp422SecondarySuccessStillPostsUnknownComments() {
  const files = [
    { filename: "src/valid.js", patch: "@@ -1,2 +1,2 @@\n ctx\n+added" },
    { filename: "src/binary.js" }, // in the PR, but GitHub omitted its patch
  ];
  const result = {
    comments: [
      // valid: inside the only hunk (lines 1-2).
      { path: "src/valid.js", content: "in hunk", start_line: 1, end_line: 1 },
      // unknown: the file IS in the PR, so it is not "outside the diff", but
      // without a patch we know nothing about its lines.
      { path: "src/binary.js", content: "no patch to check against", start_line: 1, end_line: 1 },
      // invalid: provably outside the diff — and the reason the secondary
      // batch fires at all (filtering must remove something).
      { path: "src/valid.js", content: "out of diff", start_line: 99, end_line: 99 },
    ],
  };
  const gh = makeGithub({
    files,
    // Only the PRIMARY batch fails; the secondary filtered batch succeeds.
    batchErrorSpec: [{ message: "Line could not be resolved", status: 422 }],
  });

  await runPostReviewComments({
    github: gh,
    context,
    core: { setOutput() {} },
    fs: mockFs(JSON.stringify(result), ""),
    out: {},
  });

  // Call #0: primary batch (3 comments) -> 422.
  // Call #1: secondary filtered batch (the 1 valid comment) -> succeeded.
  // Call #2: per-comment fallback for the unknown comment.
  assert.strictEqual(gh.createReviewCalls.length, 3);
  assert.strictEqual(gh.createReviewCalls[1].comments.length, 1, "only the provably valid comment may be re-batched");
  assert.strictEqual(gh.createReviewCalls[1].comments[0].path, "src/valid.js");

  // THE LOAD-BEARING ASSERTION: a successful secondary batch must hand the
  // unknown comments to the per-comment loop, not discard them with the
  // filtered-out ones. Per-comment reviews are identified by body === "".
  const perComment = gh.createReviewCalls.filter((c) => (c.body || "") === "");
  assert.strictEqual(perComment.length, 1, "the unknown comment must still be attempted individually");
  assert.strictEqual(
    perComment[0].comments[0].path,
    "src/binary.js",
    "a patchless file is UNKNOWN, not out-of-diff; it must survive a successful secondary batch"
  );

  const summaryText = gh.updatedComments[0].body;
  assert.strictEqual(summaryText.includes("Successfully posted inline: 2 comment(s)"), true);
  assert.strictEqual(summaryText.includes("Failed to post inline: 1 comment(s)"), true);
  assert.strictEqual(summaryText.includes("Line 99 could not be resolved"), true);
}

// REGRESSION: a secondary batch that LANDS but whose response is lost (5xx /
// network error) must be reconciled, not blindly re-posted. Without this, every
// surviving comment is duplicated — reintroducing exactly the churn the 422
// fallback exists to remove.
async function testHttp422SecondaryFailureReconcilesInsteadOfDuplicating() {
  const files = [{ filename: "src/valid.js", patch: "@@ -1,3 +1,3 @@\n a\n b\n c" }];
  // The out-of-diff comment is what makes this fixture realistic: the secondary
  // batch only fires when classification actually REMOVED something (an
  // unchanged payload would just 422 again), so a batch of nothing but valid
  // comments never reaches the secondary path this test exists to pin.
  const result = {
    comments: [
      { path: "src/valid.js", content: "c1", start_line: 1, end_line: 1 },
      { path: "src/valid.js", content: "c2", start_line: 2, end_line: 2 },
      { path: "src/valid.js", content: "out of diff", start_line: 99, end_line: 99 },
    ],
  };
  const gh = makeGithub({
    files,
    batchErrorSpec: [
      { message: "Line could not be resolved", status: 422 },
      { message: "Bad gateway", status: 502 },
    ],
    batchLanded: true, // listReviews reports a review carrying this run's tag
    echoBatchIdx: 1, // ...and the SECONDARY batch's comments are on the server
  });

  await runPostReviewComments({
    github: gh,
    context,
    core: { setOutput() {} },
    fs: mockFs(JSON.stringify(result), ""),
    out: {},
  });

  assert.strictEqual(
    gh.createReviewCalls.length,
    2,
    "secondary batch landed: no comment may be re-posted individually"
  );
  assert.strictEqual(gh.listReviewsCalls.length > 0, true, "secondary 5xx must trigger the idempotency read");
  assert.strictEqual(gh.listReviewCommentsCalls.length > 0, true, "secondary 5xx must reconcile against posted comment IDs");
  const summaryText = gh.updatedComments[0].body;
  assert.strictEqual(summaryText.includes("Successfully posted inline: 2 comment(s)"), true);
  // Only the comment the filter removed is reported as failed; the two that
  // landed in the secondary batch must not be counted twice or listed here.
  assert.strictEqual(summaryText.includes("Failed to post inline: 1 comment(s)"), true);
  assert.strictEqual(summaryText.includes("Line 99 could not be resolved"), true);
}

// REGRESSION: the posted-comment read inside the reconciler can fail after a
// tagged review was found. Letting the error escape would skip finalization;
// retrying the comments would duplicate anything that landed in that review.
// The safe outcome is visible uncertainty: no retry, final summary, and each
// unverified survivor accounted as failed.
async function testPostedCommentReadFailureDoesNotUnwindTheRun() {
  const files = [{ filename: "src/valid.js", patch: "@@ -1,3 +1,3 @@\n a\n b\n c" }];
  const result = {
    comments: [
      { path: "src/valid.js", content: "c1", start_line: 1, end_line: 1 },
      { path: "src/valid.js", content: "c2", start_line: 2, end_line: 2 },
      { path: "src/valid.js", content: "out of diff", start_line: 99, end_line: 99 },
    ],
  };
  const gh = makeGithub({
    files,
    batchErrorSpec: [
      { message: "Line could not be resolved", status: 422 },
      { message: "Bad gateway", status: 502 },
    ],
    batchLanded: true, // listReviews finds a review carrying this run's tag...
    listReviewCommentsThrow: true, // ...but the posted-comment read is down.
  });

  // "Must RETURN, not throw" is itself an assertion here, so make it an
  // explicit one: letting the rejection propagate would abort the runner with a
  // bare read error naming no test.
  try {
    await runPostReviewComments({
      github: gh,
      context,
      core: { setOutput() {} },
      fs: mockFs(JSON.stringify(result), ""),
      out: {},
    });
  } catch (e) {
    assert.fail(`a failed posted-comment read must not unwind the run: ${e.message}`);
  }

  assert.strictEqual(gh.listReviewCommentsCalls.length > 0, true, "the reconciler must have attempted the posted-comment read");
  // The summary was still finalized rather than left on its pre-review body.
  assert.strictEqual(gh.updatedComments.length, 1, "the run must still finalize the summary");
  assert.strictEqual(
    gh.updatedComments[0].body.includes("Posting review comments"),
    false,
    "an unwound run would leave the summary on its pre-review body"
  );

  // The primary and secondary batch attempts are the only writes. Once the
  // tagged secondary review is known to exist, an unavailable posted-ID read
  // makes every survivor uncertain; retrying either could duplicate it.
  assert.strictEqual(gh.createReviewCalls.length, 2);
  const perComment = gh.createReviewCalls.filter((c) => (c.body || "") === "");
  assert.strictEqual(perComment.length, 0, "unverified comments must not be re-posted");

  const summaryText = gh.updatedComments[0].body;
  assert.strictEqual(summaryText.includes("Successfully posted inline: 0 comment(s)"), true);
  assert.strictEqual(summaryText.includes("Failed to post inline: 3 comment(s)"), true);
  assert.strictEqual(summaryText.split("Could not verify whether").length - 1, 2);
  assert.strictEqual(summaryText.includes("c1"), true);
  assert.strictEqual(summaryText.includes("c2"), true);
}

// The mirror case: the secondary batch genuinely did NOT land, so the
// per-comment loop must still run and post every surviving comment exactly once.
async function testHttp422SecondaryFailureFallsBackWhenNothingLanded() {
  const files = [{ filename: "src/valid.js", patch: "@@ -1,3 +1,3 @@\n a\n b\n c" }];
  // As above: one provably out-of-diff comment so filtering removes something
  // and the secondary batch actually fires.
  const result = {
    comments: [
      { path: "src/valid.js", content: "c1", start_line: 1, end_line: 1 },
      { path: "src/valid.js", content: "c2", start_line: 2, end_line: 2 },
      { path: "src/valid.js", content: "out of diff", start_line: 99, end_line: 99 },
    ],
  };
  const gh = makeGithub({
    files,
    batchErrorSpec: [
      { message: "Line could not be resolved", status: 422 },
      { message: "Bad gateway", status: 502 },
    ],
  });

  await runPostReviewComments({
    github: gh,
    context,
    core: { setOutput() {} },
    fs: mockFs(JSON.stringify(result), ""),
    out: {},
  });

  // 2 batch calls + 2 per-comment calls. The filtered-out comment is never
  // attempted, so it contributes no per-comment call.
  assert.strictEqual(gh.createReviewCalls.length, 4);
  assert.strictEqual(gh.listReviewsCalls.length > 0, true, "secondary 5xx must still check whether the review landed");
  const summaryText = gh.updatedComments[0].body;
  assert.strictEqual(summaryText.includes("Successfully posted inline: 2 comment(s)"), true);
  assert.strictEqual(summaryText.includes("Failed to post inline: 1 comment(s)"), true);
}

// REGRESSION: the reconciler must match INDIVIDUAL comment ids against what the
// server actually holds, not treat "a review with this run's tag exists" as
// proof that every comment in the batch landed. A fully-echoed batch cannot
// tell those two implementations apart — both post nothing further — so echo
// only PART of the secondary batch and require the remainder to be re-sent.
// The primary batch path already has this coverage (see the postedCount: 1 case
// in the multi-path recovery test); this is its secondary-batch mirror.
async function testHttp422SecondaryFailureReconcilesPerCommentNotWholesale() {
  const files = [{ filename: "src/valid.js", patch: "@@ -1,3 +1,3 @@\n a\n b\n c" }];
  // As above: one provably out-of-diff comment so filtering removes something
  // and the secondary batch actually fires.
  const result = {
    comments: [
      { path: "src/valid.js", content: "c1", start_line: 1, end_line: 1 },
      { path: "src/valid.js", content: "c2", start_line: 2, end_line: 2 },
      { path: "src/valid.js", content: "out of diff", start_line: 99, end_line: 99 },
    ],
  };
  const gh = makeGithub({
    files,
    batchErrorSpec: [
      { message: "Line could not be resolved", status: 422 },
      { message: "Bad gateway", status: 502 },
    ],
    batchLanded: true, // listReviews reports a review carrying this run's tag
    echoBatchIdx: 1, // ...and the SECONDARY batch is the one that landed...
    postedCount: 1, // ...but only its FIRST comment actually made it.
  });

  await runPostReviewComments({
    github: gh,
    context,
    core: { setOutput() {} },
    fs: mockFs(JSON.stringify(result), ""),
    out: {},
  });

  // Call #0: initial batch -> 422. Call #1: secondary batch -> 502 (partly
  // landed). Call #2: per-comment retry of ONLY the comment that never landed.
  assert.strictEqual(gh.createReviewCalls.length, 3, "the un-posted comment must be retried individually");
  assert.strictEqual(gh.createReviewCalls[2].body, "", "the retry must be a per-comment review, not another batch");
  assert.strictEqual(gh.createReviewCalls[2].comments.length, 1, "only the missing comment may be re-sent");
  assert.strictEqual(
    gh.createReviewCalls[2].comments[0].line,
    2,
    "the retried comment must be the one the server never received, not the one it already has"
  );

  const summaryText = gh.updatedComments[0].body;
  assert.strictEqual(summaryText.includes("Successfully posted inline: 2 comment(s)"), true);
  // Exactly the filtered-out comment is reported failed — reconciliation must
  // not turn the already-posted comment into a second failure.
  assert.strictEqual(summaryText.includes("Failed to post inline: 1 comment(s)"), true);
  assert.strictEqual(summaryText.includes("Line 99 could not be resolved"), true);
}

// A 429/403 rate-limit error surfacing from the secondary batch must cool down
// (honoring Retry-After) before the per-comment loop issues another write.
async function testHttp422SecondaryRateLimitCoolsDownBeforeRetrying() {
  const realBase = process.env.OCR_RETRY_BASE_DELAY;
  const realSetTimeout = global.setTimeout;
  process.env.OCR_RETRY_BASE_DELAY = "1";
  const logs = [];
  let releaseCooldown = null;
  let gh;
  try {
    const files = [{ filename: "src/valid.js", patch: "@@ -1,2 +1,2 @@\n a\n b" }];
    // The out-of-diff comment is what makes the secondary batch fire at all:
    // filtering must remove something, or the resend is skipped as identical.
    const result = {
      comments: [
        { path: "src/valid.js", content: "c1", start_line: 1, end_line: 1 },
        { path: "src/valid.js", content: "out of diff", start_line: 99, end_line: 99 },
      ],
    };
    gh = makeGithub({
      files,
      batchErrorSpec: [
        { message: "Line could not be resolved", status: 422 },
        { message: "You have exceeded a secondary rate limit", status: 429, headers: { "retry-after": "1" } },
      ],
    });
    // Hold the positive-delay timer instead of letting it fire. With the
    // required `await sleep(...)`, the runner must stop at two writes until the
    // test releases this callback. If the await (or sleep) is removed, the
    // per-comment write happens first and the assertion below sees three.
    global.setTimeout = (callback, delay, ...args) => {
      if (delay > 0 && releaseCooldown === null) {
        releaseCooldown = () => callback(...args);
        return 0;
      }
      return realSetTimeout(callback, delay, ...args);
    };

    const runPromise = runPostReviewComments({
      github: gh,
      context,
      // runPostReviewComments logs through core.info when available.
      core: { setOutput() {}, info: (m) => logs.push(m) },
      fs: mockFs(JSON.stringify(result), ""),
      out: {},
    });
    for (let i = 0; i < 50 && releaseCooldown === null; i++) {
      await new Promise((resolve) => realSetTimeout(resolve, 0));
    }
    assert.notStrictEqual(releaseCooldown, null, "secondary 429 must schedule a positive cooldown");
    assert.strictEqual(
      gh.createReviewCalls.length,
      2,
      "the per-comment write must remain blocked until the secondary cooldown completes"
    );
    releaseCooldown();
    await runPromise;

    const cooled = logs.some((m) => /Secondary filtered batch createReview failed \(HTTP 429\)\. Cooling down/.test(m));
    assert.strictEqual(cooled, true, "secondary rate-limit must announce its cooldown");
    // 429 cannot have created the review, so no idempotency read should fire.
    assert.strictEqual(gh.listReviewsCalls.length, 0, "a 429 rejects before creation; no idempotency read needed");
    // Asserting the cooldown alone would still pass if the code cooled down and
    // then gave up: that proves "cool down" but not "before retrying". Pin the
    // retry down too — initial batch + secondary batch + per-comment call.
    assert.strictEqual(gh.createReviewCalls.length, 3, "secondary 429 must fall through to the per-comment loop, not abandon the comment");
    assert.strictEqual(gh.createReviewCalls[2].body, "", "the third call must be a per-comment review (batches carry the run tag as body)");
    assert.strictEqual(gh.createReviewCalls[2].comments.length, 1, "the per-comment retry carries exactly the surviving comment");
    assert.strictEqual(gh.createReviewCalls[2].comments[0].line, 1, "the survivor, not the comment the filter removed");
    assert.strictEqual(gh.updatedComments[0].body.includes("Successfully posted inline: 1 comment(s)"), true);
    assert.strictEqual(gh.updatedComments[0].body.includes("Failed to post inline: 1 comment(s)"), true);
  } finally {
    global.setTimeout = realSetTimeout;
    if (realBase === undefined) delete process.env.OCR_RETRY_BASE_DELAY;
    else process.env.OCR_RETRY_BASE_DELAY = realBase;
  }
}

// Matching line-resolution wording is not sufficient without HTTP 422. The
// status guard prevents unrelated validation failures from discarding comments.
async function testMatchingWordingOnNon422SkipsFilteredBatch() {
  const files = [{ filename: "src/valid.js", patch: "@@ -1,2 +1,2 @@\n a\n b" }];
  const result = {
    comments: [{ path: "src/valid.js", content: "c1", start_line: 1, end_line: 1 }],
  };
  const gh = makeGithub({
    files,
    batchErrorSpec: [{ message: "Validation Failed: line must be part of the diff", status: 400 }],
  });

  await runPostReviewComments({
    github: gh,
    context,
    core: { setOutput() {} },
    fs: mockFs(JSON.stringify(result), ""),
    out: {},
  });

  assert.strictEqual(gh.listFilesCalls.length, 0, "matching wording on non-422 must not fetch the diff");
  assert.strictEqual(gh.createReviewCalls.length, 2, "non-422 must use the existing per-comment fallback");
  assert.strictEqual(gh.createReviewCalls[1].body, "");
}

// A 422 that is NOT a line-resolution failure (GitHub returns 422 for spam
// detection too) must not activate the filter or re-send a batch.
async function testNonLineResolution422SkipsFilteredBatch() {
  const files = [{ filename: "src/valid.js", patch: "@@ -1,2 +1,2 @@\n a\n b" }];
  const result = { comments: [{ path: "src/valid.js", content: "c1", start_line: 1, end_line: 1 }] };
  const gh = makeGithub({
    files,
    batchErrorSpec: [{ message: "Validation Failed: the endpoint has been spammed", status: 422 }],
  });

  await runPostReviewComments({
    github: gh,
    context,
    core: { setOutput() {} },
    fs: mockFs(JSON.stringify(result), ""),
    out: {},
  });

  assert.strictEqual(gh.listFilesCalls.length, 0, "an unrecognized 422 must not fetch the diff inventory");
  // Batch call + per-comment call only — no secondary batch.
  assert.strictEqual(gh.createReviewCalls.length, 2);
  assert.strictEqual(gh.createReviewCalls[1].body, "", "second call must be the per-comment fallback, not a batch");
}

// REGRESSION: a file whose patch GitHub omitted (binary / oversized diff) is
// UNKNOWN, not out-of-diff. Its comments must still be attempted individually
// rather than silently routed to the summary.
async function testUnknownDiffMetadataStillPostsComments() {
  const files = [{ filename: "src/huge.js" }]; // in the PR, but no `patch`
  const result = {
    comments: [
      { path: "src/huge.js", content: "c1", start_line: 1, end_line: 1 },
      { path: "src/huge.js", content: "c2", start_line: 2, end_line: 2 },
    ],
  };
  const gh = makeGithub({
    files,
    batchErrorSpec: [{ message: "Line could not be resolved", status: 422 }],
  });

  await runPostReviewComments({
    github: gh,
    context,
    core: { setOutput() {} },
    fs: mockFs(JSON.stringify(result), ""),
    out: {},
  });

  // Batch (422) + 2 per-comment calls. No secondary batch (nothing was provably
  // valid), and critically NOTHING was discarded.
  assert.strictEqual(gh.createReviewCalls.length, 3);
  const summaryText = gh.updatedComments[0].body;
  assert.strictEqual(summaryText.includes("Successfully posted inline: 2 comment(s)"), true);
  // buildSummaryBody omits the failed line entirely when the count is zero.
  assert.strictEqual(summaryText.includes("Failed to post inline"), false);
}

// When the diff inventory fetch itself fails, every comment is unknown and the
// original per-comment behavior is preserved.
async function testDiffFetchFailureDegradesToPerComment() {
  const result = { comments: [{ path: "src/a.js", content: "c1", start_line: 1, end_line: 1 }] };
  const gh = makeGithub({
    listFilesThrow: true,
    batchErrorSpec: [{ message: "Line could not be resolved", status: 422 }],
  });

  await runPostReviewComments({
    github: gh,
    context,
    core: { setOutput() {} },
    fs: mockFs(JSON.stringify(result), ""),
    out: {},
  });

  assert.strictEqual(gh.createReviewCalls.length, 2, "batch + per-comment fallback");
  assert.strictEqual(gh.updatedComments[0].body.includes("Successfully posted inline: 1 comment(s)"), true);
}

// All comments provably out of diff: nothing is posted, accounting still
// reconciles, and no stray createReview is issued.
async function testAllCommentsFilteredOutAccounting() {
  const files = [{ filename: "src/a.js", patch: "@@ -1,2 +1,2 @@\n a\n b" }];
  const result = {
    comments: [
      { path: "src/a.js", content: "c1", start_line: 90, end_line: 90 },
      { path: "src/gone.js", content: "c2", start_line: 1, end_line: 1 },
    ],
  };
  const gh = makeGithub({
    files,
    batchErrorSpec: [{ message: "Line could not be resolved", status: 422 }],
  });

  await runPostReviewComments({
    github: gh,
    context,
    core: { setOutput() {} },
    fs: mockFs(JSON.stringify(result), ""),
    out: {},
  });

  assert.strictEqual(gh.createReviewCalls.length, 1, "nothing survivable: only the original failed batch");
  const summaryText = gh.updatedComments[0].body;
  assert.strictEqual(summaryText.includes("Successfully posted inline: 0 comment(s)"), true);
  assert.strictEqual(summaryText.includes("Failed to post inline: 2 comment(s)"), true);
  assert.strictEqual(summaryText.includes("Line 90 could not be resolved"), true);
  assert.strictEqual(summaryText.split("c1").length - 1, 1, "first filtered finding content must appear once");
  assert.strictEqual(summaryText.split("c2").length - 1, 1, "second filtered finding content must appear once");
}

// A multi-line span straddling two hunks is provably invalid; the in-hunk span
// beside it still goes out in the grouped secondary batch.
async function testCrossHunkRangeIsFilteredOut() {
  const files = [{ filename: "src/a.js", patch: "@@ -1,3 +1,3 @@\n a\n b\n c\n@@ -50,3 +50,3 @@\n x\n y\n z" }];
  const result = {
    comments: [
      { path: "src/a.js", content: "straddles two hunks", start_line: 2, end_line: 51 },
      { path: "src/a.js", content: "inside one hunk", start_line: 50, end_line: 52 },
    ],
  };
  const gh = makeGithub({
    files,
    batchErrorSpec: [{ message: "Line could not be resolved", status: 422 }],
  });

  await runPostReviewComments({
    github: gh,
    context,
    core: { setOutput() {} },
    fs: mockFs(JSON.stringify(result), ""),
    out: {},
  });

  assert.strictEqual(gh.createReviewCalls.length, 2);
  assert.strictEqual(gh.createReviewCalls[1].comments.length, 1, "only the single-hunk span may be re-batched");
  assert.strictEqual(gh.createReviewCalls[1].comments[0].start_line, 50);
  const summaryText = gh.updatedComments[0].body;
  // The failure names the whole span, not just its (valid) end line.
  assert.strictEqual(summaryText.includes("Lines 2-51 could not be resolved"), true);
}

// ---------------------------------------------------------------------------
// Cross-push checkpoints (#476) — read path
// ---------------------------------------------------------------------------

const CK_OLD = "1a".repeat(20);
const CK_MID = "2b".repeat(20);
const CK_NEW = "3c".repeat(20);
const CK_MB = "4d".repeat(20);
const CK_MARKER_RE = /^<!-- ocr-checkpoint:v1 [A-Za-z0-9+/]+={0,2} -->$/;

function ckPayload(over = {}) {
  return Object.assign(
    {
      v: 1,
      pr: 123,
      head: CK_OLD,
      base_ref: "main",
      merge_base: CK_MB,
      terminal_state: "complete",
      fingerprint: "fp1",
      run: "run-1",
    },
    over
  );
}

// A sticky summary comment carrying (or not carrying) a checkpoint marker.
function ckComment({ payload, login = "github-actions[bot]", body } = {}) {
  const tail = body != null ? body : payload ? buildCheckpointMarker(payload) : "";
  return {
    id: 1,
    html_url: "http://ex/1",
    user: { login },
    body: `${SUMMARY_MARKER}\nSummary prose\n\n${tail}`,
  };
}

function ckGithub({ comments, login = "github-actions[bot]", listThrows = false, authThrows = false } = {}) {
  return {
    rest: {
      users: {
        getAuthenticated: async () => {
          if (authThrows) throw new Error("auth unavailable");
          return { data: { login } };
        },
      },
      issues: {
        listComments: async () => {
          if (listThrows) throw new Error("listComments 503");
          return { data: comments || [] };
        },
      },
    },
  };
}

function ckArgs(over = {}) {
  return Object.assign(
    {
      github: ckGithub({ comments: [ckComment({ payload: ckPayload() })] }),
      owner: "owner",
      repo: "repo",
      prNumber: 123,
      enabled: true,
      sticky: true,
      fullReview: false,
      headSha: CK_NEW,
      baseRef: "main",
      mergeBase: CK_MB,
      fingerprint: "fp1",
      isAncestor: async () => 0,
      log: () => {},
    },
    over
  );
}

// C10: the marker survives payloads that could break out of an HTML comment,
// and every malformed body reads as "no checkpoint" instead of throwing.
function testCheckpointMarkerRoundTrip() {
  const payloads = [
    ckPayload(),
    ckPayload({ note: "contains --> a closing sequence" }),
    ckPayload({ note: "line one\nline two" }),
    ckPayload({ note: "多字节 テキスト ✅" }), // allow-non-english: fixture proves the base64 round trip is byte-exact for a non-ASCII note
  ];
  for (const p of payloads) {
    const marker = buildCheckpointMarker(p);
    assert.strictEqual(CK_MARKER_RE.test(marker), true, `marker shape for ${JSON.stringify(p.note)}`);
    assert.deepStrictEqual(parseCheckpointMarker(marker), p, "round trip must be lossless");
    // Embedded in a real body, surrounded by prose.
    assert.deepStrictEqual(parseCheckpointMarker(`${SUMMARY_MARKER}\nprose\n${marker}\n`), p);
  }

  const b64 = (s) => Buffer.from(s, "utf8").toString("base64");
  const malformed = [
    ["empty string", ""],
    ["non-base64 payload", "<!-- ocr-checkpoint:v1 !!!not-base64!!! -->"],
    ["base64 of non-JSON", `<!-- ocr-checkpoint:v1 ${b64("not json at all")} -->`],
    ["base64 of a JSON array", `<!-- ocr-checkpoint:v1 ${b64("[1,2,3]")} -->`],
    ["--> inside summary prose", "Summary mentioning --> an arrow, with no marker"],
    ["two markers in one body", `${buildCheckpointMarker(ckPayload())}\n${buildCheckpointMarker(ckPayload({ head: CK_MID }))}`],
  ];
  for (const [label, body] of malformed) {
    assert.strictEqual(parseCheckpointMarker(body), null, `${label} must read as no checkpoint`);
  }
  // Non-string input must not throw either.
  assert.strictEqual(parseCheckpointMarker(undefined), null);
  assert.strictEqual(parseCheckpointMarker(null), null);
}

// C2: each of the twelve fail-closed reasons has its own input class, and every
// one of them reviews the FULL range.
async function testCheckpointTwelveReasonsAreDistinct() {
  const cases = [
    ["disabled", { enabled: false }],
    ["sticky_disabled", { sticky: false }],
    ["manual_full_review", { fullReview: true }],
    ["no_summary_comment", { github: ckGithub({ comments: [] }) }],
    ["author_unverified", { github: ckGithub({ comments: [ckComment({ payload: ckPayload(), login: "fork-user" })] }) }],
    ["corrupt_checkpoint", { github: ckGithub({ comments: [ckComment({ body: "no marker here" })] }) }],
    ["schema_invalid", { github: ckGithub({ comments: [ckComment({ payload: ckPayload({ v: 2 }) })] }) }],
    ["base_changed", { github: ckGithub({ comments: [ckComment({ payload: ckPayload({ merge_base: CK_MID }) })] }) }],
    ["config_changed", { github: ckGithub({ comments: [ckComment({ payload: ckPayload({ fingerprint: "other" }) })] }) }],
    ["not_ancestor", { isAncestor: async () => 1 }],
    ["unknown_object", { isAncestor: async () => 128 }],
    ["resolver_error", { github: ckGithub({ listThrows: true }) }],
  ];
  const seen = new Set();
  for (const [reason, over] of cases) {
    const r = await resolveCheckpointRange(ckArgs(over));
    assert.strictEqual(r.reason, reason, `expected reason ${reason}, got ${r.reason}`);
    assert.strictEqual(r.mode, "full", `${reason} must review the full range`);
    assert.strictEqual(r.from, "", `${reason} must not narrow the range`);
    seen.add(reason);
  }
  assert.strictEqual(seen.size, 12, "all twelve reasons must be reachable");

  // Exit 128 is "could not check", never "is an ancestor": it must not slip
  // through as a checkpoint under any other exit code either.
  for (const status of [2, -1, 129, null, undefined, "0"]) {
    const r = await resolveCheckpointRange(ckArgs({ isAncestor: async () => status }));
    assert.strictEqual(r.mode, "full", `ancestry status ${status} must fail closed`);
  }
  // A throwing resolver is an error, not an ancestor.
  const thrown = await resolveCheckpointRange(
    ckArgs({ isAncestor: async () => { throw new Error("git missing"); } })
  );
  assert.strictEqual(thrown.reason, "resolver_error");
  assert.strictEqual(thrown.mode, "full");
}

// C3: from one fully-valid input, every single-field mutation falls back to the
// full range; only the unmutated row narrows it.
async function testCheckpointSingleFieldMutationsFailClosed() {
  const mutations = [
    ["v", { github: ckGithub({ comments: [ckComment({ payload: ckPayload({ v: 2 }) })] }) }],
    ["pr", { github: ckGithub({ comments: [ckComment({ payload: ckPayload({ pr: 999 }) })] }) }],
    ["head format", { github: ckGithub({ comments: [ckComment({ payload: ckPayload({ head: "not-a-sha" }) })] }) }],
    ["terminal_state", { github: ckGithub({ comments: [ckComment({ payload: ckPayload({ terminal_state: "partial" }) })] }) }],
    ["merge_base", { github: ckGithub({ comments: [ckComment({ payload: ckPayload({ merge_base: CK_MID }) })] }) }],
    ["base_ref", { github: ckGithub({ comments: [ckComment({ payload: ckPayload({ base_ref: "develop" }) })] }) }],
    ["fingerprint", { github: ckGithub({ comments: [ckComment({ payload: ckPayload({ fingerprint: "fp2" }) })] }) }],
    ["ancestry exit 1", { isAncestor: async () => 1 }],
    ["ancestry exit 128", { isAncestor: async () => 128 }],
    ["comment author", { github: ckGithub({ comments: [ckComment({ payload: ckPayload(), login: "someone-else" })] }) }],
    ["stickySummary:false", { sticky: false }],
  ];
  assert.strictEqual(mutations.length, 11, "eleven single-field mutations");
  for (const [label, over] of mutations) {
    const r = await resolveCheckpointRange(ckArgs(over));
    assert.strictEqual(r.mode, "full", `mutating ${label} must review the full range`);
    assert.notStrictEqual(r.reason, "ok", `mutating ${label} must carry a fail-closed reason`);
  }
  const control = await resolveCheckpointRange(ckArgs());
  assert.strictEqual(control.mode, "checkpoint");
  assert.strictEqual(control.reason, "ok");
  assert.strictEqual(control.from, CK_OLD, "the unmutated row narrows to the checkpoint head");
  assert.strictEqual(control.to, CK_NEW);
}

// C9: authorship is verified against what GitHub attests about the WRITER — an
// app slug, or a bot account — both of which the API derives from the token
// that posted and neither of which a commenter can set. A well-formed marker
// planted by a fork contributor is ignored, and an author GitHub attributes to
// no bot at all is never given the benefit of the doubt.
//
// GET /user is deliberately not part of this (see
// testCheckpointAuthorSurvivesUnavailableGetUser): it 403s for the default
// GITHUB_TOKEN and for App installation tokens, i.e. for every token this
// action actually runs with. The `authThrows: true` fixtures below keep that
// endpoint broken to prove the gate no longer depends on it in either
// direction.
async function testCheckpointAuthorVerificationFailsClosed() {
  const planted = ckComment({ payload: ckPayload(), login: "fork-contributor" });
  const forked = await resolveCheckpointRange(ckArgs({ github: ckGithub({ comments: [planted] }) }));
  assert.strictEqual(forked.reason, "author_unverified");
  assert.strictEqual(forked.mode, "full");

  // A human account with write permission is still not this action: no app
  // attribution, no bot user, so the marker it carries is unusable.
  const unresolved = await resolveCheckpointRange(
    ckArgs({
      github: ckGithub({
        comments: [ckComment({ payload: ckPayload(), login: "release-manager" })],
        authThrows: true,
      }),
    })
  );
  assert.strictEqual(unresolved.reason, "author_unverified");
  assert.strictEqual(unresolved.mode, "full");

  // Same via the lower-level reader, so the fail-closed behavior is pinned at
  // the boundary and not only through the gate.
  const read = await readCheckpointComment({
    github: ckGithub({ comments: [ckComment({ payload: ckPayload(), login: "release-manager" })], authThrows: true }),
    owner: "owner",
    repo: "repo",
    prNumber: 123,
    log: () => {},
  });
  assert.strictEqual(read.reason, "author_unverified");
  assert.strictEqual(read.payload, null);
}

// C9 (continued): a caller that knows which app its token belongs to can pin
// the identity exactly. With appSlug set to this run's app, a marker left by
// the default GITHUB_TOKEN ("github-actions[bot]") is a different writer and
// must be rejected even though it is a perfectly good bot comment.
async function testCheckpointRejectsDifferentBotIdentity() {
  const github = ckGithub({ comments: [ckComment({ payload: ckPayload() })], login: "ocr-app[bot]" });
  const gated = await resolveCheckpointRange(ckArgs({ github, appSlug: "ocr-app" }));
  assert.strictEqual(gated.reason, "author_unverified");
  assert.strictEqual(gated.mode, "full");
  assert.strictEqual(gated.from, "");

  const read = await readCheckpointComment({
    github,
    owner: "owner",
    repo: "repo",
    prNumber: 123,
    appSlug: "ocr-app",
    log: () => {},
  });
  assert.strictEqual(read.reason, "author_unverified");
  assert.strictEqual(read.raw, "");
}

// C7: widen-only. An older checkpoint yields a wider range; the resolver never
// moves the start forward past what the marker recorded.
async function testCheckpointWidenOnly() {
  // Ancestry chain OLD -> MID -> NEW under the real 0/1/128 exit-code contract.
  const chain = [CK_OLD, CK_MID, CK_NEW];
  const isAncestor = async (a, b) => {
    const i = chain.indexOf(a);
    const j = chain.indexOf(b);
    if (i < 0 || j < 0) return 128;
    return i <= j ? 0 : 1;
  };
  const withHead = async (head) =>
    resolveCheckpointRange(
      ckArgs({ github: ckGithub({ comments: [ckComment({ payload: ckPayload({ head }) })] }), isAncestor })
    );

  const older = await withHead(CK_OLD);
  assert.strictEqual(older.mode, "checkpoint");
  assert.strictEqual(older.from, CK_OLD, "an older checkpoint reviews the wider range");
  const newer = await withHead(CK_MID);
  assert.strictEqual(newer.mode, "checkpoint");
  assert.strictEqual(newer.from, CK_MID);

  // An object outside this clone is unprovable, never an ancestor.
  const unknown = await withHead("9".repeat(40));
  assert.strictEqual(unknown.reason, "unknown_object");
  assert.strictEqual(unknown.mode, "full");
}

// C12: rerunning on the same head is a legal (empty) checkpoint range, not an
// error and not a silent full re-review. It carries its own reason so the
// posting step can tell "nothing to review" from "reviewed and found nothing"
// (testCheckpointNoopLeavesSummaryUntouched).
async function testCheckpointSameHeadRerun() {
  const r = await resolveCheckpointRange(
    ckArgs({ github: ckGithub({ comments: [ckComment({ payload: ckPayload({ head: CK_NEW }) })] }) })
  );
  assert.strictEqual(r.mode, "checkpoint");
  assert.strictEqual(r.reason, "same_head_noop");
  assert.strictEqual(r.from, CK_NEW);
  assert.strictEqual(r.from, r.to);
}

// C11: the resolver's contract is a fixed eight-key shape in both modes, so a
// consumer can read every field without existence checks.
async function testCheckpointResolveShape() {
  const expected = [
    "ancestry",
    "checkpointBefore",
    "fingerprint",
    "from",
    "mode",
    "reason",
    "sourceRun",
    "to",
  ];
  const checkpoint = await resolveCheckpointRange(ckArgs());
  assert.deepStrictEqual(Object.keys(checkpoint).sort(), expected);
  assert.deepStrictEqual(
    [checkpoint.checkpointBefore, checkpoint.ancestry, checkpoint.sourceRun, checkpoint.fingerprint],
    [CK_OLD, "ancestor", "run-1", "fp1"]
  );

  const disabled = await resolveCheckpointRange(ckArgs({ enabled: false }));
  assert.deepStrictEqual(Object.keys(disabled).sort(), expected);
  assert.deepStrictEqual(
    [disabled.from, disabled.to, disabled.checkpointBefore, disabled.ancestry, disabled.sourceRun, disabled.fingerprint],
    ["", CK_NEW, "", "", "", ""]
  );

  // A marker that was read but rejected still reports what it recorded, so the
  // "why was this a full review" question is answerable from the outputs alone.
  const rejected = await resolveCheckpointRange(ckArgs({ isAncestor: async () => 1 }));
  assert.deepStrictEqual(Object.keys(rejected).sort(), expected);
  assert.deepStrictEqual(
    [rejected.mode, rejected.checkpointBefore, rejected.ancestry, rejected.from],
    ["full", CK_OLD, "not_ancestor", ""]
  );
}

// validateCheckpointPayload is the schema half of the gate; pin its verdicts
// directly so a future field addition cannot silently loosen them.
function testValidateCheckpointPayload() {
  assert.strictEqual(validateCheckpointPayload(ckPayload(), { prNumber: 123 }), null);
  const rejects = [
    [null, {}],
    [ckPayload({ v: "1" }), { prNumber: 123 }],
    [ckPayload({ pr: 124 }), { prNumber: 123 }],
    [ckPayload({ head: "ABCDEF0123456789".padEnd(40, "0") }), { prNumber: 123 }],
    [ckPayload({ head: CK_OLD.slice(0, 39) }), { prNumber: 123 }],
    [ckPayload({ terminal_state: "failed" }), { prNumber: 123 }],
    [ckPayload({ terminal_state: "skipped" }), { prNumber: 123 }],
    [ckPayload({ base_ref: "" }), { prNumber: 123 }],
    [ckPayload({ merge_base: "" }), { prNumber: 123 }],
    [ckPayload({ fingerprint: "" }), { prNumber: 123 }],
  ];
  for (const [payload, opts] of rejects) {
    assert.strictEqual(typeof validateCheckpointPayload(payload, opts), "string");
  }
}

// ---------------------------------------------------------------------------
// Cross-push checkpoints (#476) — write path (advance / carry-forward)
// ---------------------------------------------------------------------------

const CK_RESOLVED = "5e".repeat(20);
const CARRY = buildCheckpointMarker(ckPayload({ head: CK_OLD, run: "carried" }));

function ckManifest(over = {}) {
  return Object.assign(
    { terminal_state: "complete", input: { mode: "range", resolved_head: CK_RESOLVED } },
    over
  );
}

function ckRunOptions(over = {}) {
  return Object.assign(
    {
      checkpointEnabled: true,
      checkpointCarry: "",
      checkpointBaseRef: "main",
      checkpointMergeBase: CK_MB,
      checkpointFingerprint: "fp1",
    },
    over
  );
}

// Read back the single body this run actually wrote to the sticky summary.
function lastSummaryBody(gh) {
  if (gh.updatedComments.length > 0) return gh.updatedComments[gh.updatedComments.length - 1].body;
  if (gh.issueComments.length > 0) return gh.issueComments[gh.issueComments.length - 1].body;
  return null;
}

// K2/C4: the advance is gated on publication completeness. Only a run that is
// terminal-complete, failed nothing, and published a summary may move the
// checkpoint forward.
async function testCheckpointAdvanceGateTable() {
  const terminals = ["complete", "partial", "failed", "skipped", null];
  const failures = [0, 1];
  const published = [true, false];
  let advancing = 0;
  for (const terminal of terminals) {
    for (const failed of failures) {
      for (const isPublished of published) {
        // failed=1 is produced the way production produces it: a finding whose
        // line is provably outside the diff, so the 422 fallback can neither
        // repost nor reconcile it.
        const result = {
          comments: [
            failed === 1
              ? { path: "src/a.js", content: "c1", start_line: 90, end_line: 90 }
              : { path: "src/a.js", content: "c1", start_line: 1, end_line: 1 },
          ],
          manifest: terminal === null ? undefined : ckManifest({ terminal_state: terminal }),
        };
        const gh = makeGithub(
          failed === 1
            ? {
                files: [{ filename: "src/a.js", patch: "@@ -1,2 +1,2 @@\n a\n b" }],
                batchErrorSpec: [{ message: "Line could not be resolved", status: 422 }],
              }
            : {}
        );
        // published=false: the summary cannot be written at all (the issue
        // comment API is down), so summaryUrl stays empty.
        if (!isPublished) {
          gh.rest.issues.listComments = async () => { throw makeErr("listComments unavailable", 503); };
        }
        const outputs = {};
        await runPostReviewComments(
          Object.assign(
            {
              github: gh,
              context,
              core: { setOutput: (k, v) => { outputs[k] = v; } },
              fs: mockFs(JSON.stringify(result), ""),
            },
            ckRunOptions()
          )
        );
        const body = lastSummaryBody(gh) || "";
        const advanced = body.includes("ocr-checkpoint");
        const label = `terminal=${terminal} failed=${failed} published=${isPublished}`;
        // Pin the fixture itself: the "failed" axis must really have failed a
        // finding, otherwise the row proves nothing.
        assert.strictEqual(outputs.comments_failed, String(failed), `${label}: fixture failure count`);
        assert.strictEqual(outputs.summary_comment_url === "", !isPublished, `${label}: fixture publication`);
        const expectAdvance = terminal === "complete" && failed === 0 && isPublished;
        assert.strictEqual(advanced, expectAdvance, `${label}: marker written=${advanced}`);
        if (expectAdvance) {
          advancing++;
          // C5: the recorded head is the manifest's resolved head, not the
          // runner's HEAD_SHA (which the fixture deliberately differs on).
          const payload = parseCheckpointMarker(body);
          assert.strictEqual(payload.head, CK_RESOLVED);
          assert.notStrictEqual(payload.head, context.payload.pull_request.head.sha);
          assert.strictEqual(payload.terminal_state, "complete");
          assert.strictEqual(payload.pr, context.issue.number);
          // C11: the advanced head is exported for the caller.
          assert.strictEqual(outputs.checkpoint_after, CK_RESOLVED);
        } else {
          assert.strictEqual(outputs.checkpoint_after, "", `${label}: checkpoint_after must stay empty`);
        }
      }
    }
  }
  assert.strictEqual(advancing, 1, "exactly one of the 20 cells may advance");
}

// A manifest whose resolved_head is not a full sha cannot identify a range, so
// it never advances however complete the run was.
async function testCheckpointAdvanceRequiresFullSha() {
  for (const head of ["", "abc123", CK_RESOLVED.toUpperCase(), `${CK_RESOLVED}0`, undefined]) {
    const result = {
      comments: [],
      manifest: ckManifest({ input: { resolved_head: head } }),
    };
    const gh = makeGithub({});
    await runPostReviewComments(
      Object.assign(
        { github: gh, context, core: { setOutput() {} }, fs: mockFs(JSON.stringify(result), "") },
        ckRunOptions()
      )
    );
    assert.strictEqual(
      (lastSummaryBody(gh) || "").includes("ocr-checkpoint"),
      false,
      `resolved_head ${JSON.stringify(head)} must not advance the checkpoint`
    );
  }
}

// C6/K7: every body-composition path re-emits the carried marker byte for byte
// when it does not advance, so a rewritten summary never erases the checkpoint.
async function testCheckpointCarryForwardOnEveryBodyPath() {
  const paths = [
    [
      "json parse failure",
      { raw: "not json", stderr: "ocr blew up", manifest: null },
    ],
    [
      "zero findings, incomplete run",
      { raw: JSON.stringify({ comments: [], manifest: ckManifest({ terminal_state: "partial" }) }), stderr: "" },
    ],
    [
      "findings, incomplete run",
      {
        raw: JSON.stringify({
          comments: [{ path: "src/a.js", content: "c1", start_line: 1, end_line: 1 }],
          manifest: ckManifest({ terminal_state: "failed" }),
        }),
        stderr: "",
      },
    ],
    [
      "findings, no manifest at all",
      {
        raw: JSON.stringify({ comments: [{ path: "src/a.js", content: "c1", start_line: 1, end_line: 1 }] }),
        stderr: "",
      },
    ],
  ];
  for (const [label, spec] of paths) {
    const gh = makeGithub({});
    await runPostReviewComments(
      Object.assign(
        { github: gh, context, core: { setOutput() {} }, fs: mockFs(spec.raw, spec.stderr) },
        ckRunOptions({ checkpointCarry: CARRY })
      )
    );
    const body = lastSummaryBody(gh);
    assert.notStrictEqual(body, null, `${label}: a summary must be written`);
    assert.strictEqual(body.split(CARRY).length, 2, `${label}: the carried marker must appear exactly once`);
    assert.deepStrictEqual(
      parseCheckpointMarker(body),
      parseCheckpointMarker(CARRY),
      `${label}: the carried marker must not be re-encoded`
    );

    // With no carry, the same path writes no checkpoint at all.
    const bare = makeGithub({});
    await runPostReviewComments(
      Object.assign(
        { github: bare, context, core: { setOutput() {} }, fs: mockFs(spec.raw, spec.stderr) },
        ckRunOptions({ checkpointCarry: "" })
      )
    );
    assert.strictEqual(
      (lastSummaryBody(bare) || "").includes("ocr-checkpoint"),
      false,
      `${label}: no carry means no marker`
    );
  }
}

// C8: a clean run with no findings is the most common complete run there is; it
// must advance the checkpoint through the zero-findings early return.
async function testCheckpointAdvancesOnZeroFindings() {
  const gh = makeGithub({});
  const outputs = {};
  await runPostReviewComments(
    Object.assign(
      {
        github: gh,
        context,
        core: { setOutput: (k, v) => { outputs[k] = v; } },
        fs: mockFs(JSON.stringify({ comments: [], manifest: ckManifest() }), ""),
      },
      ckRunOptions({ checkpointCarry: CARRY })
    )
  );
  const body = lastSummaryBody(gh);
  const payload = parseCheckpointMarker(body);
  assert.strictEqual(payload.head, CK_RESOLVED, "the zero-findings path must advance");
  assert.strictEqual(body.includes(CARRY), false, "advancing replaces the carried marker, never duplicates it");
  assert.strictEqual(outputs.checkpoint_after, CK_RESOLVED);
}

// A non-sticky run has nowhere durable to keep a checkpoint, so it must never
// advance one even when everything else is green.
async function testCheckpointNeverAdvancesWithoutSticky() {
  const gh = makeGithub({});
  await runPostReviewComments(
    Object.assign(
      {
        github: gh,
        context,
        core: { setOutput() {} },
        fs: mockFs(JSON.stringify({ comments: [], manifest: ckManifest() }), ""),
        stickySummary: false,
      },
      ckRunOptions()
    )
  );
  assert.strictEqual((lastSummaryBody(gh) || "").includes("ocr-checkpoint"), false);
}

// The carry must be gated on exactly the same two conditions as the advance.
// A non-sticky run posts a fresh comment every time, so re-emitting a marker
// read from an older comment would plant a checkpoint on a body that never
// carried one; a run with checkpointing off must never emit a marker at all.
// Both cases are reachable from the action: the resolve step exports
// OCR_CHECKPOINT_CARRY without consulting sticky_summary.
async function testCheckpointCarryIsGatedLikeTheAdvance() {
  const cases = [
    ["non-sticky", { stickySummary: false }, ckRunOptions({ checkpointCarry: CARRY })],
    ["checkpointing off", {}, { checkpointEnabled: false, checkpointCarry: CARRY }],
  ];
  for (const [label, runOver, ckOver] of cases) {
    // An otherwise-complete run: only the gate under test can suppress a marker.
    const gh = makeGithub({});
    const outputs = {};
    await runPostReviewComments(
      Object.assign(
        {
          github: gh,
          context,
          core: { setOutput: (k, v) => { outputs[k] = v; } },
          fs: mockFs(JSON.stringify({ comments: [], manifest: ckManifest() }), ""),
        },
        runOver,
        ckOver
      )
    );
    const body = lastSummaryBody(gh) || "";
    assert.strictEqual(body.includes("ocr-checkpoint"), false, `${label}: no marker may be emitted`);
    assert.strictEqual(body.includes(CARRY), false, `${label}: the carry must not be re-emitted`);
    assert.strictEqual(outputs.checkpoint_after, "", `${label}: nothing advanced`);
  }

  // The same run WITH both gates satisfied still carries, so the assertions
  // above are pinning the gate rather than a body that never carries anything.
  const gh = makeGithub({});
  await runPostReviewComments(
    Object.assign(
      {
        github: gh,
        context,
        core: { setOutput() {} },
        fs: mockFs(JSON.stringify({ comments: [], manifest: ckManifest({ terminal_state: "partial" }) }), ""),
      },
      ckRunOptions({ checkpointCarry: CARRY })
    )
  );
  assert.strictEqual(lastSummaryBody(gh).includes(CARRY), true, "sticky + enabled still carries");
}

// The resolver accepts a pre-read comment so the action can list comments once
// instead of twice (the caller needs the raw marker for the carry regardless).
// The pre-read must be the observation the decision is made from — not merely
// tolerated and then re-read.
async function testCheckpointResolverUsesPreReadComment() {
  let listCalls = 0;
  const counting = ckGithub({ comments: [ckComment({ payload: ckPayload() })] });
  const inner = counting.rest.issues.listComments;
  counting.rest.issues.listComments = async (args) => {
    listCalls += 1;
    return inner(args);
  };

  const read = await readCheckpointComment({
    github: counting,
    owner: "owner",
    repo: "repo",
    prNumber: 123,
    log: () => {},
  });
  assert.strictEqual(read.reason, "ok");
  assert.strictEqual(listCalls, 1);

  const range = await resolveCheckpointRange(ckArgs({ github: counting, read }));
  assert.strictEqual(range.mode, "checkpoint");
  assert.strictEqual(range.from, CK_OLD);
  assert.strictEqual(listCalls, 1, "the pre-read must not be followed by a second listComments");

  // A pre-read that failed closed still decides the outcome, and still costs no
  // further reads.
  const rejected = await resolveCheckpointRange(
    ckArgs({ github: counting, read: { reason: "author_unverified", payload: null, raw: "" } })
  );
  assert.strictEqual(rejected.mode, "full");
  assert.strictEqual(rejected.reason, "author_unverified");
  assert.strictEqual(listCalls, 1);

  // Omitting it keeps the standalone behavior: the resolver reads for itself.
  const standalone = await resolveCheckpointRange(ckArgs({ github: counting }));
  assert.strictEqual(standalone.mode, "checkpoint");
  assert.strictEqual(listCalls, 2);
}

// C1/K3: with today's option set (no checkpoint options at all) nothing about
// the emitted body or the existing outputs changes.
async function testCheckpointOptOutLeavesBodyUnchanged() {
  const result = { comments: [], manifest: ckManifest() };
  const outputs = {};
  const gh = makeGithub({});
  await runPostReviewComments({
    github: gh,
    context,
    core: { setOutput: (k, v) => { outputs[k] = v; } },
    fs: mockFs(JSON.stringify(result), ""),
  });
  const body = lastSummaryBody(gh);
  assert.strictEqual(body.includes("ocr-checkpoint"), false, "opt-out runs write no checkpoint");
  assert.strictEqual(body, `${SUMMARY_MARKER}\n✅ **OpenCodeReview**: No comments generated. Looks good to me.`);
  assert.strictEqual(outputs.comments_total, "0");
  assert.strictEqual(outputs.checkpoint_after, "");
}

// The two halves must actually meet: a marker written by a real run has to be
// accepted by the resolver on the next run, and the range it yields has to start
// where the previous run stopped. Tested separately, each half can drift into a
// shape the other rejects (a field the writer omits, a value the reader rejects)
// while both suites stay green.
async function testCheckpointWriteThenReadRoundTrip() {
  const gh = makeGithub({});
  await runPostReviewComments(
    Object.assign(
      {
        github: gh,
        context,
        core: { setOutput() {} },
        fs: mockFs(JSON.stringify({ comments: [], manifest: ckManifest() }), ""),
      },
      ckRunOptions()
    )
  );
  const written = lastSummaryBody(gh);

  // Next run, same configuration and same base: the marker just written is the
  // only input, and the new head descends from it.
  const nextHead = "6f".repeat(20);
  const range = await resolveCheckpointRange({
    github: ckGithub({ comments: [{ id: 7, user: { login: "github-actions[bot]" }, body: written }] }),
    owner: "owner",
    repo: "repo",
    prNumber: context.issue.number,
    enabled: true,
    sticky: true,
    fullReview: false,
    headSha: nextHead,
    baseRef: "main",
    mergeBase: CK_MB,
    fingerprint: "fp1",
    isAncestor: async (a, b) => (a === CK_RESOLVED && b === nextHead ? 0 : 1),
    log: () => {},
  });
  assert.strictEqual(range.mode, "checkpoint");
  assert.strictEqual(range.reason, "ok");
  assert.strictEqual(range.from, CK_RESOLVED, "the next run starts where this one stopped");
  assert.strictEqual(range.to, nextHead);

  // Same marker, one setting different: the writer's fingerprint is what the
  // reader compares, so a configuration change is caught end to end.
  const reconfigured = await resolveCheckpointRange({
    github: ckGithub({ comments: [{ id: 7, user: { login: "github-actions[bot]" }, body: written }] }),
    owner: "owner",
    repo: "repo",
    prNumber: context.issue.number,
    enabled: true,
    sticky: true,
    fullReview: false,
    headSha: nextHead,
    baseRef: "main",
    mergeBase: CK_MB,
    fingerprint: "fp2",
    isAncestor: async () => 0,
    log: () => {},
  });
  assert.strictEqual(reconfigured.reason, "config_changed");
  assert.strictEqual(reconfigured.mode, "full");
}

// ---------------------------------------------------------------------------
// Cross-push checkpoints (#476) — defect repairs
// ---------------------------------------------------------------------------

const REPO_ROOT = path.join(__dirname, "..", "..");
const ACTION_YML = require("fs").readFileSync(path.join(REPO_ROOT, "action.yml"), "utf8");
const ACTION_README = require("fs").readFileSync(
  path.join(REPO_ROOT, "examples", "github_actions", "README.md"),
  "utf8"
);
const HELPER_SOURCE = require("fs").readFileSync(path.join(__dirname, "post-review-comments.js"), "utf8");

// The github-script blocks in action.yml cannot be executed from here (they
// need the runner's octokit and env), so what this suite can pin is their text:
// the load-bearing strings and the structure a fix depends on. That is weaker
// than the js-side tests by construction, which is why the resolve step keeps
// every decision it can inside post-review-comments.js.
function actionStepBlock(name) {
  const start = ACTION_YML.indexOf(`    - name: ${name}\n`);
  assert.notStrictEqual(start, -1, `action.yml must have a "${name}" step`);
  const next = ACTION_YML.indexOf("\n    - name: ", start + 1);
  return next === -1 ? ACTION_YML.slice(start) : ACTION_YML.slice(start, next);
}

// …with one exception: the resolve step's script IS extractable from the block
// scalar and runnable, because github-script just evaluates it as the body of an
// async function with (github, context, core) in scope. Running it for real is
// the only way to prove the two properties that matter most — it never throws,
// and its fingerprint reacts to the repo's own rule file.
function actionScriptSource(stepName) {
  const block = actionStepBlock(stepName);
  const header = "        script: |\n";
  const at = block.indexOf(header);
  assert.notStrictEqual(at, -1, `${stepName} must have an inline script`);
  const body = [];
  for (const line of block.slice(at + header.length).split("\n")) {
    if (line.trim() === "") {
      body.push("");
      continue;
    }
    if (!line.startsWith("          ")) break;
    body.push(line.slice(10));
  }
  return body.join("\n");
}

async function runActionScript(source, { github, context, core, env }) {
  const saved = {};
  for (const [k, v] of Object.entries(env)) {
    saved[k] = process.env[k];
    if (v === undefined) delete process.env[k];
    else process.env[k] = v;
  }
  try {
    const fn = new Function("github", "context", "core", "require", `return (async () => {\n${source}\n})();`);
    return await fn(github, context, core, require);
  } finally {
    for (const [k, v] of Object.entries(saved)) {
      if (v === undefined) delete process.env[k];
      else process.env[k] = v;
    }
  }
}

function scriptCore() {
  return {
    outputs: {},
    warnings: [],
    infos: [],
    setOutput(name, value) { this.outputs[name] = value; },
    info(message) { this.infos.push(message); },
    warning(message) { this.warnings.push(message); },
  };
}

// The complete env the resolve step reads, so a test can vary one axis at a
// time and know nothing else moved. GITHUB_WORKSPACE is a fresh empty dir:
// no .opencodereview/rule.json, so that axis stays out of the picture.
function resolveEnv(over = {}) {
  const os = require("os");
  return Object.assign(
    {
      GITHUB_ACTION_PATH: REPO_ROOT,
      GITHUB_WORKSPACE: require("fs").mkdtempSync(path.join(os.tmpdir(), "ocr-fp-")),
      OCR_HEAD_SHA: CK_NEW,
      OCR_BASE_REF: "main",
      OCR_MERGE_BASE: CK_MB,
      OCR_STICKY_SUMMARY: "true",
      OCR_FULL_REVIEW: "false",
      OCR_EVENT_ACTION: "synchronize",
      OCR_RULE_PATH: "",
      OCR_FP_LLM_URL: "https://llm.example",
      OCR_FP_LLM_MODEL: "model",
      OCR_FP_LLM_USE_ANTHROPIC: "false",
      OCR_FP_LANGUAGE: "en",
      OCR_FP_LLM_EXTRA_BODY: "",
      OCR_FP_LLM_AUTH_HEADER: "",
      OCR_FP_LLM_TIMEOUT: "",
      OCR_FP_LLM_EXTRA_HEADERS: "",
      OCR_FP_RULE: "",
      OCR_FP_ROUTE_SEVERITY_BELOW: "",
      OCR_FP_ROUTE_CATEGORIES: "",
      OCR_FP_BACKGROUND: "",
      OCR_CHECKPOINT_APP_SLUG: "",
      OCR_VERSION_ACTUAL: "1.2.3",
    },
    over
  );
}

// U2 (executed, not just read). A missing helper is the real-world failure —
// `uses: owner/repo@sha` without a checkout, a container that cannot see
// GITHUB_ACTION_PATH — and it used to throw, failing a job whose review would
// otherwise have run fine against the full range.
async function testActionResolveScriptFallsBackInsteadOfThrowing() {
  const fsReal = require("fs");
  const os = require("os");
  const empty = fsReal.mkdtempSync(path.join(os.tmpdir(), "ocr-noscript-"));
  const core = scriptCore();
  await runActionScript(actionScriptSource("Resolve review range"), {
    github: {},
    context: { repo: { owner: "owner", repo: "repo" }, issue: { number: 123 } },
    core,
    env: {
      GITHUB_ACTION_PATH: empty,
      GITHUB_WORKSPACE: empty,
      OCR_HEAD_SHA: CK_NEW,
      OCR_BASE_REF: "main",
      OCR_MERGE_BASE: CK_MB,
      OCR_STICKY_SUMMARY: "true",
      OCR_FULL_REVIEW: "false",
      OCR_EVENT_ACTION: "synchronize",
      OCR_RULE_PATH: "",
      OCR_FP_LLM_URL: "https://llm.example",
      OCR_VERSION_ACTUAL: "1.2.3",
    },
  });
  assert.strictEqual(core.warnings.length, 1, "the failure is reported as a warning");
  assert.strictEqual(/could not resolve a range/.test(core.warnings[0]), true);
  assert.strictEqual(core.outputs.range_mode, "full");
  assert.strictEqual(core.outputs.range_reason, "resolver_error");
  assert.strictEqual(core.outputs.range_from, "", "empty range_from means ${RANGE_FROM:-$MERGE_BASE}");
  assert.strictEqual(core.outputs.range_to, CK_NEW);
  assert.strictEqual(core.outputs.checkpoint_carry, "");
}

// U3/U4 (executed). The fingerprint is what makes "same configuration" mean
// something. It must move when the repo's own rule file appears or changes —
// that file is loaded by every review whether or not `rule` is set — and when
// an input that changes what the model is told changes.
async function testActionScriptFingerprintCoversRepoLocalRules() {
  const fsReal = require("fs");
  const os = require("os");
  const source = actionScriptSource("Resolve review range");
  const workspace = fsReal.mkdtempSync(path.join(os.tmpdir(), "ocr-ws-"));
  const fingerprint = async (over = {}) => {
    const core = scriptCore();
    await runActionScript(source, {
      github: { rest: { issues: { listComments: async () => ({ data: [] }) } } },
      context: { repo: { owner: "owner", repo: "repo" }, issue: { number: 123 } },
      core,
      env: Object.assign(
        {
          GITHUB_ACTION_PATH: REPO_ROOT,
          GITHUB_WORKSPACE: workspace,
          OCR_HEAD_SHA: CK_NEW,
          OCR_BASE_REF: "main",
          OCR_MERGE_BASE: CK_MB,
          OCR_STICKY_SUMMARY: "true",
          OCR_FULL_REVIEW: "false",
          OCR_EVENT_ACTION: "synchronize",
          OCR_RULE_PATH: "",
          OCR_FP_LLM_URL: "https://llm.example",
          OCR_FP_LLM_MODEL: "model",
          OCR_FP_LLM_USE_ANTHROPIC: "false",
          OCR_FP_LANGUAGE: "en",
          OCR_FP_LLM_EXTRA_BODY: "",
          OCR_FP_RULE: "",
          OCR_FP_ROUTE_SEVERITY_BELOW: "low",
          OCR_FP_ROUTE_CATEGORIES: "",
          OCR_FP_BACKGROUND: "",
          OCR_VERSION_ACTUAL: "1.2.3",
        },
        over
      ),
    });
    assert.strictEqual(core.outputs.range_reason, "no_summary_comment", "fixture: an empty PR reads no marker");
    assert.strictEqual(core.warnings.length, 0, "a readable workspace produces no warning");
    return core.outputs.config_fingerprint;
  };

  const none = await fingerprint();
  assert.strictEqual(/^[0-9a-f]{16}$/.test(none), true, "the fingerprint is a short hex digest");

  const ruleDir = path.join(workspace, ".opencodereview");
  fsReal.mkdirSync(ruleDir, { recursive: true });
  fsReal.writeFileSync(path.join(ruleDir, "rule.json"), '[{"path":"**","rule":"be strict"}]');
  const added = await fingerprint();
  assert.notStrictEqual(added, none, "adding .opencodereview/rule.json must invalidate the checkpoint");

  fsReal.writeFileSync(path.join(ruleDir, "rule.json"), '[{"path":"**","rule":"be lenient"}]');
  const edited = await fingerprint();
  assert.notStrictEqual(edited, added, "editing its contents must invalidate the checkpoint");

  // The inputs axis still works, `background` included.
  const otherInputs = await fingerprint({ OCR_FP_BACKGROUND: "some background" });
  assert.notStrictEqual(otherInputs, edited, "a changed input must invalidate the checkpoint");

  // Two configurations that differ only in where one field ends must not hash
  // alike. Joining the axes into a single "|"-delimited string made this pair
  // identical ("low|a" + "" + "x" and "low" + "a|" + "x" both read as
  // low|a||x), which is a checkpoint surviving a config change that should
  // have invalidated it — the one direction this feature may never fail in.
  const shifted = await fingerprint({ OCR_FP_ROUTE_SEVERITY_BELOW: "low|a", OCR_FP_ROUTE_CATEGORIES: "", OCR_FP_BACKGROUND: "x" });
  const shiftedBack = await fingerprint({ OCR_FP_ROUTE_SEVERITY_BELOW: "low", OCR_FP_ROUTE_CATEGORIES: "a|", OCR_FP_BACKGROUND: "x" });
  assert.notStrictEqual(shifted, shiftedBack, "no input value may shift a fingerprint field boundary");
  const otherVersion = await fingerprint({ OCR_VERSION_ACTUAL: "1.2.4" });
  assert.notStrictEqual(otherVersion, edited, "a changed OCR version must invalidate the checkpoint");

  // The effort preset decides how many review rounds run, so a checkpoint a
  // `low` run recorded must not let a later `high` run skip the commits it
  // covered less thoroughly.
  const otherEffort = await fingerprint({ OCR_FP_EFFORT: "high" });
  assert.notStrictEqual(otherEffort, edited, "a changed effort preset must invalidate the checkpoint");
}

// U1. GET /user is 403 ("Resource not accessible by integration") for the
// default GITHUB_TOKEN and for App installation tokens — i.e. for every token
// this action runs with — so gating authorship on it meant production runs
// always fell back to the full range and the feature never did anything. The
// read path must establish authorship from what GitHub attests on the comment,
// and must still reject an author it cannot attribute to a bot.
async function testCheckpointAuthorSurvivesUnavailableGetUser() {
  const viaApp = ckComment({ payload: ckPayload() });
  viaApp.user = { login: "github-actions[bot]", type: "Bot" };
  viaApp.performed_via_github_app = { slug: "github-actions" };
  const github = ckGithub({ comments: [viaApp] });
  let authCalls = 0;
  github.rest.users.getAuthenticated = async () => {
    authCalls += 1;
    throw new Error("Resource not accessible by integration");
  };

  const range = await resolveCheckpointRange(ckArgs({ github }));
  assert.strictEqual(range.reason, "ok", "a 403 on GET /user must not force a full review");
  assert.strictEqual(range.mode, "checkpoint");
  assert.strictEqual(range.from, CK_OLD);
  assert.strictEqual(authCalls, 0, "the checkpoint read path must not call GET /user at all");

  // What counts as "written by us", at the unit that decides it.
  const shapes = [
    ["app attribution only", { user: { login: "anything" }, performed_via_github_app: { slug: "github-actions" } }, true],
    ["bot-typed user", { user: { login: "ocr-app[bot]", type: "Bot" } }, true],
    ["bot login, no type", { user: { login: "github-actions[bot]" } }, true],
    ["fork contributor", { user: { login: "fork-contributor", type: "User" } }, false],
    ["maintainer", { user: { login: "release-manager", type: "User" } }, false],
    ["no user at all", {}, false],
  ];
  for (const [label, comment, ours] of shapes) {
    assert.strictEqual(isCheckpointAuthorOurs(comment), ours, `${label}: ours=${ours}`);
  }
  // A caller that knows its app pins the identity exactly.
  assert.strictEqual(isCheckpointAuthorOurs({ user: { login: "github-actions[bot]" } }, "ocr-app"), false);
  assert.strictEqual(isCheckpointAuthorOurs({ user: { login: "ocr-app[bot]" } }, "ocr-app"), true);
}

// U1 (corner). `user.type` alone carries the whole decision when there is no app
// attribution and the login has no "[bot]" suffix. That shape is accepted: type
// is GitHub's own attestation about the writer, derived from the posting token
// and not settable by a commenter, so it is exactly the evidence the trust
// boundary is written against — the login string is only ever a fallback way of
// reading the same fact. Pinned because it is the one input where the two
// signals disagree, and because narrowing it to "[bot]"-suffixed logins would
// re-introduce name matching as a security check.
function testCheckpointBotTypeAloneEstablishesAuthorship() {
  assert.strictEqual(isCheckpointAuthorOurs({ user: { login: "plain", type: "Bot" } }), true);
  // The same login without the Bot type is a human account: no evidence, no trust.
  assert.strictEqual(isCheckpointAuthorOurs({ user: { login: "plain" } }), false);
  // And it cannot satisfy a pinned app slug, since it names no app.
  assert.strictEqual(isCheckpointAuthorOurs({ user: { login: "plain", type: "Bot" } }, "ocr-app"), false);
}

// U1 (hole). `performed_via_github_app` is also set on comments written with a
// user-to-server token, i.e. by a HUMAN acting through some GitHub App (a CLI,
// an editor integration, a browser extension). There the slug attests the
// client, not the author, so accepting app attribution on its own would have
// let a fork contributor plant a checkpoint just by commenting through any App
// — exactly the party the documented trust boundary excludes. GitHub's
// `user.type` is the discriminator, and it wins over the app attribution.
async function testCheckpointRejectsUserToServerAppComment() {
  const viaUserToServer = ckComment({ payload: ckPayload(), login: "fork-contributor" });
  viaUserToServer.user.type = "User";
  viaUserToServer.performed_via_github_app = { slug: "some-cli" };

  assert.strictEqual(isCheckpointAuthorOurs(viaUserToServer), false, "a human's App client is not a bot writer");
  // Even when the slug is the very app this action normally posts as.
  const spoofedSlug = ckComment({ payload: ckPayload(), login: "fork-contributor" });
  spoofedSlug.user.type = "User";
  spoofedSlug.performed_via_github_app = { slug: "github-actions" };
  assert.strictEqual(isCheckpointAuthorOurs(spoofedSlug), false);
  assert.strictEqual(isCheckpointAuthorOurs(spoofedSlug, "github-actions"), false, "and a pinned slug does not rescue it");

  // End to end: the planted marker is well-formed and would otherwise narrow.
  const range = await resolveCheckpointRange(ckArgs({ github: ckGithub({ comments: [spoofedSlug] }) }));
  assert.strictEqual(range.reason, "author_unverified");
  assert.strictEqual(range.mode, "full");
  assert.strictEqual(range.from, "");

  // The bot-typed writer with the same app attribution is still accepted, so
  // this pins the discriminator rather than the app check as a whole.
  const viaBot = ckComment({ payload: ckPayload() });
  viaBot.user.type = "Bot";
  viaBot.performed_via_github_app = { slug: "github-actions" };
  assert.strictEqual(isCheckpointAuthorOurs(viaBot), true);
}

// U6 (robustness). The no-op is checked before the OCR output is read, because
// an empty range is also the range OCR is most likely to trip over: an error
// banner written over the previous run's findings destroys them just as
// thoroughly as "No comments generated" does.
async function testCheckpointNoopSurvivesUnreadableOcrOutput() {
  const previous = `${SUMMARY_MARKER}\n🔍 **OpenCodeReview** found **2** issue(s) in this PR.\n\n${CARRY}`;
  const gh = makeGithub({
    existingSummary: [{ id: 42, html_url: "http://ex/42", user: { login: "github-actions[bot]" }, body: previous }],
  });
  await runPostReviewComments(
    Object.assign(
      {
        github: gh,
        context,
        core: { setOutput() {} },
        fs: mockFs("not json at all", "ocr: empty commit range"),
      },
      ckRunOptions({ checkpointCarry: CARRY, checkpointNoop: true })
    )
  );
  assert.deepStrictEqual(gh.updatedComments, [], "a no-op rerun must not post an error banner over the summary");
  assert.deepStrictEqual(gh.issueComments, []);
}

// U6. A rerun on the same head reviews an empty range and therefore has nothing
// to report. Writing "No comments generated" over the previous run's summary
// would destroy findings for commits nobody touched, so the no-op leaves the
// existing comment exactly as it is.
async function testCheckpointNoopLeavesSummaryUntouched() {
  const previous = `${SUMMARY_MARKER}\n🔍 **OpenCodeReview** found **2** issue(s) in this PR.\n\n${CARRY}`;
  const stickyComment = () => [{ id: 42, html_url: "http://ex/42", user: { login: "github-actions[bot]" }, body: previous }];

  const gh = makeGithub({ existingSummary: stickyComment() });
  const outputs = {};
  await runPostReviewComments(
    Object.assign(
      {
        github: gh,
        context,
        core: { setOutput: (k, v) => { outputs[k] = v; } },
        fs: mockFs(JSON.stringify({ comments: [], manifest: ckManifest() }), ""),
      },
      ckRunOptions({ checkpointCarry: CARRY, checkpointNoop: true })
    )
  );
  assert.deepStrictEqual(gh.updatedComments, [], "a no-op rerun must not rewrite the summary");
  assert.deepStrictEqual(gh.issueComments, [], "and must not post a second one");
  assert.strictEqual(outputs.checkpoint_after, "", "nothing was reviewed, so nothing advanced");

  // The same run without the flag DOES rewrite it, so the assertions above pin
  // the no-op and not a path that never writes.
  const rewriting = makeGithub({ existingSummary: stickyComment() });
  await runPostReviewComments(
    Object.assign(
      {
        github: rewriting,
        context,
        core: { setOutput() {} },
        fs: mockFs(JSON.stringify({ comments: [], manifest: ckManifest() }), ""),
      },
      ckRunOptions({ checkpointCarry: CARRY })
    )
  );
  assert.strictEqual(rewriting.updatedComments.length, 1);
  assert.strictEqual(rewriting.updatedComments[0].body.includes("No comments generated"), true);
}

// U7. Reopening a PR, or marking a draft ready, is a request for a fresh look
// at the whole diff — not for the delta since the last push — even when a
// perfectly valid checkpoint is sitting there.
async function testCheckpointEventFullScopeTable() {
  const rows = [
    ["reopened", "full", "event_full_scope"],
    ["ready_for_review", "full", "event_full_scope"],
    ["synchronize", "checkpoint", "ok"],
    ["opened", "checkpoint", "ok"],
    ["", "checkpoint", "ok"],
  ];
  for (const [eventAction, mode, reason] of rows) {
    const r = await resolveCheckpointRange(ckArgs({ eventAction }));
    assert.strictEqual(r.mode, mode, `event ${JSON.stringify(eventAction)} -> mode ${mode}`);
    assert.strictEqual(r.reason, reason, `event ${JSON.stringify(eventAction)} -> reason ${reason}`);
    if (mode === "full") assert.strictEqual(r.from, "", "a full-scope event must not narrow the range");
  }
}

// U9. The carry can come back empty for reasons that say nothing about the
// marker's usefulness (author unprovable, listComments down). A run that then
// completes without advancing must not blank a checkpoint it merely failed to
// read.
async function testCheckpointCarryNeverErasesExistingMarker() {
  const planted = `${SUMMARY_MARKER}\nan older summary\n\n${CARRY}`;
  const sticky = () => [{ id: 7, html_url: "http://ex/7", user: { login: "github-actions[bot]" }, body: planted }];
  const nonAdvancing = {
    "findings path (anchor + finalize)": {
      comments: [{ path: "src/a.js", content: "c1", start_line: 1, end_line: 1 }],
      manifest: ckManifest({ terminal_state: "partial" }),
    },
    "zero-findings path (postSummary)": { comments: [], manifest: ckManifest({ terminal_state: "partial" }) },
  };
  for (const [label, result] of Object.entries(nonAdvancing)) {
    const gh = makeGithub({ existingSummary: sticky() });
    await runPostReviewComments(
      Object.assign(
        { github: gh, context, core: { setOutput() {} }, fs: mockFs(JSON.stringify(result), "") },
        ckRunOptions({ checkpointCarry: "" })
      )
    );
    const body = lastSummaryBody(gh);
    assert.strictEqual(body.split(CARRY).length, 2, `${label}: the existing marker must survive exactly once`);
  }

  // An advancing run still replaces it rather than keeping both.
  const advancing = makeGithub({ existingSummary: sticky() });
  await runPostReviewComments(
    Object.assign(
      {
        github: advancing,
        context,
        core: { setOutput() {} },
        fs: mockFs(JSON.stringify({ comments: [], manifest: ckManifest() }), ""),
      },
      ckRunOptions({ checkpointCarry: "" })
    )
  );
  const advanced = lastSummaryBody(advancing);
  assert.strictEqual(advanced.includes(CARRY), false, "advancing supersedes the old marker");
  assert.strictEqual(parseCheckpointMarker(advanced).head, CK_RESOLVED);

  // The rescue itself, at the unit: it only ever fills a gap.
  const marker = buildCheckpointMarker(ckPayload({ head: CK_MID }));
  assert.strictEqual(preserveCheckpointMarker("fresh body", ""), "fresh body", "nothing to preserve");
  assert.strictEqual(preserveCheckpointMarker("fresh body", `old\n\n${marker}`), `fresh body\n\n${marker}`);
  assert.strictEqual(
    preserveCheckpointMarker(`fresh\n\n${CARRY}`, `old\n\n${marker}`),
    `fresh\n\n${CARRY}`,
    "a body that already carries a marker is never given a second one"
  );

  // With checkpointing off, today's body is written unchanged: no rescue.
  const optedOut = makeGithub({ existingSummary: sticky() });
  await runPostReviewComments({
    github: optedOut,
    context,
    core: { setOutput() {} },
    fs: mockFs(JSON.stringify({ comments: [], manifest: ckManifest({ terminal_state: "partial" }) }), ""),
  });
  assert.strictEqual(lastSummaryBody(optedOut).includes("ocr-checkpoint"), false);
}

// U9 (continued). parseCheckpointMarker rejects a body carrying two markers as
// ambiguous. The rescue must apply the same rule: copying one of the two into
// the rewritten body would pick a winner and hand the next run a single
// well-formed marker to narrow on. Rescuing nothing keeps the fail-closed
// answer.
async function testCheckpointRescueSkipsAmbiguousMarkers() {
  const other = buildCheckpointMarker(ckPayload({ head: CK_MID, run: "other" }));
  assert.strictEqual(parseCheckpointMarker(`old\n\n${CARRY}\n\n${other}`), null, "two markers are unreadable");

  // At the unit.
  assert.strictEqual(
    preserveCheckpointMarker("fresh body", `old\n\n${CARRY}\n\n${other}`),
    "fresh body",
    "an ambiguous body is not resolved by picking one of its markers"
  );

  // And through the posting path: a non-advancing run over an ambiguous sticky
  // summary leaves no marker behind, so the next run still reviews everything.
  const gh = makeGithub({
    existingSummary: [
      {
        id: 9,
        html_url: "http://ex/9",
        user: { login: "github-actions[bot]" },
        body: `${SUMMARY_MARKER}\nolder summary\n\n${CARRY}\n\n${other}`,
      },
    ],
  });
  await runPostReviewComments(
    Object.assign(
      {
        github: gh,
        context,
        core: { setOutput() {} },
        fs: mockFs(JSON.stringify({ comments: [], manifest: ckManifest({ terminal_state: "partial" }) }), ""),
      },
      ckRunOptions({ checkpointCarry: "" })
    )
  );
  assert.strictEqual(lastSummaryBody(gh).includes("ocr-checkpoint"), false, "neither marker is carried forward");
}

// The resolve step's failure path publishes an empty fingerprint (it fell over
// before computing one). validateCheckpointPayload rejects a marker without a
// fingerprint, so stamping one would trade a usable checkpoint for a dead one.
async function testCheckpointAdvanceRequiresAFingerprint() {
  const sticky = () => [
    { id: 11, html_url: "http://ex/11", user: { login: "github-actions[bot]" }, body: `${SUMMARY_MARKER}\nolder\n\n${CARRY}` },
  ];
  const run = async (over) => {
    const gh = makeGithub({ existingSummary: sticky() });
    const outputs = {};
    await runPostReviewComments(
      Object.assign(
        {
          github: gh,
          context,
          core: { setOutput: (k, v) => { outputs[k] = v; } },
          fs: mockFs(JSON.stringify({ comments: [], manifest: ckManifest() }), ""),
        },
        ckRunOptions(over)
      )
    );
    return { body: lastSummaryBody(gh), outputs };
  };

  const blind = await run({ checkpointFingerprint: "", checkpointCarry: "" });
  assert.strictEqual(blind.outputs.checkpoint_after, "", "a run with no fingerprint does not advance");
  assert.strictEqual(blind.body.includes(CARRY), true, "and keeps the checkpoint it could not replace");
  assert.strictEqual(validateCheckpointPayload(parseCheckpointMarker(blind.body), { prNumber: 123 }), null);

  // Control: the same run with a fingerprint advances, so the assertion above
  // pins the guard and not a path that never advances.
  const control = await run({ checkpointCarry: "" });
  assert.strictEqual(control.outputs.checkpoint_after, CK_RESOLVED);
  assert.strictEqual(parseCheckpointMarker(control.body).head, CK_RESOLVED);
}

// U12. A summary that reports three findings for a two-commit slice of a
// forty-commit PR is misleading unless it says so. One visible line, only when
// the range really was narrowed.
async function testSummaryShowsNarrowedRangeLabel() {
  const withRange = async (over, result) => {
    const gh = makeGithub({});
    await runPostReviewComments(
      Object.assign(
        { github: gh, context, core: { setOutput() {} }, fs: mockFs(JSON.stringify(result), "") },
        ckRunOptions(over)
      )
    );
    return lastSummaryBody(gh);
  };
  const findings = {
    comments: [{ path: "src/a.js", content: "c1", start_line: 1, end_line: 1 }],
    manifest: ckManifest(),
  };
  const label = `Reviewed \`${CK_OLD.slice(0, 7)}..${CK_NEW.slice(0, 7)}\``;

  const narrowed = await withRange({ rangeMode: "checkpoint", rangeFrom: CK_OLD, rangeTo: CK_NEW }, findings);
  assert.strictEqual(narrowed.includes(label), true, "a narrowed run must name the range it reviewed");
  assert.strictEqual(narrowed.includes("reviewed in a previous run"), true);
  // Visible to a reader: not hidden inside an HTML comment. Scanned with
  // indexOf, deliberately without a regex — HTML closes a comment on "--!>" as
  // well as on "-->", and a pattern that knows only the common one silently
  // reports a body as safe when it is not. An unterminated "<!--" hides
  // everything after it, so running off the end counts as inside.
  const insideComment = (text, at) => {
    let i = 0;
    while (i <= at) {
      const open = text.indexOf("<!--", i);
      if (open === -1 || open > at) return false;
      const ends = ["-->", "--!>"].map((t) => text.indexOf(t, open + 4)).filter((e) => e !== -1);
      const close = ends.length === 0 ? text.length : Math.min(...ends);
      if (at < close) return true;
      i = close + 1;
    }
    return false;
  };
  assert.strictEqual(
    insideComment(narrowed, narrowed.indexOf(label)),
    false,
    "the label must be outside the HTML comments"
  );
  // Also on the zero-findings path, which is where a narrowed range is most
  // likely to be mistaken for "the whole PR is clean".
  const clean = await withRange({ rangeMode: "checkpoint", rangeFrom: CK_OLD, rangeTo: CK_NEW }, {
    comments: [],
    manifest: ckManifest(),
  });
  assert.strictEqual(clean.includes(label), true);

  const full = await withRange({ rangeMode: "full", rangeFrom: "", rangeTo: CK_NEW }, findings);
  assert.strictEqual(full.includes("Reviewed `"), false, "a full run must not claim a narrowed range");
  const empty = await withRange({ rangeMode: "checkpoint", rangeFrom: CK_NEW, rangeTo: CK_NEW }, findings);
  assert.strictEqual(empty.includes("Reviewed `"), false, "an empty range has nothing to label");
  const off = await withRange({ checkpointEnabled: false, rangeMode: "checkpoint", rangeFrom: CK_OLD, rangeTo: CK_NEW }, findings);
  assert.strictEqual(off.includes("Reviewed `"), false, "opt-out runs render today's body");
}

// U2. The resolve step only chooses where the review starts; every failure it
// can hit has the same safe answer. It must never be the reason a job fails.
function testActionResolveStepNeverFailsTheJob() {
  const block = actionStepBlock("Resolve review range");
  const tryAt = block.indexOf("\n          try {\n");
  assert.notStrictEqual(tryAt, -1, "the resolve script body must run inside a try");
  // The outer catch, at the script's own indentation — the rule-file reads have
  // their own nested ones.
  const catchAt = block.indexOf("\n          } catch (e) {");
  assert.notStrictEqual(catchAt, -1, "…with a catch around the whole body");
  for (const inside of [
    "Could not locate ${REL}",
    "require(helper)",
    "readCheckpointComment(common)",
    "resolveCheckpointRange(",
  ]) {
    const at = block.indexOf(inside);
    assert.notStrictEqual(at, -1, `${inside} must still be part of the step`);
    assert.strictEqual(at > tryAt && at < catchAt, true, `${inside} must sit inside the try block`);
  }
  const failurePath = block.slice(catchAt);
  assert.strictEqual(failurePath.includes("core.warning("), true, "the failure path warns instead of throwing");
  assert.strictEqual(failurePath.includes("mode: 'full'"), true, "…and falls back to the full range");
  assert.strictEqual(failurePath.includes("reason: 'resolver_error'"), true);
  assert.strictEqual(/core\.setFailed|throw /.test(failurePath), false, "the failure path must not fail the job");
  assert.strictEqual(block.indexOf("throw new Error") > tryAt, true, "no throw may escape the try");
}

// The list the fingerprint is hashed over, which is what decides when a
// checkpoint stops being comparable.
function fingerprintDigestSource() {
  const block = actionStepBlock("Resolve review range");
  const at = block.indexOf(".update(JSON.stringify([");
  assert.notStrictEqual(at, -1, "the fingerprint digest input must still be readable");
  const end = block.indexOf("].map(", at);
  assert.notStrictEqual(end, -1, "the fingerprint digest input must be a closed list");
  return block.slice(at, end);
}

// U3. .opencodereview/rule.json is read from the repo whether or not `rule` is
// set, so a commit that edits it changes what a review says and must invalidate
// the checkpoint.
function testActionFingerprintsRepoLocalRuleFile() {
  const block = actionStepBlock("Resolve review range");
  assert.strictEqual(block.includes("'.opencodereview/rule.json'"), true, "the repo-local rule file is looked up");
  assert.strictEqual(
    fingerprintDigestSource().includes("localRuleDigest"),
    true,
    "the repo-local rule digest must be part of the fingerprint"
  );
  // Absent file -> 'none'; unreadable file -> the existing fail-closed path.
  assert.strictEqual(block.includes("let localRuleDigest = 'none';"), true);
  const localAt = block.indexOf("const localRulePath");
  assert.strictEqual(block.slice(localAt).includes("ruleUnverified = true;"), true, "unreadable means unverified");
  assert.strictEqual(block.includes("range.reason = 'rule_unreadable';"), true, "…which forces a full review");
}

// U4. `background` changes what the model is told, so it changes what a review
// would say, so it belongs in the fingerprint.
function testActionFingerprintIncludesBackground() {
  const block = actionStepBlock("Resolve review range");
  const digest = fingerprintDigestSource();
  // Both halves, per axis: passed in as its own variable, and read by the
  // digest. Either one alone is the silent failure — an axis that looks covered
  // and quietly stops invalidating checkpoints.
  for (const [envVar, input] of [
    ["OCR_FP_LLM_URL", "llm_url"],
    ["OCR_FP_LLM_MODEL", "llm_model"],
    ["OCR_FP_LLM_USE_ANTHROPIC", "llm_use_anthropic"],
    ["OCR_FP_LANGUAGE", "language"],
    ["OCR_FP_LLM_EXTRA_BODY", "llm_extra_body"],
    ["OCR_FP_RULE", "rule"],
    ["OCR_FP_ROUTE_SEVERITY_BELOW", "route_severity_below"],
    ["OCR_FP_ROUTE_CATEGORIES", "route_categories"],
    ["OCR_FP_BACKGROUND", "background"],
  ]) {
    assert.strictEqual(
      block.includes(`${envVar}: \${{ inputs.${input} }}`),
      true,
      `the step must pass inputs.${input} as ${envVar}`
    );
    assert.strictEqual(
      digest.includes(`process.env.${envVar},`),
      true,
      `${envVar} must be hashed into the fingerprint`
    );
  }
}

// U5. A narrowed range published through $GITHUB_ENV outlives the step: a
// second use of this action in the same job would inherit it and skip commits
// it was never told about. Step outputs are scoped to the step that set them.
function testActionRangeFromIsAStepOutput() {
  assert.strictEqual(/exportVariable\(\s*'RANGE_FROM'/.test(ACTION_YML), false, "RANGE_FROM must not be job env");
  assert.strictEqual(/core\.exportVariable\(/.test(ACTION_YML), false, "nothing about the range may leak via $GITHUB_ENV");

  const resolve = actionStepBlock("Resolve review range");
  assert.strictEqual(resolve.includes("core.setOutput('range_from'"), true);

  const review = actionStepBlock("Run OpenCodeReview");
  assert.strictEqual(review.includes("RANGE_FROM: ${{ steps.range.outputs.range_from }}"), true);
  assert.strictEqual(review.includes('--from "${RANGE_FROM:-$MERGE_BASE}"'), true, "empty output still means merge-base");

  const post = actionStepBlock("Post review comments");
  for (const name of ["checkpoint_carry", "config_fingerprint", "range_mode", "range_from", "range_to", "range_reason"]) {
    assert.strictEqual(post.includes(`steps.range.outputs.${name}`), true, `the post step reads ${name} as a step output`);
  }
  assert.strictEqual(post.includes("same_head_noop"), true, "…including the no-op signal");
}

// U8. "Why was this a full review" has to be answerable by a workflow, not just
// by a human reading the step log.
function testActionEmitsMachineReadableRangeOutputs() {
  const names = [
    "range_from",
    "range_to",
    "range_mode",
    "range_reason",
    "range_summary",
    "checkpoint_before",
    "ancestry",
    "source_run",
  ];
  const resolve = actionStepBlock("Resolve review range");
  const outputsBlock = ACTION_YML.slice(ACTION_YML.indexOf("\noutputs:"), ACTION_YML.indexOf("\nruns:"));
  for (const name of names) {
    assert.strictEqual(resolve.includes(`core.setOutput('${name}'`), true, `the resolve step emits ${name}`);
    assert.strictEqual(outputsBlock.includes(`\n  ${name}:`), true, `action.yml declares the ${name} output`);
    assert.strictEqual(
      outputsBlock.includes(`steps.range.outputs.${name} }}`),
      true,
      `${name} is mapped to the resolve step`
    );
    assert.strictEqual(ACTION_README.includes(`\`${name}\``), true, `the README documents ${name}`);
  }
  // checkpoint_after comes from the posting step, where it is recorded.
  assert.strictEqual(outputsBlock.includes("steps.post.outputs.checkpoint_after }}"), true);
  assert.strictEqual(ACTION_README.includes("`checkpoint_after`"), true);
}

// U10. A floating tag on a step that runs with the repo's token is a supply
// chain hole, and this file already pins every other action by sha.
function testActionPinsGithubScriptSha() {
  assert.strictEqual(/actions\/github-script@v/.test(ACTION_YML), false, "no floating github-script tag may remain");
  const uses = [...ACTION_YML.matchAll(/uses: actions\/github-script@([0-9a-f]{40})/g)].map((m) => m[1]);
  assert.strictEqual(uses.length, 2, "both github-script steps are pinned");
  assert.strictEqual(new Set(uses).size, 1, "…to the same sha");
}

// U11. `ocr_version` defaults to "latest", so falling back to `spec:${VERSION}`
// pinned the axis to the constant "spec:latest" — it stopped distinguishing
// versions in exactly the case it was written to cover. Nothing replaces it:
// an unresolvable version produces an EMPTY fingerprint, which matches no
// stored one (full review) and blocks the write (no advance).
async function testActionUnresolvedVersionEmptiesTheFingerprint() {
  const install = actionStepBlock("Install OpenCodeReview");
  assert.strictEqual(install.includes("spec:${OCR_VERSION}"), false, "no constant may stand in for a version");
  assert.strictEqual(install.includes('echo "OCR_VERSION_ACTUAL=${VERSION_ACTUAL}"'), true);

  const core = scriptCore();
  await runActionScript(actionScriptSource("Resolve review range"), {
    github: ckGithub({ comments: [ckComment({ payload: ckPayload() })] }),
    context: { repo: { owner: "owner", repo: "repo" }, issue: { number: 123 } },
    core,
    env: resolveEnv({ OCR_VERSION_ACTUAL: "" }),
  });
  assert.strictEqual(core.outputs.config_fingerprint, "", "an unresolved version fingerprints as nothing");
  assert.strictEqual(core.outputs.range_mode, "full", "…so no stored fingerprint can match it");
  assert.strictEqual(core.outputs.range_reason, "config_changed");
  assert.strictEqual(core.outputs.range_from, "", "empty range_from means ${RANGE_FROM:-$MERGE_BASE}");
  assert.strictEqual(
    core.warnings.some((w) => /ocr version/.test(w)),
    true,
    "and the run says why it went wide"
  );
  // The empty fingerprint also blocks the advance
  // (testCheckpointAdvanceRequiresAFingerprint pins that half).
}

// The gap the review caught: `llm_extra_headers` can point a byte-identical
// `llm_model` at a different backend model, or at a different provider
// altogether, so it changes what a review would say. `llm_auth_header` picks
// the header the credential rides in, and `llm_timeout` shifts which runs
// finish and which are cut short — i.e. the partial/complete distribution the
// checkpoint gates on. All three belong in the fingerprint.
async function testActionFingerprintCoversLlmHeaderAxes() {
  const source = actionScriptSource("Resolve review range");
  const emptyPr = { rest: { issues: { listComments: async () => ({ data: [] }) } } };
  const runWith = async (over) => {
    const core = scriptCore();
    await runActionScript(source, {
      github: emptyPr,
      context: { repo: { owner: "owner", repo: "repo" }, issue: { number: 123 } },
      core,
      env: resolveEnv(over),
    });
    assert.strictEqual(core.outputs.range_reason, "no_summary_comment", "fixture: an empty PR reads no marker");
    return core;
  };

  const base = (await runWith({})).outputs.config_fingerprint;
  for (const [envVar, value] of [
    ["OCR_FP_LLM_EXTRA_HEADERS", "X-Model=gpt-4o"],
    ["OCR_FP_LLM_AUTH_HEADER", "X-Api-Key"],
    ["OCR_FP_LLM_TIMEOUT", "600"],
  ]) {
    const changed = (await runWith({ [envVar]: value })).outputs.config_fingerprint;
    assert.notStrictEqual(changed, base, `a changed ${envVar} must invalidate the checkpoint`);
  }
  // Two different header sets must not collide with each other either.
  const a = (await runWith({ OCR_FP_LLM_EXTRA_HEADERS: "X-Model=a" })).outputs.config_fingerprint;
  const b = (await runWith({ OCR_FP_LLM_EXTRA_HEADERS: "X-Model=b" })).outputs.config_fingerprint;
  assert.notStrictEqual(a, b, "two header sets must not hash alike");

  // Header VALUES carry credentials, so the axis goes in as a digest: the value
  // may reach neither the step log nor anything the step publishes (the
  // fingerprint is what the stored checkpoint carries).
  const secret = "X-Api-Key=sk-live-must-never-be-published";
  const leaky = await runWith({ OCR_FP_LLM_EXTRA_HEADERS: secret });
  const published = JSON.stringify([leaky.outputs, leaky.warnings, leaky.infos]);
  assert.strictEqual(published.includes("sk-live"), false, "the header value must not be published anywhere");
  const digest = fingerprintDigestSource();
  assert.strictEqual(digest.includes("process.env.OCR_FP_LLM_EXTRA_HEADERS"), false, "…so it is not inlined raw");
  assert.strictEqual(digest.includes("extraHeadersDigest"), true, "…it is hashed first, like the rule file contents");
  const resolve = actionStepBlock("Resolve review range");
  for (const input of ["llm_extra_headers", "llm_auth_header", "llm_timeout"]) {
    const envVar = `OCR_FP_${input.toUpperCase()}`;
    assert.strictEqual(resolve.includes(`${envVar}: \${{ inputs.${input} }}`), true, `the step passes inputs.${input}`);
  }
}

// The author check trusted any bot, not this action: `dependabot[bot]`,
// `renovate[bot]` and any linter App all read as ours, so the effective trust
// set was "any bot that can post an issue comment carrying the summary
// marker". The default `github.token` is always the "github-actions" app, so
// when the caller did not override the token the check can be pinned to that
// one app. A caller-supplied token keeps the wider check: an installation
// token cannot ask GitHub which app it is.
async function testActionPinsAuthorToTheDefaultTokenApp() {
  const resolve = actionStepBlock("Resolve review range");
  assert.strictEqual(
    resolve.includes("OCR_CHECKPOINT_APP_SLUG: ${{ inputs.github_token == github.token && 'github-actions' || '' }}"),
    true,
    "the slug is pinned only when the token is the default one"
  );
  assert.strictEqual(
    resolve.includes("appSlug: process.env.OCR_CHECKPOINT_APP_SLUG || ''"),
    true,
    "…and it reaches the reader"
  );

  // The three cases from the review, straight through the predicate.
  const linter = ckComment({ payload: ckPayload(), login: "some-linter" });
  linter.user = { login: "some-linter", type: "Bot" };
  linter.performed_via_github_app = { slug: "some-linter" };
  for (const comment of [
    ckComment({ payload: ckPayload(), login: "dependabot[bot]" }),
    ckComment({ payload: ckPayload(), login: "renovate[bot]" }),
    linter,
  ]) {
    const who = comment.user.login;
    assert.strictEqual(isCheckpointAuthorOurs(comment, "github-actions"), false, `${who} is not this action`);
    assert.strictEqual(isCheckpointAuthorOurs(comment, ""), true, `${who} still passes an unpinned run`);
  }

  // Executed through the action's own script: the same marker is rejected under
  // a foreign bot and gets past the author gate under ours.
  const reasonFor = async (login) => {
    const core = scriptCore();
    await runActionScript(actionScriptSource("Resolve review range"), {
      github: ckGithub({ comments: [ckComment({ payload: ckPayload(), login })] }),
      context: { repo: { owner: "owner", repo: "repo" }, issue: { number: 123 } },
      core,
      env: resolveEnv({ OCR_CHECKPOINT_APP_SLUG: "github-actions" }),
    });
    assert.strictEqual(core.outputs.range_mode, "full");
    return core.outputs.range_reason;
  };
  assert.strictEqual(await reasonFor("dependabot[bot]"), "author_unverified", "another bot's marker is not ours");
  assert.strictEqual(
    await reasonFor("github-actions[bot]"),
    "config_changed",
    "our own app gets past the author gate and is stopped by the next one"
  );
}

// U13. The reason set is documented in three places (the resolver's contract
// comment, the README table, the action outputs) and drifted between them once
// already. Enumerate it once, here, and fail if any copy falls behind.
async function testCheckpointReasonsMatchTheDocs() {
  const failClosed = [
    ["disabled", { enabled: false }],
    ["sticky_disabled", { sticky: false }],
    ["manual_full_review", { fullReview: true }],
    ["event_full_scope", { eventAction: "reopened" }],
    ["no_summary_comment", { github: ckGithub({ comments: [] }) }],
    ["author_unverified", { github: ckGithub({ comments: [ckComment({ payload: ckPayload(), login: "fork-user" })] }) }],
    ["corrupt_checkpoint", { github: ckGithub({ comments: [ckComment({ body: "no marker here" })] }) }],
    ["schema_invalid", { github: ckGithub({ comments: [ckComment({ payload: ckPayload({ v: 2 }) })] }) }],
    ["base_changed", { github: ckGithub({ comments: [ckComment({ payload: ckPayload({ merge_base: CK_MID }) })] }) }],
    ["config_changed", { github: ckGithub({ comments: [ckComment({ payload: ckPayload({ fingerprint: "other" }) })] }) }],
    ["not_ancestor", { isAncestor: async () => 1 }],
    ["unknown_object", { isAncestor: async () => 128 }],
    ["resolver_error", { github: ckGithub({ listThrows: true }) }],
  ];
  const narrowing = [
    ["ok", {}],
    ["same_head_noop", { github: ckGithub({ comments: [ckComment({ payload: ckPayload({ head: CK_NEW }) })] }) }],
  ];
  const seen = new Set();
  for (const [reason, over] of failClosed) {
    const r = await resolveCheckpointRange(ckArgs(over));
    assert.strictEqual(r.reason, reason, `expected reason ${reason}, got ${r.reason}`);
    assert.strictEqual(r.mode, "full", `${reason} must review the full range`);
    seen.add(reason);
  }
  for (const [reason, over] of narrowing) {
    const r = await resolveCheckpointRange(ckArgs(over));
    assert.strictEqual(r.reason, reason);
    assert.strictEqual(r.mode, "checkpoint");
    seen.add(reason);
  }
  assert.strictEqual(seen.size, failClosed.length + narrowing.length, "every reason has its own input class");
  assert.strictEqual(failClosed.length, 13, "thirteen fail-closed reasons");

  // The resolver's own contract comment must enumerate them, with a count that
  // matches what it lists.
  const contract = HELPER_SOURCE.slice(
    HELPER_SOURCE.indexOf("// The ordered gate."),
    HELPER_SOURCE.indexOf("async function resolveCheckpointRange")
  );
  assert.notStrictEqual(contract, "", "the resolver contract comment must exist");
  assert.strictEqual(contract.includes("thirteen fail-closed"), true, "the documented count must match the list");
  for (const [reason] of failClosed.concat(narrowing)) {
    assert.strictEqual(contract.includes(reason), true, `the contract comment lists ${reason}`);
  }

  // …as must the README table, plus rule_unreadable, which only the action's
  // last gate can produce.
  const rows = new Set([...ACTION_README.matchAll(/^\| `([a-z_]+)` \|/gm)].map((m) => m[1]));
  for (const [reason] of failClosed) {
    assert.strictEqual(rows.has(reason), true, `the README reason table documents ${reason}`);
  }
  assert.strictEqual(rows.has("rule_unreadable"), true, "…including the action's own last gate");
  // The two narrowing reasons are not table rows (the table says why a run went
  // wide), so they are documented in prose instead.
  assert.strictEqual(ACTION_README.includes("`same_head_noop`"), true, "the README explains the no-op rerun");
  assert.strictEqual(ACTION_README.includes("checkpoint (ok)"), true, "…and shows what a narrowed run reports");
  // Force-push lands on unknown_object (the orphaned commit is not in this
  // shallow clone), not on not_ancestor.
  const forcePush = /\| `unknown_object` \|([^\n]*)/.exec(ACTION_README);
  assert.notStrictEqual(forcePush, null);
  assert.strictEqual(/force-push/.test(forcePush[1]), true, "unknown_object is where a force-push actually lands");
}

// The resolve step's git refs come from $GITHUB_ENV, written by two earlier
// steps. Reading them straight off the ambient job env made that dependency
// invisible: nothing in the step said it needed those steps to have run. Pin
// both halves — the declaration, and what happens when the declaration is empty
// because an upstream step was skipped.
async function testActionResolveStepDeclaresItsRefInputs() {
  const block = actionStepBlock("Resolve review range");
  for (const [envVar, source] of [
    ["OCR_HEAD_SHA", "env.HEAD_SHA"],
    ["OCR_BASE_REF", "env.BASE_REF"],
    ["OCR_MERGE_BASE", "env.MERGE_BASE"],
  ]) {
    assert.strictEqual(
      block.includes(`${envVar}: \${{ ${source} }}`),
      true,
      `the step must declare ${envVar} instead of reading the job env implicitly`
    );
    assert.strictEqual(block.includes(`process.env.${envVar}`), true, `…and read it as ${envVar}`);
  }
  for (const bare of ["process.env.HEAD_SHA", "process.env.BASE_REF", "process.env.MERGE_BASE"]) {
    assert.strictEqual(block.includes(bare), false, `no undeclared ${bare} may remain`);
  }

  // Upstream skipped: all three arrive empty. The resolver must still go wide
  // rather than narrow to a half-known range, and must not throw.
  const core = scriptCore();
  await runActionScript(actionScriptSource("Resolve review range"), {
    github: ckGithub({ comments: [ckComment({ payload: ckPayload() })] }),
    context: { repo: { owner: "owner", repo: "repo" }, issue: { number: 123 } },
    core,
    env: resolveEnv({ OCR_HEAD_SHA: "", OCR_BASE_REF: "", OCR_MERGE_BASE: "" }),
  });
  assert.strictEqual(core.outputs.range_mode, "full", "an empty ref set never narrows");
  assert.strictEqual(core.outputs.range_reason, "base_changed", "…because the stored base cannot match an empty one");
  assert.strictEqual(core.outputs.range_from, "", "empty range_from means ${RANGE_FROM:-$MERGE_BASE}");
  assert.strictEqual(core.warnings.length, 0, "and it is a decision, not a crash");
}

// The flagless marker RegExp is shared across calls now. That is only safe
// because a RegExp without `g` keeps no lastIndex; sharing a global one would
// make the second call on an identical body behave differently from the first.
// Both shared uses are exercised twice here, which is what a `g` flag on the
// shared instance would break.
async function testCheckpointMarkerMatchingIsStateless() {
  const marker = buildCheckpointMarker(ckPayload({ head: CK_MID }));
  const withMarker = `${SUMMARY_MARKER}\nprose\n\n${marker}`;

  // preserveCheckpointMarker: the new body already carries a marker, so the old
  // one is never appended — on every call, not just the first.
  for (const pass of [1, 2]) {
    assert.strictEqual(
      preserveCheckpointMarker(withMarker, `${SUMMARY_MARKER}\nold\n\n${buildCheckpointMarker(ckPayload())}`),
      withMarker,
      `pass ${pass}: a body that already has a marker is returned untouched`
    );
  }

  // readCheckpointComment: the raw marker it carries forward must be the marker,
  // byte for byte, on every read of the same body.
  const read = async () =>
    readCheckpointComment({
      github: ckGithub({ comments: [ckComment({ payload: ckPayload({ head: CK_MID }) })] }),
      owner: "owner",
      repo: "repo",
      prNumber: 123,
      log: () => {},
    });
  const first = await read();
  const second = await read();
  assert.strictEqual(first.raw, marker, "the carried marker is the marker itself");
  assert.deepStrictEqual(second, first, "a second read of the same body reads the same thing");
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
