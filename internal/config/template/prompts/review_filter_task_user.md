### Task

Given a code diff and a set of review comments, identify those that are **provably incorrect based solely on the diff**.

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

⚠️ Only determine as incorrect when the diff can directly prove the description is wrong.

### Code Diff

```{{path}}
{{diff}}
```

### Review Comments

{{comments}}

### Output

Return all incorrect review comment IDs directly, without any explanation. Use JSON array format:

```json
["id-xxx", "id-yyy"]
```

If there are no review comments that can be confirmed as incorrect, return an empty array:

```json
[]
```
