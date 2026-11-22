# PR mode for the native review skill

**Decision:** The maintainer accepted the phase-one pilot. This is the design of the native skill's PR mode for [#2025](https://github.com/alibaba/open-code-review/issues/2025); pilot evidence is tracked separately in [#2023](https://github.com/alibaba/open-code-review/issues/2023).

## User interface

A bare positive integer selects PR mode in the [native skill](../../skills/open-code-review/SKILL.md): `42` and `pr 42` both review PR 42 in the current checkout's GitHub repository. `42 --repo owner/repo` selects a different repository; a GitHub PR URL selects the same mode without needing `--repo`. If the repository cannot be resolved unambiguously or the URL conflicts with `--repo`, report the error rather than guessing. This is skill input, not a new `ocr` CLI command. Local workspace, commit, and branch reviews keep their existing behavior and dependencies.

## The entire PR-specific skill instruction

> For a bare PR number, `pr <id>`, or a GitHub PR URL, run the installed `scripts/review_pr.py` once, as in `python3 "$SKILL_DIR/scripts/review_pr.py" 42`. Forward an explicit `--repo` or `--background` if supplied. Read the returned run summary and OCR result. Report the reviewed revisions and freshness alongside the normal Review and Analysis outcomes. On failure, report the failed step; do not substitute a branch review.

`SKILL_DIR` denotes the installed skill directory, never the PR checkout. Ship the same script alongside the standalone and [plugin](../../plugins/open-code-review/skills/open-code-review/SKILL.md) instructions. The skill has no Git command sequence, cleanup checklist, or dependency on the pilot wrapper.

## What the script guarantees

| Step | Deterministic behavior |
| --- | --- |
| Resolve | Read the canonical PR identity, actual target branch, base SHA, and head SHA from GitHub. For a fork PR, fetch from the base repository's PR ref, not a similarly named local branch. |
| Freeze | Give each run its own bare Git repository, borrowing local objects from a matching non-shallow checkout when available. Fetch and verify both commit IDs, require a unique merge-base, and create a detached worktree at the verified head. No refs or worktrees are registered in the user's repository. If a fetched revision differs or the comparison cannot be established, stop before OCR. |
| Review once | Invoke installed `ocr review` with `--repo <worktree> --from <merge-base> --to <head> --audience agent --analysis --format json --output <result>`. Forward background as data. Keep the result outside the worktree. Disable checkout hooks and avoid submodule recursion, external diff drivers, PR-provided scripts, builds, tests, and GitHub writes. |
| Release | On success, failure, or handled interruption, wait for the OCR child to stop, then remove only this run's verified, clean detached worktree without force. Drop the private object store and its refs; there are no shared refs to delete. Preserve the run summary and any OCR output. Record incomplete cleanup rather than pruning other worktrees or silently deleting modified files. |
| Recheck and return | Before returning a review, reread `(target branch, base SHA, head SHA)`. All equal means `current` **at the recorded check time**; any difference means `stale`; an unavailable or incomplete response means `unverified`. Never rerun OCR automatically. |

The script prints the absolute path to `run.json`. It records the frozen identity and revisions, OCR exit status and result path (if produced), freshness and check time (if checked), cleanup status, and any failed step. `review.json` remains the native OCR output; do not copy its findings or reinterpret coverage in the script. For example, a run can be `current` with partial Analysis, or `stale` with complete Analysis. An interrupted run must not be reported as reviewed merely because its worktree was cleaned up.

## Acceptance checks

Use real local Git fixtures, stub GitHub responses and a recording OCR executable. Test the installed script as a process, not just its internal functions.

- **Routing:** A numeric ID uses the current repository; an override or URL selects the stated repository from an unrelated checkout. Conflicting inputs stop before fetch. Non-PR reviews work without `gh` or Python.
- **Revision identity:** A fork PR targets a non-default branch that has advanced since divergence. Assert exactly one OCR call, from the merge-base rather than the target tip, to the advertised head. A mismatch during fetch or a non-unique merge-base yields zero calls.
- **Isolation and cleanup:** Start two reviews of one PR from a dirty checkout; fail or interrupt one. Confirm independent refs, worktrees, and artifacts, an unchanged source HEAD/index/files, and that one run cannot clean the other. A modified review worktree is left intact and reported.
- **Freshness:** Exercise unchanged, retargeted, advanced-base, changed-head, and unavailable responses. Assert the reported state, retained result, and absence of an automatic rerun.
- **Distribution:** Run the same contract tests from each installed skill layout without the external wrapper. Finish with a read-only smoke review against a real PR.
