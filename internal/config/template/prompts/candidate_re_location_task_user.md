A review comment's existing_code matched multiple locations. Choose the one location the comment actually targets.

Rules:
1. Use the review comment, suggestion, optional reasoning, and candidate context to decide.
2. Return ONLY one JSON object, no Markdown and no explanation.
3. If no candidate is clearly correct, return {"candidate_id":null}.
4. Candidates are provided as a JSON array. Use only a candidate_id value from that array.

Output schema:
{"candidate_id":1}

Examples:

Input candidates:
[{"candidate_id":"1"},{"candidate_id":"2"}]
Correct output when candidate 2 is the target:
{"candidate_id":2}

Input candidates:
[{"candidate_id":"1"},{"candidate_id":"2"}]
Correct output when neither candidate is clearly the target:
{"candidate_id":null}

Review comment:
{suggestion_content}

Original existing_code:
```
{existing_code}
```

Suggestion:
```
{suggestion_code}
```

Reviewer reasoning:
{thinking}

Candidates:
{candidates}
