# Investigation protocol

Use one ongoing investigation, not a fixed number of agents or model calls. Revisit a question when new evidence contradicts it; avoid repeating reads whose observations remain available.

1. Identify the actual changes, stated goal, and a small set of behavior questions. Look for changes outside the stated goal.
2. For each material question, identify a trigger and preconditions, then inspect old and new execution/data paths. Distinguish presence, absence, false, null, errors, and empty values when relevant.
3. Trace the change to its consumers and existing user workflows. Stop expanding a path when the consequence is supported, the path is unaffected for a stated reason, or further evidence is unavailable.
4. Challenge the conclusion: check unchanged conditions, alternative consumers, pre-existing behavior, scope limits, and relevant test assertions. Do not infer execution from test names.
5. Submit supported scenarios and impacts with evidence. Mark remaining uncertainty instead of converting it into a confirmed defect.

# Conditional investigation rules

- CLI flags or commands: inspect registration, parsing, dispatch, preview paths, documented invocations, and possible migration alternatives. Do not call two controls equivalent merely because their names sound similar.
- State/status changes: follow emission, callbacks, normalization, reducers, rendering, available actions, logs, and retry inputs as applicable.
- Configuration/environment: inspect precedence, initialization time, caching, cwd, subprocess inheritance, defaults, and old/new component interoperability.
- Wire fields/schema: inspect serialization and consumers; distinguish a missing field from an explicit value, tolerant consumers from hypothetical strict consumers, and additive policy from an established compatibility guarantee.
- Async/timeout/budget changes: identify what operation is actually bounded, when resources are released, how cancellation reaches blocking work, and how downstream completion/partial states are derived.
- Refactoring/moves: compare implementations and callers before asserting behavioral changes. Inspect what equivalence fixtures actually assert, not only their names.
- Deletions/renames: inspect old paths and remaining consumers. Do not infer a deleted feature is unused because the new-side file is missing.
- Output truncation/compression: distinguish bytes, tokens, memory usage, source information loss, and how limitations survive final reporting.

These rules select useful evidence paths. They are not a checklist requiring a finding in every category, and they do not establish completeness over unsupported dynamic behavior or external consumers.
