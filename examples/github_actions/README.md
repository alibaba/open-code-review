# OpenCodeReview - GitHub Actions Workflow

This directory provides two ready-to-use GitHub Actions workflow demos that integrate OpenCodeReview into your repository to automatically review Pull Requests and post inline review comments. Copy the one that fits your needs into `.github/workflows/` and configure the required secrets/vars.

## Recommended: `ocr-review-reusable.yml`

The simplest adoption path: this demo delegates every step — checkout, OCR install, review, comment posting, artifact upload — to the official reusable composite action at [`action/`](../../action/) via a single `uses: alibaba/open-code-review/action@v1` step. It covers both automatic PR review (`pull_request_target: opened/synchronize/reopened`) and on-demand re-review via comments (`/open-code-review` or `@open-code-review`). No inline scripts to maintain — upgrades are as simple as bumping the `@v1` tag.

```bash
mkdir -p .github/workflows
cp ocr-review-reusable.yml .github/workflows/ocr-review.yml
```

The core of the demo is a single action step:

```yaml
- uses: alibaba/open-code-review/action@v1
  with:
    llm_url: ${{ secrets.OCR_LLM_URL }}
    llm_auth_token: ${{ secrets.OCR_LLM_AUTH_TOKEN }}
    llm_model: ${{ vars.OCR_LLM_MODEL }}
    llm_use_anthropic: ${{ vars.OCR_LLM_USE_ANTHROPIC }}
```

See [`action/README.md`](../../action/README.md) for the full list of inputs, outputs, security guidance, and the four comment-posting modes (sticky summary
+ incremental).

## Full Control: `ocr-review.yml`

Need to customise the review or comment-posting logic beyond what the action's inputs allow? This demo implements the entire pipeline inline — checkout, merge-base computation, `ocr` CLI invocation, JSON parsing, rate-limit-aware comment posting with idempotency checks — without depending on the reusable action. The trade-off is that you maintain the full script yourself and will not automatically benefit from upstream improvements to the action.

```bash
mkdir -p .github/workflows
cp ocr-review.yml .github/workflows/ocr-review.yml
```

## How It Works

```
PR Created/Updated → GitHub Actions Triggered → OCR Reviews Diff → Comments Posted on PR
     OR
Comment with trigger keyword ↗
```

1. When a PR is opened, the workflow triggers (uses `pull_request_target` for fork secret access).
2. Alternatively, when a comment containing `/open-code-review` or `@open-code-review` is posted on a PR, the workflow triggers.
3. The reusable action installs OCR, fetches the PR head blobs, computes `git merge-base`, and runs `ocr review --from <merge-base> --to <head> --format json`.
4. It parses the JSON output and posts inline review comments with a sticky summary on the PR using GitHub's Pull Request Review API.

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

> These knobs are action inputs — they apply to `ocr-review-reusable.yml` and any workflow calling `alibaba/open-code-review/action@v1`. When using `ocr-review.yml`, edit the inline script directly.

See [`action/README.md`](../../action/README.md) for the full input list. Workflow-level settings (triggers, keywords) are edited in the workflow file itself.

### Change the trigger events

Modify the `on.pull_request_target.types` array in the workflow file:

```yaml
on:
  pull_request_target:
    types: [opened, synchronize, reopened, ready_for_review]
```

### Customize comment trigger keywords

By default the workflow triggers when a PR comment starts with `/open-code-review` or `@open-code-review`. Edit the `if` condition to change the keywords:

```yaml
if: |
  github.event_name == 'pull_request_target' ||
  (github.event_name == 'issue_comment' && github.event.issue.pull_request && startsWith(github.event.comment.body, '/review')) ||
  (github.event_name == 'issue_comment' && github.event.issue.pull_request && startsWith(github.event.comment.body, '@mybot'))
```

Or use `contains` for a substring match:

```yaml
if: |
  github.event_name == 'pull_request_target' ||
  (github.event_name == 'issue_comment' && github.event.issue.pull_request && contains(github.event.comment.body, '/review'))
```

`github.event.issue.pull_request` ensures the comment is on a PR, not a regular issue.

