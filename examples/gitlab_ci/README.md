# OpenCodeReview - GitLab CI Demo

This demo shows how to integrate OpenCodeReview into your GitLab CI/CD pipeline to automatically review Merge Requests and post review comments as inline discussions.

## How It Works

```
MR Created/Updated → GitLab Pipeline Triggered → OCR Reviews Diff → Discussions Posted on MR
```

1. When a Merge Request is opened or updated, the pipeline triggers
2. It installs OCR via npm in a `node:20` Docker image
3. Runs `ocr review --from origin/<target> --to <commit_sha> --format json --audience agent` to analyze the diff (uses commit SHA to support fork MRs)
4. Parses the JSON output and posts inline discussions on the MR using GitLab's Discussions API

## Setup

### 1. Copy the pipeline file

Copy `.gitlab-ci.yml` to your repository root (or include it via `include:`):

```bash
cp .gitlab-ci.yml /path/to/your/repo/.gitlab-ci.yml
```

Or use GitLab's `include` feature in your existing `.gitlab-ci.yml`:

```yaml
include:
  - local: 'ci_demo/gitlab_ci/.gitlab-ci.yml'
```

### 2. Configure CI/CD Variables

Go to your project's **Settings → CI/CD → Variables** and add:

| Variable | Required | Masked | Description |
|----------|----------|--------|-------------|
| `OCR_LLM_URL` | Yes | No | LLM API endpoint URL (e.g., `https://api.openai.com/v1/chat/completions`) |
| `OCR_LLM_AUTH_TOKEN` | Yes | Yes | API authentication token |
| `OCR_LLM_MODEL` | No | No | Model name (defaults to `gpt-4o`) |
| `GITLAB_API_TOKEN` | No | Yes | GitLab access token with `api` scope (falls back to `CI_JOB_TOKEN` if not set) |

> **Note:** GitLab CI/CD does not support variables with values shorter than 8 characters, so `use_anthropic` cannot be set as a CI variable. The pipeline sets it to `false` by default. If you need to use Anthropic Claude models, you'll need to modify the `.gitlab-ci.yml` script directly.
>
> The pipeline also configures `llm.extra_body` to disable thinking mode for compatibility with various LLM providers.

### 3. Create a GitLab Access Token

You need a token with `api` scope to post discussions on MRs. Options:

- **Project Access Token** (recommended): Settings → Access Tokens → Create with `api` scope
- **Personal Access Token**: User Settings → Access Tokens → Create with `api` scope
- **Group Access Token**: For organization-wide usage

> **Note:** The built-in `CI_JOB_TOKEN` has limited API scope and may not support all discussion features (e.g., creating new threads on older GitLab versions). If `GITLAB_API_TOKEN` is not set, the pipeline falls back to `CI_JOB_TOKEN` automatically — but for best results, a dedicated token with `api` scope is recommended.
>
> **Tip:** For Project Access Tokens and Group Access Tokens, the token name determines the bot name shown in MR discussions. For example, naming your token `OpenCodeReview Bot` will make review comments appear as posted by `OpenCodeReview Bot`.

## Example Output

When an MR is reviewed, comments appear as:

- **Inline discussions**: Directly on the changed lines in the MR diff view
- **Summary note**: A final note summarizing the total number of issues found
- **Fallback notes**: If inline posting fails for specific comments, they appear as regular MR notes with file/line references

### Inline Discussion Example

Comments are posted using GitLab's Discussion API with position data, so they appear directly next to the relevant code in the "Changes" tab.

## Supported LLM Providers

OCR supports both OpenAI and Anthropic API formats:

- **OpenAI-compatible APIs** (default):
  - OpenAI (GPT-4o, GPT-4, etc.)
  - Azure OpenAI
  - Self-hosted models (vLLM, Ollama, etc.)
- **Anthropic APIs** (modify `.gitlab-ci.yml` to set `use_anthropic: true`):
  - Anthropic Claude models

## Customization

### Use a specific OCR version

```yaml
script:
  - npm install -g @alibaba-group/open-code-review@1.0.0
```

### Add custom review rules

Use the `--rule` flag to pass a custom rules JSON file:

```yaml
script:
  - ocr review --rule ./my-rules.json --from origin/$CI_MERGE_REQUEST_TARGET_BRANCH_NAME --to $CI_COMMIT_SHA
```

### Adjust retry and delay settings

