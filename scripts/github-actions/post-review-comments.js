// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

"use strict";

// OpenCodeReview PR review comment poster.
//
// Extracted from the inline actions/github-script step that used to live in
// examples/github_actions/ocr-review.yml and .github/workflows/ocr-review.yml,
// so that the reusable composite action (action/action.yml) and the in-repo
// workflows share a single source of truth.
//
// Dependencies are injected by the caller (actions/github-script provides
// `github`/`context`/`core`; `fs` is required by the caller). The module has
// no external (npm) requires — only the Node.js built-in `crypto` — which
// keeps it runnable inside actions/github-script without bundling.

const crypto = require("crypto");

const SUMMARY_MARKER = "<!-- ocr-summary -->";

// Reason attached to comments that have no valid line range and therefore can
// never be posted as inline comments. Surfaced in the summary via the same
// `⚠️ GitHub could not post this as an inline comment: <reason>` line as
// posting failures, so every summary-only comment explains why it is here.
const NO_LINE_REASON = "No line information provided";

// Default IoU threshold for the incremental multi-line overlap test. Two
// multi-line comments are considered the same when their line-range
// intersection-over-union exceeds this value. Exposed for tuning via the
// incrementalOverlapThreshold option / incremental_overlap_threshold input.
const DEFAULT_OVERLAP_THRESHOLD = 0.6;

// Default maximum number of inline comments packed into a single createReview
// call. Production once failed at 71 comments in one request (GitHub Server
// Error after partial success); 50 stays at GitHub's documented soft guidance
// for inline comments per review while keeping typical (sub-50) runs on a
// single batch. Tunable via the reviewCommentBatchSize option /
// review_comment_batch_size input.
const DEFAULT_BATCH_SIZE = 50;

// Enumerations for category/severity routing, sourced from the LLM output
// schema (internal/config/toolsconfig/tools.json:55-84). Used both to validate
// the routing policy and to normalize the metadata before comparison. Kept as
// plain arrays (not Sets) so tests can inspect ordering for severity ranking.
const CATEGORIES = [
  "bug",
  "security",
  "performance",
  "maintainability",
  "test",
  "style",
  "documentation",
  "other",
];
// Severity rank: higher = more severe. An unknown/empty severity has no rank
// (never matched by the routing policy). Order matches the enum (critical is
// the most severe, low the least).
const SEVERITIES = ["critical", "high", "medium", "low"];
const SEVERITY_RANK = new Map(
  SEVERITIES.map((s, i) => [s, SEVERITIES.length - i])
); // critical=4, high=3, medium=2, low=1

// Sentinel policy object: "do not route anything". Returned by buildPolicy on
// any parse problem so the partition loop falls open to today's behavior (I1).
// Equivalently produced by an empty policy (no threshold, no categories).
const NO_ROUTING = Object.freeze({ routeBySeverity: false, routeByCategory: false });

async function runPostReviewComments({
  github,
  context,
  core,
  fs,
  resultPath = "/tmp/ocr-result.json",
  stderrPath = "/tmp/ocr-stderr.log",
  stickySummary = true,
  incremental = false,
  incrementalOverlapThreshold = DEFAULT_OVERLAP_THRESHOLD,
  reviewCommentBatchSize = DEFAULT_BATCH_SIZE,
  approveOnClean = true,
  // Fail-open finding-publication controls (#478). Both optional and empty by
  // default: with neither set, behavior is byte-identical to today (modulo the
  // additive badge prefix on rendered comments). buildPolicy parses them once
  // before the partition loop and degrades to NO_ROUTING on any malformed value
  // (fail-open for the policy itself, upholding I1).
  routeSeverityBelow = "",
  routeCategories = "",
}) {
  const log = (msg) => {
    if (core && typeof core.info === "function") core.info(msg);
    else console.log(msg);
  };
  const out = (name, value) => {
    if (core && typeof core.setOutput === "function") core.setOutput(name, value);
  };

  const owner = context.repo.owner;
  const repo = context.repo.repo;
  const prNumber = context.issue.number;

  // Per-run idempotency tags. context.runId / context.runAttempt come from
  // @actions/github's Context (parsed from GITHUB_RUN_ID / GITHUB_RUN_ATTEMPT).
  // Number.isFinite guards against NaN when the env vars are missing, falling
  // back to safe defaults. The tags are embedded in review/comment bodies as
  // HTML comments so the idempotency check can detect whether a batch
  // createReview actually landed on the server before retrying, which prevents
  // duplicate review posts on retry.
  const { RUN_TAG, REVIEW_TAG, SUMMARY_TAG } = buildRunTags(context.runId, context.runAttempt);

  const stats = {
    total: 0,
    inline: 0,
    skipped: 0,
    failed: 0,
    routed: 0,
    summaryUrl: "",
  };

  // Read OCR output.
  let result;
  try {
    const raw = fs.readFileSync(resultPath, "utf8");
    result = JSON.parse(raw);
  } catch (e) {
    log(`Failed to parse OCR output: ${e.message}`);
    const stderr = safeRead(fs, stderrPath).trim();
    if (stderr) {
      const body = `${SUMMARY_MARKER}\n⚠️ **OpenCodeReview** encountered an error:\n${fencedBlock(stderr)}`;
      const posted = await postSummary({ github, owner, repo, prNumber, body, sticky: stickySummary, log });
      stats.summaryUrl = posted.url;
    }
    setStatsOutputs(out, stats);
    return;
  }

  const comments = result.comments || [];
  const warnings = result.warnings || [];
  stats.total = comments.length;

  if (comments.length === 0) {
    if (approveOnClean && result.status === "complete") {
      const approved = await submitCleanApproval({
        github,
        context,
        owner,
        repo,
        prNumber,
        message: result.message || "No comments generated. Looks good to me.",
        log,
      });
      if (approved) stats.summaryUrl = approved.html_url;
      setStatsOutputs(out, stats);
      return;
    }
    const message = result.message || "No comments generated. Looks good to me.";
    const body = `${SUMMARY_MARKER}\n✅ **OpenCodeReview**: ${message}`;
    const posted = await postSummary({ github, owner, repo, prNumber, body, sticky: stickySummary, log });
    stats.summaryUrl = posted.url;
    setStatsOutputs(out, stats);
    return;
  }

  // Resolve the PR head commit sha to attach the review to.
  let commitSha;
  if (context.eventName === "pull_request_target") {
    commitSha = context.payload.pull_request.head.sha;
  } else {
    const { data: pullRequest } = await github.rest.pulls.get({
      owner,
      repo,
      pull_number: prNumber,
    });
    commitSha = pullRequest.head.sha;
  }

  // Partition: inline (with valid line info) vs summary (without) vs routed
  // (valid line but the publication policy moves it to the summary).
  // Each inline comment gets a random per-comment ID (assigned once) embedded
  // in its body as an HTML comment, so the retry/idempotency logic can detect
  // whether a comment already landed on the server and avoid posting a
  // duplicate. Random (not content-derived) so two distinct comments that
  // share path/line/content still get different IDs.
  //
  // Routing is a placement decision in this loop, not a post-hoc filter: a
  // finding the policy routes to summary is pushed to commentsRouted (mirroring
  // commentsWithoutLine) instead of reviewComments. Routed findings therefore
  // never enter reviewComments -> never enter toSend/toRetry -> never reach a
  // createReview call (I4: no double-post surface on retry).
  const policy = buildPolicy({ severityThreshold: routeSeverityBelow, categories: routeCategories });
  const reviewComments = [];
  const commentsWithoutLine = [];
  const commentsRouted = [];
  for (const comment of comments) {
    const hasValidLine = comment.start_line >= 1 || comment.end_line >= 1;
    if (!hasValidLine) {
      commentsWithoutLine.push({ comment, body: formatComment(comment), reason: NO_LINE_REASON });
      continue;
    }
    // Routing applies only to findings that COULD be posted inline (valid
    // line). No-line findings already go to the summary via commentsWithoutLine,
    // so they are never re-routed (avoids double-counting in the summary).
    const route = routeComment(comment, policy);
    if (route.routed) {
      commentsRouted.push({ comment, body: formatComment(comment), reason: route.reason });
      continue;
    }
    const id = newCommentId(RUN_TAG);
    const reviewComment = { path: comment.path, body: formatComment(comment, id) };
    if (comment.start_line >= 1 && comment.end_line >= 1 && comment.start_line !== comment.end_line) {
      reviewComment.start_line = comment.start_line;
      reviewComment.line = comment.end_line;
      reviewComment.start_side = "RIGHT";
      reviewComment.side = "RIGHT";
    } else if (comment.end_line >= 1) {
      reviewComment.line = comment.end_line;
      reviewComment.side = "RIGHT";
    } else if (comment.start_line >= 1) {
      reviewComment.line = comment.start_line;
      reviewComment.side = "RIGHT";
    }
    reviewComments.push({ comment, reviewComment, id });
  }

  // Incremental filtering (non-destructive): drop current inline comments
  // whose (path, line range) overlaps an existing bot review comment, so we
  // only append comments on lines not yet covered. History is never deleted.
  let toSend = reviewComments;
  if (incremental && reviewComments.length > 0) {
    const existing = await listExistingReviewComments(github, owner, repo, prNumber, log);
    const botLogin = await getAuthenticatedLogin(github, log);
    const hist = existing.filter((c) => isBotComment(c, botLogin));
    toSend = reviewComments.filter(
      ({ reviewComment }) => !overlapsHistory(reviewComment, hist, incrementalOverlapThreshold)
    );
    stats.skipped = reviewComments.length - toSend.length;
    if (stats.skipped > 0) {
      log(`[incremental] skipped ${stats.skipped} overlapping comment(s); ${toSend.length} to post.`);
    }
  }

  // ---- Summary anchor (before the review) ----
  // Create the summary issue comment BEFORE posting the review so that on a
  // cold start (the first review on this PR) the summary's timeline position is
  // above the review. GitHub orders issue comments oldest-first, so creating
  // the summary first pins it at the top; later runs merely update it in place
  // (sticky) or post a fresh per-run comment (non-sticky), so the ordering
  // stays stable and the summary is never sandwiched between review blocks.
  // The anchor carries a pre-review body (issue count, warnings, and comments
  // without line info — all known upfront); final posting statistics are
  // written in the finalize phase below, once the review has landed.
  const wrapSummary = (content) => `${SUMMARY_MARKER}\n${SUMMARY_TAG}\n${content}`;
  const anchor = await ensureSummaryAnchor({
    github,
    owner,
    repo,
    prNumber,
    sticky: stickySummary,
    tag: SUMMARY_TAG,
    body: wrapSummary(
      buildPreReviewSummaryBody(stats.total, commentsWithoutLine, commentsRouted, warnings)
    ),
    log,
  });

  // Submit inline comments (the to-send set) as one or more PR reviews.
  let successCount = 0;
  let failedCount = 0;
  const failedComments = [];

  // Sort before partitioning so identical reruns produce identical batches
  // (B2/AS4): a partial-success retry reproduces the same partition, which is
  // what makes per-batch reconciliation against already-posted fence IDs work.
  const batchSize = resolveBatchSize(reviewCommentBatchSize);
  const sorted = sortToSendDeterministically(toSend);
  const batches = chunkArray(sorted, batchSize);
  const batchCounters = { total: batches.length, attempted: 0, succeeded: 0, reconciled: 0 };

  if (toSend.length > 0) {
    // The summary lives in its own issue comment (anchored above), so the
    // review body carries only the per-run REVIEW_TAG. The tag lets the
    // idempotency check locate the batch review on retry (a batch createReview
    // may have landed on the server even though we received a 5xx response).
    const reviewBody = REVIEW_TAG;

    // Shared across batches so the PR's diff inventory is fetched at most once
    // per run even if several batches trip the 422 line-resolution fallback.
    const diffCache = {};

    for (const chunk of batches) {
      const r = await publishBatch({
        chunk,
        github,
        owner,
        repo,
        prNumber,
        commitSha,
        reviewBody,
        REVIEW_TAG,
        log,
        diffCache,
      });
      successCount += r.succeeded;
      failedCount += r.failed;
      for (const fc of r.failedComments) failedComments.push(fc);
      batchCounters.attempted++;
      if (r.reconciled) batchCounters.reconciled++;
      if (r.succeeded > 0) batchCounters.succeeded++;
    }
  } else {
    log("No inline comments to post after filtering (all overlapping or none had line info).");
  }

  stats.inline = successCount;
  stats.failed = failedCount;
  stats.routed = commentsRouted.length;

  // ---- Finalize the summary with the complete body ----
  // Now that the review has landed (or failed per-comment), write the final
  // summary body. Posting statistics are merged into the leading summary
  // header (see buildSummaryBody), so here we only append the per-comment
  // renderings: every comment that did not go out as inline — whether because
  // it had no line info, was routed by the publication policy, or because
  // posting failed — is rendered as one continuous block, each carrying the
  // reason it ended up in the summary (so the reader always knows why it is
  // here). Routed findings render BEFORE the failed block so the order is:
  // counts → no-line summary → routed summary → failed.
  let summaryBody = buildSummaryBody({
    total: stats.total,
    inline: successCount,
    summary: commentsWithoutLine.length,
    skipped: stats.skipped,
    routed: commentsRouted.length,
    failed: failedCount,
    warnings,
  });
  summaryBody += formatSummaryComments(commentsWithoutLine);
  summaryBody += formatSummaryComments(commentsRouted);
  for (const { comment, error } of failedComments) {
    summaryBody += "\n\n---\n\n";
    summaryBody += formatCommentMarkdown(comment, error);
  }
  if (toSend.length === 0 && stats.skipped > 0) {
    summaryBody += "\n\n---\n\nℹ️ All inline comments overlapped with existing reviews; nothing new was posted.";
  }
  summaryBody += formatWarnings(warnings);

  // Update the anchored comment directly when its id is known (no extra read);
  // otherwise upsert (find-then-update-or-create), which also covers the case
  // where the anchor phase was skipped because the read API was unavailable.
  // Returns null only when the summary could not be written without risking a
  // duplicate; the review content remains available via inline comments.
  const finalized = await finalizeSummary({
    github,
    owner,
    repo,
    prNumber,
    anchor,
    sticky: stickySummary,
    tag: SUMMARY_TAG,
    body: wrapSummary(summaryBody),
    log,
  });
  if (finalized) stats.summaryUrl = finalized.url;

  setStatsOutputs(out, stats, batchCounters, batchSize);
}

