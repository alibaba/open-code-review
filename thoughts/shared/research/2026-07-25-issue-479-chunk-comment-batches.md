---
date: 2026-07-25T15:22:19+0000
researcher: ZCode
git_commit: c9b145635c6b6343b108941c2a627ac636836c6b
branch: main
repository: alibaba/open-code-review
topic: "Issue #479 — chunk reusable-Action review comments into deterministic bounded batches; prioritize local testability"
tags: [research, codebase, github-action, batch-publish, idempotency, local-testability]
scale: small
status: complete
last_updated: 2026-07-25
last_updated_by: ZCode
---

# Research: Issue #479 — Bounded comment batches in the reusable Action

## Research Question
Verbatim (from user): *"Run it parallely for issues #409, #479 and #59. mAKE SURE PRIORITISE ONES WHICH YOU CAN TEST LOCALLY. Drop if that is not the case."*

This document covers **#479** in isolation. Issue #479 ([link](https://github.com/alibaba/open-code-review/issues/479)): *"Chunk reusable-Action review comments into deterministic bounded batches."* A production run tried to publish 71 inline candidates as one review request → GitHub Server Error after partial success; the integrating workflow summary then reported only 50.

**Local-testability verdict: FULLY testable offline (highest confidence of the three).** The bug site is a Node script with an injected-mock test harness; no LLM, no network. Two documented caveats (untested surfaces): a new `action.yml` input has no YAML test coverage, and the mock's call-index contract needs rework for N>1 batch calls.

## Summary
- The bug site is `scripts/github-actions/post-review-comments.js:202-210`: a single `github.rest.pulls.createReview` call posts **ALL** `toSend` inline comments at once (`comments: toSend.map(({ reviewComment }) => reviewComment)`). `[V]`
- **No chunking exists** anywhere — confirmed no `slice`/`chunk`/pagination of `toSend`; the only `Math.ceil` calls are in rate-limit delay formatting. `[V]`
- A sophisticated reconciliation/idempotency path already handles partial success (`findExistingBatchReview`, `getPostedCommentIds`, per-comment fallback) — any batching change must preserve it, not replace it. `[V]`
- The test harness is fully offline: `makeGithub()` (`post-review-comments.test.js:84-251`) injects a mock `github.rest.pulls.createReview`, simulates 502/429/422, and asserts `createReviewCalls.length` / `.comments`. Existing tests already cover partial-success scenarios. `[V]`
- This issue is unclaimed (filer `acoliver` has zero open PRs in this repo) and has CONTRIBUTOR (`stay-foolish-forever`) engagement with design settled ("sequential batches, each reconciled before the next, sufficient"). `[V]`

## Detailed Findings

### The bug site (cited by #479)
- `scripts/github-actions/post-review-comments.js:194-213` — the batch-publish block:
  ```
  if (toSend.length > 0) {
    const reviewBody = REVIEW_TAG;   // "ocr-review-run:{runId}-{runAttempt}"
    try {
      const batchRes = await github.rest.pulls.createReview({
        owner, repo, pull_number: prNumber, commit_id: commitSha,
        body: reviewBody, event: "COMMENT",
        comments: toSend.map(({ reviewComment }) => reviewComment),  // ALL in ONE call
      });
      successCount = toSend.length;
  ```
  `[V]` — read directly.
- **No batching**: only one `createReview` call carries all of `toSend`; there is no loop, slice, or `batchSize` cap. The catch block (line 214+) handles the error via reconciliation, not chunking. `[V]`

