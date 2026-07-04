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
  const runId = Number.isFinite(context.runId) ? context.runId : 0;
  const runAttempt = Number.isFinite(context.runAttempt) ? context.runAttempt : 1;
  const RUN_TAG = `${runId}-${runAttempt}`;
  const REVIEW_TAG = `<!-- ocr-review-run:${RUN_TAG} -->`;
  const SUMMARY_TAG = `<!-- ocr-summary-run:${RUN_TAG} -->`;

  const stats = {
    total: 0,
    inline: 0,
    skipped: 0,
    failed: 0,
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
      const posted = await postSummary({ github, owner, repo, prNumber, body, sticky: stickySummary });
      stats.summaryUrl = posted.url;
    }
    setStatsOutputs(out, stats);
    return;
  }

  const comments = result.comments || [];
  const warnings = result.warnings || [];
  stats.total = comments.length;

  // No comments: post a "looks good" summary.
  if (comments.length === 0) {
    const message = result.message || "No comments generated. Looks good to me.";
    const body = `${SUMMARY_MARKER}\n✅ **OpenCodeReview**: ${message}`;
    const posted = await postSummary({ github, owner, repo, prNumber, body, sticky: stickySummary });
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

  // Partition: inline (with valid line info) vs summary (without).
  // Each inline comment gets a random per-comment ID (assigned once) embedded
  // in its body as an HTML comment, so the retry/idempotency logic can detect
  // whether a comment already landed on the server and avoid posting a
  // duplicate. Random (not content-derived) so two distinct comments that
  // share path/line/content still get different IDs.
  const reviewComments = [];
  const commentsWithoutLine = [];
  for (const comment of comments) {
    const hasValidLine = comment.start_line >= 1 || comment.end_line >= 1;
    if (!hasValidLine) {
      commentsWithoutLine.push({ comment, body: formatComment(comment), reason: NO_LINE_REASON });
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
    body: wrapSummary(buildPreReviewSummaryBody(stats.total, commentsWithoutLine, warnings)),
    log,
  });

  // Submit inline comments (the to-send set) as a single PR review.
  let successCount = 0;
  let failedCount = 0;
  const failedComments = [];

  if (toSend.length > 0) {
    // The summary lives in its own issue comment (anchored above), so the
    // review body carries only the per-run REVIEW_TAG. The tag lets the
    // idempotency check locate the batch review on retry (a batch createReview
    // may have landed on the server even though we received a 5xx response).
    const reviewBody = REVIEW_TAG;

    try {
      const batchRes = await github.rest.pulls.createReview({
        owner,
        repo,
        pull_number: prNumber,
        commit_id: commitSha,
        body: reviewBody,
        event: "COMMENT",
        comments: toSend.map(({ reviewComment }) => reviewComment),
      });
      successCount = toSend.length;
      log(`Successfully posted review with ${successCount} inline comment(s) (${commentsWithoutLine.length} in summary).`);
      logRateLimitQuota(batchRes, "after batch createReview", log);
    } catch (e) {
      log(`Failed to post review with inline comments: ${e.message}`);

      // Retry/pacing configuration (shared by write and read API calls).
      // parseNonNegInt guards against nonsensical env values (negative, NaN,
      // non-numeric) that `parseInt(...) || default` would let through for
      // negative numbers, since a negative parseInt result is truthy and would
      // bypass the `|| default` fallback.
      const MAX_RETRIES = parseNonNegInt(process.env.OCR_MAX_RETRIES, 3);
      const SUCCESS_DELAY = parseNonNegInt(process.env.OCR_SUCCESS_DELAY, 2000);
      const FAILURE_DELAY = parseNonNegInt(process.env.OCR_FAILURE_DELAY, 1000);
      const LOW_REMAINING_THRESHOLD = parseNonNegInt(process.env.OCR_LOW_REMAINING_THRESHOLD, 3);
      const LOW_REMAINING_SPACING = parseNonNegInt(process.env.OCR_LOW_REMAINING_SPACING, 10000);
      // Read APIs are cheaper and have higher thresholds; use shorter pacing.
      const READ_SUCCESS_DELAY = parseNonNegInt(process.env.OCR_READ_SUCCESS_DELAY, 500);
      const READ_LOW_REMAINING_SPACING = parseNonNegInt(process.env.OCR_READ_LOW_REMAINING_SPACING, 5000);

      // Rate-limit cooldown: honor the batch error's retry/rate-limit headers
      // BEFORE any further API call — including the idempotency reads below.
      // Firing reads immediately after a rate-limit/5xx would further pressure
      // the already-struggling API; this is the same cool-down-before-read
      // discipline the per-comment loop applies before isCommentAlreadyPosted.
      const batchRetry = computeRetryDelayMs(e, 0);
      if (batchRetry != null) {
        const secs = (batchRetry.delayMs / 1000).toFixed(1);
        log(
          `Batch createReview failed (HTTP ${e.status}). ` +
            `Cooling down ${secs}s via '${batchRetry.source}' (${batchRetry.detail}) before any retry or read.`
        );
        await sleep(batchRetry.delayMs);
      }

      // The idempotency read ("did the batch land?") is only meaningful when the
      // request MAY have reached the server: 5xx, 408 timeout, or a network
      // error with no status. For a pure rate-limit (429 / 403 abuse) or a
      // validation error (422), the request was rejected before the review was
      // created, so the batch definitely did not land — querying would be both
      // pointless AND an extra read fired during a rate-limit episode. Skip it
      // and retry all comments. This mirrors the per-comment maybeReachedServer
      // predicate so the two layers stay consistent.
      const batchStatus = e.status;
      const batchMaybeReachedServer =
        (typeof batchStatus === "number" && (batchStatus >= 500 || batchStatus === 408)) ||
        batchStatus == null; // network errors (ECONNRESET, ETIMEDOUT, ...)

      let existingReview = null;
      if (batchMaybeReachedServer) {
        log("Checking whether the batch review actually landed on the server before retrying...");
        try {
          existingReview = await findExistingBatchReview({ github, owner, repo, prNumber, tag: REVIEW_TAG, log });
        } catch (checkErr) {
          log(`Idempotency check failed (${checkErr.message}). Degrading to original fallback (accepting duplicate risk).`);
        }
      } else {
        log(`Batch did not reach the server (HTTP ${batchStatus || "n/a"}); skipping idempotency check and retrying all comments.`);
      }

      // Compute the list of inline comments that still need to be posted. If
      // the batch review landed, only retry the missing ones; otherwise retry
      // all of them.
      let toRetry = toSend;
      if (existingReview && existingReview.found) {
        const postedIds = await getPostedCommentIds({ github, owner, repo, prNumber, log });
        toRetry = toSend.filter((item) => !postedIds.has(item.id));
        successCount = toSend.length - toRetry.length;
        log(
          `Batch review already exists (review_id=${existingReview.review.id}). ` +
            `${successCount}/${toSend.length} inline comments already posted. ` +
            `${toRetry.length} missing, will retry only those.`
        );
      } else {
        log("Batch review not found on server. Falling back to per-comment posting...");
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
            successCount++;
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
                successCount++;
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
                failedCount++;
                const reason = "idempotency check unavailable (read API failed)";
                failedComments.push({ comment, error: `${innerE.message} [${reason}]` });
                log(`Cannot verify whether comment for ${reviewComment.path} was posted (${reason}, HTTP ${innerE.status || "n/a"}); skipping retry to avoid duplicate.`);
                await sleep(SUCCESS_DELAY);
                break;
              }
              // Not found on server. If retries are exhausted or the error is
              // non-retryable, this is a real failure.
              if (!willRetry) {
                failedCount++;
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
              failedCount++;
              failedComments.push({ comment, error: innerE.message });
              log(`Failed to post comment for ${reviewComment.path} (non-retryable error, HTTP ${innerE.status || "n/a"}): ${innerE.message}`);
              await sleep(FAILURE_DELAY);
              break;
            }
          }
        }
      }
    }
  } else {
    log("No inline comments to post after filtering (all overlapping or none had line info).");
  }

  stats.inline = successCount;
  stats.failed = failedCount;

  // ---- Finalize the summary with the complete body ----
  // Now that the review has landed (or failed per-comment), write the final
  // summary body. All comments that did not go out as inline — whether because
  // they had no line info or because posting failed — are rendered as one
  // continuous block BEFORE the posting statistics, each carrying the reason it
  // ended up in the summary (so the reader always knows why it is here).
  let summaryBody = buildSummaryBody(stats.total, successCount, commentsWithoutLine.length + failedComments.length, warnings);
  summaryBody += formatSummaryComments(commentsWithoutLine);
  for (const { comment, error } of failedComments) {
    summaryBody += "\n\n---\n\n";
    summaryBody += formatCommentMarkdown(comment, error);
  }

  const extraStats = [];
  extraStats.push(`\n- ✅ Successfully posted: ${successCount} comment(s)`);
  if (stats.skipped > 0) {
    extraStats.push(`\n- ⏭️ Skipped (overlap with history): ${stats.skipped} comment(s)`);
  }
  if (failedCount > 0) {
    extraStats.push(`\n- ❌ Failed to post: ${failedCount} comment(s)`);
  }
  if (extraStats.length > 0) {
    summaryBody += `\n\n---\n\n📊 **Posting Statistics:**` + extraStats.join("");
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

  setStatsOutputs(out, stats);
}