// Publish a single bounded batch of inline comments via one createReview call,
// then reconcile + per-comment-retry on failure. This is the per-batch body of
// the previous all-in-one publish block, factored out so it can run once per
// chunk. The reconciliation/idempotency logic is unchanged: the only behavioral
// difference is that it operates on `chunk` (a slice of the sorted toSend) and
// returns its counts/failed-list so the caller can accumulate them across
// batches (B3/B4/B5/B6).
//
// Returns { succeeded, failed, failedComments, reconciled }.
//   - succeeded: comments in this batch that ended up on the server (posted by
//     the batch call, or reconciled-already-posted, or per-comment retry).
//   - failed: comments that could not be posted AND could not be reconciled.
//   - reconciled: true if the batch call failed and at least one of this
//     batch's comments was proven already-posted (idempotency read succeeded).
async function publishBatch({
  chunk,
  github,
  owner,
  repo,
  prNumber,
  commitSha,
  reviewBody,
  REVIEW_TAG,
  log,
  diffCache,
}) {
  let succeeded = 0;
  let failed = 0;
  const failedComments = [];
  let reconciled = false;

  try {
    const batchRes = await github.rest.pulls.createReview({
      owner,
      repo,
      pull_number: prNumber,
      commit_id: commitSha,
      body: reviewBody,
      event: "COMMENT",
      comments: chunk.map(({ reviewComment }) => reviewComment),
    });
    succeeded = chunk.length;
    log(`Successfully posted review batch with ${succeeded} inline comment(s).`);
    logRateLimitQuota(batchRes, "after batch createReview", log);
  } catch (e) {
    log(`Failed to post review batch with ${chunk.length} inline comment(s): ${e.message}`);

    // Retry/pacing configuration (shared by write and read API calls).
    // parseNonNegInt guards against nonsensical env values (negative, NaN,
    // non-numeric) that `parseInt(...) || default` would let through for
    // negative numbers, since a negative parseInt result is truthy and would
    // bypass the `|| default` fallback. These are re-read here (not threaded
    // through a config bag) to keep this helper a behavior-preserving move of
    // the existing catch block — scoping config to only the batch size would
    // silently drop per-comment pacing.
    const MAX_RETRIES = parseNonNegInt(process.env.OCR_MAX_RETRIES, 3);
    const SUCCESS_DELAY = parseNonNegInt(process.env.OCR_SUCCESS_DELAY, 2000);
    const FAILURE_DELAY = parseNonNegInt(process.env.OCR_FAILURE_DELAY, 1000);
    const LOW_REMAINING_THRESHOLD = parseNonNegInt(process.env.OCR_LOW_REMAINING_THRESHOLD, 3);
    const LOW_REMAINING_SPACING = parseNonNegInt(process.env.OCR_LOW_REMAINING_SPACING, 10000);
    // Note: read-API pacing (OCR_READ_SUCCESS_DELAY / OCR_READ_LOW_REMAINING_SPACING)
    // is handled internally by readWithPacing() for the read calls below
    // (findExistingBatchReview / getPostedCommentIds / isCommentAlreadyPosted),
    // so it is not read here — only the write-path pacing knobs are.

    // Rate-limit cooldown + idempotency reconciliation, both handled by
    // cooldownAndReconcile(). The SAME helper runs for the secondary filtered
    // batch below, so the two write paths cannot drift apart: every batch-level
    // failure honors the error's retry/rate-limit headers before any further API
    // call, and every failure that MAY have reached the server is reconciled
    // against what actually landed before we retry anything.
    const primary = await cooldownAndReconcile({
      github,
      owner,
      repo,
      prNumber,
      log,
      error: e,
      items: chunk,
      tag: REVIEW_TAG,
      label: "Batch",
      labelLower: "batch",
    });
    let toRetry = primary.toRetry;
    succeeded += primary.alreadyPosted;
    reconciled = primary.reconciled;
    for (const { item, error } of primary.unverified) {
      failed++;
      failedComments.push({ comment: item.comment, error });
    }
    const batchStatus = primary.status;

    // ---- HTTP 422 line-resolution fallback -------------------------------
    // GitHub rejects a whole createReview batch when ANY inline comment points
    // at a line outside the PR diff. Rather than degrading straight to N
    // separate per-comment reviews (N timeline entries — the churn issue #624
    // is about), drop the comments we can PROVE are unresolvable and re-send
    // the survivors as one review.
    //
    // Two guards keep this from making things worse:
    //   * isLineResolutionFailure() — a 422 from this endpoint means
    //     "Validation failed, OR the endpoint has been spammed". Only a
    //     confirmed line/diff validation error activates the fallback; an
    //     unrecognized 422 (including spam/abuse detection) falls straight
    //     through to the per-comment loop, which has its own retry discipline.
    //     Re-sending a batch into a spam-throttled endpoint would deepen the
    //     incident rather than fix it.
    //   * classifyCommentAgainstDiff() is TRI-state. A comment is only dropped
    //     when the diff inventory is complete AND proves the line is outside
    //     it. "unknown" (incomplete file list, file present but patch omitted
    //     for a binary/oversized diff, LEFT-side comment, no line info) keeps
    //     the pre-existing per-comment behavior instead of silently voiding a
    //     comment that might well post.
    if (batchStatus === 422 && toRetry.length > 0 && isLineResolutionFailure(e)) {
      log(`[422-fallback] Batch createReview rejected by line/diff validation (HTTP 422). Filtering unresolvable comments against PR diff hunks...`);
      let diff = null;
      try {
        diff = await getPrDiffHunks({
          github,
          owner,
          repo,
          prNumber,
          commitSha,
          log,
          cache: diffCache,
        });
      } catch (hunkErr) {
        log(`[422-fallback] Failed to fetch PR diff hunks (${hunkErr.message}); proceeding without diff hunk filter.`);
      }

      // valid   -> provably inside the diff, safe to re-batch
      // unknown -> cannot prove either way, fall through to the per-comment loop
      // invalid -> provably outside the diff, route to the summary
      const validItems = [];
      const unknownItems = [];
      for (const item of toRetry) {
        const verdict = classifyCommentAgainstDiff(item, diff);
        if (verdict === "valid") {
          validItems.push(item);
        } else if (verdict === "unknown") {
          unknownItems.push(item);
        } else {
          failed++;
          failedComments.push({
            comment: item.comment,
            error: `${describeCommentLocation(item.reviewComment)} could not be resolved (outside PR diff hunks)`,
          });
          log(`[422-fallback] Comment for ${item.reviewComment.path} (${describeCommentLocation(item.reviewComment)}) is outside PR diff hunks; routing to summary failure.`);
        }
      }
      if (unknownItems.length > 0) {
        log(`[422-fallback] ${unknownItems.length} comment(s) could not be checked against the diff (incomplete or unavailable patch data); posting them individually rather than discarding them.`);
      }

      // A secondary batch is only worth sending if filtering actually removed
      // something. If every comment survived classification, the payload would
      // be byte-identical to the one GitHub just rejected (validItems preserves
      // toRetry's order), so the resend is guaranteed to fail the same way — a
      // wasted write into an endpoint that just returned 422, which is exactly
      // what the spam-throttle reasoning above says to avoid. This is reachable
      // whenever our view of the diff disagrees with GitHub's: `commitSha` is
      // the head SHA captured at trigger time, while getPrDiffHunks reports the
      // PR's CURRENT diff, so a push landing in between makes GitHub reject
      // lines that our classifier still considers valid.
      const filteredSomething = validItems.length < toRetry.length;
      if (validItems.length > 0 && filteredSomething) {
        log(`[422-fallback] Attempting secondary filtered batch createReview with ${validItems.length} valid comment(s)...`);
        try {
          const secondaryRes = await github.rest.pulls.createReview({
            owner,
            repo,
            pull_number: prNumber,
            commit_id: commitSha,
            body: reviewBody,
            event: "COMMENT",
            comments: validItems.map(({ reviewComment }) => reviewComment),
          });
          succeeded += validItems.length;
          log(`Successfully posted secondary filtered review batch with ${validItems.length} inline comment(s).`);
          logRateLimitQuota(secondaryRes, "after secondary batch createReview", log);
          toRetry = unknownItems;
        } catch (secondaryE) {
          log(`Secondary filtered batch createReview failed (HTTP ${secondaryE.status || "n/a"}): ${secondaryE.message}. Falling back to per-comment loop.`);
          // Same cooldown + idempotency discipline as the primary batch: a 5xx
          // or network error can mean the secondary review LANDED and only the
          // response was lost, in which case dropping straight into the
          // per-comment loop would repost every surviving comment.
          const secondary = await cooldownAndReconcile({
            github,
            owner,
            repo,
            prNumber,
            log,
            error: secondaryE,
            items: validItems,
            tag: REVIEW_TAG,
            label: "Secondary filtered batch",
            labelLower: "secondary filtered batch",
          });
          succeeded += secondary.alreadyPosted;
          if (secondary.reconciled) reconciled = true;
          for (const { item, error } of secondary.unverified) {
            failed++;
            failedComments.push({ comment: item.comment, error });
          }
          toRetry = secondary.toRetry.concat(unknownItems);
        }
      } else {
        if (validItems.length > 0) {
          log(
            `[422-fallback] Classification cleared all ${validItems.length} remaining comment(s), so a ` +
              `filtered batch would be identical to the one GitHub just rejected; skipping the secondary ` +
              `batch and going straight to the per-comment fallback.`
          );
        }
        // NOTE: validItems must be carried here, not dropped. Falling back to
        // `toRetry = unknownItems` alone would silently discard every comment
        // that passed classification.
        toRetry = validItems.concat(unknownItems);
      }

      if (toRetry.length === 0) {
        log(`[422-fallback] No comments remain for the per-comment fallback.`);
        return { succeeded, failed, failedComments, reconciled };
      }
    }

    for (const { comment, reviewComment, id } of toRetry) {
      let posted = false;
      for (let attempt = 0; attempt <= MAX_RETRIES && !posted; attempt++) {
        try {
          const res = await github.rest.pulls.createReview({
            owner,
            repo,
            pull_number: prNumber,
            commit_id: commitSha,
            body: "",
            event: "COMMENT",
            comments: [reviewComment],
          });
          succeeded++;
          posted = true;
          log(`Successfully posted comment for ${reviewComment.path}`);
          // Proactive throttle: if remaining quota is low, slow down to
          // avoid hitting the limit (GitHub best practice: watch the header).
          const remaining = logRateLimitQuota(res, `after ${reviewComment.path}`, log);
          const lowQuota = remaining != null && remaining <= LOW_REMAINING_THRESHOLD;
          if (lowQuota) {
            log(`[rate-limit] quota low (remaining=${remaining} <= ${LOW_REMAINING_THRESHOLD}); increasing spacing to ${LOW_REMAINING_SPACING}ms.`);
            await sleep(LOW_REMAINING_SPACING);
          } else {
            await sleep(SUCCESS_DELAY);
          }
        } catch (innerE) {
          // Decide whether to retry and how long to wait, based on GitHub's
          // rate-limit documentation (retry-after / x-ratelimit-* headers).
          const retryInfo = computeRetryDelayMs(innerE, attempt);
          const willRetry = retryInfo != null && attempt < MAX_RETRIES;
          // Any error whose request may have reached GitHub (5xx server
          // errors, 408 timeout, or network-layer errors with no status) can
          // mean the comment was actually created but the response was lost.
          // Before retrying (which would post a duplicate) or before giving
          // up (which would wrongly list it as failed in the summary), check
          // whether it already landed.
          //
          // IMPORTANT: do the check AFTER cooling down, not immediately. If
          // the error is rate-limit-related (5xx under load, or a network
          // blip), firing read requests right away further pressures the
          // already-struggling API. Honor the computed retry delay first,
          // then query.
          const status = innerE.status;
          const maybeReachedServer =
            (typeof status === "number" && (status >= 500 || status === 408)) ||
            status == null; // network errors (ECONNRESET, ETIMEDOUT, ...)
          if (maybeReachedServer) {
            // Cool down first: even read requests count against rate limits,
            // and querying during an ongoing 5xx/rate-limit episode can
            // worsen the situation. Use the retry delay when available; for
            // non-retryable errors (retryInfo == null) there is no
            // header-derived wait, so use a short fixed cool down before the
            // read.
            const coolDownMs = retryInfo != null ? retryInfo.delayMs : FAILURE_DELAY;
            if (coolDownMs > 0) {
              const secs = (coolDownMs / 1000).toFixed(1);
              log(
                `Cooling down ${secs}s before idempotency check for ${reviewComment.path} ` +
                  `(HTTP ${innerE.status || "n/a"}, attempt ${attempt + 1}/${MAX_RETRIES + 1}).`
              );
              await sleep(coolDownMs);
            }
            const alreadyPosted = await isCommentAlreadyPosted({ github, owner, repo, prNumber, id, log });
            if (alreadyPosted === true) {
              succeeded++;
              posted = true;
              log(`Comment for ${reviewComment.path} already posted (id=${id}); treating as success.`);
              await sleep(SUCCESS_DELAY);
              continue;
            }
            // Unknown (null): the read API is unavailable, so we cannot tell
            // whether the comment landed. To avoid a duplicate, do NOT retry
            // posting; record as failed so the summary surfaces the
            // uncertainty rather than silently risking a duplicate.
            if (alreadyPosted === null) {
              failed++;
              const reason = "idempotency check unavailable (read API failed)";
              failedComments.push({ comment, error: `${innerE.message} [${reason}]` });
              log(`Cannot verify whether comment for ${reviewComment.path} was posted (${reason}, HTTP ${innerE.status || "n/a"}); skipping retry to avoid duplicate.`);
              await sleep(SUCCESS_DELAY);
              break;
            }
            // Not found on server. If retries are exhausted or the error is
            // non-retryable, this is a real failure.
            if (!willRetry) {
              failed++;
              failedComments.push({ comment, error: innerE.message });
              const reason = retryInfo == null ? "non-retryable error" : "rate-limit retries exhausted";
              log(`Failed to post comment for ${reviewComment.path} (${reason}, HTTP ${innerE.status || "n/a"}): ${innerE.message}`);
              await sleep(SUCCESS_DELAY);
              break;
            }
            // willRetry: cool down already consumed above, loop back.
          } else if (willRetry) {
            // Pure 429/403 rate-limit: the request never reached the server,
            // so no duplicate is possible and the idempotency check can be
            // skipped. Just honor the retry delay.
            const secs = (retryInfo.delayMs / 1000).toFixed(1);
            log(
              `Rate-limited on ${reviewComment.path} ` +
                `(HTTP ${innerE.status}, attempt ${attempt + 1}/${MAX_RETRIES}). ` +
                `Waiting ${secs}s via '${retryInfo.source}' (${retryInfo.detail}). ` +
                `Error: ${innerE.message}`
            );
            await sleep(retryInfo.delayMs);
          } else {
            // Non-retryable error that definitely did not reach the server
            // (e.g. 4xx validation error): record as failed.
            failed++;
            failedComments.push({ comment, error: innerE.message });
            log(`Failed to post comment for ${reviewComment.path} (non-retryable error, HTTP ${innerE.status || "n/a"}): ${innerE.message}`);
            await sleep(FAILURE_DELAY);
            break;
          }
        }
      }
    }
  }

  return { succeeded, failed, failedComments, reconciled };
}

