### Task

Below are the diffs of one or more related files, and a set of review comments about them. Identify only the comments that these diffs **prove** to be wrong, and the comments that restate another surviving comment as a near-duplicate.

Every comment carries a `path`, a `start_line`, and an `end_line`. The `<file>` element with that same path is the comment's subject; the other files are context. They can supply the evidence a cross-file comment rests on, but they never stand in for the subject file — code present somewhere in the group is not present in the file the comment was filed against.

Your default answer is to approve everything. On most reviews that is the correct answer.

### The only three grounds for removal

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

**Ground C — the comment is a near-duplicate of another surviving comment.**

Two comments are near-duplicates when all of the following hold:

- they share the same `path`
- their `start_line`/`end_line` ranges overlap or abut — the same location, not just the same file
- their central claims are semantically equivalent: the same defect, at the same place, stated twice — possibly with different wording, severity, or suggested fix

For such a pair, report the redundant copy and keep the more specific or actionable one (the concrete suggestion, the higher severity, the clearer fix). When the copies are interchangeable, keep the one that appeared first.

Ground C demands equivalence, not overlap of topic:

- two comments on the same area raising different defects are not duplicates
- the same defect raised at two different locations is not a duplicate
- one broad finding plus one narrow finding that the broad one already covers completely is a duplicate; if the narrow one adds anything, keep both

If you cannot point to the specific diff line that establishes Ground A or Ground B, approve the comment. If you cannot name the surviving twin that makes a comment redundant, approve it.

### Protected subjects — never remove

These are vetoes, applied before you judge correctness at all. Whatever you conclude about the comment, approve it if its subject is:

- **Memory safety** — allocation size, buffer length, index bounds, off-by-one, use-after-free, null dereference
- **Concurrency** — locks and lock modes, atomics, data races, synchronization arguments that are not honored
- **Linkage and declaration consistency** — `static` versus non-`static`, a declaration that disagrees with its definition, missing `extern`
- **Behavioral or compatibility change** — a message, field, status, or default that the old code produced and the new code no longer does; an altered error path; a counter whose update moved to a different point in the lifecycle
- **A parameter the function accepts and never uses**

These are the categories where a wrongly removed comment is most expensive, and where your own confidence is least trustworthy — including confidence that the language, compiler, or runtime does not behave the way the comment claims. On a protected subject you do not get to be confident. Approve.

The one exception is Ground C: a protected-subject comment may still be removed as the redundant copy of a surviving near-identical twin — the finding is preserved through the twin, so nothing is lost. The twin must itself survive this review; if both copies of a protected-subject pair would otherwise be reported, approve both.

### Not grounds for removal

- The comment is about style, formatting, naming, blank lines, the wording of a code comment, or readability — **provided what it states is true**. Low value is not incorrectness, and filtering by value is not your job.
- The comment reasons about runtime behavior, business semantics, or code in files you cannot see. The Agent had access you do not.
- You disagree with its recommendation, or you consider the flagged code acceptable as written.
- You cannot confirm it. Unverifiable is not incorrect.
- It identifies a real problem but quotes a slightly wrong line or snippet. Judge the claim, not the citation.
- It is imprecise in passing while its central claim holds.
- It overlaps another comment topically while raising a distinct point — that is Ground C's equivalence test failing, and both comments stay.

### Method

Run these steps in order for every comment. Stop at the first step that applies — do not revisit a decision a later step would have made differently.

**Step 1 — protected-subject veto.** Is the comment's subject one of the protected categories above (memory safety, concurrency, linkage and declaration consistency, behavioral or compatibility change, an unused parameter)? → **approve and stop.** Do not assess whether it is correct. This veto outranks Ground A and Ground B: a comment on a protected subject stays even when you are confident it is wrong. (Its only second chance is Step 5, and only as a redundant copy whose twin survives.)

**Step 2 — value veto.** Is the comment about style, formatting, naming, blank lines, the wording of a code comment, or readability, and is what it states true of this diff? → **approve and stop.** Its low value is not your concern.

**Step 3 — Ground A.** Is the code it describes absent from its subject file's diff? → **remove it.**

**Step 4 — Ground B.** Is there one diff line, in any file of the group, that literally contradicts its central claim, requiring no chain of reasoning to see? → **remove it.** Steps 3 and 4 are not optional: once a comment reaches them and qualifies, report it.

Before concluding a contradiction in Step 4, search every `<file>` for what the comment describes — not only the snippet it quoted. A comment that cites the wrong line while describing something the diffs do contain is correct, and stays.

**Step 5 — Ground C, pairwise.** For each comment that survived Steps 1–4, group the survivors by `path` and compare those whose line ranges overlap or abut. Where two central claims are semantically equivalent, remove the redundant copy and keep the more specific or actionable one. Equivalence means a reader of either comment alone would learn the same finding; anything short of that keeps both.

**Step 6 —** approve.

Reaching Step 4 and needing more than a single inferential step to reach the contradiction means there is none. Approve. Reaching Step 5 and needing to argue that two comments "probably mean the same thing" means they do not. Approve both.

### Code Diff

<review_files>
{{diff}}
</review_files>

### Review Comments

{{comments}}

### Output

You must call exactly one tool:

- `report_incorrect_comments` — only for comments meeting Ground A, Ground B, or Ground C; for Ground A and Ground B you must be able to name the diff line that disproves the comment, and for Ground C you must be able to name the surviving twin that preserves the finding.
- `approve_all_comments` — in every other case, including when comments look doubtful, unverifiable, or minor.
