# OpenCodeReview - GitHub Actions Demo

This demo shows how to integrate OpenCodeReview into your GitHub Actions workflow to automatically review Pull Requests and post review comments.

## How It Works

```
PR Created/Updated → GitHub Actions Triggered → OCR Reviews Diff → Comments Posted on PR
     OR
Comment with trigger keyword ↗
```

1. When a PR is opened, the workflow triggers (uses `pull_request_target` for fork secret access)
2. Alternatively, when a comment containing `/open-code-review` or `@open-code-review` is posted on a PR, the workflow triggers
3. It installs OCR via `npm install -g @alibaba-group/open-code-review`
4. Runs `ocr review --from origin/<base> --to <head_sha> --format json` to analyze the diff (uses commit SHA to support fork PRs)
5. Parses the JSON output and posts inline review comments on the PR using GitHub's Pull Request Review API

## Setup

### 1. Copy the workflow file

Copy `ocr-review.yml` to your repository's `.github/workflows/` directory:

```bash
mkdir -p .github/workflows
cp ocr-review.yml .github/workflows/ocr-review.yml
```

### 2. Configure secrets

Go to your repository's **Settings → Secrets and variables → Actions** and add:

| Secret | Required | Description |
|--------|----------|-------------|
| `OCR_LLM_URL` | Yes | LLM API endpoint URL (e.g., `https://api.openai.com/v1/chat/completions`) |
| `OCR_LLM_AUTH_TOKEN` | Yes | API authentication token |
| `OCR_LLM_MODEL` | No | Model name (defaults to `gpt-4o`) |
| `OCR_LLM_USE_ANTHROPIC` | No | Set to `true` if using Anthropic Claude models |

> **Note:** `GITHUB_TOKEN` is automatically provided by GitHub Actions with the required `pull-requests: write` permission.
>
> The workflow also configures `llm.extra_body` to disable thinking mode for compatibility with various LLM providers.

## Customization

### Change the trigger events

Modify the `on.pull_request_target.types` array in the workflow file:

```yaml
on:
  pull_request_target:
    types: [opened, synchronize, reopened, ready_for_review]
```

### Customize comment trigger keywords

By default, the workflow triggers when a PR comment starts with `/open-code-review` or `@open-code-review`. You can customize these keywords by modifying the `if` condition in the workflow:

```yaml
if: |
  github.event_name == 'pull_request_target' ||
  (github.event_name == 'issue_comment' && github.event.issue.pull_request && startsWith(github.event.comment.body, '/review')) ||
  (github.event_name == 'issue_comment' && github.event.issue.pull_request && startsWith(github.event.comment.body, '@mybot'))
```

Or use a more flexible pattern with `contains` to trigger on any comment containing the keyword:

```yaml
if: |
  github.event_name == 'pull_request_target' ||
  (github.event_name == 'issue_comment' && github.event.issue.pull_request && contains(github.event.comment.body, '/review'))
```

> **Note:** The condition `github.event.issue.pull_request` ensures the comment is on a PR, not a regular issue.

### Use a specific OCR version

```yaml
- name: Install OpenCodeReview
  run: npm install -g @alibaba-group/open-code-review@1.0.0
```

### Add custom review rules

Use the `--rule` flag to pass a custom rules JSON file:

```yaml
- name: Run OCR review
  run: ocr review --rule ./my-rules.json --from origin/${{ github.base_ref }} --to origin/${{ github.head_ref }}
```

### Adjust retry and delay settings

When posting review comments individually (fallback mode), the workflow includes rate-limit handling with exponential backoff. The retry strategy follows GitHub's documented guidance for REST API rate limits — see [Rate limits for the REST API](https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api?apiVersion=2026-03-10) for details on primary/secondary rate limits and recommended retry behavior. You can configure the retry and delay behavior via **repository variables** (Settings → Secrets and variables → Actions → Variables):

| Variable | Default | Description |
|----------|---------|-------------|
| `OCR_RETRY_BASE_DELAY` | `60000` | Base delay (ms) for exponential backoff when no retry header is present (per GitHub's "at least one minute" recommendation for secondary limits) |
| `OCR_RETRY_MAX_DELAY` | `300000` | Maximum delay (ms) cap applied to every computed wait, including retry-after and x-ratelimit-reset, so a far-future reset cannot stall the job past its timeout |
| `OCR_MAX_RETRIES` | `3` | Maximum retry attempts per comment when rate-limited |
| `OCR_SUCCESS_DELAY` | `2000` | Delay (ms) after a successful comment post to pace subsequent requests |
| `OCR_FAILURE_DELAY` | `1000` | Delay (ms) after a non-rate-limit failure to pace subsequent requests |
| `OCR_LOW_REMAINING_THRESHOLD` | `3` | When x-ratelimit-remaining is at or below this value, proactively increase request spacing to avoid hitting the limit |
| `OCR_LOW_REMAINING_SPACING` | `10000` | Request spacing (ms) used when remaining quota is low |

These variables are optional — if not configured, sensible defaults are used. Consider increasing delays for repositories with many concurrent workflows or large PRs that generate numerous review comments.

### Limit concurrency

Adjust the `--concurrency` flag for large PRs to control the number of concurrent LLM requests:

```yaml
- name: Run OCR review
  run: ocr review --concurrency 5 --from origin/${{ github.base_ref }} --to origin/${{ github.head_ref }}
```

### Provide background context

Use the `--background` flag to pass additional context that helps OCR better understand the purpose of the changes:

```yaml
- name: Run OCR review
  run: ocr review --background "${{ github.event.pull_request.title }}" --from origin/${{ github.base_ref }} --to origin/${{ github.head_ref }}
```

This is particularly useful when your PR titles follow semantic conventions (e.g., `feat(auth): add OAuth2 support`) that clearly summarize what the PR implements. The background information helps OCR provide more relevant and context-aware review comments.

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
| `GITHUB_APP_INSTALLATION_ID` | The Installation ID from Step 3 |

#### Step 5: Update the Workflow

Add a step to obtain a token from the GitHub App, then use it in the "Post review comments to PR" step:

```yaml
- name: Get GitHub App Token
  id: app-token
  uses: actions/create-github-app-token@v1
  with:
    app-id: ${{ secrets.GITHUB_APP_ID }}
    private-key: ${{ secrets.GITHUB_APP_PRIVATE_KEY }}

- name: Post review comments to PR
  uses: actions/github-script@v7
  with:
    github-token: ${{ steps.app-token.outputs.token }}
    script: |
      # ... existing script
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
- **Anthropic APIs** (set `OCR_LLM_USE_ANTHROPIC: true`):
  - Anthropic Claude models

## Troubleshooting

### Common Issues

1. **"Failed to parse OCR output"**: Check that `OCR_LLM_URL` and `OCR_LLM_AUTH_TOKEN` secrets are correctly set
2. **"Cannot find merge-base"**: Ensure `fetch-depth: 0` is set in the checkout step
3. **Review comments not appearing on correct lines**: This can happen when the diff has changed since the review started; the workflow handles this gracefully with a fallback to issue comments

### Debugging

Enable debug logging by adding to the OCR review step:

```yaml
env:
  OCR_DEBUG: "1"
```