// Build the per-run idempotency tags from the GitHub Actions run identity.
// runId / runAttempt come from @actions/github's Context (GITHUB_RUN_ID /
// GITHUB_RUN_ATTEMPT); Number.isFinite guards against NaN when the env vars are
// missing, falling back to safe defaults. REVIEW_TAG is the body marker the
// batch createReview carries so findExistingBatchReview can locate a batch that
// landed despite a 5xx; SUMMARY_TAG is the analogous marker for the summary
// issue comment. Pure so it can be tested and reused.
function buildRunTags(runId, runAttempt) {
  const id = Number.isFinite(runId) ? runId : 0;
  const attempt = Number.isFinite(runAttempt) ? runAttempt : 1;
  const RUN_TAG = `${id}-${attempt}`;
  return {
    RUN_TAG,
    REVIEW_TAG: `<!-- ocr-review-run:${RUN_TAG} -->`,
    SUMMARY_TAG: `<!-- ocr-summary-run:${RUN_TAG} -->`,
  };
}

// Resolve the configured batch size. A batch size is a positive integer (N>=1):
// 0, negatives, NaN, and non-numeric strings all fall back to the default.
// Mirrors the parseNonNegInt discipline but with a lower bound of 1, since a
// zero-size batch would be nonsensical (B1).
function resolveBatchSize(raw) {
  const n = parseInt(raw, 10);
  return Number.isFinite(n) && n >= 1 ? n : DEFAULT_BATCH_SIZE;
}

// Deterministically order the toSend set before partitioning so identical
// inputs produce identical batches across reruns (B2/AS4). Returns a NEW array
// (does not mutate the caller's array). Sort key: path → start_line → end_line
// → original array index. The explicit original-index tiebreak guarantees
// stable ordering even for same-file same-line findings, on engines where
// Array.prototype.sort stability would otherwise be incidental. Severity/
// category ordering is intentionally absent — comment objects carry no such
// fields (verified at the call site), and adding them is a cross-cutting schema
// change explicitly out of scope (sibling issue #478).
function sortToSendDeterministically(items) {
  return items
    .map((item, origIndex) => ({ item, origIndex }))
    .sort((a, b) => {
      const ca = a.item.comment;
      const cb = b.item.comment;
      const byPath = String(ca.path).localeCompare(String(cb.path));
      if (byPath !== 0) return byPath;
      const byStart = (ca.start_line || 0) - (cb.start_line || 0);
      if (byStart !== 0) return byStart;
      const byEnd = (ca.end_line || 0) - (cb.end_line || 0);
      if (byEnd !== 0) return byEnd;
      return a.origIndex - b.origIndex;
    })
    .map(({ item }) => item);
}

// Partition a sorted array into contiguous slices of at most `size` items
// (B1/AS2/AS3). Contiguity + sorted input ⇒ deterministic partition: the last
// slice is the remainder (length === size when items.length is a multiple of
// size, otherwise items.length mod size).
function chunkArray(items, size) {
  const chunks = [];
  for (let i = 0; i < items.length; i += size) {
    chunks.push(items.slice(i, i + size));
  }
  return chunks;
}

function setStatsOutputs(out, stats, batchCounters, batchSize) {
  out("comments_total", String(stats.total));
  out("comments_inline", String(stats.inline));
  out("comments_skipped", String(stats.skipped));
  out("comments_routed", String(stats.routed));
  out("comments_failed", String(stats.failed));
  out("summary_comment_url", stats.summaryUrl || "");
  // Per-batch telemetry (B7). These are additional outputs; the five above are
  // unchanged so existing consumers of comments_* / summary_comment_url are
  // unaffected. batch_summary is a single JSON string so a fleet dashboard can
  // read one value instead of correlating multiple scalars.
  if (batchCounters) {
    out("batches_total", String(batchCounters.total));
    out("batches_attempted", String(batchCounters.attempted));
    out("batches_succeeded", String(batchCounters.succeeded));
    out("batches_reconciled", String(batchCounters.reconciled));
    out(
      "batch_summary",
      JSON.stringify({
        total: batchCounters.total,
        attempted: batchCounters.attempted,
        succeeded: batchCounters.succeeded,
        reconciled: batchCounters.reconciled,
        batch_size: batchSize != null ? batchSize : DEFAULT_BATCH_SIZE,
        inline: stats.inline,
        failed: stats.failed,
      })
    );
  }
}

