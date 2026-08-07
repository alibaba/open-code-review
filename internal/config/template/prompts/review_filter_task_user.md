### Task

Given a code diff and a set of review comments, identify comments that should be removed for one of two reasons:

1. They are **provably incorrect based solely on the diff**.
2. They are high-confidence near-duplicates of another comment in the same file and describe the same underlying technical claim.

### Evaluation Principles

**Core principle: You need to falsify, not verify.**

- ✅ Should flag: The diff contains **direct counter-evidence** that proves the key claim of the review comment is wrong
- ❌ Should NOT flag: The review comment references context not visible in the diff (may have been obtained by the Agent via tools)
- ❌ Should NOT flag: You merely "cannot verify" but also cannot disprove the review comment

### Evaluation Method

For each review comment, perform the following two steps:

#### Step 1: Fact Check (Veto Rule)

- Only verify claims that are verifiable within the diff
- Only determine a comment as incorrect when the diff provides counter-evidence. **If a claim involves information outside the diff (such as logic in other files, business semantics, runtime behavior), and the diff contains no evidence contradicting it, do not make a determination.**

⚠️ Fact check fails → Immediately determine as incorrect, skip Step 2.

#### Step 2: Issue Classification

After confirming that the facts visible in the diff are accurate, determine whether the description contains a **significant deviation that can be disproved from the diff**:

- Does it misidentify clearly normal code in the diff as a defect?
- Does it attribute behavior visible in the diff in a way that contradicts the code?

⚠️ Only determine a comment is incorrect when the diff can directly prove the description is wrong.

### Duplicate Detection Rules

- Compare duplicates only within the current file.
- When both comments have non-zero `start_line` and `end_line`, consider them duplicates only when both line ranges are identical and the comments make the same underlying technical claim.
- When line resolution failed for either comment, use the fallback only when both comments clearly target the same non-empty `existing_code` and make the same underlying technical claim.
- Do not deduplicate comments merely because they occur near each other, share a category, share a severity, suggest similar-looking code, or mention the same function or component.
- Preserve complementary findings at the same location.
- Preserve repeated instances of the same issue at different locations.
- When comments are duplicates, keep one canonical comment and remove only the redundant IDs. Prefer the comment with the clearest cause and impact, most precise remediation, useful `suggestion_code`, and most appropriate category and severity.
- Do not rewrite, merge, or synthesize comment content. If the relationship is uncertain, keep both comments.
- A duplicate group must retain at least one canonical comment unless every comment in that group is independently provably incorrect.

### Code Diff

```{{path}}
{{diff}}
```

### Review Comments

{{comments}}

### Output

Return all comment IDs to remove directly, without any explanation. This includes provably incorrect comments and redundant near-duplicate comments. Use JSON array format:

```json
["id-xxx", "id-yyy"]
```

If there are no comments to remove, return an empty array:

```json
[]
```
