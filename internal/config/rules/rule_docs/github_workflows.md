#### Security
- **pull_request_target misuse**: Using `pull_request_target` with `actions/checkout` referencing PR head code is dangerous — it runs untrusted code with write permissions. Flag if checkout ref points to PR head without isolation
- **Secrets exposure**: Secrets must not be printed to logs (e.g., `echo ${{ secrets.X }}`). Verify secrets are only passed via `env:` blocks to steps that need them
- **Excessive permissions**: Check if `permissions` is set to least-privilege. Flag `permissions: write-all` or missing `permissions` key (defaults to broad access). Each job should declare only the permissions it needs
- **Unpinned action versions**: Third-party actions should be pinned to a full commit SHA (e.g., `uses: actions/checkout@<sha>`), not just a tag. Tags are mutable and can be hijacked. First-party (`actions/*`) pinned to `v4` is acceptable
- **Script injection**: Expressions like `${{ github.event.issue.title }}` used directly in `run:` blocks enable code injection. These must be passed through environment variables instead
- **Hardcoded credentials**: Tokens, passwords, or API keys directly in the workflow file (not via secrets)

#### Correctness
- **Missing `fetch-depth: 0`**: When a workflow needs git history (tags, merge-base, changelog generation), verify `actions/checkout` uses `fetch-depth: 0`
- **Incorrect condition logic**: Verify `if:` conditions are correct (e.g., `github.event_name == 'pull_request'` vs `'pull_request_target'`); ensure boolean expressions are properly quoted
- **Matrix strategy gaps**: Check that matrix combinations cover required platforms. Flag if `fail-fast` is true (default) but all matrix legs must succeed
- **Missing `shell` specification**: When using `run:` with multi-line scripts on self-hosted runners, shell should be explicit (bash vs sh vs pwsh)
- **Broken job dependencies**: Verify `needs:` references exist as actual job IDs in the same workflow. Check for circular dependencies
- **Typos in action inputs**: Misspelled input names for actions (e.g., `fetch-detph` instead of `fetch-depth`) are silently ignored

#### Reliability
- **Missing timeout**: Jobs without `timeout-minutes` can run indefinitely and consume runner resources. Flag jobs that lack timeout (especially on self-hosted runners)
- **No concurrency control**: Workflows triggered by push/PR without `concurrency` group may create redundant runs. Suggest `concurrency` with `cancel-in-progress` where appropriate
- **Uncached dependencies**: Build workflows that install dependencies without caching (no `actions/cache` or built-in caching) on every run

#### Best Practices
- **Deprecated features**: Flag usage of deprecated syntax (`set-output`, `save-state`, `::set-output`, `actions/checkout@v2/v3` when v4 is available)
- **Missing `continue-on-error` awareness**: If a step failure should not fail the whole job, it needs `continue-on-error: true`; conversely, verify non-critical steps don't silently swallow failures with `|| true` hiding real errors
- **Container image tags**: Using `latest` tag for container images is unreliable; prefer specific version tags
