---
title: CI/CD
sidebar:
  order: 4
---

Run OCR on every Pull Request or Merge Request. The upstream repo
ships two ready-made pipelines you copy and configure — one for
GitHub Actions, one for GitLab CI. Both are thin wrappers around the
core command from [Direct Subprocess](../subprocess/).

## How CI/CD integration works

Every recipe on this page follows the same pattern — the GitHub
Actions and GitLab CI sections below are just the concrete
implementations of it:

1. **Trigger on a PR / MR event.** A new pull request, an updated
   merge request, or a manual `/open-code-review` comment kicks off
   the job.
2. **Install `ocr`** in the runner, typically
   `npm install -g @alibaba-group/open-code-review`. The runner is
   ephemeral, so this happens on every run.
3. **Configure the LLM** from CI secrets via `ocr config set`
   (endpoint, token, model). There is no persisted
   `~/.opencodereview` to fall back on.
4. **Run the review in range mode** with machine-readable output, so
   stdout is a clean JSON envelope:

   ```bash
   ocr review \
     --from "origin/<base-branch>" \
     --to "origin/<head-branch>" \
     --format json \
     --audience agent
   ```

   `--format json` gives a parseable payload; `--audience agent`
   suppresses progress lines. See the
   [JSON shape](../subprocess/#json-shape) for the envelope every
   recipe consumes.
5. **Parse the JSON** and walk `comments[]`.
6. **Post comments back** to the PR / MR via the provider's review
   API. Entries without valid line info (file-level findings) are
   folded into a summary note instead of being posted inline; the
   posting step also falls back to a plain summary comment if the
   inline-batch API rejects the request.

Two kinds of credentials are always in play: the **LLM credentials**
OCR uses to generate findings, and a **PR/MR write token** the
posting step uses to comment back. The GitHub recipe gets the latter
for free via `GITHUB_TOKEN`; GitLab recommends an explicit
`GITLAB_API_TOKEN`, but the built-in `CI_JOB_TOKEN` is used as a
fallback for fork MRs (it can post discussions via `/discussions`) —
a dedicated token is recommended for reliability.

## GitHub Actions

The upstream workflow lives at
[`examples/github_actions/ocr-review.yml`](https://github.com/alibaba/open-code-review/blob/main/examples/github_actions/ocr-review.yml).

### What it does

- Triggers on `pull_request_target` (`opened`) **and** `issue_comment` events
  whose body starts with `/open-code-review` or `@open-code-review` —
  the latter lets reviewers re-run OCR on demand by commenting on a PR.
  (`pull_request_target` is used instead of `pull_request` so that
  secrets are available even for PRs opened from forks; OCR only reads
  the diff and does not execute code from the PR.)
- Installs OCR via `npm install -g @alibaba-group/open-code-review`,
  writes config with `ocr config set`, then runs the core command in
  branch-range mode.
- Parses the JSON envelope and posts each finding as an inline review
  comment via the GitHub Pull Request Review API. Comments without
  line info are folded into the summary body. If batch submission
  fails, it falls back to posting comments one-by-one and surfaces
  statistics in a summary comment.

### Install

Drop the workflow into your repo:

```bash
mkdir -p .github/workflows
curl -o .github/workflows/ocr-review.yml \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/examples/github_actions/ocr-review.yml
```

### Required secrets

Set under **Settings → Secrets and variables → Actions**:

| Secret | Required | Description |
|---|---|---|
| `OCR_LLM_URL` | Yes | LLM API endpoint (e.g. `https://api.openai.com/v1/chat/completions`). |
| `OCR_LLM_AUTH_TOKEN` | Yes | Authentication token for the LLM API. This CI secret is passed to `ocr config set llm.auth_token`. (OCR's direct env var is `OCR_LLM_TOKEN`, not `OCR_LLM_AUTH_TOKEN`.) |
| `OCR_LLM_MODEL` | No | Model name. No default — must be set explicitly. |
| `OCR_LLM_USE_ANTHROPIC` | No | Set to `true` for Anthropic Claude models. |

`GITHUB_TOKEN` is auto-provided; the workflow declares
`pull-requests: write` so it can post review comments.

> The workflow also runs
> `ocr config set llm.extra_body '{"thinking": {"type": "disabled"}}'`
> at startup, which turns off thinking-mode requests for
> compatibility across LLM providers that don't support that field.
> Remove the line if your provider needs thinking-mode left on.

### Customization

All of the following are edits to the workflow file you just copied
(`.github/workflows/ocr-review.yml`).

#### Background context

`--background` is the single highest-leverage flag — see the
[tips that apply to every pattern](../#tips-that-apply-to-every-pattern).
Feed the PR title (works especially well when titles follow a
semantic convention like `feat(auth): add OAuth2 support`):

```yaml
- name: Run OCR review
  run: |
    ocr review \
      --background "${{ github.event.pull_request.title }}" \
      --from "origin/${{ github.base_ref }}" \
      --to "origin/${{ github.head_ref }}" \
      --format json --audience agent
```

#### Custom rules

Pass a project-specific rule file with `--rule`:

```yaml
- name: Run OCR review
  run: |
    ocr review --rule ./my-rules.json \
      --from "origin/${{ github.base_ref }}" \
      --to "origin/${{ github.head_ref }}"
```

See [Review Rules](../../review-rules/) for the schema.

#### Concurrency

The default is 8 parallel per-file sub-agents. Lower it on large PRs
to stay under your LLM provider's rate limits:

```yaml
- name: Run OCR review
  run: |
    ocr review --concurrency 5 \
      --from "origin/${{ github.base_ref }}" \
      --to "origin/${{ github.head_ref }}"
```

#### Trigger pattern

The default workflow triggers on PR **opened** and on PR comments
beginning with `/open-code-review` or `@open-code-review`. Two common
adjustments:

Run on more PR lifecycle events (e.g., re-review when new commits
are pushed):

```yaml
on:
  pull_request:
    types: [opened, synchronize, reopened, ready_for_review]
```

Use a different comment keyword:

```yaml
if: |
  github.event_name == 'pull_request' ||
  (github.event_name == 'issue_comment'
    && github.event.issue.pull_request
    && startsWith(github.event.comment.body, '/review'))
```

The `github.event.issue.pull_request` check ensures the comment is
on a PR, not a regular issue.

#### Pin the OCR version

The default workflow installs the latest published version. To pin:

```yaml
- name: Install OpenCodeReview
  run: npm install -g @alibaba-group/open-code-review@1.0.0
```

#### Post under a GitHub App identity

By default, review comments come from `github-actions[bot]`. To post
under a branded bot like `OpenCodeReview Bot`, swap `GITHUB_TOKEN`
for a GitHub App installation token.

1. **Create the app** at *Settings → Developer settings → GitHub
   Apps → New GitHub App*. Disable the webhook (not needed for this
   use case). Under *Repository permissions* grant:
   - **Pull requests**: Read and write
   - **Contents**: Read-only (for fetching diffs)
   - **Metadata**: Read-only (required)

2. **Generate a private key** from the app settings page and download
   the `.pem` file. Note the **App ID** from the same page.

3. **Install the app** on the repositories you want OCR to review.
   The Installation ID appears in the post-install URL, e.g.
   `https://github.com/settings/installations/12345` → ID is `12345`.

4. **Add three secrets** under *Settings → Secrets and variables →
   Actions*:

   | Secret | Value |
   |---|---|
   | `GITHUB_APP_ID` | The App ID. |
   | `GITHUB_APP_PRIVATE_KEY` | Full contents of the `.pem` file, including the `-----BEGIN RSA PRIVATE KEY-----` and `-----END RSA PRIVATE KEY-----` lines. |
   | `GITHUB_APP_INSTALLATION_ID` | The Installation ID. |

5. **Mint a token and use it** in the comment-posting step:

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
         # ...existing post script...
   ```

Reviews will now appear as posted by your app's name instead of
`github-actions[bot]`.

### Troubleshooting

| Symptom | Cause / Fix |
|---|---|
| `Cannot find merge-base` | The checkout step used a shallow clone, but range-mode review needs full history. The upstream workflow sets `fetch-depth: 0` on `actions/checkout` — preserve that setting if you edit the file. |
| `Failed to parse OCR output` | `OCR_LLM_URL` or `OCR_LLM_AUTH_TOKEN` is missing or wrong. Re-check the values under *Settings → Secrets and variables → Actions*. |
| Review comments land on the wrong lines | Usually means the diff shifted between the moment the review started and when comments were posted. The posting script falls back to a plain issue comment in that case — no action needed. |

> **Note.** The `OCR_DEBUG` env var is **not currently implemented**
> in OCR — setting `OCR_DEBUG: "1"` has no effect. It's documented
> here in case it is wired up later. For verbose output today, inspect
> the raw review JSON and stderr that the workflow writes to
> `/tmp/ocr-result.json` and `/tmp/ocr-stderr.log` (see troubleshooting
> below), or run `ocr review` locally.

## GitLab CI

The upstream pipeline lives at
[`examples/gitlab_ci/.gitlab-ci.yml`](https://github.com/alibaba/open-code-review/blob/main/examples/gitlab_ci/.gitlab-ci.yml).

### What it does

- Triggers on `merge_requests` events (all MR events — creation,
  updates, reopen).
- Runs in a `node:20` image, installs OCR, configures it via
  `ocr config set`, then runs the core command in MR diff mode.
- Parses the JSON envelope with an inlined Python script and posts
  each finding as a GitLab Discussion (inline on the diff), using
  the MR's `versions` endpoint to compute correct `base_sha` /
  `start_sha` / `head_sha` for accurate positioning. Falls back to
  regular MR notes for any comment that can't be posted inline, and
  closes with a summary note.

### Install

Drop the pipeline into your repo root:

```bash
curl -o .gitlab-ci.yml \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/examples/gitlab_ci/.gitlab-ci.yml
```

If you already have a `.gitlab-ci.yml` and want to keep it, vendor
the recipe to a different path and pull it in with `include:`:

```yaml
include:
  - local: 'ci/ocr-review.gitlab-ci.yml'
```

### Required CI/CD variables

Set under **Settings → CI/CD → Variables**:

| Variable | Required | Masked | Description |
|---|---|---|---|
| `OCR_LLM_URL` | Yes | No | LLM API endpoint URL. |
| `OCR_LLM_AUTH_TOKEN` | Yes | Yes | API authentication token. This CI variable is passed to `ocr config set llm.auth_token`. (OCR's direct env var is `OCR_LLM_TOKEN`, not `OCR_LLM_AUTH_TOKEN`.) |
| `OCR_LLM_MODEL` | No | No | Model name. No default — must be set explicitly. |
| `GITLAB_API_TOKEN` | No | Yes | Project / personal / group access token with `api` scope. Optional — the built-in `CI_JOB_TOKEN` is used as a fallback when this is absent (e.g. for fork MRs). A dedicated `GITLAB_API_TOKEN` is recommended for reliability. |

> GitLab rejects variables shorter than 8 characters, so
> `llm.use_anthropic` is hardcoded to `false` in the pipeline. To use
> Anthropic Claude models, edit the script directly.

> The pipeline also runs
> `ocr config set llm.extra_body '{"thinking": {"type": "disabled"}}'`
> at startup, which turns off thinking-mode requests for
> compatibility across LLM providers that don't support that field.
> Remove the line if your provider needs thinking-mode left on.

> **Quick bot-naming tip.** For Project Access Tokens and Group
> Access Tokens, the token's **name** is what appears next to MR
> discussions. Naming the token `OpenCodeReview Bot` is a fast way
> to brand the reviewer without setting up anything else — handy
> when you don't need the more durable service-account setup
> documented under [Post under a service account identity](#post-under-a-service-account-identity).

### Customization

All of the following are edits to the `.gitlab-ci.yml` you just
copied.

#### Background context

Pass the MR title to `--background` — especially useful when titles
follow a semantic convention like `feat(auth): add OAuth2 support`:

```yaml
script:
  - |
    ocr review \
      --background "$CI_MERGE_REQUEST_TITLE" \
      --from "origin/$CI_MERGE_REQUEST_TARGET_BRANCH_NAME" \
      --to "${CI_COMMIT_SHA}" \
      --format json --audience agent
```

#### Custom rules and concurrency

Same flags as the GitHub Actions recipe — pass `--rule` for a
project-specific rule file, and `--concurrency` to throttle parallel
sub-agents (default 8):

```yaml
script:
  - |
    ocr review --rule ./my-rules.json --concurrency 5 \
      --from "origin/$CI_MERGE_REQUEST_TARGET_BRANCH_NAME" \
      --to "${CI_COMMIT_SHA}"
```

See [Review Rules](../../review-rules/) for the rule schema.

#### Pin the OCR version

```yaml
script:
  - npm install -g @alibaba-group/open-code-review@1.0.0
```

#### Avoid re-reviewing on every push

`only: [merge_requests]` triggers on **every** MR update, which can
burn a lot of LLM tokens on long-running MRs. GitLab has no native
"only on creation" event, so the recommended pattern is to detect
existing OCR notes before running the review and bail out if any are
found. Replace the `ocr review` invocation with a Python wrapper:

```python
import json, os, sys, urllib.request

GITLAB_URL = os.environ.get("CI_SERVER_URL", "https://gitlab.com")
PROJECT_ID = os.environ["CI_PROJECT_ID"]
MR_IID     = os.environ["CI_MERGE_REQUEST_IID"]
API_TOKEN  = os.environ["GITLAB_API_TOKEN"]

url = (
    f"{GITLAB_URL}/api/v4/projects/{PROJECT_ID}"
    f"/merge_requests/{MR_IID}/notes?per_page=100"
)
req = urllib.request.Request(url, headers={"PRIVATE-TOKEN": API_TOKEN})
with urllib.request.urlopen(req) as resp:
    notes = json.loads(resp.read().decode())

if any("OpenCodeReview" in n.get("body", "") for n in notes):
    print("OCR already reviewed this MR. Skipping to save tokens.")
    sys.exit(0)

# ...otherwise call `ocr review ...` as usual and write the JSON to
# the file the posting step expects.
```

To force a re-review after this, delete the previous OCR notes from
the MR — the next pipeline run will see no OCR notes and proceed.

#### Self-hosted GitLab

No code change needed. The posting script reads `CI_SERVER_URL`
(which GitLab sets automatically on every runner), so it talks to
your own instance out of the box. Just make sure
`GITLAB_API_TOKEN` is issued by your self-hosted instance, not
`gitlab.com`.

#### Post under a service account identity

By default, review discussions appear under whichever user owns
`GITLAB_API_TOKEN`. Swap in a project-scoped service account for a
branded bot identity like `OpenCodeReview Bot`.

1. **Create the service account** at *Project → Settings → Service
   Accounts → New service account*. The name you pick (e.g.
   `OpenCodeReview Bot`) is what appears next to MR discussions.

2. **Invite it to the project** at *Settings → Members → Invite
   member*. Search for the service-account name and assign
   `Developer` or `Maintainer` — both have the permissions needed
   to post discussions.

3. **Issue an access token** at *Settings → Service Accounts → (the
   account) → Add new token*. Required scope: `api`. Copy the token
   immediately — GitLab only shows it once.

4. **Swap the token value** at *Settings → CI/CD → Variables* —
   replace the existing `GITLAB_API_TOKEN` value with the service
   account's token (keep the variable name the same).

Discussions are now posted under the service account name instead
of the user who originally created the token.

### Troubleshooting

| Symptom | Cause / Fix |
|---|---|
| `Cannot find merge-base` | The runner used a shallow clone. The upstream pipeline sets `GIT_DEPTH: 0` to force a full clone — preserve that setting if you edit the file. |
| `API error 403` when posting | `GITLAB_API_TOKEN` is missing the `api` scope, isn't a member of the project, or — on self-hosted — was issued by a different instance. Reissue with `api` scope and re-add it under *Settings → CI/CD → Variables*. |
| `Failed to parse OCR output` | `OCR_LLM_URL` or `OCR_LLM_AUTH_TOKEN` is wrong. Re-check the values under *Settings → CI/CD → Variables*. |
| Inline comments land on the wrong lines | GitLab requires exact SHA matching for inline discussions; the posting script fetches `versions` metadata to get the right `base_sha` / `start_sha` / `head_sha`. If a finding still can't be anchored, it falls back to a plain MR note. |

The pipeline writes raw review JSON to `/tmp/ocr-result.json` and
stderr to `/tmp/ocr-stderr.log`. Cat them in a debug step to inspect
what OCR returned:

```yaml
script:
  - cat /tmp/ocr-result.json
  - cat /tmp/ocr-stderr.log
```

## See Also

- [Direct Subprocess](../subprocess/) — the JSON shape both pipelines
  consume, useful when writing your own CI script from scratch.
- [Configuration](../../configuration/) — every env var and config
  key OCR honors.