async function submitCleanApproval({ github, context, owner, repo, prNumber, message, log }) {
  let commitSha;
  if (context.eventName === "pull_request_target") {
    commitSha = context.payload.pull_request.head.sha;
  } else {
    try {
      const { data: pullRequest } = await github.rest.pulls.get({
        owner,
        repo,
        pull_number: prNumber,
      });
      commitSha = pullRequest.head.sha;
    } catch (e) {
      log(`[approve] could not resolve head commit (${e.message}); skipping approval.`);
      return null;
    }
  }
  if (!commitSha) {
    log("[approve] no head commit sha available; skipping approval.");
    return null;
  }

  // Idempotency: do not re-approve a commit this bot already approved.
  try {
    const botLogin = await getAuthenticatedLogin(github, log);
    const reviews = await readAllPages("listReviews", (page, per_page) =>
      github.rest.pulls.listReviews({ owner, repo, pull_number: prNumber, per_page, page }), log);
    const alreadyApproved = reviews.some(
      (r) =>
        r.state === "APPROVED" &&
        r.commit_id === commitSha &&
        botLogin &&
        r.user &&
        r.user.login === botLogin
    );
    if (alreadyApproved) {
      log("[approve] this commit is already approved by the bot; skipping.");
      return null;
    }
  } catch (e) {
    log(`[approve] could not check existing reviews (${e.message}); proceeding to approve.`);
  }

  try {
    const body = `✅ **OpenCodeReview**: ${message}`;
    const { data: review } = await github.rest.pulls.createReview({
      owner,
      repo,
      pull_number: prNumber,
      commit_id: commitSha,
      event: "APPROVE",
      body,
    });
    log("Submitted formal APPROVE review for the clean run.");
    return review;
  } catch (e) {
    log(`[approve] failed to submit APPROVE review (${e.message}); continuing without approval.`);
    return null;
  }
}

// ---- Summary posting (sticky vs new) ----

async function postSummary({ github, owner, repo, prNumber, body, sticky, log }) {
  const fullBody = body;
  if (sticky) {
    const existing = await findExistingSummaryComment({ github, owner, repo, prNumber, log });
    if (existing) {
      const { data: updated } = await github.rest.issues.updateComment({
        owner,
        repo,
        comment_id: existing.id,
        body: fullBody,
      });
      return { id: updated.id, url: updated.html_url, updated: true };
    }
  }
  const { data: created } = await github.rest.issues.createComment({
    owner,
    repo,
    issue_number: prNumber,
    body: fullBody,
  });
  return { id: created.id, url: created.html_url, updated: false };
}

async function findExistingSummaryComment({ github, owner, repo, prNumber, log }) {
  const comments = await readAllPages("listIssueComments", (page, per_page) =>
    github.rest.issues.listComments({ owner, repo, issue_number: prNumber, per_page, page }), log
  );
  // Issue comments are returned oldest-first; pick the newest matching.
  for (let i = comments.length - 1; i >= 0; i--) {
    const body = comments[i].body;
    if (typeof body === "string" && body.includes(SUMMARY_MARKER)) {
      return comments[i];
    }
  }
  return null;
}

// ---- Summary anchor + finalize (cold-start ordering) ----
//
// The summary issue comment is created BEFORE the review so that on a cold
// start (first review on the PR) it lands above the review in the timeline
// (GitHub orders issue comments oldest-first). It is then updated in place
// with the final body once the review has posted. This keeps the summary from
// being sandwiched between review blocks on subsequent sticky runs.

// Find the issue comment that should carry the summary, or null if none.
// Sticky matches the persistent cross-run marker (SUMMARY_MARKER); non-sticky
// matches this run's tag (SUMMARY_TAG) so each run gets its own comment while
// retries within a run reuse it. Throws on read failure so callers can degrade.
async function findSummaryIssueComment({ github, owner, repo, prNumber, sticky, tag, log }) {
  const comments = await readAllPages("listIssueComments", (page, per_page) =>
    github.rest.issues.listComments({ owner, repo, issue_number: prNumber, per_page, page }), log
  );
  for (let i = comments.length - 1; i >= 0; i--) {
    const body = comments[i].body || "";
    if (sticky ? body.includes(SUMMARY_MARKER) : body.includes(tag)) {
      return comments[i];
    }
  }
  return null;
}

// Phase 1 (before review): create a summary comment only if none exists yet, so
// its timeline position is pinned above the not-yet-posted review. Returns
// { id, url } for the existing/created comment, or null when the existence
// check fails (read API unavailable) — callers then defer to finalizeSummary.
async function ensureSummaryAnchor({ github, owner, repo, prNumber, body, sticky, tag, log }) {
  let existing = null;
  try {
    existing = await findSummaryIssueComment({ github, owner, repo, prNumber, sticky, tag, log });
  } catch (e) {
    log(`[summary] cannot check for existing summary before review (${e.message}); skipping anchor.`);
    return null;
  }
  if (existing) {
    return { id: existing.id, url: existing.html_url };
  }
  const { data: created } = await github.rest.issues.createComment({
    owner,
    repo,
    issue_number: prNumber,
    body,
  });
  return { id: created.id, url: created.html_url };
}

// Phase 2 (after review): write the final summary body. When the anchor's id is
// known, update it directly (no extra read). Otherwise upsert: find then update
// or create. Returns { id, url }, or null when the read API is unavailable and
// the summary cannot be safely written without risking a duplicate.
async function finalizeSummary({ github, owner, repo, prNumber, anchor, body, sticky, tag, log }) {
  if (anchor && anchor.id != null) {
    const { data: updated } = await github.rest.issues.updateComment({
      owner,
      repo,
      comment_id: anchor.id,
      body,
    });
    return { id: updated.id, url: updated.html_url };
  }
  let existing = null;
  try {
    existing = await findSummaryIssueComment({ github, owner, repo, prNumber, sticky, tag, log });
  } catch (e) {
    log(`[summary] cannot check for existing summary at finalize (${e.message}); skipping to avoid duplicate.`);
    return null;
  }
  if (existing) {
    const { data: updated } = await github.rest.issues.updateComment({
      owner,
      repo,
      comment_id: existing.id,
      body,
    });
    return { id: updated.id, url: updated.html_url };
  }
  const { data: created } = await github.rest.issues.createComment({
    owner,
    repo,
    issue_number: prNumber,
    body,
  });
  return { id: created.id, url: created.html_url };
}

// ---- Incremental helpers ----

async function getAuthenticatedLogin(github, log) {
  try {
    const { data: user } = await github.rest.users.getAuthenticated();
    return user && user.login ? user.login : null;
  } catch (e) {
    log(`[incremental] could not resolve authenticated user: ${e.message}`);
    return null;
  }
}

async function listExistingReviewComments(github, owner, repo, prNumber, log) {
  const all = [];
  let page = 1;
  // Cap pagination so a pathological PR cannot stall the job; 10 pages = 1000.
  const MAX_PAGES = 10;
  // Sort newest-first so the page cap keeps the most recent comments: the
  // incremental dedup cares about the latest coverage state, and on truncation
  // we'd rather drop ancient comments than the recent ones the bot just posted.
  // GitHub's default is ascending (oldest-first), which would keep the oldest
  // 1000 and silently drop the newest — the exact comments dedup needs most.
  try {
    while (page <= MAX_PAGES) {
      const res = await github.rest.pulls.listReviewComments({
        owner,
        repo,
        pull_number: prNumber,
        sort: "created",
        direction: "desc",
        per_page: 100,
        page,
      });
      const items = res.data || [];
      all.push(...items);
      if (items.length < 100) break;
      page++;
    }
  } catch (e) {
    log(`[incremental] listing review comments failed (${e.message}); degrading to no history.`);
    return [];
  }
  if (page > MAX_PAGES) {
    log(`[incremental] listing review comments reached max page limit (${MAX_PAGES}); results may be incomplete.`);
  }
  return all;
}

function isBotComment(comment, botLogin) {
  if (!comment || !comment.user) return false;
  if (botLogin && comment.user.login === botLogin) return true;
  // GITHUB_TOKEN posts as "github-actions[bot]"; GitHub Apps post as the app.
  const login = comment.user.login || "";
  return /github-actions\[bot\]$/i.test(login) || (botLogin != null && login === botLogin);
}

// Incremental overlap test. The current comment is considered a duplicate of
// an existing bot comment (and thus skipped) when they target the same path
// and RIGHT side AND one of these holds:
//   1. both are single-line comments on the same line;
//   2. both are multi-line comments whose line-range IoU (intersection over
//      union) exceeds `threshold`.
// A single-line comment is NEVER considered the same as a multi-line one, so
// revisiting a line with a finer-grained single-line note is not suppressed by
// a prior multi-line block (and vice versa).
function overlapsHistory(reviewComment, history, threshold = DEFAULT_OVERLAP_THRESHOLD) {
  const t = resolveThreshold(threshold);
  const path = reviewComment.path;
  const cur = lineSpan(reviewComment);
  if (!cur) return false;
  for (const h of history) {
    if (h.path !== path) continue;
    if (h.side && h.side !== "RIGHT") continue;
    const other = lineSpan(h);
    if (!other) continue;
    if (sameCommentSpan(cur, other, t)) return true;
  }
  return false;
}

// Clamp/validate the caller-provided threshold to a sane (0, 1] number,
// falling back to the default when it is missing, NaN, or out of range. This
// keeps the public overlapsHistory API robust even when the value arrives from
// an env var / action input as a malformed string.
function resolveThreshold(threshold) {
  const n = Number(threshold);
  return Number.isFinite(n) && n > 0 && n <= 1 ? n : DEFAULT_OVERLAP_THRESHOLD;
}

// Resolve a comment into a line span tagged as single- or multi-line.
// Returns { start, end, multiline } or null when no line can be resolved.
// Handles both our own reviewComment shape ({start_line, line}) and GitHub's
// historical comment shape ({start_line, line}; start_line null for
// single-line). A comment is multi-line only when start_line and line are both
// present and differ; start_line === line (or a missing start_line) is treated
// as a single-line comment on that line.
function lineSpan(c) {
  const start = num(c.start_line);
  const end = num(c.line != null ? c.line : c.end_line);
  if (start == null && end == null) return null;
  if (start != null && end != null && start !== end) {
    return { start: Math.min(start, end), end: Math.max(start, end), multiline: true };
  }
  const single = end != null ? end : start;
  return { start: single, end: single, multiline: false };
}

// Same-comment predicate implementing the incremental rules. The IoU
// comparison is strict (>), so a span that exactly meets the threshold is NOT
// treated as a duplicate.
function sameCommentSpan(cur, other, threshold) {
  if (cur.multiline !== other.multiline) return false;
  if (!cur.multiline) return cur.start === other.start;
  const overlap = Math.min(cur.end, other.end) - Math.max(cur.start, other.start) + 1;
  if (overlap <= 0) return false;
  const union = cur.end - cur.start + 1 + (other.end - other.start + 1) - overlap;
  if (union <= 0) return false;
  return overlap / union > threshold;
}

function num(v) {
  if (v == null || v === "") return null;
  const n = Number(v);
  return Number.isFinite(n) && n >= 1 ? n : null;
}

// ---- Rate-limit / retry helpers (ported verbatim) ----

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

// Parse a non-negative integer env value, falling back to defaultVal when the
// value is missing, NaN, or negative. Unlike `parseInt(...) || default`, this
// guards against negative numbers: a negative parseInt result is truthy, so
// `parseInt || default` would let a nonsensical negative value bypass the
// fallback.
function parseNonNegInt(val, defaultVal) {
  const n = parseInt(val, 10);
  return Number.isFinite(n) && n >= 0 ? n : defaultVal;
}

// Case-insensitive header lookup. Octokit normalizes response headers to
// lowercase, but this defensive check also handles original casing so that
// quota logging and retry delay computation never silently miss a header.
function getHeader(headers, name) {
  const v = headers[name] != null ? headers[name] : headers[name.toLowerCase()];
  return v != null ? String(v).trim() : undefined;
}

