# QCA Forward integration

This integration runs Open Code Review in delegation mode inside a QCA
Forward session. OCR performs deterministic file selection and rule
resolution; the QCA host model performs the review. No OCR LLM endpoint or API
key is required.

## Assets

- [`template.example.json`](template.example.json) is a Forward Template
  example. Replace the skill and environment placeholders before publishing.
- [`system-prompt.md`](system-prompt.md) is the canonical system prompt for the
  template.
- [`../../../skills/open-code-review-delegate/SKILL.md`](../../../skills/open-code-review-delegate/SKILL.md)
  is the Skill package to publish and bind to the template.

## Runtime requirements

The environment referenced by the template must provide:

- Git 2.41 or later.
- A compatible `ocr` binary on `PATH`.
- A checked-out repository as the session workspace.

For production, preinstall and pin OCR in the runtime image. Installing the
latest NPM package during every session adds network, startup, and compatibility
risk.

## Publish flow

1. Package and publish `skills/open-code-review-delegate` as a QCA Skill.
2. Build or select an environment with Git and OCR preinstalled.
3. Replace `skill_open_code_review_delegate`, `env_code_review`, and
   `PIN_AT_PUBLISH_TIME` in the template example with real resource IDs and
   pinned Skill/OCR versions.
4. Create the Forward Template and bind a repository through the Template or
   Identity configuration.
5. Create a new session and send a workspace, range, or commit review request.

Template updates affect new sessions only. Keep the template metadata aligned
with the pinned OCR, Skill, and delegation schema versions.

## Supported requests

```text
Review the current workspace changes.
```

```text
Review feature/order-refactor against main. The change must preserve order
state transition idempotency. Report medium-severity and higher findings.
```

```text
Review commit abc123.
```

## Execution contract

The QCA agent must:

1. Run `ocr delegate preview --format json` with the requested target.
2. Put every `reviewable_files` entry into an explicit checklist.
3. Run `ocr delegate rule --format json` for those files.
4. Review bounded batches grouped by rule and diff size.
5. Mark every file reviewed or skipped with a reason.
6. Report findings plus total, reviewed, skipped, and coverage counts.

Use `(path, status)` as the checklist identity. In workspace mode, a staged
deletion followed by an untracked recreation can legitimately produce two
entries with the same path and different statuses.

The agent must not run `ocr review`, `ocr llm test`, or request OCR model
credentials.

## Validation

- `ocr delegate preview --format json` returns `schema_version: "1"`.
- All previewed files are accounted for in the final response.
- Write and Edit are disabled. Bash read-only behavior is prompt-enforced unless
  the QCA runtime applies a stricter command policy.
- The session completes without `OCR_LLM_*` variables.
- Workspace, branch range, and single-commit reviews all work.
