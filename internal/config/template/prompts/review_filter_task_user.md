### Task

Validate each comment against the supplied requirements and source context. A comment's `path` identifies its subject file; other files and retrieved sources may establish caller contracts and cross-file facts.

### Review rules

{{system_rule}}

### Requirement background

{{requirement_background}}

### Retrieved context

These are the reviewing agent's tool results, including the tool name, arguments, result and success status. Use successful results as source evidence. A failed or partial lookup is not evidence that a symbol, dependency or requirement does not exist. An empty list does not prove that no external context is needed.

<review_context>
{{review_context}}
</review_context>

### Code diff

<review_files>
{{diff}}
</review_files>

### Review comments

{{comments}}

### Decision method

For every candidate, identify its concrete supported trigger, alleged failure or violated requirement, and the relevant source evidence before deciding:

1. **Missing required context:** If judging the claim requires unavailable, failed, truncated or contradictory source or requirement context, call `report_incomplete_review`. Retain every comment; do not guess what the missing source would show.
2. **Actual defect:** Keep concrete correctness, security, memory safety, concurrency, compatibility, performance and maintainability defects. Severity, a proposed alternative, or a documented limitation is not a reason to remove a current defect. Do not infer that a safe failure violates a requirement for automatic recovery unless that requirement exists.
3. **Contradicted claim:** Remove a comment only when its central factual claim is refuted by the diff or retrieved source evidence. Identify the exact source and requirement. Check cross-file context before concluding that a definition, check or dependency is missing. A slightly wrong citation does not invalidate a correct central claim.
4. **Optional change:** Remove a comment that only proposes a refactor, style change, richer diagnostic or hypothetical future mode when the supplied contract and source establish no current defect. For example, one physical source reused in two build targets is not two independently maintained implementations; a deliberately generic error category is not an omitted validation. Keep either finding if evidence instead establishes a real supported failure or violated requirement. Never remove by category or wording alone.
5. **Otherwise:** Keep the comment. Preference or disagreement with a recommendation is not evidence against a defect.

### Output

Call exactly one tool:

- `report_incorrect_comments` for evidence-backed removals. Analyze every candidate before listing removable IDs; retain all actual defects.
- `approve_all_comments` when the available context supports keeping every comment. This retains findings; it is not approval to merge.
- `report_incomplete_review` when required context is missing or contradictory. Identify the missing evidence; this fails the review and removes nothing.