// Decide whether an error is worth retrying and, if so, how long to wait.
// Implements GitHub's documented rate-limit retry strategy using the
// response headers (retry-after, x-ratelimit-remaining, x-ratelimit-reset).
// Returns { delayMs, source, detail } when retryable, or null otherwise.
// See: https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api
function computeRetryDelayMs(error, attempt) {
  if (!error) return null;
  const status = error.status;
  const message = String(error.message || "");
  const isRateLimit = status === 429 || (status === 403 && /rate limit|abuse|secondary/i.test(message));
  const isTransient = (status >= 500 && status < 600) || status === 408;
  if (!isRateLimit && !isTransient) return null;

  const headers = ((error.response || {}).headers) || {};
  const header = (name) => getHeader(headers, name);
  const nowSec = Math.floor(Date.now() / 1000);

  const cap = parseNonNegInt(process.env.OCR_RETRY_MAX_DELAY, 300000);
  const base = parseNonNegInt(process.env.OCR_RETRY_BASE_DELAY, 60000);

  let info = null;

  if (isRateLimit) {
    // (1) Honor "retry-after" when present (seconds, or an HTTP-date).
    const retryAfter = header("retry-after");
    if (retryAfter) {
      const secs = Number(retryAfter);
      if (!isNaN(secs) && secs >= 0) {
        info = { rawMs: secs * 1000, source: "retry-after", detail: `${secs}s (from header)` };
      } else {
        const dateMs = Date.parse(retryAfter);
        if (!isNaN(dateMs)) {
          info = { rawMs: Math.max(0, dateMs - Date.now()), source: "retry-after (HTTP-date)", detail: retryAfter };
        }
      }
    }

    // (2) Primary limit exhausted (x-ratelimit-remaining=0): wait until reset.
    if (!info) {
      const remaining = header("x-ratelimit-remaining");
      const reset = header("x-ratelimit-reset");
      if (reset != null && Number(remaining) === 0) {
        const rawMs = Math.max(0, Number(reset) - nowSec) * 1000;
        info = { rawMs, source: "x-ratelimit-reset", detail: `remaining=0, reset epoch=${reset} (in ${Math.ceil(rawMs / 1000)}s)` };
      }
    }

    // (3) Secondary limit with no retry hint: docs say wait at least one
    //     minute, then increase exponentially between retries.
    if (!info) {
      const backoff = Math.min(base * Math.pow(2, attempt), cap);
      const jitter = Math.floor(Math.random() * 1000);
      info = { rawMs: backoff + jitter, source: "exponential-backoff", detail: `base=${base}ms*2^${attempt} (cap ${cap}ms) +${jitter}ms jitter` };
    }
  } else {
    // Transient server error (5xx / 408): back off without the 60s floor.
    const transientBase = 2000;
    const backoff = Math.min(transientBase * Math.pow(2, attempt), cap);
    const jitter = Math.floor(Math.random() * 1000);
    info = { rawMs: backoff + jitter, source: "transient-backoff", detail: `base=${transientBase}ms*2^${attempt} (cap ${cap}ms) +${jitter}ms jitter (HTTP ${status})` };
  }

  const delayMs = Math.min(info.rawMs, cap);
  if (delayMs < info.rawMs) {
    info.detail += ` [CAPPED to ${cap}ms; GitHub recommended ${Math.ceil(info.rawMs / 1000)}s]`;
  }
  return { delayMs, source: info.source, detail: info.detail };
}

// Best-effort logging of remaining rate-limit quota from a successful response.
// Returns the parsed x-ratelimit-remaining value (or null) for proactive throttling.
function logRateLimitQuota(response, tag, log) {
  try {
    const h = (response && response.headers) || {};
    const header = (name) => getHeader(h, name);
    const remaining = header("x-ratelimit-remaining");
    const limit = header("x-ratelimit-limit");
    const reset = header("x-ratelimit-reset");
    if (remaining != null) {
      log(
        `[rate-limit] ${tag}: remaining=${remaining}/${limit != null ? limit : "?"}` +
          (reset != null ? `, reset epoch=${reset}` : "")
      );
    }
    return remaining != null ? Number(remaining) : null;
  } catch (_) {
    return null;
  }
}

// ---- Read API + idempotency helpers ----
//
// The helpers below back the "prevent duplicate review posts on retry"
// strategy: when a batch createReview fails with a 5xx, the request may still
// have landed on the server. Before retrying, we query existing reviews and
// review comments (each tagged with a per-run HTML comment) and only retry the
// comments that are actually missing. Read calls are paced (shorter delays
// than writes) and degrade to "unknown" (null) when the read API itself fails,
// so the caller skips retrying rather than risking a duplicate.

// Retry wrapper shared by write and read API calls. Reuses computeRetryDelayMs
// so rate-limit headers (retry-after / x-ratelimit-*) are honored uniformly.
// Throws on final failure so the caller can decide how to degrade.
async function withRetry(tag, fn, log) {
  const MAX_RETRIES = parseNonNegInt(process.env.OCR_MAX_RETRIES, 3);
  let lastErr;
  for (let attempt = 0; attempt <= MAX_RETRIES; attempt++) {
    try {
      return await fn();
    } catch (e) {
      lastErr = e;
      const retryInfo = computeRetryDelayMs(e, attempt);
      const willRetry = retryInfo != null && attempt < MAX_RETRIES;
      if (willRetry) {
        const secs = (retryInfo.delayMs / 1000).toFixed(1);
        log(
          `[${tag}] transient/rate-limited (HTTP ${e.status}, attempt ${attempt + 1}/${MAX_RETRIES}). ` +
            `Waiting ${secs}s via '${retryInfo.source}' (${retryInfo.detail}). ${e.message}`
        );
        await sleep(retryInfo.delayMs);
      } else {
        log(`[${tag}] failed after ${attempt + 1} attempts: ${e.message}`);
        throw e;
      }
    }
  }
  throw lastErr != null
    ? lastErr
    : new Error(`withRetry(${tag}): exhausted retries with no error captured`);
}

// Read API wrapper with retry + proactive pacing. Read requests are cheaper
// than writes but still consume the primary rate limit and can trigger the
// secondary limit when issued in a tight loop. Use shorter delays than writes
// (READ_SUCCESS_DELAY / READ_LOW_REMAINING_SPACING).
async function readWithPacing(tag, fn, log) {
  const res = await withRetry(tag, fn, log);
  const remaining = logRateLimitQuota(res, tag, log);
  const LOW_REMAINING_THRESHOLD = parseNonNegInt(process.env.OCR_LOW_REMAINING_THRESHOLD, 3);
  const lowQuota = remaining != null && remaining <= LOW_REMAINING_THRESHOLD;
  if (lowQuota) {
    const READ_LOW_REMAINING_SPACING = parseNonNegInt(process.env.OCR_READ_LOW_REMAINING_SPACING, 5000);
    log(`[rate-limit] quota low after read (${remaining} <= ${LOW_REMAINING_THRESHOLD}); spacing ${READ_LOW_REMAINING_SPACING}ms.`);
    await sleep(READ_LOW_REMAINING_SPACING);
  } else {
    const READ_SUCCESS_DELAY = parseNonNegInt(process.env.OCR_READ_SUCCESS_DELAY, 500);
    await sleep(READ_SUCCESS_DELAY);
  }
  return res;
}

// Paginated helper that walks all pages of a list endpoint with retry and
// pacing. Returns the concatenated array of items.
async function readAllPages(tag, pageFn, log, maxPages = 50) {
  if (!Number.isFinite(maxPages) || maxPages < 1) {
    throw new Error(`readAllPages: maxPages must be a positive integer, got ${maxPages}`);
  }
  const all = [];
  let page = 1;
  const PER_PAGE = 100;
  while (page <= maxPages) {
    const res = await readWithPacing(`${tag} (page ${page})`, () => pageFn(page, PER_PAGE), log);
    const items = res.data || [];
    all.push(...items);
    if (items.length < PER_PAGE) break;
    page++;
  }
  // NOTE: Truncation here is intentional and acts as a safety valve against
  // unbounded loops (e.g. a bug or malicious activity), not as a normal
  // operating mode. A PR accumulating >5000 review comments is far outside
  // expected usage; in that rare case we log a warning and proceed with
  // partial data rather than failing the whole review.
  //
  // Caveat: this is NOT the same as a read failure. When the read API throws
  // (rate limit, 5xx), isCommentAlreadyPosted catches it and returns null
  // (unknown), so the caller skips retrying and creates no duplicate. A
  // truncated walk does not throw; it returns a partial set silently, so
  // isCommentAlreadyPosted returns false (definitively "not posted") for any
  // comment beyond the cap, and the retry loop will repost it, producing a
  // duplicate. This tradeoff is accepted because the trigger is far outside
  // expected usage; if that ceiling ever needs to rise, make maxPages
  // configurable.
  if (page > maxPages) {
    log(`[${tag}] reached max page limit (${maxPages}); results may be incomplete.`);
  }
  return all;
}

// Idempotency check: find whether a batch review with this run tag already
// exists on the PR. Returns { found, review } or throws on final failure
// (caller degrades to the original fallback).
async function findExistingBatchReview({ github, owner, repo, prNumber, tag, log }) {
  const reviews = await readAllPages("listReviews", (page, per_page) =>
    github.rest.pulls.listReviews({ owner, repo, pull_number: prNumber, per_page, page }), log
  );
  for (const r of reviews) {
    if ((r.body || "").includes(tag)) {
      return { found: true, review: r };
    }
  }
  return { found: false };
}

// Collect the set of comment-level IDs already posted on the PR (across all
// reviews). Uses listReviewComments (PR-level, cross-review) so a single
// paginated walk covers everything, avoiding the O(missing) amplification of
// per-comment lookups.
async function getPostedCommentIds({ github, owner, repo, prNumber, log }) {
  const comments = await readAllPages("listReviewComments", (page, per_page) =>
    github.rest.pulls.listReviewComments({ owner, repo, pull_number: prNumber, per_page, page }), log
  );
  const ids = new Set();
  // Anchor the regex to the HTML comment wrapper (<!-- ocr-... -->) so
  // user-generated content or code suggestions cannot trigger false positives
  // in the idempotency check. The ID format is `ocr-<RUN_TAG>-<random>` where
  // RUN_TAG is `<runId>-<runAttempt>` and <random> is a per-comment random
  // hex token. Capture group 1 holds the bare ID, so we can add it directly
  // without stripping comment markers.
  const ID_RE = /<!--\s*(ocr-\d+-\d+-[a-f0-9]+)\s*-->/g;
  for (const c of comments) {
    const body = c.body || "";
    let m;
    while ((m = ID_RE.exec(body)) !== null) {
      ids.add(m[1]);
    }
  }
  return ids;
}

// Check whether a specific comment-level ID has already landed on the server.
// Used by the per-comment retry loop: when a createReview call fails with a
// transient 5xx/408, the request may have reached GitHub and succeeded even
// though the response was lost. Querying before retrying prevents posting a
// duplicate inline comment.
// Returns true/false when the check succeeds, or null when the read API is
// unavailable (rate limit, 5xx, etc.). Returning null (rather than defaulting
// to false) prevents the caller from assuming the comment was not posted and
// risking a duplicate on retry.
//
// Each call walks listReviewComments fresh — no cached snapshot. A snapshot
// reused across retries would go stale as comments land during the loop, and a
// stale miss for a 5xx-landed comment would trigger a retry that posts a
// duplicate. Read calls are paced via readAllPages/readWithPacing and degrade
// to null (skip retry) if the read API itself fails, so the extra walks cannot
// produce duplicates.
async function isCommentAlreadyPosted({ github, owner, repo, prNumber, id, log }) {
  try {
    const posted = await getPostedCommentIds({ github, owner, repo, prNumber, log });
    return posted.has(id);
  } catch (e) {
    log(`[isCommentAlreadyPosted] check failed for ${id} (${e.message}); treating as unknown to avoid duplicates.`);
    return null;
  }
}

