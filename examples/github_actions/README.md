# OpenCodeReview - GitHub Actions Workflow

This directory provides a ready-to-use GitHub Actions workflow demo that integrates OpenCodeReview into your repository to automatically review Pull Requests and post inline review comments. Copy it into `.github/workflows/` and configure the required secrets/vars.

## Quick Start: `ocr-review.yml`

The simplest adoption path: this demo delegates every step — checkout, OCR install, review, comment posting, artifact upload — to the official reusable composite action at [`action.yml`](../../action.yml) via a single `uses: alibaba/open-code-review@main` step. It covers both automatic PR review (`pull_request_target: opened/synchronize/reopened`) and on-demand re-review via comments (`/open-code-review` or `@open-code-review`). No inline scripts to maintain — `@main` always runs the latest action; see [Reproducible pinning](#reproducible-pinning) when you need runs to be repeatable.

```bash
mkdir -p .github/workflows
cp ocr-review.yml .github/workflows/ocr-review.yml
```

The core of the demo is a single action step:

```yaml
- uses: alibaba/open-code-review@main
  with:
    llm_url: ${{ secrets.OCR_LLM_URL }}
    llm_auth_token: ${{ secrets.OCR_LLM_AUTH_TOKEN }}
    llm_model: ${{ vars.OCR_LLM_MODEL }}
    llm_use_anthropic: ${{ vars.OCR_LLM_USE_ANTHROPIC }}
```

See [`action.yml`](../../action.yml) for the full list of inputs, outputs, security guidance, and the four comment-posting modes (sticky summary + incremental).

## Reproducible pinning

The Action is an orchestrator: it installs the OCR CLI from npm at run time (`ocr_version`, default `latest`). Pinning only the Action reference therefore does **not** freeze review behavior — a new CLI release still changes what runs. For a fully reproducible setup, pin both:

```yaml
- uses: alibaba/open-code-review@<full-commit-sha> # vX.Y.Z
  with:
    ocr_version: 'X.Y.Z'
    llm_url: ${{ secrets.OCR_LLM_URL }}
    llm_auth_token: ${{ secrets.OCR_LLM_AUTH_TOKEN }}
    llm_model: ${{ vars.OCR_LLM_MODEL }}
    llm_use_anthropic: ${{ vars.OCR_LLM_USE_ANTHROPIC }}
```

Take the commit SHA from the [releases page](https://github.com/alibaba/open-code-review/releases) and keep the `# vX.Y.Z` comment next to it so update tooling (Dependabot, Renovate) can track it. Every action referenced *inside* [`action.yml`](../../action.yml) is itself pinned to a full commit SHA (enforced by `scripts/verify-action-pins.sh` in CI), so the outer SHA transitively freezes the whole workflow — only the two coordinates above are yours to choose.

## Running on a self-hosted runner

The demo above runs on GitHub-hosted runners (`runs-on: ubuntu-latest`) and pulls the action from `alibaba/open-code-review@main`. If you prefer to run OCR on your own self-hosted runner — to reach private network resources, keep LLM traffic on-prem, or avoid runner-minute costs — the OCR project itself does exactly this in its own CI.

See [`.github/workflows/ocr-review.yml`](../../.github/workflows/ocr-review.yml) for that workflow. It runs on `runs-on: self-hosted` inside a `node:24` container. One important caveat: it invokes the action with `uses: ./` only because `action.yml` lives in that same repository — that is an internal shortcut and will not resolve in your repo. As an external user, keep `uses: alibaba/open-code-review@main` (the runner fetches the action automatically); only the runner environment needs to change. What is worth borrowing from it:

- `runs-on: self-hosted`, optionally with a `container:` image such as `node:24` (the action needs Node.js; git is installed automatically if missing).
- Marking the workspace as a trusted git `safe.directory` when running inside a container (e.g. `git config --global --replace-all safe.directory '*'`) to avoid "dubious ownership" errors. Use `--replace-all` (not `--add`) so repeated runs across multiple self-hosted actions replace rather than accumulate entries in the global git config.
- Pinning action inputs explicitly (`sticky_summary`, `incremental`, `upload_artifacts`, `llm_extra_body`, etc.).

The action performs its own full `fetch-depth: 0` checkout of the PR internally, so no extra checkout step is needed for the review diff. Adapt the runner settings to your environment and secret layout.

## How It Works

```
PR Created/Updated → GitHub Actions Triggered → OCR Reviews Diff → Comments Posted on PR
     OR
Comment with trigger keyword ↗
```

1. When a PR is opened, the workflow triggers (uses `pull_request_target` for fork secret access).
2. Alternatively, when a comment containing `/open-code-review` or `@open-code-review` is posted on a PR, the workflow triggers.
3. The reusable action installs OCR, fetches the PR head blobs, computes `git merge-base`, and runs `ocr review --from <merge-base> --to <head> --format json`.
4. It parses the JSON output and posts inline review comments on the PR via the Pull Request Review API, plus a summary comment (an issue comment on the PR).

## Setup

### Configure secrets and variables

Go to your repository's **Settings → Secrets and variables → Actions**.

**Secrets:**

| Secret | Required | Description |
|--------|----------|-------------|
| `OCR_LLM_URL` | Yes | LLM API endpoint URL (e.g., `https://api.openai.com/v1/chat/completions`) |
| `OCR_LLM_AUTH_TOKEN` | Yes | API authentication token (mapped to env `OCR_LLM_TOKEN` internally) |

**Variables:**

| Variable | Required | Description |
|----------|----------|-------------|
| `OCR_LLM_MODEL` | Yes | Model name |
| `OCR_LLM_USE_ANTHROPIC` | Yes | `true` for Anthropic Claude, `false` for OpenAI-compatible |

> **Note:** `GITHUB_TOKEN` is automatically provided by GitHub Actions with the required `pull-requests: write` permission. By default the action sets `llm.extra_body` to `{"thinking": {"type": "disabled"}}`, disabling thinking mode for compatibility with various LLM providers; override it with the `llm_extra_body` input when your model needs different behavior, or use `llm_reasoning_effort` to steer reasoning depth on OpenAI-compatible protocols.

## Customization

> These knobs are action inputs — they apply to the demo workflow and any workflow calling `alibaba/open-code-review@main`.

See [`action.yml`](../../action.yml) for the full input list. Workflow-level settings (triggers, keywords) are edited in the workflow file itself.

### Change the trigger events

Modify the `on.pull_request_target.types` array in the workflow file:

```yaml
on:
  pull_request_target:
    types: [opened, synchronize, reopened, ready_for_review]
```

### Customize comment trigger keywords

By default the workflow also re-reviews on demand when a PR comment starts with `/open-code-review` or `@open-code-review`. The `if` condition is more defensive than a bare keyword check — it gates comment triggers so only authorized humans can spend LLM quota:

```yaml
if: |
  github.event_name == 'pull_request_target'
  || (
    github.event_name == 'issue_comment'
    && github.event.issue.pull_request
    && github.event.comment.user.type != 'Bot'
    && (
      github.event.comment.author_association == 'MEMBER'
      || github.event.comment.author_association == 'OWNER'
      || github.event.comment.author_association == 'COLLABORATOR'
    )
    && (
      startsWith(github.event.comment.body, '/open-code-review')
      || startsWith(github.event.comment.body, '@open-code-review')
    )
  )
```

Each clause guards against a different abuse vector:

- `github.event.issue.pull_request` — the comment must be on a PR, not a regular issue.
- `github.event.comment.user.type != 'Bot'` — ignore bot comments. `GITHUB_TOKEN` already suppresses events from comments it posted, but a PAT or GitHub App token would not, so this is a safety net against self-triggering loops.
- `author_association == 'MEMBER' | 'OWNER' | 'COLLABORATOR'` — only repository collaborators can trigger a (billable) re-review, preventing arbitrary commenters from draining LLM quota.
- The `startsWith(...)` pair — the actual trigger keywords.

To change the keywords, edit only that final pair (e.g. `/review` and `@mybot`), or swap `startsWith` for `contains` to match a substring anywhere in the comment body. Keep the preceding guards intact.

The same predicate is mirrored in the workflow's `concurrency.group`: matching events share a per-PR group (`ocr-<pr_number>`) so a new review cancels any stale one, while non-matching comments land in a unique `noop-<run_id>` group and are skipped instantly without disrupting a running review. If you change the keywords in `if`, mirror the change in `concurrency.group` too.

### Use a specific OCR version

The action requires `ocr_version` 1.9.6 or newer because its secure token configuration uses
`llm.auth_token_cmd`. The action fails early with a version-specific error when an older or
unverifiable CLI is installed.

```yaml
- uses: alibaba/open-code-review@main
  with:
    ocr_version: 1.9.10
```

### Configure review and LLM timeouts

The task and request timeouts are independent:

| Input | Default | Description |
|-------|---------|-------------|
| `review_task_timeout` | `'15'` | Per-file/concurrent-task timeout in minutes passed to `ocr review --timeout`; it is not a whole-review wall-clock cap. |
| `llm_timeout` | `'300'` | LLM HTTP request timeout in seconds, applied independently to each model request. |

```yaml
- uses: alibaba/open-code-review@main
  with:
    review_task_timeout: '30'
    llm_timeout: '900'
```

### Control review effort and token budget

| Input | Default | Description |
|-------|---------|-------------|
| `effort` | `''` | Review effort preset passed to `ocr review --effort`: `low`, `medium`, or `high` (case-insensitive). Higher effort runs more review rounds. Empty keeps the CLI default (the configured value, or medium). |
| `max_tokens_budget` | `''` | Total token cap (input+output) passed to `ocr review --max-tokens-budget`. Empty or `'0'` means unlimited. Once the cap is exceeded, dispatch stops, skipped files are reported as failed(budget), partial results are still published, and the review exits 0. |

```yaml
- uses: alibaba/open-code-review@main
  with:
    effort: high
    max_tokens_budget: '10000000'
```

### Set reasoning effort

For models with steerable reasoning depth (e.g. GLM-5.x, OpenAI reasoning models), the `llm_reasoning_effort` input is merged into the request body as `reasoning_effort` via the action's existing `llm.extra_body` plumbing — no CLI support beyond the published versions is needed. OpenAI-compatible protocols only: the Anthropic API rejects unknown body fields, so the action fails fast when the input is set there — steer Anthropic thinking through an explicit `llm_extra_body` key instead. An explicit `reasoning_effort` key inside `llm_extra_body` wins over this input.

| Input | Default | Description |
|-------|---------|-------------|
| `llm_reasoning_effort` | `''` | One of `minimal`, `low`, `medium`, `high`, `max` (case-insensitive). Empty sends nothing. |

```yaml
- uses: alibaba/open-code-review@main
  with:
    llm_reasoning_effort: low
```

### Stream live review progress

By default the review runs with the machine-oriented `agent` audience and stays silent in the workflow log until it finishes; stderr is captured to a log file and uploaded as an artifact. Set `stream_progress: 'true'` to switch to the human audience and tee stderr into the workflow log, so `[ocr]` progress lines stream live while the review runs — stderr is still captured to the file for artifacts and comment posting.

| Input | Default | Description |
|-------|---------|-------------|
| `stream_progress` | `'false'` | Stream live `[ocr]` review progress to the workflow log (human audience on stderr) instead of staying silent until the run finishes. One of `'true'` / `'false'`. |

```yaml
- uses: alibaba/open-code-review@main
  with:
    stream_progress: 'true'
```

### Add custom review rules

```yaml
- uses: alibaba/open-code-review@main
  with:
    rule: ./my-rules.json
```

> Security: do not point `rule` at a file sourced from the PR branch when secrets are in scope; use a trusted rules file from your base branch.

### Control comment posting (sticky summary & incremental)

The action posts a summary issue comment plus inline review comments. Two inputs select the posting mode (combined, they give the four modes referenced above); a third tunes the incremental overlap test:

| Input | Default | Description |
|-------|---------|-------------|
| `sticky_summary` | `'true'` | Update an existing summary comment in place instead of posting a new one each run. |
| `incremental` | `'false'` | Only append inline comments whose `(path, line range)` does not overlap an existing bot review comment. History is never deleted (non-destructive). |
| `incremental_overlap_threshold` | `'0.6'` | IoU threshold `incremental` uses to decide whether a multi-line comment overlaps an existing one. Two single-line comments match on the same line; single- vs multi-line never match. Ignored unless `incremental` is `'true'`. |

```yaml
- uses: alibaba/open-code-review@main
  with:
    sticky_summary: 'true'
    incremental: 'true'
    incremental_overlap_threshold: '0.75'
```

> `sticky_summary` and `incremental` must be quoted strings (`'true'`/`'false'`); the action compares them as strings, so an unquoted YAML boolean will not match.

### Review only what changed since the last run (checkpoints)

`incremental` filters the comments a run produces; it still reviews the whole `merge-base..head` diff every time. On a long-lived PR that means re-reading the same 40 commits on every push. `checkpoint_range` fixes the other half: a run that reviewed everything it selected records the head it covered in a hidden marker inside its sticky summary comment, and the next run reviews `<that head>..<new head>` instead.

| Input | Default | Description |
|-------|---------|-------------|
| `checkpoint_range` | `'false'` | Review only the range since the last recorded checkpoint. Requires `sticky_summary: 'true'` (the checkpoint lives in that comment). |
| `full_review` | `'false'` | Force one full review even with `checkpoint_range` enabled. The run still records a new checkpoint. |

```yaml
- uses: alibaba/open-code-review@main
  with:
    sticky_summary: 'true'
    checkpoint_range: 'true'
```

**When in doubt, this reviews the full range.** A checkpoint is used only when every one of these holds; otherwise the run reviews `merge-base..head` exactly as it does today, and the reason is reported in the `range_summary` output and the step log:

| Reason | The run reviewed the full range because |
|--------|------------------------------------------|
| `disabled` | `checkpoint_range` is not `'true'` |
| `sticky_disabled` | `sticky_summary` is not `'true'`, so there is nowhere durable to keep a checkpoint |
| `manual_full_review` | `full_review: 'true'` was requested |
| `event_full_scope` | the PR was reopened or marked ready for review — both ask for a fresh look at the whole diff |
| `no_summary_comment` | the PR has no sticky summary yet (the first run) |
| `author_unverified` | the summary comment was not written by the identity this run expects (see the trust boundary below) |
| `corrupt_checkpoint` | the summary carries no readable checkpoint marker (absent, malformed, or two of them) |
| `schema_invalid` | the marker is for another PR, another marker version, or records a run that did not complete |
| `base_changed` | the base ref or the merge-base moved, so the diff basis is no longer the one the checkpoint was taken against |
| `config_changed` | the model, language, `llm_extra_body`, `llm_reasoning_effort`, `llm_extra_headers`, `llm_auth_header`, `llm_timeout`, `effort`, `max_tokens_budget`, `background`, routing inputs, the resolved OCR version, or the contents of `rule` / `.opencodereview/rule.json` changed — or `ocr version` printed nothing, so the version could not be established at all |
| `not_ancestor` | the checkpoint commit is in this clone but is not on the new head's history (the branch was reset to an earlier commit) |
| `unknown_object` | the checkpoint commit is not in this clone, so ancestry could not be checked — where a force-push usually lands, since the replaced commit is no longer fetched |
| `rule_unreadable` | a rule file was given but could not be read, so no stored fingerprint can be trusted to mean "same rules" |
| `resolver_error` | the comment could not be read, or `git merge-base --is-ancestor` could not run |

One reason is not a fallback: `same_head_noop`, when the recorded checkpoint already *is* the current head. There is nothing to review, so the run leaves the existing summary comment exactly as it is instead of replacing it with "No comments generated".

These outputs report what happened. All of them are empty when `checkpoint_range` is not enabled:

| Output | Value |
|--------|-------|
| `range_mode` | `checkpoint` or `full` |
| `range_reason` | the reason from the table above |
| `range_summary` | mode, reason and range in one line, e.g. `checkpoint (ok): <from>..<to>` |
| `range_from` | the commit the review started from, empty for a full review |
| `range_to` | the head the review ran up to |
| `checkpoint_before` | the head recorded by the marker that was read, whether or not it was used |
| `ancestry` | `ancestor`, `not_ancestor`, `unknown_object`, `error`, or empty when ancestry was not probed |
| `source_run` | the workflow run id that wrote the marker that was read |
| `checkpoint_after` | the head recorded as the new checkpoint, or empty when the run did not advance one |

Three properties are worth knowing before you enable it:

- **Widen-only.** The start of the range only ever moves back. An older checkpoint produces a wider review, never a narrower one, and a checkpoint only advances past a run whose manifest reported `terminal_state: complete`, whose findings all posted, and whose summary comment actually published. A run that fails halfway carries the previous checkpoint forward unchanged rather than skipping the range it did not review — and a run that cannot read the existing marker leaves it in place rather than erasing it.
- **Same-head reruns change nothing.** Re-running the workflow without pushing reports `same_head_noop` and leaves the previous run's summary untouched.
- **The sticky summary shows the latest range, not the whole PR.** The summary comment is rewritten on every run, so findings it reported for an earlier range (findings with no line information, routed findings, warnings) are replaced by the new range's; a run that narrowed the range says so in one line at the end of the summary. Inline review comments are separate comments and stay. If you rely on the summary as a running list for the whole PR, use `full_review: 'true'` to rebuild it, or leave `checkpoint_range` off.

> **Caveat — what `complete` covers.** `terminal_state: complete` means nothing in the set the run *selected* failed. Items the run waived, or excluded before selection (unsupported files, size limits), are inside that guarantee. So a checkpoint means "everything this configuration chose to review was reviewed", not "every byte of the diff was read". Changing the configuration invalidates the checkpoint (`config_changed`), which is what keeps that promise honest across runs.

> **Custom rules are fingerprinted by content.** Both rule sources are covered by their *contents*, not just their paths: the file you pass as `rule`, and the repo's own `.opencodereview/rule.json`, which OCR loads whether or not `rule` is set. Editing either invalidates the checkpoint (`config_changed`), so the next run re-reviews from the merge-base under the new rules rather than narrowing to the newest commits. The built-in rule set is embedded in the binary and moves with `ocr_version`, which is already part of the fingerprint. A rule file that exists but cannot be read forces a full review (`rule_unreadable`).

> **Caveat — the trust boundary is write permission.** The checkpoint is read only from a comment GitHub attributes to the writer this run expects. On the default `github_token` that is exactly one app, `github-actions`, since the default token always belongs to it: a marker in a comment posted by any other bot — `dependabot[bot]`, a linter app, another workflow's App — is rejected. If you pass your own `github_token`, the run cannot learn which app that token belongs to (there is no API an installation token can call for it), so the check widens to "any writer GitHub attributes to a bot" and any bot that can post an issue comment carrying the summary marker is trusted. A comment from a human account is rejected either way, even when it names an app, since GitHub sets `performed_via_github_app` for comments people write through an App as well. What GitHub attests is who *posted* the comment, not that its body is unmodified: anyone with write permission on the repository can edit a bot comment and move the checkpoint forward, causing a range to be skipped. The boundary this buys is "write-permission holders are trusted" — a fork contributor, who is exactly the untrusted party under `pull_request_target`, posts as themselves and so cannot plant or alter a marker. If that is not an acceptable assumption for your repository, leave `checkpoint_range` off.
>
> Because a sticky summary keeps its original author, switching a repository from a custom App token to the default one leaves the old comment attributed to the old app, and every run reports `author_unverified` until that comment is deleted. That is the fail-closed direction (a full review, never a skipped range), and the step log names the app it expected.

### Adjust retry and delay settings

When posting review comments individually (fallback mode), the action honors GitHub rate-limit headers (`retry-after`, `x-ratelimit-*`) with exponential backoff. The retry strategy follows GitHub's documented guidance for REST API rate limits — see [Rate limits for the REST API](https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api?apiVersion=2026-03-10) for details on primary/secondary rate limits and recommended retry behavior:

- **Primary rate limit exhausted** (`x-ratelimit-remaining=0`): wait until `x-ratelimit-reset`.
- **Secondary rate limit with a `retry-after` header**: wait exactly that long.
- **Secondary rate limit with no header**: wait at least one minute, then use exponential backoff on continued failures.

These are environment variables read by the posting module with sensible defaults; set them at the **job `env:` level** to tune (they propagate into the action):

| Variable | Default | Description |
|----------|---------|-------------|
| `OCR_RETRY_BASE_DELAY` | `60000` | Base delay (ms) for exponential backoff when no retry header is present |
| `OCR_RETRY_MAX_DELAY` | `300000` | Maximum delay (ms) cap applied to every computed wait |
| `OCR_MAX_RETRIES` | `3` | Maximum retry attempts per comment when rate-limited |
| `OCR_SUCCESS_DELAY` | `2000` | Delay (ms) after a successful comment post |
| `OCR_FAILURE_DELAY` | `1000` | Delay (ms) after a non-retryable failure |
| `OCR_LOW_REMAINING_THRESHOLD` | `3` | When x-ratelimit-remaining is at or below this value, proactively increase request spacing |
| `OCR_LOW_REMAINING_SPACING` | `10000` | Request spacing (ms) used when remaining quota is low |
| `OCR_READ_SUCCESS_DELAY` | `500` | Delay (ms) after a successful read API call (`listReviews` / `listReviewComments` / `listIssueComments`) used for the idempotency check. Reads are cheaper than writes, so the default is shorter |
| `OCR_READ_LOW_REMAINING_SPACING` | `5000` | Request spacing (ms) for read calls when remaining quota is low |

For example, to raise the per-comment retry count to 5, set `OCR_MAX_RETRIES` on the **job's** `env:` — not on the `uses:` step. A composite action does not forward the caller's step-level `env:` into its internal steps' process environment, so a step-level value would be silently ignored; the job-level value is inherited by the action's comment-posting step and read via `process.env`:

```yaml
jobs:
  code-review:
    runs-on: ubuntu-latest
    env:
      OCR_MAX_RETRIES: 5
    steps:
      - uses: alibaba/open-code-review@main
        with:
          llm_url: ${{ secrets.OCR_LLM_URL }}
          # ...other inputs
```

These variables are optional. See GitHub's [Rate limits for the REST API](https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api).

#### Idempotency: avoiding duplicate review comments

When the batch `createReview` call fails with a `5xx` error, the request may still have landed on the GitHub server (the response was simply lost). Before retrying per-comment, the action queries existing reviews and review comments — each tagged with a per-run HTML comment (e.g. `<!-- ocr-<runId>-<attempt>-<token> -->`) — and only retries the comments that are actually missing. This prevents duplicate review posts.

The summary comment is deduplicated too: in sticky mode (the default) the action finds the existing summary by its persistent marker and updates it in place rather than posting a new one; in non-sticky mode it reuses this run's summary if it already exists. If the read API is unavailable, it skips posting the summary rather than risking a duplicate.

If the read API itself is unavailable (rate-limited or `5xx`), the check returns *unknown* rather than assuming the comment was not posted. In that case the action **skips retrying** to avoid risking a duplicate, and surfaces the uncertainty in the summary instead of silently producing duplicates.

### Limit LLM concurrency

```yaml
- uses: alibaba/open-code-review@main
  with:
    review_concurrency: 5
```

### Provide background context

```yaml
- uses: alibaba/open-code-review@main
  with:
    background: ${{ github.event.pull_request.title }}
```

Particularly useful when PR titles follow semantic conventions (e.g., `feat(auth): add OAuth2 support`).

> Note: `github.event.pull_request.title` is only present on `pull_request_target` events, so it is empty for comment-triggered re-reviews. To cover both trigger types, have the pr-context step also output the title and fall back to it:
>
> ```yaml
> # inside the pr-context script (which only runs for issue_comment):
> core.setOutput('title', pullRequest.title);
> ```
> ```yaml
> - uses: alibaba/open-code-review@main
>   with:
>     background: ${{ steps.pr-context.outputs.title || github.event.pull_request.title }}
> ```

### Customize the review comment author with GitHub App

By default, review comments are posted using the built-in `GITHUB_TOKEN`, which appears as `github-actions[bot]`. You can customize this by creating a GitHub App and using its credentials instead.

For more details about GitHub Apps, see the [GitHub Apps documentation](https://docs.github.com/en/apps).

#### Step 1: Create a GitHub App

1. Go to your organization or personal account **Settings → Developer settings → GitHub Apps → New GitHub App**
2. Fill in the following:
   - **GitHub App name**: e.g., `OpenCodeReview Bot`
   - **Homepage URL**: Your repository or documentation URL
   - **Webhook**: Uncheck "Active" (not needed for this use case)
3. Under **Repository permissions**, set:
   - **Pull requests**: Read and write
   - **Contents**: Read-only (for fetching diffs)
   - **Metadata**: Read-only (required)
4. Click **Create GitHub App**

#### Step 2: Generate a Private Key

1. After creating the app, scroll down to **Private keys**
2. Click **Generate a private key**
3. Download and save the `.pem` file securely

Note your App ID from the app settings page.

#### Step 3: Install the App

1. In the left sidebar, click **Install App**
2. Select the repositories where you want to use OCR
3. After installation, note the **Installation ID** from the URL (e.g., `https://github.com/settings/installations/12345` → Installation ID is `12345`)

#### Step 4: Configure Repository Secrets

Add the following secrets to your repository (**Settings → Secrets and variables → Actions**):

| Secret | Description |
|--------|-------------|
| `GITHUB_APP_ID` | Your GitHub App's ID |
| `GITHUB_APP_PRIVATE_KEY` | Contents of the `.pem` file (including `-----BEGIN RSA PRIVATE KEY-----` and `-----END RSA PRIVATE KEY-----`) |
| `GITHUB_APP_INSTALLATION_ID` | (Optional) The Installation ID from Step 3 — only needed for apps with multiple installations |

#### Step 5: Pass the App token to the action

Mint a token with `actions/create-github-app-token` and pass it via the `github_token` input:

```yaml
- name: Get GitHub App Token
  id: app-token
  uses: actions/create-github-app-token@main
  with:
    app-id: ${{ secrets.GITHUB_APP_ID }}
    private-key: ${{ secrets.GITHUB_APP_PRIVATE_KEY }}

- uses: alibaba/open-code-review@main
  with:
    github_token: ${{ steps.app-token.outputs.token }}
    llm_url: ${{ secrets.OCR_LLM_URL }}
    llm_auth_token: ${{ secrets.OCR_LLM_AUTH_TOKEN }}
    llm_model: ${{ vars.OCR_LLM_MODEL }}
    llm_use_anthropic: ${{ vars.OCR_LLM_USE_ANTHROPIC }}
```

Now review comments will be posted with your custom GitHub App identity (e.g., `OpenCodeReview Bot`), providing a more professional and distinguishable appearance in your PRs.

## Example Output

The action posts two kinds of output on the PR: a **summary issue comment** (in the PR conversation) and **inline review comments** (in the "Files changed" tab).

### Summary comment

A single comment — updated in place on each run when `sticky_summary` is `'true'` (the default) — carries the review outcome and posting statistics.

- ✅ No issues: `✅ **OpenCodeReview**: No comments generated. Looks good to me.`
- 🔍 Issues found: a header line plus per-outcome counts, for example:

```markdown
🔍 **OpenCodeReview** found **3** issue(s) in this PR.
- ✅ Successfully posted inline: 2 comment(s)
- 📝 In summary (no line info): 1 comment(s)
```

The counts are mutually exclusive and sum to the total: `inline` (landed as review inline comments), `summary` (no line info, rendered in the summary body), `skipped` (suppressed by incremental overlap filtering), and `failed` (had line info but could not be posted). Any warnings are appended as a bulleted list.

### Inline comments

Comments with valid line info are posted as PR review comments in "Files changed". Each carries the review content plus, when a fix is available, a GitHub-native `suggestion` block so reviewers can apply it with one click:

````markdown
**Suggestion:**
```suggestion
// Fixed code here
```
````

Comments that have no line info, or that could not be posted inline (e.g. their line fell outside the current diff), are rendered in the summary body instead — each under a `### 📄 <path>` heading, with a collapsible `<details>` "💡 Suggested Change" (Before/After) when a fix is available.

## Supported LLM Providers

OCR supports both OpenAI and Anthropic API formats:

- **OpenAI-compatible APIs** (default):
  - OpenAI (GPT-4o, GPT-4, etc.)
  - Azure OpenAI
  - Self-hosted models (vLLM, Ollama, etc.)
- **Anthropic APIs** (set variable `OCR_LLM_USE_ANTHROPIC=true`, i.e. `llm_use_anthropic: true`):
  - Anthropic Claude models

## Troubleshooting

### Common Issues

1. **Job fails / "Failed to parse OCR output"**: When `ocr review` exits non-zero the action fails the job with that exit code (the comment-posting step is skipped); a zero exit with malformed JSON surfaces as a parse error in the summary. In both cases, check that `OCR_LLM_URL` and `OCR_LLM_AUTH_TOKEN` are set correctly, then inspect the uploaded `ocr-stderr.log` artifact (also printed in the "Run OpenCodeReview" step log) for the underlying error.
2. **"Cannot find merge-base"**: The action fetches full history (`fetch-depth: 0`) and the PR head (`git fetch origin pull/<n>/head`); if this still fails, ensure `permissions: contents: read` is set and the base branch is accessible (e.g., not deleted).
3. **Review comments not on the expected lines**: Comments are attached to the PR head commit. If a comment's line falls outside the current diff (the PR was force-pushed or updated mid-review), GitHub rejects the inline post and the comment is rendered in the summary instead. The workflow's concurrency group cancels stale runs on new pushes.
4. **No summary or comments at all**: Confirm the job's `permissions` include `pull-requests: write`, and that `github_token` (defaults to `${{ github.token }}`) is not overridden with a token lacking those scopes.

### Debugging

The action does not use an `OCR_DEBUG` flag. To diagnose a run:

- **Artifacts**: with `upload_artifacts: 'true'` (the default), the raw `ocr-result.json` and `ocr-stderr.log` are uploaded as workflow artifacts named `ocr-review-result-<run_id>-<run_attempt>`. Download them from the run's **Artifacts** section.
- **Step log**: the "Run OpenCodeReview" step prints both the JSON result and stderr to the workflow log.
- **Action outputs**: the step exposes `comments_total`, `comments_inline`, `comments_skipped`, `comments_failed`, and `summary_comment_url` outputs — inspect them in the job's step outputs.
- **GitHub step debug**: for verbose Actions runner diagnostics, enable the repository secret `ACTIONS_STEP_DEBUG=true` (standard GitHub Actions mechanism).

To stop uploading the raw artifacts, set `upload_artifacts: 'false'`.
