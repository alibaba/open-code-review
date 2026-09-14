# Provider preset autofix

This workflow is an optional convenience on top of the generated-catalog check
introduced by [#1221](https://github.com/alibaba/open-code-review/pull/1221).
That PR must be merged first. The existing CI check continues to validate the
committed artifact independently.

The helper targets this public GitHub.com repository and uses anonymous ref
reads. Private-repository authentication and GitHub Enterprise Server support
are outside this proposal; another server is rejected before any request.

For a pull request whose branch belongs to the same repository, the workflow
first checks out `github.workflow_sha`, tests that revision's helper, and copies
it to `RUNNER_TEMP` outside the Git worktree. It then checks out the event's exact
PR head commit. Both generation and committing use the preserved helper, so a
PR branch created before this workflow was merged does not need to contain the
helper scripts. Nothing from the workflow revision is committed to the PR.

The token-free `--generate` step removes the previous generated output in this
ephemeral checkout before running `go generate ./internal/llm`, then requires a
new regular artifact. A successful Go command that runs no generator therefore
fails explicitly, even if an old catalog was committed. Generation failure stops
the job before the push step; the disposable checkout may be left without its
old output. This mode is intended for CI, not as a replacement for the local
regeneration instructions below.

The helper commits only `extensions/vscode/src/shared/providers.generated.ts` if
its contents changed or it is no longer tracked. A deleted artifact is restored
even when `.gitignore` hides the regenerated replacement. Force-adding is scoped
to that exact allowlisted file; other ignored paths are not added. The commit
uses the standard `github-actions[bot]` identity. Freshly generated, identical
tracked output does not create a commit.

The workflow grants `contents: write` only to the autofix job. It runs on an
ephemeral GitHub-hosted runner, uses `pull_request`, and does not persist checkout
credentials. The helper passes authentication only to the push command. It
rejects changes to other tracked or untracked files and verifies the remote head
before and after pushing. The fix must be a direct child of the expected PR head.
The push uses `--force-with-lease=<ref>:<expected-sha>` to require that exact remote
head: an accepted update is a fast-forward, and a concurrent update, deletion, or
rewind rejects the push. A rejected push fails with manual recovery instructions;
it does not rebase or retry a stale generated result. Preserving the helper
outside the worktree is a checkout-lifecycle fix, not a sandbox: the job still
executes same-repository PR code under the existing contributor trust model.

Fork and Dependabot PRs skip this write job. Their tokens do not have the needed
write access; the read-only CI check still reports a stale catalog. Contributors
can update their branch locally with:

```sh
go generate ./internal/llm
git add extensions/vscode/src/shared/providers.generated.ts
git commit -m "chore(vscode): regenerate provider presets"
git push
```

Repository rules, required checks, and CLA policy still apply to bot commits.
No personal access token, GitHub App installation, or CLA exemption is introduced.
GitHub currently creates approval-gated `pull_request` workflow runs for bot-token
updates with the `synchronize` activity type. A maintainer may therefore need to
approve the checks for the new bot commit. The previous commit's checks do not
validate the new commit.

References:

- [GitHub token permissions and PR-triggered runs](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows#pull_request)
- [Checkout's bot commit example](https://github.com/actions/checkout#push-a-commit-using-the-built-in-token)
- [Original suggestion by wu21-web](https://github.com/alibaba/open-code-review/pull/1221#discussion_r3999972570)

The helper's tests use temporary local Git repositories and a bare remote. Run
them with `node scripts/github-actions/autofix-provider-presets.test.js` from a
checkout containing this workflow. They include older PR heads without helper
scripts, ignored artifact restoration, and missing/no-op generator regressions.
The generation unit tests inject the Go subprocess; they do not require Go or
network access and do not replace a live Actions push/check-cycle validation.