// Random per-comment ID, assigned once when the inline-comment item is built
// and carried on the item struct. Random (rather than content-derived) so two
// distinct comments that share the same path/line/content still get different
// IDs and the idempotency check never mistakes one for the other (which would
// silently drop the second). Embedded in the comment body as an HTML comment
// so getPostedCommentIds can match it back on retry.
function newCommentId(runTag) {
  return `ocr-${runTag}-${crypto.randomBytes(8).toString("hex")}`;
}

// ---- Badge + publication policy helpers (#478) ----
//
// These are pure functions (no I/O, no side effects) so they can be unit-tested
// directly. buildBadge byte-matches the CLI's cmd/opencodereview/output.go
// buildBadge degeneration so review output is consistent across surfaces (I6).
// buildPolicy/routeComment implement fail-open finding-publication routing
// (I1, I4): a finding never matches the policy on unknown/malformed metadata,
// and routing is a placement decision (findings route OUT of the inline write
// path), so a routed finding can never be double-posted on retry.

// Strip C0/C1 control characters from a metadata value. The CLI's
// sanitizeTerminal (cmd/opencodereview/output.go:197-206) strips control chars
// but PRESERVES \t and \n (harmless in a terminal). This Action sanitizer is
// intentionally STRICTER: it strips ALL control chars including \t and \n,
// because a newline/tab in a category or severity would break the comment
// body's layout (the badge renders in Markdown in a browser). This is a
// deliberate, documented divergence from strict OC1 byte-parity: clean enum
// values (the overwhelmingly common case) render identically across surfaces,
// and the divergence only manifests for malformed model output, in a SAFER
// direction (the Action cannot have its layout broken by a control char).
function sanitizeMetadata(value) {
  return String(value == null ? "" : value).replace(/[\x00-\x1f\x7f-\x9f]/g, "");
}

// Build the category/severity badge for a comment, byte-matching the CLI's
// buildBadge degeneration (cmd/opencodereview/output.go:98-114):
//   both non-empty -> "[category · severity]" (with a middot, U+00B7)
//   only category  -> "[category]"
//   only severity  -> "[severity]"
//   neither        -> "" (no badge line)
// Returns the empty string (not a newline) when nothing renders, so callers
// can prepend conditionally without leaving a blank line.
function buildBadge(comment) {
  const category = sanitizeMetadata(comment && comment.category);
  const severity = sanitizeMetadata(comment && comment.severity);
  if (category && severity) return `[${category} · ${severity}]`;
  if (category) return `[${category}]`;
  if (severity) return `[${severity}]`;
  return "";
}

// Shields.io color per severity, following the reviewer-suggested scale in
// #882 (low -> green, medium -> orange, high -> red). "critical" is not in
// that three-level scale; it extends it one step hotter (darkred, a standard
// CSS color name shields.io accepts). Keys match SEVERITIES.
const SEVERITY_BADGE_COLOR = Object.freeze({
  critical: "darkred",
  high: "red",
  medium: "orange",
  low: "green",
});
// Fallback color when severity is missing or not a known enum member. The
// reviewer's guidance was to render these rather than fall back to plain text,
// with any color acceptable; blue reads as informational.
const CATEGORY_BADGE_COLOR = "blue";

// Escape a value for a shields.io static-badge path segment ("-" separates
// segments, "_" renders as a space). Our enum values contain neither, but the
// escape keeps the helper correct if the enums ever grow such a value.
function shieldsEscape(value) {
  return encodeURIComponent(value.replace(/-/g, "--").replace(/_/g, "__"));
}

// Escape Markdown image alt text. The alt is derived from the plain-text badge
// whose metadata has had control chars stripped but may still carry
// Markdown-special characters (e.g. a stray "]" from high-temperature model
// output), which would prematurely close the image's [...] alt span. Escaping
// "\", "[", and "]" keeps the image intact and the alt rendering to the literal
// text. "[" is escaped defensively; "]" and "\" are the structural ones.
function escapeMarkdownAlt(value) {
  return value
    .replace(/\\/g, "\\\\")
    .replace(/\[/g, "\\[")
    .replace(/\]/g, "\\]");
}

// Build the category/severity badge as a single shields.io static badge
// (#882) for surfaces that render Markdown (GitHub review/summary comments),
// in the format img.shields.io/badge/<label>-<color>. flat is shields.io's
// default style, so the URL carries no style parameter. The image's alt text
// is the plain-text badge content from buildBadge (with Markdown-special
// characters escaped), so screen readers and image-load failures degrade to
// the previous text form.
//
//   both fields -> ![category · severity](…/badge/category-severity-<color>)
//   one field   -> ![field](…/badge/field-<color>)
//   neither     -> "" (no badge line)
//
// The color comes from the severity when known, otherwise the fixed category
// color — no lookup can ever yield "undefined". Values are sanitized (control
// chars stripped) and URL-escaped before interpolation, so rendering an
// unrecognized value is safe: it is only ever the same text already shown in
// the plain-text badge. This is a deliberate divergence from strict CLI
// byte-parity (I6): the CLI keeps the plain-text badge (a terminal cannot
// render images); the alt text preserves the shared form across surfaces.
function buildBadgeImage(comment) {
  const text = buildBadge(comment);
  if (text === "") return "";
  const category = sanitizeMetadata(comment && comment.category).trim().toLowerCase();
  const severity = sanitizeMetadata(comment && comment.severity).trim().toLowerCase();
  const color = SEVERITY_BADGE_COLOR[severity] || CATEGORY_BADGE_COLOR;
  const alt = escapeMarkdownAlt(text.slice(1, -1)); // "[bug · high]" -> "bug · high"
  const label = category && severity
    ? `${shieldsEscape(category)}-${shieldsEscape(severity)}`
    : shieldsEscape(category || severity);
  return `![${alt}](https://img.shields.io/badge/${label}-${color})`;
}

// Parse the publication policy from the raw opt-in inputs. Returns a normalized
// policy object, or the NO_ROUTING sentinel when no routing is requested or any
// value is malformed (fail-open for the policy itself, upholding I1).
//
//   severityThreshold: a severity name (case-insensitive) at-or-below which
//     findings route. An unknown/empty value disables severity routing.
//   categories: a comma-separated category list (case-insensitive). Unknown
//     category tokens are dropped; an empty list (or all-unknown) disables
//     category routing.
//
// The returned object has two booleans so routeComment can short-circuit
// without re-parsing, plus the normalized values it needs to decide:
//   {
//     routeBySeverity: bool,
//     severityRank: number,           // rank of the threshold; -1 when disabled
//     routeByCategory: bool,
//     categories: Set<string>,        // lowercase enum members; empty when disabled
//   }
function buildPolicy({ severityThreshold, categories } = {}) {
  let routeBySeverity = false;
  let severityRank = -1;
  if (severityThreshold != null) {
    const norm = String(severityThreshold).trim().toLowerCase();
    if (SEVERITY_RANK.has(norm)) {
      routeBySeverity = true;
      severityRank = SEVERITY_RANK.get(norm);
    }
    // Any other value (empty, unknown, garbage) leaves routeBySeverity=false
    // (fail-open for the policy: unknown threshold -> no routing).
  }

  let routeByCategory = false;
  const categorySet = new Set();
  if (categories != null) {
    const tokens = String(categories)
      .split(",")
      .map((t) => t.trim().toLowerCase())
      .filter((t) => t.length > 0);
    for (const t of tokens) {
      if (CATEGORIES.includes(t)) categorySet.add(t);
      // Unknown category tokens are silently dropped (fail-open: an unknown
      // category in the policy never matches a finding's category, so including
      // it would be a no-op anyway; dropping keeps the set clean).
    }
    if (categorySet.size > 0) routeByCategory = true;
  }

  if (!routeBySeverity && !routeByCategory) return NO_ROUTING;
  return { routeBySeverity, severityRank, routeByCategory, categories: categorySet };
}

// Decide whether a comment routes to the summary per the policy. Returns
// { routed: true, reason } or { routed: false }. A finding matches when its
// severity is at-or-below the threshold (when severity routing is on) OR its
// category is in the category list (when category routing is on). Unknown or
// malformed metadata on the finding NEVER matches (I1): an empty/unknown
// category or severity has no rank and no enum membership, so it falls through
// to the normal inline path (visible), never dropped.
function routeComment(comment, policy) {
  if (!policy || (!policy.routeBySeverity && !policy.routeByCategory)) {
    return { routed: false };
  }
  const catRaw = comment && comment.category != null ? String(comment.category).trim().toLowerCase() : "";
  const sevRaw = comment && comment.severity != null ? String(comment.severity).trim().toLowerCase() : "";
  const catKnown = catRaw !== "" && CATEGORIES.includes(catRaw);
  const sevKnown = sevRaw !== "" && SEVERITY_RANK.has(sevRaw);

  if (policy.routeBySeverity && sevKnown && SEVERITY_RANK.get(sevRaw) <= policy.severityRank) {
    return { routed: true, reason: `Routed to summary (severity ${sevRaw}${catKnown ? ` · category ${catRaw}` : ""})` };
  }
  if (policy.routeByCategory && catKnown && policy.categories.has(catRaw)) {
    return { routed: true, reason: `Routed to summary (category ${catRaw}${sevKnown ? ` · severity ${sevRaw}` : ""})` };
  }
  return { routed: false };
}

// ---- Formatting helpers (ported verbatim) ----

// Assemble the visible comment body. When `id` is provided (inline comments),
// the per-comment ID tag is prepended as an HTML comment (invisible when
// rendered) so getPostedCommentIds can match it back on retry for the
// idempotency check. The category/severity badge is then prepended (when
// present) on its own leading line AFTER the id comment, so the idempotency
// regex (unanchored, scans the whole body) still matches and the badge renders
// as the first visible line. The code suggestion block is appended if present.
function formatComment(comment, id) {
  let body = id ? `<!-- ${id} -->\n` : "";
  const badge = buildBadgeImage(comment);
  if (badge) body += `${badge}\n`;
  body += comment.content || "";
  if (comment.suggestion_code && comment.existing_code) {
    body += "\n\n**Suggestion:**\n";
    body += fencedBlock(comment.suggestion_code, "suggestion");
  }
  return body;
}

function formatCommentMarkdown(comment, error) {
  let md = "";
  // The badge renders as a leading line before the path heading (consistent
  // with formatComment), so the heading/reason lines still anchor the comment
  // and existing substring assertions on them are unaffected. The badge is ""
  // for any finding without category/severity metadata.
  const badge = buildBadgeImage(comment);
  if (badge) md += `${badge}\n`;
  md += `### 📄 \`${comment.path}\``;
  if (comment.start_line && comment.end_line) {
    md += ` (L${comment.start_line}-L${comment.end_line})`;
  }
  md += "\n\n";
  if (error) {
    md += `⚠️ GitHub could not post this as an inline comment: ${error}\n\n`;
  }
  md += comment.content || "";

  if (comment.suggestion_code && comment.existing_code) {
    md += "\n\n<details><summary>💡 Suggested Change</summary>\n\n";
    md += "**Before:**\n" + fencedBlock(comment.existing_code) + "\n\n";
    md += "**After:**\n" + fencedBlock(comment.suggestion_code) + "\n\n";
    md += "</details>";
  }
  return md;
}

