---
name: debug-session
description: >
  Debug an OpenCodeReview session from a full session UUID. Use when a user
  asks why an OCR review or provider request failed, wants a local session
  investigated, or supplies a session UUID for diagnosis. Read
  ~/.opencodereview, correlate recorded failures with the session repository
  and current codebase, and return an evidence-backed report without changing
  code.
---

# Debug OpenCodeReview Session

Input: one full session UUID. The UUID is lookup input, not proof that the
review is resumable.

This is a read-only diagnosis. Do not modify the repository, retry the
provider, resume a review, or execute commands copied from session payloads
unless the user explicitly asks for that separate action.

## 1. Locate the session

Validate the input as a full UUID before using it in a path or command:

```text
^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$
```

Search the local stores by exact filename. Include normal and test sessions:

```bash
state_dir="${HOME}/.opencodereview"
rg --hidden --files \
  -g "${session_uuid}.jsonl" \
  "${state_dir}/sessions" "${state_dir}/test-sessions" 2>/dev/null
```

Stop with a precise report when the input is invalid, the state directory is
missing, or the lookup returns zero or multiple files. Do not fall back to a
partial UUID or the newest session. Assign the single lookup result to
`session_file`.

Completion criterion: exactly one local JSONL file is identified and its path
is recorded in the report.

## 2. Build a bounded session trace

Confirm that every line is valid JSON, then inspect metadata and failure
records. Do not dump `messages`, `content`, `tool_calls`, `arguments`, or
full tool results because they can contain source code, prompts, credentials, or
provider payloads.

Use a bounded summary like this:

```bash
jq -s '
  def scrub:
    gsub("(?i)(api[_-]?key|authorization|bearer|token|password|secret)[=:][^[:space:],}]+";
         "<redacted>") | .[0:1000];
  def short:
    if . == null then null
    elif type == "string" then scrub
    elif type == "object" then
      {type: .type?, code: .code?, status: .status?,
       message: (.message? // .error?)} | tostring | scrub
    else tostring | scrub
    end;
  {
    counts: (group_by(.type) | map({type: .[0].type, count: length})),
    paths: ([.[] | .filePath? // empty] | unique),
    start: [.[] | select(.type == "session_start") |
      {timestamp, sessionId, cwd, diffFrom, diffTo, gitBranch, model, reviewMode}],
    terminal: [.[] | select(.type == "session_end") |
      {timestamp, sessionId, duration_seconds, files_reviewed, llm_failures}],
    failures: [.[] |
      select(.type == "llm_error" or .type == "review_item_failed") |
      {type, timestamp, sessionId, filePath, request_no, model,
       error: (.error | short)}],
    failed_tool_calls: [.[] |
      select(.type == "tool_call" and .ok == false) |
      {timestamp, sessionId, filePath, taskType, tool_name, ok}]
  }
' "$session_file"
```

Interpret the records with these boundaries:

- `session_end` is the terminal session record. If it is absent, report an
  incomplete local trace and do not claim a terminal review result.
- `llm_error` is provider/request evidence. Connect it to a later
  `review_item_failed` or `session_end` before calling the whole review
  failed.
- A failed `tool_call` is one tool invocation. Check whether the same file or
  conversation later has an `llm_response` or `review_item_done`; a recovered
  tool call is not a failed file review.
- `review_item_done` and `review_item_failed` determine file-level coverage.
  Do not infer coverage from an intermediate error alone.
- An interrupted session without `session_end` is not automatically a user
  cancellation, timeout, or provider failure. State which evidence is missing.
- Report `resumable` only when the session data explicitly provides it.

Completion criterion: the report has the session model, repository metadata,
terminal state, affected paths, exact failure event types, timestamps, and
request or file context where available.

## 3. Bind the trace to the codebase

Set `repo_dir` to `session_start.cwd`. Check that it still exists and is a
Git worktree before reading code:

```bash
git -C "$repo_dir" rev-parse --show-toplevel
git -C "$repo_dir" status --short --branch
git -C "$repo_dir" log -1 --format='%H %D %s'
```

Compare the current repository with the recorded `gitBranch`, `diffFrom`,
and `diffTo`. If the recorded path no longer exists, do not silently
substitute the current working directory or a similarly named worktree. Report
that the session repository is unavailable and limit the result to local
session evidence.

Read `CONTEXT.md` when present and check relevant ADRs before interpreting
session terminology or ownership boundaries.

For structural code discovery, use `codebase-memory-mcp` first:

1. Check `index_status` for the recorded repository.
2. If it is absent or stale, run one `index_repository` for that repository.
3. Use `search_graph` for the error symbol, provider adapter, or session
   writer; use `trace_path` for callers and callees; use
   `get_code_snippet` to verify the exact source.
4. Use `search_code` or `rg` for literal error text, JSONL field names, and
   files outside graph coverage. Mark any fallback limitation.

Start from the recorded `filePath`, `taskType`, `tool_name`, model, and
error text. Recover a moved path only when the repository contains an
unambiguous canonical match. Never guess a filename from a basename alone.

Completion criterion: the current code revision, repository cleanliness,
session-to-codebase match, and relevant entry-to-provider or persistence path
are all stated explicitly.

## 4. Establish the evidence chain

Build this chain for each distinct failure:

```text
session record -> failure event -> file/task -> code path -> current revision
```

Separate provider failures, host interruptions, path lookup recovery, tool-call
failures, file review failures, and session-finalization failures. Preserve the
original error text in a short, redacted excerpt and attach its timestamp,
file, request number, model, and parent record when available.

Before testing a theory, write 3–5 ranked hypotheses. Each must have a
falsifiable prediction. Test one variable at a time. Prefer an existing unit
test, fixture replay, or small read-only command that exercises the exact seam.
A captured session is a trace artifact, but it is not a red-capable
reproduction unless a command has actually been run and catches the reported
symptom.

Use these confidence labels:

- **Confirmed**: session evidence and the current code path agree, or a
  targeted reproduction fails on the same symptom.
- **Likely**: evidence and code strongly correlate, but no red-capable
  reproduction or complete terminal record exists.
- **Unknown**: the local trace or repository lacks information needed to
  distinguish causes.

Do not claim that a provider is fixed, compatible, resumable, or healthy from a
single successful event or from the presence of a session UUID.

Completion criterion: every diagnosis has an evidence reference and a
confidence label; unverified hypotheses remain explicitly unverified.

## 5. Report to the user

Return a compact report in this order:

```markdown
## Session Debug Report

- Session: <uuid>
- Local record: <path>
- Terminal state: <success | failure | incomplete trace | unknown>
- Repository: <path and match status>
- Model: <model>

### Confirmed evidence
- <timestamp> `<record type>`: <redacted error or event>, file/task context.

### Diagnosis
- **Confirmed/Likely/Unknown**: <cause and the code path that supports it>.

### Codebase status
- Revision, branch, dirty state, relevant changed files, and any mismatch with
  the session context.

### Next action
- <smallest evidence-backed next step, or the exact missing evidence>.
```

Report all independent failures that affect the outcome. Distinguish a
provider error from a host-side interruption and an intermediate tool error.
State when the result is limited to static session evidence. Never include API
keys, auth headers, tokens, full prompts, full diffs, or unredacted provider
payloads.

The skill is complete only when the local session lookup, terminal-state
classification, repository binding, code-path check, confidence labels, and
report limits are all present. Do not modify files or leave debug
instrumentation behind.
