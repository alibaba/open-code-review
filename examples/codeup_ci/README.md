# CodeUp (云效) Integration

This integrates `open-code-review` into a **Yunxiao Flow (云效流水线)**
pipeline so that AI review comments are posted automatically on
**CodeUp merge requests**, alongside the existing GitHub Actions, GitLab CI,
GitFlic CI, and Gerrit integrations.

## How it works

1. Configure your Flow pipeline's trigger to run on
   **合并请求 新建/更新** (merge request created/updated) code-source events.
2. Add a custom step (Shell or Flow-CLI step) that runs
   [`scripts/codeup-review.sh`](../../scripts/codeup-review.sh).
3. The script:
   - Installs the `ocr` CLI (`npm install -g @alibaba-group/open-code-review`)
   - Runs `ocr review --format json` against the merge request diff
   - Formats findings into Markdown
   - Posts them as a single comment via CodeUp's
     [`CreateChangeRequestComment`](https://help.aliyun.com/zh/yunxiao/developer-reference/createchangerequestcomment)
     OpenAPI

## Setup

### 1. Create a CodeUp personal access token

Generate a personal access token (个人访问令牌) with repository read/comment
scope. See: `Codeup 个人访问令牌` in the Yunxiao docs.

### 2. Configure pipeline variables

In your Flow pipeline → **变量和缓存 (Variables & Cache)**, add the
following as (ideally *private/secret*) string variables:

| Variable            | Description                                              |
| -------------------- | --------------------------------------------------------- |
| `CODEUP_DOMAIN`       | Your CodeUp domain, e.g. `codeup.aliyun.com`               |
| `CODEUP_TOKEN`        | The personal access token from step 1 (mark as **私密**)   |
| `CODEUP_ORG_ID`        | Your Yunxiao organization ID                               |
| `CODEUP_REPO_ID`        | Numeric repository ID (or `namespace/repo` form)           |
| `CODEUP_MR_LOCAL_ID`     | The merge request's local ID for this run                  |

`CODEUP_MR_LOCAL_ID`, `CODEUP_REPO_ID`, and `CODEUP_ORG_ID` should generally
be populated from Flow's **built-in merge-request trigger variables**, not
hardcoded — the exact built-in variable names can differ by Flow template
version, so map them explicitly in your pipeline step, e.g.:

```yaml
# Illustrative — adapt variable names to whatever your Flow template
# exposes for merge-request-triggered runs.
env:
  CODEUP_DOMAIN: codeup.aliyun.com
  CODEUP_TOKEN: ${CODEUP_TOKEN}
  CODEUP_ORG_ID: ${CODEUP_ORG_ID}
  CODEUP_REPO_ID: ${REPO_ID}
  CODEUP_MR_LOCAL_ID: ${CHANGE_REQUEST_LOCAL_ID}
```

### 3. Configure the LLM (same as other integrations)

```bash
ocr config set llm.url https://api.anthropic.com/v1/messages
ocr config set llm.auth_token "$OCR_LLM_TOKEN"
ocr config set llm.model claude-opus-4-6
ocr config set llm.use_anthropic true
```

or via env vars (`OCR_LLM_URL`, `OCR_LLM_TOKEN`, `OCR_LLM_MODEL`,
`OCR_USE_ANTHROPIC`) as documented in the main README.

### 4. Add the pipeline step

In the Flow UI, add a **自定义步骤 (Custom Step)** / Shell step with:

```bash
chmod +x scripts/codeup-review.sh scripts/format_ocr_report.py
./scripts/codeup-review.sh
```

(Ensure `jq`, `python3`, and `curl` are available in the build image —
these are present in most default Flow shell images; install them
explicitly in the step if using a minimal custom image.)

## Notes / Known limitations

- This first version posts one **global comment** per review run rather
  than true per-line inline comments. CodeUp's API supports inline
  comments (`comment_type: INLINE_COMMENT` with `file_path`/`line_number`/
  `patchset_biz_id`), which is a natural follow-up — it requires fetching
  the merge request's current patchset ID first via `ListMergeRequestPatchSets`
  (or equivalent) so comments anchor to the correct diff version.
- No native "云效" GitHub-Actions-style marketplace action exists yet, so
  this is wired as a plain shell step, matching the current GitLab CI
  approach used elsewhere in this repo.