function setStatsOutputs(out, stats) {
  out("comments_total", String(stats.total));
  out("comments_inline", String(stats.inline));
  out("comments_skipped", String(stats.skipped));
  out("comments_failed", String(stats.failed));
  out("summary_comment_url", stats.summaryUrl || "");
}

// ---- Summary posting (sticky vs new) ----

async function postSummary({ github, owner, repo, prNumber, body, sticky }) {
  const fullBody = `${SUMMARY_MARKER}\n${body}`;
  if (sticky) {
    const existing = await findExistingSummaryComment(github, owner, repo, prNumber);
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

async function findExistingSummaryComment(github, owner, repo, prNumber) {
  const { data: comments } = await github.rest.issues.listComments({
    owner,
    repo,
    issue_number: prNumber,
    per_page: 100,
  });
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

// Returns [start, end] inclusive line range, or null if not resolvable.
// Handles both our own reviewComment shape ({start_line, line}) and GitHub's
// historical comment shape ({start_line, line}; start_line null for single-line).
function rangeOf(c) {
  const start = num(c.start_line);
  const end = num(c.line != null ? c.line : c.end_line);
  if (start != null && end != null) return [Math.min(start, end), Math.max(start, end)];
  if (end != null) return [end, end];
  if (start != null) return [start, start];
  return null;
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

  const cap = parseInt(process.env.OCR_RETRY_MAX_DELAY, 10) || 300000;
  const base = parseInt(process.env.OCR_RETRY_BASE_DELAY, 10) || 60000;

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
  for (let attempt = 0; attempt <= MAX_RETRIES; attempt++) {
    try {
      return await fn();
    } catch (e) {
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
  // (rate limit, 5xx), isCommentAlreadyPosted and hasIssueCommentWithId catch
  // it and return null (unknown), so the caller skips retrying and creates no
  // duplicate. A truncated walk does not throw; it returns a partial set
  // silently, so isCommentAlreadyPosted returns false (definitively "not
  // posted") for any comment beyond the cap, and the retry loop will repost
  // it, producing a duplicate. This tradeoff is accepted because the trigger
  // is far outside expected usage; if that ceiling ever needs to rise, make
  // maxPages configurable.
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

// Check whether an issue comment with the given tag already exists. Used to
// avoid posting a duplicate summary comment when a previous attempt within the
// run already posted one. Returns true/false when the check succeeds, or null
// when the read API is unavailable. Returning null (rather than defaulting to
// false) lets the caller skip posting instead of silently risking a duplicate
// summary comment.
async function hasIssueCommentWithId({ github, owner, repo, issueNumber, id, log }) {
  try {
    const comments = await readAllPages("listIssueComments", (page, per_page) =>
      github.rest.issues.listComments({ owner, repo, issue_number: issueNumber, per_page, page }), log
    );
    // `id` is a run-specific HTML comment like `<!-- ocr-summary-run:0-1 -->`.
    // Match it as a literal substring: it already carries the `<!-- ... -->`
    // wrapper and is parameterized by runId+runAttempt, so ordinary user
    // content cannot trigger a false positive. (A regex of the form
    // `<!--\s*<escaped id>\s*-->` would be wrong here: `id` already includes
    // its own `<!--` / `-->` delimiters, so that pattern would require the tag
    // to be double-wrapped and never match a normally-posted summary.)
    // `id` is a run-specific HTML comment like `<!-- ocr-summary-run:0-1 -->`.
    // Match it as a literal substring: it already carries the `<!-- ... -->`
    // wrapper and is parameterized by runId+runAttempt, so ordinary user
    // content cannot trigger a false positive. (A regex of the form
    // `<!--\s*<escaped id>\s*-->` would be wrong here: `id` already includes
    // its own `<!--` / `-->` delimiters, so that pattern would require the tag
    // to be double-wrapped and never match a normally-posted summary.)
    return comments.some((c) => (c.body || "").includes(id));
  } catch (e) {
    log(`[listIssueComments] check failed (${e.message}); treating as unknown to avoid duplicates.`);
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

// ---- Formatting helpers (ported verbatim) ----

// Assemble the visible comment body. When `id` is provided (inline comments),
// the per-comment ID tag is prepended as an HTML comment (invisible when
// rendered) so getPostedCommentIds can match it back on retry for the
// idempotency check. The code suggestion block is then appended if present.
function formatComment(comment, id) {
  let body = id ? `<!-- ${id} -->\n` : "";
  body += comment.content || "";
  if (comment.suggestion_code && comment.existing_code) {
    body += "\n\n**Suggestion:**\n";
    body += fencedBlock(comment.suggestion_code, "suggestion");
  }
  return body;
}

function formatCommentMarkdown(comment, error) {
  let md = `### 📄 \`${comment.path}\``;
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

function buildSummaryBody(totalCount, inlineCount, summaryCount, warnings) {
  let body = `🔍 **OpenCodeReview** found **${totalCount}** issue(s) in this PR.`;
  if (totalCount > 0) {
    body += `\n- ✅ ${inlineCount} posted as inline comment(s)`;
    body += `\n- 📝 ${summaryCount} posted as summary`;
  }
  if (warnings.length > 0) {
    body += `\n\n⚠️ ${warnings.length} warning(s) occurred during review.`;
  }
  return body;
}

// Pre-review summary body: shown in the anchor comment while inline comments
// are being posted. Includes only what is known before the review lands (issue
// count, warnings, comments without line info) — final posting statistics are
// added by the finalize phase. Kept informative (not an empty placeholder) so
// the summary is useful even if the run is interrupted before finalize.
function buildPreReviewSummaryBody(totalCount, summaryComments, warnings) {
  let body = `🔍 **OpenCodeReview** found **${totalCount}** issue(s) in this PR.`;
  if (totalCount > 0) {
    body += `\n- ⏳ _Posting review comments…_`;
  }
  if (warnings.length > 0) {
    body += `\n\n⚠️ ${warnings.length} warning(s) occurred during review.`;
  }
  body += formatSummaryComments(summaryComments);
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
  rangeOf,
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
  hasIssueCommentWithId,
  newCommentId,
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
};