When posting review discussions, the script includes rate-limit handling with exponential backoff (with jitter), `Retry-After` header support, and proactive throttling based on GitLab's `RateLimit-Remaining` response header. All API requests — including summary notes and MR version fetches — use the same retry logic. See [GitLab Rate Limits](https://docs.gitlab.com/security/rate_limits/) for details on GitLab's rate-limiting policies and recommended handling. You can configure the retry and delay behavior via **CI/CD Variables** (Settings → CI/CD → Variables):

| Variable | Default | Description |
|----------|---------|-------------|
| `OCR_RETRY_BASE_DELAY` | `2000` | Base delay (ms) for exponential backoff when a rate-limit error is hit |
| `OCR_MAX_RETRIES` | `3` | Maximum retry attempts per discussion when rate-limited |
| `OCR_MAX_RETRY_DELAY` | `60000` | Maximum delay (ms) per single retry, caps both `Retry-After` and backoff |
| `OCR_SUCCESS_DELAY` | `2000` | Delay (ms) after a successful discussion post to pace subsequent requests |
| `OCR_FAILURE_DELAY` | `1000` | Delay (ms) after a non-rate-limit failure to pace subsequent requests |
| `OCR_RATE_LIMIT_THRESHOLD` | `10` | Proactively slow down when GitLab `RateLimit-Remaining` is at/below this value (set `0` to disable) |

These variables are optional — if not configured, sensible defaults are used. Consider increasing delays for self-hosted GitLab instances with aggressive rate-limit configurations or for large MRs that generate numerous review comments. The `OCR_RATE_LIMIT_THRESHOLD` variable enables proactive throttling: when GitLab reports low remaining quota in the `RateLimit-Remaining` response header, the script automatically doubles the pacing delay to avoid hitting 429 errors.

### Limit concurrency

Adjust the `--concurrency` flag for large MRs to control the number of concurrent LLM requests:

```yaml
script:
  - ocr review --concurrency 5 --from origin/$CI_MERGE_REQUEST_TARGET_BRANCH_NAME --to $CI_COMMIT_SHA
```

### Provide background context

Use the `--background` flag to pass additional context that helps OCR better understand the purpose of the changes:

```yaml
script:
  - ocr review --background "$CI_MERGE_REQUEST_TITLE" --from origin/$CI_MERGE_REQUEST_TARGET_BRANCH_NAME --to $CI_COMMIT_SHA
```

This is particularly useful when your MR titles follow semantic conventions (e.g., `feat(auth): add OAuth2 support`) that clearly summarize what the MR implements. The background information helps OCR provide more relevant and context-aware review comments.

### Change the trigger events

By default, the pipeline uses `only: [merge_requests]`, which triggers on **all** MR events (creation, updates, reopen). GitLab CI does not natively support fine-grained control to trigger **only on MR creation**.

To avoid re-reviewing on every push to an existing MR (and wasting LLM API tokens), you can check for existing OCR reviews **before** running `ocr review`. Use a wrapper script that skips the review step if OCR comments already exist:

