### Task

Below are the diffs of one or more related files, and a set of review comments about them. Identify only the comments that these diffs **prove** to be wrong.

Every comment carries a `path`. The `<file>` element with that same path is the comment's subject; the other files are context. They can supply the evidence a cross-file comment rests on, but they never stand in for the subject file — code present somewhere in the group is not present in the file the comment was filed against.

Your default answer is to approve everything. On most reviews that is the correct answer.

### The only two grounds for removal

**Ground A — the comment targets code that is not in its subject file's diff.**

The symbol, statement, or construct the comment describes appears nowhere in the `<file>` whose path the comment names. This ground is judged against that file alone — the same construct appearing in a sibling file does not rescue the comment. Typical shapes:

- it discusses the body of a function, on a file that only declares or references it
- it discusses host-language logic on a file that holds none — a query, build, markup, or configuration file
- it claims code was removed, or an error is handled, and its subject file's diff contains no such change

**Ground B — a specific diff line literally contradicts the comment's central claim.**

The comment asserts a concrete fact and the diffs show the opposite in plain text. Unlike Ground A, the contradicting line may sit in any `<file>` in the group: a comment calling an identifier unused is wrong once any of these files uses it. The contradiction must be readable straight off the diff, not derived through a chain of reasoning. Typical shapes:

- it says an identifier is unused, and the diff shows it in use
- it says a check, assertion, or branch is missing, and the diff contains it
- it says a value is hardcoded, and the diff shows it read from a variable
- it says something is declared twice and shadows an outer name, and the diff holds exactly one declaration
- it states a condition or type relationship that the diff's own text refutes

If you cannot point to the specific diff line that establishes Ground A or Ground B, approve the comment.

### Protected subjects — never remove

These are vetoes, applied before you judge correctness at all. Whatever you conclude about the comment, approve it if its subject is:

- **Memory safety** — allocation size, buffer length, index bounds, off-by-one, use-after-free, null dereference
- **Concurrency** — locks and lock modes, atomics, data races, synchronization arguments that are not honored
- **Linkage and declaration consistency** — `static` versus non-`static`, a declaration that disagrees with its definition, missing `extern`
- **Behavioral or compatibility change** — a message, field, status, or default that the old code produced and the new code no longer does; an altered error path; a counter whose update moved to a different point in the lifecycle
- **A parameter the function accepts and never uses**

These are the categories where a wrongly removed comment is most expensive, and where your own confidence is least trustworthy — including confidence that the language, compiler, or runtime does not behave the way the comment claims. On a protected subject you do not get to be confident. Approve.

### Not grounds for removal

- The comment is about style, formatting, naming, blank lines, the wording of a code comment, or readability — **provided what it states is true**. Low value is not incorrectness, and filtering by value is not your job.
- The comment reasons about runtime behavior, business semantics, or code in files you cannot see. The Agent had access you do not.
- You disagree with its recommendation, or you consider the flagged code acceptable as written.
- You cannot confirm it. Unverifiable is not incorrect.
- It identifies a real problem but quotes a slightly wrong line or snippet. Judge the claim, not the citation.
- It is imprecise in passing while its central claim holds.

### Method

Run these steps in order for every comment. Stop at the first step that applies — do not revisit a decision a later step would have made differently.

**Step 1 — protected-subject veto.** Is the comment's subject one of the protected categories above (memory safety, concurrency, linkage and declaration consistency, behavioral or compatibility change, an unused parameter)? → **approve and stop.** Do not assess whether it is correct. This veto outranks Ground A and Ground B: a comment on a protected subject stays even when you are confident it is wrong.

**Step 2 — value veto.** Is the comment about style, formatting, naming, blank lines, the wording of a code comment, or readability, and is what it states true of this diff? → **approve and stop.** Its low value is not your concern.

**Step 3 — Ground A.** Is the code it describes absent from its subject file's diff? → **remove it.**

**Step 4 — Ground B.** Is there one diff line, in any file of the group, that literally contradicts its central claim, requiring no chain of reasoning to see? → **remove it.** Steps 3 and 4 are not optional: once a comment reaches them and qualifies, report it.

Before concluding a contradiction in Step 4, search every `<file>` for what the comment describes — not only the snippet it quoted. A comment that cites the wrong line while describing something the diffs do contain is correct, and stays.

**Step 5 —** approve.

Reaching Step 4 and needing more than a single inferential step to reach the contradiction means there is none. Approve.

### Code Diff

<review_files>
{{diff}}
</review_files>

### Review Comments

{{comments}}

### Output

You must call exactly one tool:

- `report_incorrect_comments` — only for comments meeting Ground A or Ground B, and only if you could name the diff line that disproves each one.
- `approve_all_comments` — in every other case, including when comments look doubtful, unverifiable, or minor.
