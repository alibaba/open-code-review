# OpenCodeReview - Bitbucket Pipelines

This example reviews Bitbucket Cloud pull requests with OpenCodeReview (OCR)
and posts the findings back to the pull request.

## How it works

```text
PR created or updated → Bitbucket Pipeline → OCR reviews diff → PR comments posted
```

1. Bitbucket's `pullrequest-push` trigger starts the custom
   `ocr-pr-review` pipeline when a pull request is created or its source branch
   is updated.
2. The pipeline installs OCR in a `node:20` container and configures the LLM
   from secured repository variables.
3. OCR compares `origin/$BITBUCKET_PR_DESTINATION_BRANCH` with
   `$BITBUCKET_COMMIT` and writes JSON output.
4. A dependency-free Node.js script posts each finding through the Bitbucket
   Pull Request Comments API. It creates an inline comment when the reported
   line belongs to the PR diff, falls back to a general comment when Bitbucket
   cannot anchor the line, and posts a final summary.

## Setup

### 1. Copy the pipeline file

Copy [`bitbucket-pipelines.yml`](bitbucket-pipelines.yml) to the root of the
repository you want OCR to review:

```bash
cp examples/bitbucket_pipelines/bitbucket-pipelines.yml ./bitbucket-pipelines.yml
```

If the repository already has a pipeline file, merge the `definitions`,
`pipelines.custom`, and `triggers.pullrequest-push` entries into it. Custom
pipeline names must remain unique.

### 2. Configure repository variables

In Bitbucket, open **Repository settings → Pipelines → Repository variables**
and add:

| Variable | Required | Secured | Description |
| --- | --- | --- | --- |
| `OCR_LLM_URL` | Yes | No | LLM API endpoint, such as `https://api.openai.com/v1/chat/completions` |
| `OCR_LLM_AUTH_TOKEN` | Yes | Yes | LLM API authentication token |
| `OCR_LLM_MODEL` | Yes | No | Model name; OCR has no built-in default and fails when this is unset |
| `BITBUCKET_ACCESS_TOKEN` | Recommended | Yes | Repository access token used as a Bearer token to post PR comments |
| `BITBUCKET_API_TOKEN` | Alternative | Yes | Scoped user API token; use with `BITBUCKET_API_TOKEN_EMAIL` |
| `BITBUCKET_API_TOKEN_EMAIL` | With API token | No | Atlassian account email belonging to `BITBUCKET_API_TOKEN` |
| `OCR_LLM_USE_ANTHROPIC` | No | No | Set to `true` for the Anthropic API format; defaults to `false` |

Configure **one** Bitbucket authentication method:

- `BITBUCKET_ACCESS_TOKEN` (recommended), or
- both `BITBUCKET_API_TOKEN` and `BITBUCKET_API_TOKEN_EMAIL`.

Mark both the LLM token and Bitbucket token as secured. Do not print them or
place them directly in `bitbucket-pipelines.yml`.

Repository variables can be used by anyone with write access to the
repository. Limit write access and changes to pipeline configuration to trusted
users, even when the variables are marked as secured.

Bitbucket supplies the remaining context automatically:
`BITBUCKET_WORKSPACE`, `BITBUCKET_REPO_SLUG`, `BITBUCKET_PR_ID`,
`BITBUCKET_PR_DESTINATION_BRANCH`, and `BITBUCKET_COMMIT`.

### 3. Create a Bitbucket token

#### Option A: Repository access token (recommended)

1. Open **Repository settings → Security → Access tokens**.
2. Select **Create access token** and give it a descriptive name, such as
   `OpenCodeReview Bot`.
3. Grant **Pull requests: Write** permission.
4. Copy the token into the secured `BITBUCKET_ACCESS_TOKEN` repository
   variable.

Repository access tokens are limited to the repository and appear under their
token name when they create content, which makes them a good fit for a review
bot. The pipeline sends this token with `Authorization: Bearer`.

#### Option B: Scoped user API token

1. In the Atlassian account security settings, select
   **Create and manage API tokens → Create API token with scopes**.
2. Choose Bitbucket and grant **Pull requests: Read**
   (`read:pullrequest:bitbucket`). Bitbucket includes permission to comment on
   pull requests in this scope.
3. Store the token as the secured `BITBUCKET_API_TOKEN` variable.
4. Store the token owner's Atlassian account email as
   `BITBUCKET_API_TOKEN_EMAIL`.

The pipeline uses Basic authentication with the Atlassian account email as the
username and the API token as the password.

> **App Password retirement:** Bitbucket stopped allowing new App Passwords on
> September 9, 2025 and disabled all existing App Passwords on June 9, 2026.
> Use a repository access token or scoped API token instead.

### 4. Enable Pipelines

If Pipelines is not already enabled, open **Repository settings → Pipelines →
Settings** and enable it. Commit the root `bitbucket-pipelines.yml` file to the
repository's default branch so Bitbucket can discover the trigger definition.

The `pullrequest-push` trigger covers both PR creation and updates. It invokes
the named custom pipeline for every source branch.

## Comment behavior

OCR JSON findings contain a path, content, and new-side start/end lines. The
posting script maps those fields to Bitbucket's `content.raw` and `inline`
comment properties:

