# OpenCodeReview - Gerrit CI Demo

This demo shows how to integrate OpenCodeReview into a [Gerrit](https://www.gerritcodereview.com) code-review flow to automatically review changes and post the findings as inline comments on the patchset, plus a summary message.

Like the GitHub Actions, GitLab CI, and GitFlic examples, the posting glue lives in the CI layer rather than in the `ocr` binary. Here it is a small, dependency-free Python script — [`post_review.py`](post_review.py) — that reads `ocr review --format json` and posts it as **one batched ReviewInput** to Gerrit's set-review endpoint, so a review lands atomically: inline comments grouped per file plus a summary message, in a single request.

## How It Works

```
Gerrit Trigger (patchset-created) → Jenkins job → ocr review --format json → post_review.py → POST /a/changes/{change}/revisions/{revision}/review
```

1. The Gerrit Trigger plugin fires a Jenkins job on `patchset-created`
2. The job fetches the change ref and runs `ocr review --format json --audience agent`
3. `python3 post_review.py` reads the JSON and POSTs a single `ReviewInput` containing:
   - **Inline comments** on the changed lines, grouped per file
   - **A summary message** with the totals (plus any comments that could not be placed inline)

The script handles the Gerrit-specific wrinkles for you: preemptive HTTP basic auth on `/a/` endpoints, the `)]}'` anti-XSSI prefix on responses, a 200 that is actually an HTML login page (treated as a configuration error), and an HTTP 400 on the batch (retried once with all inline comments folded into the summary message so findings still reach the change).

## Setup

### 1. Create a bot account and HTTP password

Create a dedicated Gerrit account for the bot (its name appears on the review comments). Log in as that account and generate a token under **Settings → HTTP Credentials → Generate Password**.

> **Important:** this HTTP password is **not** the account/login password — using the login password is the most common cause of HTTP 401 from `/a/` endpoints. `post_review.py` says so explicitly when Gerrit rejects the credentials.

### 2. Grant minimal permissions

The bot only needs to read changes and comment on them. In the project's (or `All-Projects`') access settings, grant the bot's group **Read** on `refs/*` — commenting on open changes requires nothing more. Do not grant `Label: Code-Review` voting rights unless you enable label voting (see Notes).

### 3. Configure the pipeline

Copy `post_review.py` into your repository (or fetch it in the job) and wire it into your Jenkins job — see the [`Jenkinsfile`](Jenkinsfile) in this directory. Store `GERRIT_HTTP_USER` / `GERRIT_HTTP_PASSWORD` as Jenkins credentials, plus the LLM API token for the review step itself. `ocr` resolves `OCR_LLM_URL` / `OCR_LLM_TOKEN` / `OCR_LLM_MODEL` directly from the environment — no `ocr config set` needed, so the token is never written to disk on a shared agent.

## Configuration Reference

Every value can be passed via flag or environment variable; **flags override the environment**. Run `python3 post_review.py -h` for the same list.

| Flag | Env fallback | Default | Description |
|------|--------------|---------|-------------|
| `--gerrit-url` | `GERRIT_URL`, else derived from `GERRIT_CHANGE_URL` | — | Gerrit base URL (context path kept, trailing slash tolerated) |
| `--change` | `GERRIT_CHANGE_NUMBER` | — | Change number |
| `--revision` | `GERRIT_PATCHSET_REVISION` | `current` | Revision/patchset to comment on |
| `--user` | `GERRIT_HTTP_USER` | — | HTTP credentials username |
| `--password` | `GERRIT_HTTP_PASSWORD` | — | Gerrit HTTP password, **not** the account password |
| `--input` | — | `-` (stdin) | Review result JSON file (`-` = stdin); also accepted as a positional argument, which wins if both are given |
| `--dry-run` | — | off | Print the ReviewInput instead of posting it |
| `--timeout` | — | `30` | HTTP timeout in seconds |

`GERRIT_CHANGE_URL` derivation: modern change URLs (`{base}/c/{project}/+/{number}`) split at `/c/`, which keeps any context path; legacy URLs (`{base}/{number}`) drop the trailing number.

