#### Move Package Manifest Hygiene
- Git dependency resolution that is not fixed. On Sui a branch or tag `rev` is pinned to a commit in `Move.lock`, so flag a `Move.lock` that is missing, ignored by git, or repinned in the same change without a stated reason, not the branch itself. The Aptos CLI refetches branch tips on every build unless `--skip-fetch-latest-git-deps` is passed, so on Aptos flag a third-party dependency on a branch rather than a commit. A framework dependency on its release branch (`framework/mainnet` on Sui, `mainnet` on Aptos) is the documented convention and is not a finding by itself
- Framework dependencies that should share one revision pinned to different revisions, mixing incompatible versions of the same source tree
- `local =` paths that are absolute or reach outside the repository, which do not resolve on another machine or in CI
- Dependencies added to `[dependencies]` when they are only used by tests, which publishes them as part of the package

#### Addresses and Publication
- Do not flag an `[addresses]` entry of `"_"` by itself: it is a documented placeholder that a dependent package or `--named-addresses` fills in. Flag it only when the build or publish inputs in the change leave it unset or set it to the wrong address
- Named addresses changed in place for an already-published package, silently retargeting every reference

#### Dependency Replacement Consistency (Sui)
- `published-at` or `original-id` drifting between `[dep-replacements.testnet]` and `[dep-replacements.mainnet]`, or between two packages that must link against the same on-chain package
- A `dep-replacements` entry whose `rev` does not match the corresponding `[dependencies]` entry, so source typechecking and on-chain linkage disagree

#### Upgrade Policy and Metadata
- `upgrade_policy` weakened from `"compatible"` without a stated reason, which removes the guarantee that existing callers keep working
- Package `name` or `version` changed in ways that break dependents that resolve by name
