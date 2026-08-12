---
name: base
description: "Create or reset the fixed BASE_SHA and task source used by implement and code-review."
disable-model-invocation: true
---

Use `/base` explicitly before `/implement` or `/code-review`. This skill creates
or validates the repository-local `.scratch/base` manifest. It never invokes
`implement` or `code-review` automatically.

## Commands

External task source:

```text
/base <full-40-character-BASE_SHA> --source <provider> --task-ref <ref>
```

User-provided task source:

```text
/base <full-40-character-BASE_SHA> --source user --summary <one-line summary>
```

To intentionally replace an existing baseline or task source, use `/base
reset` with the same arguments. A normal invocation with different values must
stop instead of silently overwriting the existing state.

## Manifest

Write `.scratch/base` as exactly one of these forms:

```text
base_sha: <full 40-character SHA>
source: github
ref: https://github.com/org/repo/issues/123
```

```text
base_sha: <full 40-character SHA>
source: user
summary: <one-line summary>
```

`base_sha`, `source`, and the selected source value are required. `ref` and
`summary` are mutually exclusive. Provider keys are lowercase and may be
extended without changing this format. Unknown fields, duplicate fields,
empty values, invalid SHA values, blank lines, and a user source with `ref` are
invalid.

Write the file atomically. Do not add a commit trailer and do not modify
`.git/info/exclude`. The MCP adapter automatically excludes `.scratch/**` from
review input. The finding counter is created separately by MCP after a complete
review and is cleared only by `/base reset`.

The baseline is held in memory while a round runs and reloaded from this file
after interruption. The task payload itself is not copied into state; the host
agent reads `ref` once per round and passes its complete raw response to
implement and OCR.