- Findings with a valid path and line are posted inline.
- If Bitbucket rejects an inline position with HTTP 400, the script retries
  the finding as a general PR comment with the file and line range in its
  heading.
- Findings without usable line information are posted as general comments.
- Suggested replacement code is included in a fenced Markdown block.
- Every run ends with an `OpenCodeReview summary` comment, including runs that
  find no issues.

Any authentication, permission, rate-limit, or server error fails the posting
step instead of silently losing comments.

## Customization

### Pin an OCR version

For reproducible builds, replace the install command with a version validated
by your team:

```yaml
- npm install -g @alibaba-group/open-code-review@1.7.12
```

### Use an Anthropic endpoint

Set `OCR_LLM_USE_ANTHROPIC` to `true`. Leave it unset or set it to `false` for
OpenAI-compatible endpoints.

### Add custom review rules

Read a trusted rule file from the destination branch, then pass the extracted
copy to `--rule`:

```yaml
git show \
  "origin/${BITBUCKET_PR_DESTINATION_BRANCH}:.opencodereview/rule.json" \
  > /tmp/ocr-rule.json

ocr review \
  --rule /tmp/ocr-rule.json \
  --from "origin/${BITBUCKET_PR_DESTINATION_BRANCH}" \
  --to "${BITBUCKET_COMMIT}" \
  --format json \
  --audience agent
```

Do not load executable scripts or trusted configuration directly from an
unreviewed PR branch when secured variables are available to the step.

### Limit LLM concurrency

Add `--concurrency` to control parallel LLM requests:

```yaml
ocr review \
  --concurrency 5 \
  --from "origin/${BITBUCKET_PR_DESTINATION_BRANCH}" \
  --to "${BITBUCKET_COMMIT}"
```

### Review only selected destination branches

Change the trigger condition, for example:

```yaml
triggers:
  pullrequest-push:
    - condition: BITBUCKET_PR_DESTINATION_BRANCH == "main"
      pipelines:
        - ocr-pr-review
```

### Use the traditional pull-request selector

For repositories that do not use condition-based triggers, the same step can
be selected with the traditional syntax:

```yaml
pipelines:
  pull-requests:
    "**":
      - step: *ocr-pr-review
```

Do not configure both forms for the same branches, or Bitbucket may run the
review twice.

## Notes and limitations

- **Fork pull requests:** Bitbucket Cloud does not trigger pull-request
  pipelines for pull requests whose source is a fork.
- **Merged build setup:** Bitbucket prepares pull-request builds by merging the
  destination branch into the source branch. OCR still uses the explicit
  destination tracking ref and `$BITBUCKET_COMMIT` so it reviews the PR diff,
  not Bitbucket's temporary merge result.
- **Clone history:** The example uses `clone: depth: full` so OCR can find a
  merge base for long-lived branches.
- **Repeated runs:** Each source-branch update creates a new set of findings
  and a new summary. Bitbucket Cloud limits a pull request to 200 comments, so
  teams with frequently updated or very large PRs may want to add an
  idempotency policy or consolidate findings.
- **Bitbucket Cloud:** The API URL in this example is
  `https://api.bitbucket.org/2.0`. Bitbucket Data Center uses different APIs
  and is not covered by this configuration.

## Troubleshooting

### HTTP 401

Check that the selected authentication method is complete:

- Repository access token: `BITBUCKET_ACCESS_TOKEN`
- API token: both `BITBUCKET_API_TOKEN` and `BITBUCKET_API_TOKEN_EMAIL`

For API tokens, the email must be the Atlassian account email, not the
Bitbucket username or token label.

### HTTP 403

Check the token scope and confirm that the token owner or repository token can
access the current repository and comment on its pull requests.

### OCR review produced no output

The pipeline prints OCR stderr before failing. Use those diagnostics to verify
`OCR_LLM_URL`, `OCR_LLM_AUTH_TOKEN`, and `OCR_LLM_MODEL`. Also confirm that the
destination branch was fetched and that the PR changes contain file types
supported by OCR.

### Cannot find a merge base

Keep `clone: depth: full` and the explicit destination-branch `git fetch`.

### Duplicate pipeline runs

Pull-request pipelines can run in addition to `default` or `branches`
pipelines. Ensure only one configured pipeline invokes OCR for PR events, and
do not combine the `pullrequest-push` trigger with the traditional
`pull-requests` selector for the same PRs.

## References

- [Bitbucket pipeline start conditions](https://support.atlassian.com/bitbucket-cloud/docs/pipeline-start-conditions/)
- [Bitbucket Pipelines variables and secrets](https://support.atlassian.com/bitbucket-cloud/docs/variables-and-secrets/)
- [Bitbucket clone behavior](https://support.atlassian.com/bitbucket-cloud/docs/git-clone-behavior/)
- [Pull Request Comments API](https://developer.atlassian.com/cloud/bitbucket/rest/api-group-pullrequests/#api-repositories-workspace-repo-slug-pullrequests-pull-request-id-comments-post)
- [Repository access token permissions](https://support.atlassian.com/bitbucket-cloud/docs/repository-access-token-permissions/)
- [Using scoped API tokens](https://support.atlassian.com/bitbucket-cloud/docs/using-api-tokens/)