## Usage

### Jenkins (Gerrit Trigger)

See the [`Jenkinsfile`](Jenkinsfile) in this directory. The Gerrit Trigger plugin injects `GERRIT_CHANGE_NUMBER`, `GERRIT_PATCHSET_REVISION`, and `GERRIT_CHANGE_URL` into the build environment, so the script needs no positional wiring:

```bash
ocr review --format json --audience agent | python3 post_review.py
```

### Other triggers

The script is trigger-agnostic: anything that can set the environment variables above (or pass the flags) can drive it.

- **Zuul** — the same env contract can be set in a Zuul job from `zuul.change` / `zuul.patchset` variables; the script does not care who exported them.
- **`patchset-created` hook** — post directly from a server-side hook:

  ```bash
  ocr review --format json | python3 post_review.py \
    --gerrit-url https://gerrit.example.com --change "$CHANGE" --revision "$REVISION"
  ```

## Dry Run

Test the posting step locally without touching the change (no credentials required):

```bash
ocr review --from origin/main --to HEAD --format json > /tmp/r.json
python3 post_review.py /tmp/r.json --dry-run     # or: --input /tmp/r.json
```

`--dry-run` prints the exact ReviewInput that would be POSTed:

```json
{
  "message": "OpenCodeReview found 2 issue(s) in 1 file(s) reviewed; 2 posted as inline comment(s).",
  "tag": "autogenerated:opencodereview",
  "notify": "OWNER",
  "omit_duplicate_comments": true,
  "comments": {
    "internal/scan/scan.go": [
      { "message": "**Severity:** high · ...", "unresolved": true, "line": 42 },
      ...
```

## Notes & Limitations

- **Re-reviews and duplicates** — the ReviewInput sets `omit_duplicate_comments`, but Gerrit only drops **byte-identical** comments at the same location. A fresh LLM run usually rewords its findings, so re-triggered reviews **will** repeat comments. All bot comments carry the tag `autogenerated:opencodereview`, so UIs and scripts can filter or collapse them.
- **Retries** — the POST is retried (up to `MAX_ATTEMPTS`, with exponential backoff) only on failures that provably did **not** apply the review: HTTP 5xx and pre-response connection errors (refused/reset/DNS). Read timeouts are **not** retried — they are ambiguous (the server may have applied the review) and `omit_duplicate_comments` does not dedupe the summary `message`, so a retry could post a duplicate change message. 4xx responses (400/401/404/409) are not retried; they are classified and handled directly.
- **Native fix suggestions and label voting** — intentional future options. `fix_suggestions` (Gerrit's native apply-a-fix) is experiment-gated in Gerrit 3.10+ and needs character-level ranges this example does not carry, so suggested code is emitted under a `**Suggestion:**` heading in a plain fenced code block, which renders portably on any Gerrit version. Label voting (Code-Review ±1) is likewise left out of v1 — see **Label voting** below.
- **Label voting** — deliberately not in v1: the script comments, it does not vote. Teams that want gating can add one line to the ReviewInput in `build_review_input`: `"labels": {"Code-Review": -1}` (and grant the bot the label permission).
- **Revision race** — always pass the trigger-injected revision SHA (`GERRIT_PATCHSET_REVISION`). The `current` default is for manual runs: if a new patchset lands mid-pipeline, `current` would retarget the comments onto code the review never saw.
- **Exit codes** — `0` on success (HTTP 409 change-closed is tolerated and also exits 0); `1` when the input JSON is unreadable or unparsable; `2` on configuration or HTTP errors, so the CI step fails visibly. The 409 handling is defensive: modern Gerrit (verified on 3.14) accepts label-free reviews even on abandoned changes, so it matters mainly for older/stricter servers.

## Tests

`post_review.py` ships with [`post_review_test.py`](post_review_test.py) — standard-library `unittest`, no network or git required:

```bash
cd examples/gerrit_ci
python3 post_review_test.py
```
