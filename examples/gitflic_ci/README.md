# OpenCodeReview - GitFlic CI Demo

This demo shows how to integrate OpenCodeReview into a [GitFlic](https://gitflic.ru) CI/CD pipeline to automatically review Merge Requests and post the findings as MR discussions — inline on the changed lines where possible.

Like the GitHub Actions and GitLab CI examples, the posting glue lives in the CI layer rather than in the `ocr` binary. Here it is a small, dependency-free Python script — [`post_review.py`](post_review.py) — that reads `ocr review --format json` and posts to the GitFlic Discussions API. The only GitFlic-specific wrinkle it handles is the **old-side line**: GitFlic requires it even for a comment on the new side of the diff, and `ocr review` reports new-side positions only, so the script recomputes it from the same merge-base diff the review ran on.

## How It Works

```
MR Created/Updated → Merge Request Pipeline → ocr review → post_review.py → Discussions on MR
```

1. A Merge Request Pipeline triggers the `code-review` job
2. It installs OCR via npm in a `node:20` image (which also ships `python3` and `git`)
3. Runs `ocr review --from origin/<target> --to $CI_COMMIT_SHA --format json --audience agent`
4. Runs `python3 post_review.py`, which reads the JSON and posts:
   - **Inline discussions** on the changed lines (`POST .../discussions/create` with `newLine`/`oldLine`/`newPath`/`oldPath`)
   - **A fallback note** collecting comments that could not be placed inline
   - **A summary note** with the totals

The MR context (owner, project, MR id, branch refs) is picked up automatically from the predefined GitFlic CI variables (`CI_PROJECT_NAMESPACE`, `CI_PROJECT_NAME`, `CI_MERGE_REQUEST_LOCAL_ID`, `CI_MERGE_REQUEST_TARGET_BRANCH_NAME`, `CI_COMMIT_SHA`), so `post_review.py` needs no arguments in CI. Outside CI every value can be passed via flags — run `python3 post_review.py -h`.

## Setup

### 1. Enable Merge Request Pipelines

Go to **Project Settings → CI/CD Settings** and enable **Merge Request Pipeline**. New merge requests will then trigger the pipeline automatically.

### 2. Copy the pipeline files

Copy **both** `gitflic-ci.yaml` (GitFlic expects this exact file name at the repository root) and `post_review.py` into your repository. If you keep `post_review.py` somewhere other than the repo root, adjust the `python3 post_review.py` path in `gitflic-ci.yaml` accordingly.

### 3. Configure CI/CD Variables

Go to **Settings → CI/CD → Variables** and add:

| Variable | Required | Description |
|----------|----------|-------------|
| `OCR_LLM_URL` | Yes | LLM API endpoint URL |
| `OCR_LLM_AUTH_TOKEN` | Yes | LLM API authentication token |
| `GITFLIC_TOKEN` | Yes | GitFlic access token used to post discussions |
| `OCR_LLM_MODEL` | No | Model name (e.g., `gpt-4o`) |
| `GITFLIC_API_URL` | No | REST API base URL for self-hosted GitFlic (default: `https://api.gitflic.ru`) |

> **Note:** GitFlic CI/CD does not accept variables with values shorter than 8 characters, so `use_anthropic` cannot be set as a CI variable. The pipeline sets it to `false`; to use Anthropic Claude models, edit `gitflic-ci.yaml` directly.

### 4. Create a GitFlic Access Token

Create a token in **User Settings → Access Tokens** (or a dedicated service account — its name becomes the bot name shown in discussions) and store it in the `GITFLIC_TOKEN` variable. The token owner must have access to the project sufficient for commenting on merge requests.

## Notes & Limitations

- **Inline positioning** — GitFlic requires all four of `newLine`/`oldLine`/`newPath`/`oldPath` for a code comment; if any is missing it silently creates a general comment. `post_review.py` computes the old-side position from the same merge-base diff the review ran on (`git diff merge-base(from, to)..to`), and anchors added lines to the closest preceding old line.
- **Rate limit** — the GitFlic cloud API allows 500 requests/hour per token. One review posts `comments + 2` requests at most, which fits comfortably.
- **Self-hosted GitFlic** — set `GITFLIC_API_URL` to your instance's REST API base URL.
- **Re-reviews** — every push to the MR triggers a new pipeline and a new review. To skip already-reviewed MRs, check existing discussions for the `OpenCodeReview` marker before running the review step.

## Tests

`post_review.py` ships with [`post_review_test.py`](post_review_test.py) — standard-library `unittest`, no network or git required:

```bash
cd examples/gitflic_ci
python3 post_review_test.py
```

The line-mapping cases are ported from the upstream Go tests so the script keeps proven parity with the binary.

## Debugging

Test the posting step locally without touching the MR:

```bash
ocr review --from origin/main --to HEAD --format json > /tmp/r.json
python3 post_review.py /tmp/r.json \
  --owner <owner> --project <project> --mr <id> \
  --from origin/main --to HEAD --dry-run
```

`--dry-run` prints every discussion (with the computed positions) instead of posting, and does not require `GITFLIC_TOKEN`.