### Existing reconciliation / idempotency (must be preserved)
- `REVIEW_TAG` = `ocr-review-run:{runId}-{runAttempt}` is the batch review body — lets the idempotency check locate the batch review on retry even after a 5xx (`:199`, `:942`). `[V]`
- Per-comment identity: `newCommentId(runTag)` → `ocr-{runTag}-{8 random hex bytes}`, embedded as an HTML fence `<!-- ocr-... -->` in each comment (`:1007`, regex `:964`). `[V]`
- `findExistingBatchReview` paginates `listReviews` and matches by tag (`:937-947`). `[V]`
- `getPostedCommentIds` paginates `listReviewComments` and extracts already-posted fence IDs (`:953-973`). `[V]`
- Per-comment fallback loop (`:288-300`): after a batch failure, re-posts only comments whose ID is not already on the server, one per `createReview`. `[V]`
- Error classification: `computeRetryDelayMs` (`:751-813`) treats 429 / 403-rate-limit and 5xx/408 as retriable; 422 (line-unresolvable) is per-comment. `[V]`

### Test harness (offline, mock-injected)
- `scripts/github-actions/post-review-comments.test.js` — bespoke Node tests, run directly with `node` (no jest). Test entry: `npm run test:github-actions` (`package.json` scripts = `node scripts/github-actions/post-review-comments.test.js && node .../check-translation-sync.test.js`). `[V]`
- `makeGithub(opts)` (`test:84-251`) builds a mock: records `createReviewCalls`, `issueComments`, `updatedComments`, list-call counters; `bulkErrorSpec` simulates the batch-call error; `perCommentError` simulates per-comment errors; `individualErrorStatus` for 422. `[V]`
- The mock **dispatches by call index**: index 0 = the batch call (may throw `bulkErrorSpec`), index ≥ 1 = per-comment fallback (`test:142-157`). `[V]`
- Existing partial-success test cases (assert on call counts and posted IDs): `testBatchLandedRetriesOnlyMissingComments` (`test:578-612`), `testBatchRateLimitWithPartialInvalidContent` (`test:867-907`), `testBatchLandedWithPerCommentPartialInvalid` (`test:949-981`), `testBatchLandedWithPerCommentMixedStates` (`test:987-1024`). `[V]`
- Delays zeroed for speed: `OCR_MAX_RETRIES=0`, `OCR_*_DELAY=0`, `OCR_RETRY_MAX_DELAY=1`, `OCR_RETRY_BASE_DELAY=1` (`test:21-29`). `[V]`

### Config / env knobs
- Env (parsed via `parseNonNegInt`, defaults): `OCR_MAX_RETRIES=3`, `OCR_SUCCESS_DELAY=2000`, `OCR_FAILURE_DELAY=1000`, `OCR_LOW_REMAINING_THRESHOLD=3`, `OCR_LOW_REMAINING_SPACING=10000`, `OCR_RETRY_MAX_DELAY=300000`, `OCR_RETRY_BASE_DELAY=60000` (`:222-229`, `:763-764`). `[V]`
- Action inputs in `action.yml`: `incremental` and `incremental_overlap_threshold` exist (`action.yml:75-87`); **no `batch_size` / `chunk_size` input exists today**. `[V]`

### Issue status
- Filer `acoliver` has **zero open PRs** in this repo (`gh search prs --author acoliver` → empty); no assignee; no open PR cross-references the issue. CONTRIBUTOR `stay-foolish-forever` engaged and set scope expectations; design discussion resolved to "sequential batches, each reconciled before the next." `[V]`
- #479 is part of a quartet (#476/#477/#478/#479) filed by `acoliver` from an external fork (`vybestack/llxprt-code`); sibling #477 has open PR #481. #479 is the cleanest of the four to take independently. `[V]`

## Implicit Spec — invariants any change here must uphold
> Requirements, not designs.

