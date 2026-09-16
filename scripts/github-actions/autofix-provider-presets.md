# Provider preset autofix

The `provider-presets-autofix` job in `.github/workflows/ci.yml` builds on
the generator and read-only CI check from #1221. Merge #1221 first.

The job uses the same Go container version as the Linux test job. Its Bash
steps run directly from the workflow, including for older PR branches that
do not contain the autofix tooling. No separate runtime helper is required.

Only same-repository pull requests are eligible. Fork and Dependabot PRs use
the existing CI check and manual regeneration instructions. The test job
continues to validate the merge ref with read-only permissions; only the
autofix job requests `contents: write` and checks out the exact PR head.

Generation runs without persisted checkout credentials or the push token.
It removes the old catalog in the disposable checkout before running
`go generate ./internal/llm`, then requires a new regular file. Deleted or
ignored catalogs can be restored. Other changed tracked files and non-ignored
untracked files cause failure. Only the catalog is staged and committed.

The job uses the standard `github-actions[bot]` identity. A remote head check
skips outdated runs, and an explicit SHA lease protects the push against a
concurrent advance, deletion or rewind. The fix must be a direct child of
the event head, so an accepted update is a fast-forward. There is no pull,
rebase or retry of an old generated result. A failed push reports manual
recovery guidance, including checking whether the server accepted the update.

This workflow targets the public GitHub.com repository. Branch rules and CLA
policy still apply. Bot commits need checks on the new head: GitHub can require
approval for `pull_request` runs triggered by `GITHUB_TOKEN`. See
[GitHub's token documentation](https://docs.github.com/en/actions/concepts/security/github_token#when-github_token-triggers-workflow-runs).

Run the executable workflow tests with
`node scripts/github-actions/autofix-provider-presets.test.js` or the existing
`npm run test:github-actions` command. They execute the workflow's Bash steps
with a small generator fixture and local bare Git remotes. A live GitHub bot
push and subsequent check/approval cycle must be validated separately.
