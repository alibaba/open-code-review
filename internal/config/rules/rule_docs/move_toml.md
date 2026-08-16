#### Move Package Manifest Hygiene
- Git dependencies pinned to a branch or moving tag rather than a full commit hash: `rev = "main"`, `rev = "mainnet"`, and `rev = "framework/testnet"` all resolve differently over time, so the bytecode published today may not be the bytecode published tomorrow
- Framework dependencies that should share one revision pinned to different revisions, mixing incompatible versions of the same source tree
- `local =` paths that are absolute or reach outside the repository, which do not resolve on another machine or in CI
- Dependencies added to `[dependencies]` when they are only used by tests, which publishes them as part of the package

#### Addresses and Publication
- `[addresses]` entries left as the `"_"` placeholder in a package that is meant to be published, which fails or resolves to an unintended address
- Named addresses changed in place for an already-published package, silently retargeting every reference
- `[dev-dependencies]` or `[dev-addresses]` that override a production address or dependency in a way that can leak into a non-test build

#### Dependency Replacement Consistency (Sui)
- `published-at` or `original-id` drifting between `[dep-replacements.testnet]` and `[dep-replacements.mainnet]`, or between two packages that must link against the same on-chain package
- A `dep-replacements` entry whose `rev` does not match the corresponding `[dependencies]` entry, so source typechecking and on-chain linkage disagree

#### Upgrade Policy and Metadata
- `upgrade_policy` weakened from `"compatible"` without a stated reason, which removes the guarantee that existing callers keep working
- Package `name` or `version` changed in ways that break dependents that resolve by name