- **Bounded batch size** — inline-comment publication must send comments in deterministic batches of ≤ N per `createReview` call, with N configurable. **Why observable**: one run posted 71 candidates in one request → GitHub Server Error after partial success; the workflow summary reported only 50 (`post-review-comments.js:202-210` `[V]`; issue body). Edge: N ≥ 1; `toSend` shorter than N → single batch (current behavior); empty `toSend` → no call (unchanged).
- **Idempotency across batch boundaries** — a partial-success (5xx after some chunks landed) must not double-post; reconciliation must dedup against already-posted comment IDs **per chunk**, not only the first batch. **Why**: the existing `REVIEW_TAG` / `getPostedCommentIds` / `newCommentId` fence (`:937`/`:953`/`:1007`) must remain correct when N>1 batches exist. Concurrency/retry: the per-comment fallback loop (`:288-300`) must still run for any chunk that fails; a retry must not repost IDs GitHub already accepted.
- **Deterministic ordering** — batch composition and order must be deterministic so a partial success + retry is reproducible and dedup-able. Edge: identical re-run produces identical batches.
- **Non-destructive / fail-open** — a batching failure must not drop or destroy already-collected findings. (Sibling issue #478 is the dedicated fail-open request; a #479 change must not regress it.)
- **Bounding assumptions**:
  - The chunk count N and any new `action.yml` input are a **configuration surface**, not a code invariant; `action.yml` has **no automated YAML test coverage** today — a new input is validated manually, not by the Node test suite. `[V]`
  - The existing mock's call-index dispatch contract ("index 0 = batch, index ≥ 1 = per-comment") will need updating to express N>1 batch calls; today it cannot represent multi-batch scenarios. `[V]`
  - Cross-chunk idempotency on a **real GitHub server** (real review IDs, partial landing across chunk boundaries) cannot be proven offline — the mock fabricates posted-comment IDs from in-memory arrays. Local tests prove batching + reconciliation logic only. `[V]`

## Evidence Ledger
| Claim | Evidence | Trust | Load-bearing |
|---|---|---|---|
| All inline comments posted in one createReview call | `scripts/github-actions/post-review-comments.js:202-210` | V | yes |
| No existing chunk/slice logic on toSend | `post-review-comments.js` (only `Math.ceil` at :789,:810 for delays) | V | yes |
| Mock harness is offline; tests assert call counts | `post-review-comments.test.js:84-251,315,578,867,987` | V | yes |
| Reconciliation/idempotency exists (must be preserved) | `post-review-comments.js:288-300,937,953,1007` | V | yes |
| action.yml has no batch-size input (untested surface) | `action.yml:75-87` | V | yes |
| Mock dispatches by call index (coupling caveat) | `post-review-comments.test.js:142-157` | V | yes |
| Filer acoliver has zero open PRs; CONTRIBUTOR engaged | `gh search prs --author acoliver` empty; `gh issue view 479` | V | yes |
| Tests run via `node` (no jest); npm script wired | `package.json` scripts; `test:21-29` | V | yes |
| Error classification: 429/403-RL/5xx/408 retriable; 422 per-comment | `post-review-comments.js:751-813` | V | no |

## Architecture Insights
- The reusable Action is a bespoke Node script with injected-mock tests — fully separable from the Go CLI core. No Go build is involved.
- Idempotency is comment-ID-fenced (`<!-- ocr-... -->`) and tag-anchored (`REVIEW_TAG`); the established pattern is "write optimistic, reconcile pessimistically via reads." Batching extends this pattern, not replaces it.
- Status: a fresh agent (audit) confirmed all claims CONFIRMED; the only corrections were to bounding assumptions (action.yml untested; mock coupling) — baked into the spec above.

## Coverage & Open Questions
- **Searched**: `scripts/github-actions/post-review-comments.js` end-to-end (publish path, reconciliation, error classification, config parsing); its test file (mock structure, existing partial-success cases); `action.yml` inputs; `package.json` test wiring; issue status (assignee, PRs, filer's PR history).
- **Residual risk / deliberately bounded**:
  - Cross-chunk idempotency against a real GitHub API is untestable offline; local tests prove the dedup/reconciliation logic, not real-API partial landing across chunk boundaries.
  - A new `action.yml` input has no YAML test harness — validated manually.
  - The mock's call-index contract must be reworked for N>1 batch calls; today's tests cannot express multi-batch failure scenarios without that change.
  - Freshness: verified at commit `c9b1456`; maintainer triage is heavy and recent, so re-check assignee/PR status before starting work.
- This issue is **not dropped** under the local-testability criterion — it has the strongest offline-test surface of the three (mock-injected Node, no LLM, no network).
