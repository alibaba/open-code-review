# PR mode for the native review skill

**Decision:** The maintainer accepted the phase-one pilot. This is the design of the native skill's PR mode for [#2025](https://github.com/alibaba/open-code-review/issues/2025); pilot evidence is tracked separately in [#2023](https://github.com/alibaba/open-code-review/issues/2023).

## User interface

A bare positive integer selects PR mode in the [native skill](../../skills/open-code-review/SKILL.md): `42` and `pr 42` both review PR 42 in the GitHub CLI's default repository for this checkout. In a fork this can be the upstream repository rather than `origin`. Report the resolved PR URL so the target is visible. `42 --repo owner/repo` selects a different repository; a GitHub PR URL selects the same mode without needing `--repo`. If the repository cannot be resolved or the URL conflicts with `--repo`, report the error rather than guessing. This is skill input, not a new `ocr` CLI command. Local workspace, commit, and branch reviews keep their existing behavior and dependencies.

## The entire PR-specific skill instruction

> For a bare PR number, `pr <id>`, or a GitHub PR URL, run the installed `scripts/review_pr.py` once, as in `python3 "$SKILL_DIR/scripts/review_pr.py" 42`. Forward an explicit `--repo` or `--background` if supplied. Read the returned run summary and OCR result. Report the reviewed revisions and freshness alongside the normal Review and Analysis outcomes. On failure, report the failed step; do not substitute a branch review.

`SKILL_DIR` denotes the installed skill directory, never the PR checkout. Ship the same script alongside the standalone and [plugin](../../plugins/open-code-review/skills/open-code-review/SKILL.md) instructions. The skill has no Git command sequence, cleanup checklist, or dependency on the pilot wrapper.

## What the script guarantees

| Step | Deterministic behavior |
| --- | --- |
| Resolve | Read the canonical PR identity, actual target branch, base SHA, and head SHA from GitHub. For a fork PR, fetch from the base repository's PR ref, not a similarly named local branch. |
| Freeze | Give each run its own bare Git repository, borrowing local objects from a matching non-shallow checkout when available. Fetch and verify both commit IDs, require a unique merge-base, and create a detached worktree at the verified head. No refs or worktrees are registered in the user's repository. If a fetched revision differs or the comparison cannot be established, stop before OCR. |
| Check policy | Refuse to launch OCR if the user's config declares any MCP servers (local or remote); never edit or copy credentials. This is a preflight check, not a process sandbox: keep the trusted user config stable during a run. Omit the checkout's root `.opencodereview` entry using sparse checkout: built-in and trusted user rules still apply, and the committed files remain readable at the pinned revisions. Native Review and Analysis consult versioned Git metadata, so stop for independent review if the PR changes the root `.gitignore`, root `.gitmodules`, or any `.gitattributes`. |
| Review once | Invoke installed `ocr review` with `--repo <worktree> --from <merge-base> --to <head> --audience agent --analysis --format json --output <result>`. Forward background as data. Keep the result outside the worktree. Disable checkout hooks and avoid submodule recursion, external diff drivers, PR-provided scripts, builds, tests, and GitHub writes. |
| Release | On success, failure, or handled interruption, wait for the OCR child to stop, then remove only this run's verified, clean detached worktree without force. Drop the private object store and its refs; there are no shared refs to delete. Preserve the run summary and any OCR output. Record incomplete cleanup rather than pruning other worktrees or silently deleting modified files. |
| Recheck and return | Before returning a review, reread `(target branch, base SHA, head SHA)`. All equal means `current` **at the recorded check time**; any difference means `stale`; an unavailable or incomplete response means `unverified`. Never rerun OCR automatically. |

The script prints the absolute path to `run.json`. It records the frozen identity and revisions, OCR exit status and result path (if produced), freshness and check time (if checked), cleanup status, and any failed step. `review.json` remains the native OCR output; do not copy its findings or reinterpret coverage in the script. For example, a run can be `current` with partial Analysis, or `stale` with complete Analysis. An interrupted run must not be reported as reviewed merely because its worktree was cleaned up.

The host agent, not the helper, checks each Review claim against the recorded revisions. It reports reproduced findings with evidence and unresolved findings with the open question. If the pinned source is unavailable or the claim would require executing PR code, the claim stays unresolved. Refuted claims stay in the unchanged `review.json`; this triage does not alter coverage, freshness, or Analysis.

## Acceptance checks

Use real local Git fixtures, stub GitHub responses and a recording OCR executable. Test the installed script as a process, not just its internal functions.

- **Routing:** A numeric ID follows the GitHub CLI's repository selection, including an upstream default in a fork checkout. An override or URL selects the stated repository from an unrelated checkout. Conflicting inputs stop before fetch. Non-PR reviews work without `gh` or Python.
- **Revision identity:** A fork PR targets a non-default branch that has advanced since divergence. Assert exactly one OCR call, from the merge-base rather than the target tip, to the advertised head. A mismatch during fetch or a non-unique merge-base yields zero calls.
- **Isolation and cleanup:** Start two reviews of one PR from a dirty checkout; fail or interrupt one. Confirm independent refs, worktrees, and artifacts, an unchanged source HEAD/index/files, and that one run cannot clean the other. A modified review worktree is left intact and reported.
- **Freshness:** Exercise unchanged, retargeted, advanced-base, changed-head, and unavailable responses. Assert the reported state, retained result, and absence of an automatic rerun.
- **Distribution:** Run the same contract tests from each installed skill layout without the external wrapper. Finish with a read-only smoke review against a real PR.
- **Policy:** Configured MCP or changes to Git filtering metadata stop before OCR starts. A head-only project rule is still available as Git evidence but is never loaded as policy. Set `PR_TEST_NATIVE_OCR` to the absolute path of an installed OCR executable to also exercise its real, provider-free preview on the policy fixture.