// Merged summary header. All posting-outcome counts are surfaced here (and
// ONLY here) so the numbers add up to the total and the reader no longer has
// to reconcile two separately presented breakdowns (the old "posted as
// inline / posted as summary" header vs. the trailing "Posting Statistics"
// block, whose overlapping definitions made the summary hard to interpret).
//
// The five counts are mutually exclusive and, together with `inline`, sum to
// `total`:
//   inline  — comments that landed as review inline comments
//   summary — comments without line info, rendered in the summary body below
//   routed  — comments the publication policy moved from inline to summary
//             (also rendered in the body below, each tagged with its reason)
//   skipped — comments suppressed by incremental overlap filtering
//   failed  — comments that had line info but could not be posted (also
//             rendered in the body below, each tagged with its failure reason)
function buildSummaryBody({ total, inline, summary, skipped, routed = 0, failed, warnings }) {
  let body = `🔍 **OpenCodeReview** found **${total}** issue(s) in this PR.`;
  if (total > 0) {
    body += `\n- ✅ Successfully posted inline: ${inline} comment(s)`;
    if (summary > 0) {
      body += `\n- 📝 In summary (no line info): ${summary} comment(s)`;
    }
    if (routed > 0) {
      body += `\n- 📋 Routed to summary by policy: ${routed} comment(s)`;
    }
    if (skipped > 0) {
      body += `\n- ⏭️ Skipped (overlap with history): ${skipped} comment(s)`;
    }
    if (failed > 0) {
      body += `\n- ❌ Failed to post inline: ${failed} comment(s)`;
    }
  }
  if (warnings && warnings.length > 0) {
    body += `\n\n⚠️ ${warnings.length} warning(s) occurred during review.`;
  }
  return body;
}

// Pre-review summary body: shown in the anchor comment while inline comments
// are being posted. Includes only what is known before the review lands (issue
// count, warnings, comments without line info, routed comments) — final posting
// statistics are added by the finalize phase. Kept informative (not an empty
// placeholder) so the summary is useful even if the run is interrupted before
// finalize. Routed findings are known before the review (the policy decision is
// made in the partition loop) so they are rendered here too, keeping the
// pre-review anchor accurate.
function buildPreReviewSummaryBody(totalCount, summaryComments, routedComments, warnings) {
  let body = `🔍 **OpenCodeReview** found **${totalCount}** issue(s) in this PR.`;
  if (totalCount > 0) {
    body += `\n- ⏳ _Posting review comments…_`;
    if (routedComments && routedComments.length > 0) {
      body += `\n- 📋 Routed to summary by policy: ${routedComments.length} comment(s)`;
    }
  }
  if (warnings.length > 0) {
    body += `\n\n⚠️ ${warnings.length} warning(s) occurred during review.`;
  }
  body += formatSummaryComments(summaryComments);
  body += formatSummaryComments(routedComments);
  body += formatWarnings(warnings);
  return body;
}

function formatSummaryComments(summaryComments) {
  let body = "";
  for (const { comment, reason } of summaryComments) {
    body += "\n\n---\n\n";
    body += formatCommentMarkdown(comment, reason);
  }
  return body;
}

// Render the warning contents as a bulleted list under a "⚠️ Warnings" heading.
// Returns "" when there are no warnings, so callers can append unconditionally.
// OCR warning objects carry `file`, `message`, and `type`; each present field is
// surfaced so the summary shows where/why the warning happened. Plain-string
// warnings (and any unknown shape) degrade gracefully to their textual form.
function formatWarnings(warnings) {
  if (!warnings || warnings.length === 0) return "";
  let body = "\n\n---\n\n⚠️ **Warnings:**";
  for (const w of warnings) {
    body += `\n- ${formatWarningEntry(w)}`;
  }
  return body;
}

// Format a single warning as a compact bullet. Builds a `file (type): message`
// prefix from whichever of file/type are present, then appends the message.
// Missing fields are skipped so a partial warning still reads naturally.
function formatWarningEntry(w) {
  if (w == null) return "";
  if (typeof w === "string") return w;
  if (typeof w === "object") {
    const prefixParts = [];
    if (w.file != null && String(w.file) !== "") prefixParts.push(`\`${w.file}\``);
    if (w.type != null && String(w.type) !== "") prefixParts.push(`(\`${w.type}\`)`);
    const prefix = prefixParts.join(" ");
    const msg = w.message != null ? String(w.message) : "";
    if (prefix && msg) return `${prefix}: ${msg}`;
    if (msg) return msg;
    if (prefix) return prefix;
    try {
      return JSON.stringify(w);
    } catch (_) {
      return String(w);
    }
  }
  return String(w);
}

function fencedBlock(content, language = "") {
  const text = String(content || "");
  const fence = safeFence(text);
  let block = fence + language + "\n" + text;
  if (!text.endsWith("\n")) block += "\n";
  return block + fence;
}

