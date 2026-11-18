# Pull request admission

This policy protects review capacity and automated AI review resources. It does
not assess patch quality, prohibit AI assistance, or target particular contributors.

## Rules

- Each contributor may have at most **10 open PRs**, including drafts, across all
  target branches. Closed and merged PRs do not count. Authors are matched by
  stable GitHub user ID, not display name or login.
- The limit applies to maintainers and outside contributors alike. Only explicitly
  listed automation accounts are exempt. Initially this is Dependabot; both its
  bot identity and configured numeric ID and login must match. Other bots are
  subject to the same cap as people.
- Admission runs on `opened` and `reopened` events. It can close only the PR that
  triggered that run. There is no rollout sweep or scheduled cleanup of old PRs.
- New PRs are ordered by creation time, then PR number. A delayed check does not
  reject an earlier new PR just because newer submissions pushed the count over
  ten. Reopening consumes a slot: a reopened PR is closed again if the author's
  total, including that PR, exceeds ten. If a PR has since been reopened, a delayed
  or retried `opened` run skips both moderation and AI review; the `reopened` event
  is responsible for its admission instead.
- Rejected PRs receive an explanatory comment and are closed, not locked.
  Accounts are never blocked. After other PRs are merged or closed, authors can
  reopen a rejected PR when a slot is available. The bot reuses its own policy
  comment rather than posting another on every attempt. Comment creation does
  not retry automatically after an ambiguous API failure; a workflow rerun first
  checks whether the comment was already published.

Thresholds and the automation allowlist live in
[`pr-admission.json`](pr-admission.json). Do not exempt an account merely because
its type is `Bot` or because its author has repository write permission.

## Workflow and security

`OpenCodeReview PR Review` runs the lightweight `PR Admission` job first. The
`code-review` job, including access to LLM credentials, starts only if admission
succeeds and explicitly outputs `allowed=true`. Reopened PRs can therefore receive
a review after being admitted. Query errors fail admission without closing PRs
or allowing AI review; a maintainer can rerun the failed workflow after resolving
the error. Comment or closure errors also fail the job and do not allow review.

The privileged `pull_request_target` job checks out the repository's default
branch with credential persistence disabled. It never checks out a PR head,
executes PR content, or installs dependencies. It uses only `contents: read` and
`pull-requests: write` on the regular `GITHUB_TOKEN`; no extra PAT is needed.
PRs and comments are read using paginated REST APIs, not the search index. Admission
requires two consecutive matching open-PR snapshots within at most three scans;
an unstable list fails without moderation or AI review. This catches page shifts
caused by concurrent closures without retrying indefinitely.

Admission jobs queue by author ID with cancellation disabled and `queue: max`.
This serializes moderation and comment updates without keeping the author's
queue occupied during a potentially long AI review. GitHub allows at most 100
pending jobs per concurrency group; excess jobs may be canceled and need a
manual rerun. Canceled or failed admission never starts its dependent AI review.
The GitHub API is not transactional: counts reflect the snapshot read by the
job, and a contributor can retry after concurrent merges or closures free slots.

Other workflows are independent. This gate does **not** prevent ordinary CI from
starting, stop an AI review already in progress, or prevent someone from opening
PRs using another account. It does not automatically reopen rejected PRs.

## Activation, verification, and recovery

Merge the policy, script, tests, and workflow together into the default branch.
No existing PR is closed just by deployment. Verify the next genuine new PR's
`PR Admission` result and that rejected PRs have a skipped `code-review` job.

Run the local unit and workflow-contract tests without contacting GitHub:

```sh
node scripts/github-actions/check-pr-admission.test.js
npm run test:github-actions
```

The tests simulate GitHub responses and mutations; they do not certify hosted
event delivery, token permissions, or queue behavior. For recovery, free a slot
and reopen the PR, or rerun a failed admission job after fixing the API error.
To roll back, revert the policy change or disable `OpenCodeReview PR Review` in
Actions; disabling that workflow also disables automatic AI review.