### Use a specific OCR version

```yaml
- uses: alibaba/open-code-review/action@v1
  with:
    ocr_version: 1.0.0
```

### Add custom review rules

```yaml
- uses: alibaba/open-code-review/action@v1
  with:
    rule: ./my-rules.json
```

> Security: do not point `rule` at a file sourced from the PR branch when secrets are in scope; use a trusted rules file from your base branch.

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

```yaml
jobs:
  code-review:
    runs-on: ubuntu-latest
    env:
      OCR_MAX_RETRIES: 5
    steps:
      - uses: alibaba/open-code-review/action@v1
        with:
          llm_url: ${{ secrets.OCR_LLM_URL }}
          # ...other inputs
```

These variables are optional. See GitHub's [Rate limits for the REST API](https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api).

#### Idempotency: avoiding duplicate review comments

When the batch `createReview` call fails with a `5xx` error, the request may still have landed on the GitHub server (the response was simply lost). Before retrying per-comment, the action queries existing reviews and review comments — each tagged with a per-run HTML comment (e.g. `<!-- ocr-<runId>-<attempt>-<token> -->`) — and only retries the comments that are actually missing. This prevents duplicate review posts.

The same idempotency check is applied to the summary comment: before posting, the action verifies whether a summary with the same run tag already exists, and skips posting if so.

If the read API itself is unavailable (rate-limited or `5xx`), the check returns *unknown* rather than assuming the comment was not posted. In that case the action **skips retrying** to avoid risking a duplicate, and surfaces the uncertainty in the summary instead of silently producing duplicates.

### Limit LLM concurrency

```yaml
- uses: alibaba/open-code-review/action@v1
  with:
    review_concurrency: 5
```

### Provide background context

```yaml
- uses: alibaba/open-code-review/action@v1
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
> - uses: alibaba/open-code-review/action@v1
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
  uses: actions/create-github-app-token@v1
  with:
    app-id: ${{ secrets.GITHUB_APP_ID }}
    private-key: ${{ secrets.GITHUB_APP_PRIVATE_KEY }}

- uses: alibaba/open-code-review/action@v1
  with:
    github_token: ${{ steps.app-token.outputs.token }}
    llm_url: ${{ secrets.OCR_LLM_URL }}
    llm_auth_token: ${{ secrets.OCR_LLM_AUTH_TOKEN }}
    llm_model: ${{ vars.OCR_LLM_MODEL }}
    llm_use_anthropic: ${{ vars.OCR_LLM_USE_ANTHROPIC }}
```

Now review comments will be posted with your custom GitHub App identity (e.g., `OpenCodeReview Bot`), providing a more professional and distinguishable appearance in your PRs.

## Example Output

When a PR is reviewed, comments appear directly in the PR's "Files changed" tab:

- ✅ If no issues found: A comment saying "No comments generated. Looks good to me."
- 🔍 If issues found: Inline review comments with suggestions using GitHub's native suggestion syntax

### Inline Comment Example

The workflow uses GitHub's `suggestion` code block syntax, so reviewers can apply fixes with one click:

````markdown
**Suggestion:**
```suggestion
// Fixed code here
```
````

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

1. **"Failed to parse OCR output"**: Check that `OCR_LLM_URL` and `OCR_LLM_AUTH_TOKEN` secrets are correctly set
2. **"Cannot find merge-base"**: The action already fetches full history (`fetch-depth: 0`) and the PR head; if this still fails, ensure `permissions: contents: read` is set and the base branch is accessible.
3. **Review comments not appearing on correct lines**: This can happen when the diff has changed since the review started; the workflow handles this gracefully with a fallback to issue comments

### Debugging

Enable OCR debug logging by setting `OCR_DEBUG` at the job `env:` level (it propagates into the action):

```yaml
jobs:
  code-review:
    runs-on: ubuntu-latest
    env:
      OCR_DEBUG: "1"
    steps:
      - uses: alibaba/open-code-review/action@v1
        with:
          llm_url: ${{ secrets.OCR_LLM_URL }}
          # ...other inputs
```
