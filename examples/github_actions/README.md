# OpenCodeReview - GitHub Actions Workflow

This directory provides a ready-to-use GitHub Actions workflow demo that integrates OpenCodeReview into your repository to automatically review Pull Requests and post inline review comments. Copy it into `.github/workflows/` and configure the required secrets/vars.

## Quick Start: `ocr-review.yml`

The simplest adoption path: this demo delegates every step — checkout, OCR install, review, comment posting, artifact upload — to the official reusable composite action at [`action.yml`](../../action.yml) via a single `uses: alibaba/open-code-review@main` step. It covers both automatic PR review (`pull_request_target: opened/synchronize/reopened`) and on-demand re-review via comments (`/open-code-review` or `@open-code-review`). No inline scripts to maintain — `@main` always runs the latest action; pin to a version tag or commit SHA when reproducibility matters.

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

> **Note:** `GITHUB_TOKEN` is automatically provided by GitHub Actions with the required `pull-requests: write` permission. The action also sets `llm.extra_body` to disable thinking mode for compatibility with various LLM providers.

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

```yaml
- uses: alibaba/open-code-review@main
  with:
    ocr_version: 1.0.0
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