```yaml
script:
  # Install OpenCodeReview
  - npm install -g @alibaba-group/open-code-review

  # Configure OCR
  - mkdir -p ~/.open-code-review
  - |
    ocr config set llm.url $OCR_LLM_URL
    ocr config set llm.auth_token $OCR_LLM_AUTH_TOKEN
    ocr config set llm.model $OCR_LLM_MODEL
    ocr config set llm.use_anthropic false
    ocr config set llm.extra_body '{"thinking": {"type": "disabled"}}'

  # Check for existing OCR reviews and run review only if not found
  - |
    python3 << 'WRAPPER_SCRIPT'
    import json
    import os
    import subprocess
    import sys
    import urllib.request

    GITLAB_URL = os.environ.get("CI_SERVER_URL", "https://gitlab.com")
    PROJECT_ID = os.environ["CI_PROJECT_ID"]
    MR_IID = os.environ["CI_MERGE_REQUEST_IID"]
    API_TOKEN = os.environ["GITLAB_API_TOKEN"]
    SOURCE_BRANCH = os.environ["CI_MERGE_REQUEST_SOURCE_BRANCH_NAME"]
    TARGET_BRANCH = os.environ["CI_MERGE_REQUEST_TARGET_BRANCH_NAME"]

    # Check for existing OCR reviews
    url = f"{GITLAB_URL}/api/v4/projects/{PROJECT_ID}/merge_requests/{MR_IID}/notes?per_page=100"
    req = urllib.request.Request(url, headers={"PRIVATE-TOKEN": API_TOKEN})
    with urllib.request.urlopen(req) as resp:
        notes = json.loads(resp.read().decode("utf-8"))

    for note in notes:
        if "OpenCodeReview" in note.get("body", ""):
            print("⏭️ OCR has already reviewed this MR. Skipping to save tokens.")
            print("Delete previous OCR comments to re-trigger review.")
            sys.exit(0)

    # No existing review found - run OCR
    print("🔍 No existing OCR review found. Running review...")
    COMMIT_SHA = os.environ["CI_COMMIT_SHA"]
    result = subprocess.run([
        "ocr", "review",
        "--from", f"origin/{TARGET_BRANCH}",
        "--to", COMMIT_SHA,
        "--format", "json",
        "--audience", "agent"
    ], capture_output=True, text=True)

    # Save output for the posting script
    with open("/tmp/ocr-result.json", "w") as f:
        f.write(result.stdout)
    with open("/tmp/ocr-stderr.log", "w") as f:
        f.write(result.stderr)

    print("OCR review completed.")
    WRAPPER_SCRIPT

  # Post review comments to MR
  - |
    python3 << 'PYTHON_SCRIPT'
    ...existing post script...
    PYTHON_SCRIPT
```

The key logic: the Python wrapper checks for existing OCR comments before running `ocr review`. If found, it exits early with `sys.exit(0)` before consuming any LLM tokens. To re-trigger a review, users can manually delete the previous OCR comments.

### Self-hosted GitLab

The script automatically uses `CI_SERVER_URL` to determine the GitLab API base URL, so it works with self-hosted GitLab instances out of the box.

### Use a Service Account as Review Bot

By default, review comments are posted using the user who owns the access token configured in `GITLAB_API_TOKEN`. You can create a dedicated service account bot to post reviews with a custom identity, making it easier to distinguish automated reviews from human comments.

For more details about GitLab service accounts, see the [GitLab Service Accounts documentation](https://docs.gitlab.com/ee/user/profile/service_accounts.html).

#### Step 1: Create a Service Account

Create a service account in your project:

1. Go to your **Project → Settings → Service Accounts**
2. Click **New service account**
3. Fill in the following:
   - **Name**: e.g., `OpenCodeReview Bot` (this will be the bot name shown in MR discussions)
   - **Username**: Will be auto-generated based on the name
4. Click **Create service account**

#### Step 2: Invite the Service Account to Your Project

After the service account is created, invite it to your project with appropriate permissions:

1. Go to your **Project → Settings → Members**
2. Click **Invite member**
3. Search for the service account by name (e.g., `OpenCodeReview Bot`)
4. Select the service account and assign a role (`Developer` or `Maintainer` required for posting discussions)
5. Click **Invite**

#### Step 3: Create an Access Token

Generate an access token for the service account:

1. Go to your **Project → Settings → Service Accounts**
2. Click on the service account to view its details
3. Click **Add new token**
4. Configure the token:
   - **Name**: e.g., `ocr-review-token`
   - **Expiration**: As needed
   - **Scope**: Select `api` (required for Discussions API)
5. Click **Create token** and copy the token value

#### Step 4: Update CI/CD Variables

Update the `GITLAB_API_TOKEN` variable in your project's CI/CD settings:

Go to **Settings → CI/CD → Variables** and update `GITLAB_API_TOKEN` with the service account's token.

Now review comments will be posted with your service account identity (e.g., `OpenCodeReview Bot`), providing a clear and professional appearance for automated code reviews.

## Troubleshooting

### Common Issues

1. **"API error 403"**: The `GITLAB_API_TOKEN` lacks `api` scope or doesn't have access to the project
2. **"Failed to parse OCR output"**: Check that `OCR_LLM_URL` and `OCR_LLM_AUTH_TOKEN` variables are correctly set
3. **"Cannot find merge-base"**: Ensure `GIT_DEPTH: 0` is set (full clone)
4. **Inline comments on wrong lines**: GitLab requires exact SHA matching; the script fetches MR version metadata to get correct diff refs

### Debugging

Add verbose output to the review step:

```yaml
script:
  - cat /tmp/ocr-result.json
  - cat /tmp/ocr-stderr.log
```


