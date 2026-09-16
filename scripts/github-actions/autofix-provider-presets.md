# Provider preset autofix

The provider preset steps in the existing Linux `test` job in
`.github/workflows/ci.yml` build on the generator and read-only CI check from
#1298 (the restoration of #1221). Merge that prerequisite first.

Only same-repository pull requests are eligible; fork and Dependabot PRs keep
the normal CI check and manual regeneration instructions. The steps skip PRs
whose merge result has no changes to `internal/llm/providers.go`,
`internal/llm/protocol.go`, `internal/llm/gen/`, `go.mod`, `go.sum`, or
`extensions/vscode/src/shared/providers.generated.ts`. Comparing the merge
result with its base parent excludes provider changes made only on `main`.

The existing Go container and `actions/checkout@v7` are reused. Checkout keeps
the simulated merge ref, fetches both parents with `fetch-depth: 2`, and disables
persisted credentials. Generation uses a temporary Git worktree at the exact
event PR head, after checking that it matches the merge commit's second parent.
Tests keep their original merge checkout, and an `always()` cleanup step removes
the temporary worktree. The Bash steps live in the workflow, so eligible PR
branches do not need to contain separate autofix scripts.

GitHub scopes token permissions to jobs, not individual steps. Reusing the
test job therefore changes that job to `contents: write`; the workflow default
and other jobs remain read-only, and GitHub's normal fork token restrictions
still apply. Only the push step explicitly receives `GITHUB_TOKEN` in its
environment. This reduces incidental credential exposure; it does not create
a security sandbox between steps in the same job. Eligible same-repository
generator code must be trusted; it shares the container and Git metadata with
the later push step.

Generation removes the old catalog in the temporary worktree before running
`go generate ./internal/llm`, then requires a new regular, non-executable file.
Deleted or ignored catalogs can be restored. Other changed tracked files and
non-ignored untracked files cause failure. Only the catalog is staged and
committed. Creating the local commit requires no token; pushing it does.

Commits use the standard `github-actions[bot]` identity. A remote head check
skips outdated runs, and an explicit SHA lease protects the push against a
concurrent advance, deletion or rewind. The fix must be a direct child of the
event head, so an accepted update is a fast-forward. There is no pull, rebase
or retry of an old generated result. A failed push reports manual recovery
guidance, including checking whether the server accepted the update.

This workflow targets the public GitHub.com repository. The repository token
cannot push to contributor forks. Branch rules and CLA policy still apply;
bot attribution does not itself establish a CLA exemption. Bot commits need
checks on the new head. `GITHUB_TOKEN`-triggered PR synchronization runs require
workflow approval under [GitHub's token documentation](https://docs.github.com/en/actions/concepts/security/github_token#when-github_token-triggers-workflow-runs).
Checks on the earlier head do not validate the bot commit, and a successful
push does not guarantee that the remainder of the original run will pass.

Run the executable workflow tests with
`node scripts/github-actions/autofix-provider-presets.test.js` or the existing
`npm run test:github-actions` command. They execute the workflow's Bash steps
with a small generator fixture and local bare Git remotes. A live GitHub bot
push and subsequent check/approval cycle must be validated separately.
