# CodeUp CI Integration

Run `open-code-review` (`ocr`) automatically on every merge request in
[Alibaba Cloud Yunxiao CodeUp](https://codeup.aliyun.com), posting results
as a merge request comment via Flow.

## How it works

1. A Yunxiao Flow pipeline is triggered on merge request open/update.
2. The pipeline installs Node.js and the `ocr` CLI.
3. `post_review.py` runs `ocr review --format json` against the diff
   between the merge request's target and source branches.
4. Findings are formatted into a single Markdown summary and posted to the
   merge request via the
   [CreateChangeRequestComment](https://help.aliyun.com/zh/yunxiao/developer-reference/createchangerequestcomment)
   API.

This posts one summary comment per review run rather than true inline
per-line comments. Inline comments require resolving the merge request's
current patchset ID first — a reasonable follow-up, tracked as a known
limitation below.

## Setup

1. Copy `codeup-flow.yml` into your repository (or paste its contents into
   a new pipeline in the Flow console) and adjust `endpoint`,
   `serviceConnection`, and `runsOn` for your organization.
2. Copy `post_review.py` alongside it (or reference it directly if you
   keep it in a shared location).
3. Configure the following pipeline variables in Flow, marking secrets as
   protected:

   | Variable | Required | Description |
   |---|---|---|
   | `CODEUP_TOKEN` | Yes (secret) | Personal access token used for the `x-yunxiao-token` header. |
   | `CODEUP_ORG_ID` | Yes | Your Yunxiao organization ID. |
   | `CODEUP_REPO_ID` | Yes | Numeric ID of the CodeUp repository. |
   | `OCR_LLM_URL` | Yes | Your LLM endpoint, e.g. `https://api.anthropic.com/v1/messages`. |
   | `OCR_LLM_TOKEN` | Yes (secret) | API key for the LLM endpoint. |
   | `OCR_LLM_MODEL` | Yes | Model name, e.g. `<provider-model-name>`. |
   | `OCR_USE_ANTHROPIC` | If using Anthropic | Set to `true` when `OCR_LLM_URL` is an Anthropic-compatible endpoint. |

   `CODEUP_MR_LOCAL_ID`, `CODEUP_TARGET_BRANCH`, and `CODEUP_SOURCE_BRANCH`
   are populated automatically from the merge request trigger context —
   see `codeup-flow.yml` for how they're wired into the step's environment.

4. Enable the merge-request trigger on the pipeline's code source
   (**Edit pipeline → Edit code source → Enable code source trigger**,
   with trigger event set to "Merge request created/updated").

## Optional flags

Set `OCR_REVIEW_EXTRA_ARGS` as a pipeline variable to pass extra flags to
`ocr review`, e.g. `--concurrency 4`.

## Known limitations

- **Summary comment only.** Findings are posted as one combined Markdown
  comment rather than individual inline comments anchored to specific
  lines. True inline comments would require calling
  CreateChangeRequestComment once per finding with `file_path` and
  `line_number` set against the merge request's current patchset — planned
  as a follow-up.
- Requires a full (non-shallow) clone, or at minimum a fetch of the target
  branch, since `post_review.py` diffs `origin/<target>` against
  `origin/<source>`.

## Local testing

```bash
python3 -m unittest post_review_test -v
```