function safeFence(content) {
  const matches = String(content || "").match(/`+/g) || [];
  const maxTicks = matches.reduce((max, ticks) => Math.max(max, ticks.length), 0);
  return "`".repeat(Math.max(3, maxTicks + 1));
}

function safeRead(fs, p) {
  try {
    return fs.readFileSync(p, "utf8");
  } catch (_) {
    return "";
  }
}

// Rate-limit cooldown + idempotency reconciliation for a FAILED batch
// createReview. Shared by the primary batch and the 422 secondary filtered
// batch so the two can never drift apart.
//
// Order matters: cool down FIRST (honoring the error's retry/rate-limit
// headers) before any further API call, including the idempotency reads.
// Firing reads immediately after a rate-limit/5xx would further pressure an
// already-struggling API; this is the same cool-down-before-read discipline
// the per-comment loop applies before isCommentAlreadyPosted.
//
// The idempotency read ("did the review land?") is only meaningful when the
// request MAY have reached the server: 5xx, 408 timeout, or a network error
// with no status. For a pure rate-limit (429 / 403 abuse) or a validation
// error (422), the request was rejected before the review was created, so it
// definitely did not land — querying would be both pointless AND an extra read
// fired during a rate-limit episode. This mirrors the per-comment
// maybeReachedServer predicate so all layers stay consistent.
//
// Returns { toRetry, alreadyPosted, reconciled, status, maybeReachedServer,
//           unverified }.
async function cooldownAndReconcile({ github, owner, repo, prNumber, log, error, items, tag, label, labelLower }) {
  const retry = computeRetryDelayMs(error, 0);
  if (retry != null) {
    const secs = (retry.delayMs / 1000).toFixed(1);
    log(
      `${label} createReview failed (HTTP ${error.status}). ` +
        `Cooling down ${secs}s via '${retry.source}' (${retry.detail}) before any retry or read.`
    );
    await sleep(retry.delayMs);
  }

  const status = error.status;
  const maybeReachedServer =
    (typeof status === "number" && (status >= 500 || status === 408)) ||
    status == null; // network errors (ECONNRESET, ETIMEDOUT, ...)

  let existingReview = null;
  if (maybeReachedServer) {
    log(`Checking whether the ${labelLower} review actually landed on the server before retrying...`);
    try {
      existingReview = await findExistingBatchReview({ github, owner, repo, prNumber, tag, log });
    } catch (checkErr) {
      const reason =
        `Could not verify whether the failed ${labelLower} review posted this comment ` +
        `(review lookup failed: ${checkErr.message})`;
      log(
        `Idempotency check failed (${checkErr.message}). The write may have landed, so ` +
          `retrying could duplicate comments; leaving ${items.length} comment(s) unverified ` +
          `and continuing to the final summary.`
      );
      return {
        toRetry: [],
        alreadyPosted: 0,
        reconciled: false,
        status,
        maybeReachedServer,
        unverified: items.map((item) => ({ item, error: reason })),
      };
    }
  } else {
    log(`${label} did not reach the server (HTTP ${status || "n/a"}); skipping idempotency check and retrying all comments.`);
  }

  // If the review landed, only retry the missing comments; otherwise retry all
  // of them. NOTE: the run tag is shared across all batches, so
  // findExistingBatchReview may match an EARLIER batch's review — harmless for
  // correctness because getPostedCommentIds returns a server-global set of
  // every fence ID across ALL reviews, and we filter these items against that
  // global set (never the set against the items). The counts below therefore
  // reflect only the items passed in.
  if (existingReview && existingReview.found) {
    // This walk goes through readAllPages -> readWithPacing -> withRetry and
    // THROWS on final failure. Letting it escape would unwind the entire run
    // and leave the summary on its pre-review body. Retrying writes is also
    // unsafe once a tagged review is known to exist: without the posted IDs we
    // cannot distinguish landed comments from missing ones. Preserve that
    // uncertainty as summary failures instead—visible, final, and duplicate-free.
    let postedIds;
    try {
      postedIds = await getPostedCommentIds({ github, owner, repo, prNumber, log });
    } catch (readErr) {
      const reason =
        `Could not verify whether the failed ${labelLower} review posted this comment ` +
        `(posted-comment read failed: ${readErr.message})`;
      log(
        `Posted-comment read failed (${readErr.message}). The tagged review exists, so ` +
          `retrying could duplicate comments; leaving ${items.length} comment(s) unverified ` +
          `and continuing to the final summary.`
      );
      return {
        toRetry: [],
        alreadyPosted: 0,
        reconciled: false,
        status,
        maybeReachedServer,
        unverified: items.map((item) => ({ item, error: reason })),
      };
    }
    if (postedIds) {
      const toRetry = items.filter((item) => !postedIds.has(item.id));
      const alreadyPosted = items.length - toRetry.length;
      log(
        `A ${labelLower} review with this run's tag exists on the server ` +
          `(review_id=${existingReview.review.id}, may belong to an earlier batch). ` +
          `${alreadyPosted}/${items.length} of this batch's inline comments already posted. ` +
          `${toRetry.length} missing, will retry only those.`
      );
      return {
        toRetry,
        alreadyPosted,
        reconciled: alreadyPosted > 0,
        status,
        maybeReachedServer,
        unverified: [],
      };
    }
  }

  log(`${label} review not found on server. Falling back to per-comment posting...`);
  return {
    toRetry: items,
    alreadyPosted: 0,
    reconciled: false,
    status,
    maybeReachedServer,
    unverified: [],
  };
}

// GitHub documents 422 on this endpoint as "Validation failed, OR the endpoint
// has been spammed" — so a bare status code is NOT evidence that a comment
// pointed at a line outside the diff. Only re-send a filtered batch when the
// error body actually names a line/diff validation problem; anything else
// (spam/abuse detection, an unrelated validation failure) falls through to the
// per-comment loop, which paces and retries on its own.
//
// OBSERVED against live GitHub (POST /repos/{o}/{r}/pulls/{n}/reviews). The
// response body is:
//
//   { "message": "Unprocessable Entity",
//     "errors": ["Line could not be resolved and Line could not be resolved"],
//     "documentation_url": "...", "status": "422" }
//
// Real `errors[]` strings seen, all of which must keep matching:
//   "Line could not be resolved"            line outside any hunk, past EOF,
//                                           negative, LEFT side on an added-only
//                                           line, or a span straddling two hunks
//   "Start position could not be resolved"  start_line > line (inverted span)
//   "Path could not be resolved"            path not in the PR at all
//
// So /could not be resolved/i is the pattern actually carrying this feature;
// the other four are defensive. Do not "simplify" them away without re-probing
// live GitHub — a wording change that matches nothing disables the grouping
// silently, and every unit test would still pass.
//
// Note also that GitHub does NOT attribute the failure to individual comments:
// two bad comments produced ONE string joined with the English word " and ".
// That is why this file filters locally against listFiles hunks instead of
// trying to parse which comment GitHub rejected.
const LINE_RESOLUTION_PATTERNS = [
  /must be part of the diff/i,
  /not part of the diff/i,
  /could not be resolved/i,
  /outside the diff/i,
  /diff hunk/i,
];
// Defensive only: this endpoint returns `errors` as an array of plain STRINGS,
// so the entry.field branch below is unreachable for createReview. Kept because
// other REST endpoints do return structured {resource, field, code} entries.
const LINE_RESOLUTION_FIELDS = new Set(["line", "start_line", "position", "original_line", "original_start_line"]);

function isLineResolutionFailure(error) {
  if (!error) return false;
  const texts = [];
  // CAREFUL: `error.response.data.message` on its own is just "Unprocessable
  // Entity" and matches no pattern below, so narrowing this function to that
  // field alone would disable the fallback entirely. Verified against live
  // GitHub.
  //
  // On the live payload TWO independent sources carry the decisive wording:
  // Octokit's composed `error.message` ("<data.message>: <errors[] entries>")
  // and the raw `errors[]` strings collected just below. Either one alone still
  // activates the gate, so neither is individually load-bearing *for that
  // shape* — but keep both: `errors[]` is absent whenever a caller re-wraps the
  // error, and `error.message` is the only source for the bare-{message} shapes
  // this endpoint produces for some validation failures.
  if (error.message) texts.push(String(error.message));

  const errors =
    (error.response && error.response.data && error.response.data.errors) ||
    (error.data && error.data.errors) ||
    error.errors;
  if (Array.isArray(errors)) {
    for (const entry of errors) {
      if (!entry) continue;
      if (typeof entry === "string") {
        texts.push(entry);
        continue;
      }
      // A structured validation error naming a line field is conclusive on its
      // own, regardless of how the message happens to be worded.
      if (entry.field && LINE_RESOLUTION_FIELDS.has(String(entry.field))) return true;
      if (entry.message) texts.push(String(entry.message));
    }
  }

  const text = texts.join(" | ");
  if (!text) return false;
  return LINE_RESOLUTION_PATTERNS.some((re) => re.test(text));
}

// Parse a unified-diff patch into the RIGHT-side (new file) line ranges it
// covers, ONE RANGE PER HUNK. Ranges — not a flat set of line numbers — because
// GitHub requires a multi-line comment's start_line and line to sit inside the
// SAME hunk; a flat set cannot tell a legal span from one that straddles two
// hunks, and the straddling span would 422 all over again.
//
// Within a hunk the new-file line numbers are contiguous from the hunk header's
// start: additions and context lines advance the counter, deletions do not (they
// exist only in the old file). So each hunk collapses to {start, end}.
function parseDiffHunkInventory(patch) {
  if (!patch) return { ranges: [], complete: false };
  const ranges = [];
  // Capture the new-file START and declared line count. A missing count means
  // one line per unified-diff syntax. The count is needed to detect a clipped
  // `patch`: treating an observed prefix as the full hunk would falsely prove
  // later lines invalid and silently route postable findings to the summary.
  const hunkHeaderRegex = /^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@/;
  const lines = String(patch).split("\n");
  let current = null;
  let sawHunk = false;
  let complete = true;

  const flush = () => {
    if (!current) return;
    if (current.observed !== current.expected) complete = false;
    // A hunk with no RIGHT-side lines at all (a pure deletion, "+N,0") never
    // advanced the counter, so end < start and there is nothing to comment on.
    if (current.end >= current.start) ranges.push({ start: current.start, end: current.end });
  };

  for (const line of lines) {
    const match = hunkHeaderRegex.exec(line);
    if (match) {
      flush();
      const start = parseInt(match[1], 10);
      const expected = match[2] == null ? 1 : parseInt(match[2], 10);
      current = { start, end: start - 1, next: start, expected, observed: 0 };
      sawHunk = true;
      continue;
    }
    if (!current) continue;

    if (line.startsWith("\\")) continue; // "\ No newline at end of file"
    if (line.startsWith("-")) continue; // deletion: old file only, does not advance
    if (line.startsWith("+") || line.startsWith(" ")) {
      current.end = current.next;
      current.next++;
      current.observed++;
    }
    // Anything else (notably a bare "" produced by a trailing newline in the
    // patch string) is not a diff body line and must not advance the counter.
  }
  flush();
  return { ranges, complete: sawHunk && complete };
}

function parseDiffHunkRanges(patch) {
  return parseDiffHunkInventory(patch).ranges;
}

// TRI-STATE classification: "valid" | "invalid" | "unknown".
//
// "invalid" is a claim we must be able to PROVE, because it permanently routes
// a finding to the summary without ever attempting to post it. Missing or
// partial diff metadata is "unknown", not "invalid" — it means we could not
// check, and the caller keeps the pre-existing per-comment behavior.
function classifyCommentAgainstDiff(item, diff) {
  // No inventory at all, or one we know is truncated: we cannot prove anything.
  if (!diff || !diff.complete) return "unknown";

  const { reviewComment } = item;
  const path = reviewComment.path;

  // File is not among the PR's changed files at all — provably outside the diff.
  if (!diff.known.has(path)) return "invalid";

  // File IS in the PR but GitHub omitted its `patch` (binary, or a diff over
  // the size limit). We know nothing about its lines.
  const ranges = diff.files.get(path);
  if (!ranges) return "unknown";

  // We only model RIGHT-side (new file) lines. The producer builds RIGHT-side
  // comments today; if that ever changes, decline to judge rather than drop.
  if (reviewComment.side && reviewComment.side !== "RIGHT") return "unknown";

  const endLine = reviewComment.line;
  if (endLine == null) return "unknown";
  const startLine = reviewComment.start_line != null ? reviewComment.start_line : endLine;

  // A reversed span is malformed and GitHub will reject it.
  if (startLine > endLine) return "invalid";

  // Both endpoints must fall inside ONE hunk.
  const withinOneHunk = ranges.some((r) => startLine >= r.start && endLine <= r.end);
  return withinOneHunk ? "valid" : "invalid";
}

// Human-readable location for the failure summary. A multi-line comment reports
// its full span, so a range that failed on start_line is not described by the
// (perfectly valid) end line alone.
function describeCommentLocation(reviewComment) {
  const endLine = reviewComment.line;
  const startLine = reviewComment.start_line;
  if (endLine == null && startLine == null) return "Line n/a";
  if (startLine != null && endLine != null && startLine !== endLine) {
    return `Lines ${startLine}-${endLine}`;
  }
  return `Line ${endLine != null ? endLine : startLine}`;
}

// Build the PR's RIGHT-side diff inventory.
//
// Returns { files, known, complete }:
//   files    Map<path, Array<{start,end}>> — paths whose patch we could parse
//   known    Set<path>                     — every path in the PR's file list
//   complete boolean                       — the file list was fully enumerated
//
// `complete` is the guard that makes "invalid" provable: a truncated walk means
// an absent path proves nothing. Pagination goes through readWithPacing so this
// read honors the same retry/pacing/quota discipline as every other read in
// this file. GitHub caps listFiles at 3000 files, hence MAX_PAGES = 30.
//
// `cache` (optional) memoizes the result for the run: with several batches each
// failing 422, the inventory would otherwise be refetched per batch.
async function getPrDiffHunks({ github, owner, repo, prNumber, commitSha, log, cache }) {
  const logFn = typeof log === "function" ? log : () => {};
  if (cache && cache.diff !== undefined) return cache.diff;

  const files = new Map();
  const known = new Set();
  let complete = true;

  const PER_PAGE = 100;
  const MAX_PAGES = 30;
  let page = 1;
  while (page <= MAX_PAGES) {
    const res = await readWithPacing(
      `listFiles (page ${page})`,
      () => github.rest.pulls.listFiles({ owner, repo, pull_number: prNumber, per_page: PER_PAGE, page }),
      logFn
    );
    const batch = (res && res.data) || [];
    for (const file of batch) {
      if (!file || !file.filename) continue;
      known.add(file.filename);
      if (file.patch) {
        const parsed = parseDiffHunkInventory(file.patch);
        if (parsed.complete) {
          files.set(file.filename, parsed.ranges);
        } else {
          logFn(
            `[422-fallback] Patch data for ${file.filename} is incomplete or malformed; ` +
              `comments on that file will be treated as unknown rather than out-of-diff.`
          );
        }
      }
    }
    if (batch.length < PER_PAGE) break;
    page++;
  }
  if (page > MAX_PAGES) {
    complete = false;
    logFn(
      `[422-fallback] PR changed-file list exceeded ${MAX_PAGES * PER_PAGE} files; ` +
        `diff inventory is incomplete, so no comment will be discarded as out-of-diff.`
    );
  }

  // An empty inventory proves nothing. A PR that produced review comments
  // necessarily has changed files, so an empty listFiles response is an anomaly
  // (diff not yet materialized server-side, or a malformed/empty response body)
  // rather than evidence that every commented path sits outside the diff.
  // Trusting it would classify EVERY comment "invalid" and discard the whole
  // batch without a single posting attempt — the exact outcome the tri-state
  // classification exists to prevent. Note this is the mirror of the truncation
  // case above: too many files and zero files are both "cannot judge".
  if (known.size === 0) {
    complete = false;
    logFn(
      `[422-fallback] PR changed-file list came back empty, which cannot be right for a PR ` +
        `under review; treating the diff inventory as incomplete, so no comment will be ` +
        `discarded as out-of-diff.`
    );
  }

  // listFiles describes the PR's current head, while createReview targets the
  // event-time commitSha. A push between those reads can move or remove a hunk:
  // the current inventory then cannot prove anything about the older commit.
  // Check the head AFTER walking listFiles so a push during pagination is also
  // detected. If it moved, make the whole inventory conservative (`unknown`).
  if (complete && commitSha) {
    const headRes = await readWithPacing(
      "getPullRequestHead",
      () => github.rest.pulls.get({ owner, repo, pull_number: prNumber }),
      logFn
    );
    const inventoryHead = headRes && headRes.data && headRes.data.head && headRes.data.head.sha;
    if (!inventoryHead || inventoryHead !== commitSha) {
      complete = false;
      logFn(
        `[422-fallback] PR head changed while resolving diff hunks ` +
          `(review commit=${commitSha}, current head=${inventoryHead || "unknown"}); ` +
          `no comment will be discarded as out-of-diff.`
      );
    }
  }

  const diff = { files, known, complete };
  if (cache) cache.diff = diff;
  return diff;
}

module.exports = {
  runPostReviewComments,
  postSummary,
  findExistingSummaryComment,
  findSummaryIssueComment,
  ensureSummaryAnchor,
  finalizeSummary,
  listExistingReviewComments,
  getAuthenticatedLogin,
  isBotComment,
  overlapsHistory,
  lineSpan,
  sameCommentSpan,
  resolveThreshold,
  DEFAULT_OVERLAP_THRESHOLD,
  computeRetryDelayMs,
  getHeader,
  logRateLimitQuota,
  parseNonNegInt,
  withRetry,
  readWithPacing,
  readAllPages,
  findExistingBatchReview,
  getPostedCommentIds,
  isCommentAlreadyPosted,
  newCommentId,
  sanitizeMetadata,
  buildBadge,
  buildPolicy,
  routeComment,
  formatComment,
  formatCommentMarkdown,
  buildSummaryBody,
  buildPreReviewSummaryBody,
  formatSummaryComments,
  formatWarnings,
  fencedBlock,
  safeFence,
  SUMMARY_MARKER,
  NO_LINE_REASON,
  resolveBatchSize,
  sortToSendDeterministically,
  chunkArray,
  setStatsOutputs,
  DEFAULT_BATCH_SIZE,
  buildRunTags,
  buildBadgeImage,
  SEVERITY_BADGE_COLOR,
  NO_ROUTING,
  CATEGORIES,
  SEVERITIES,
  SEVERITY_RANK,
  parseDiffHunkRanges,
  classifyCommentAgainstDiff,
  describeCommentLocation,
  isLineResolutionFailure,
  cooldownAndReconcile,
  getPrDiffHunks,
};
