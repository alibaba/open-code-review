#### Move Package Manifest Hygiene
- Git dependency resolution that is not fixed. On Sui a branch or tag `rev` is pinned to a commit in `Move.lock`, so flag a `Move.lock` that is missing, ignored by git, or repinned in the same change without a stated reason, not the branch itself. On Aptos, prefer a pinned commit for third-party dependencies, but verify against the project's toolchain before reporting. A framework dependency on its release branch (`framework/mainnet` on Sui, `mainnet` on Aptos) is the documented convention and is not a finding by itself
- Framework dependencies that should share one revision pinned to different revisions, mixing incompatible versions of the same source tree
- `local =` paths that are absolute or reach outside the repository, which do not resolve on another machine or in CI
- Dependencies used only by tests placed in `[dependencies]` instead of `[dev-dependencies]`, which is only included in dev and test modes

#### Addresses and Publication
- Do not flag an `[addresses]` entry of `"_"` by itself: it is a documented placeholder that a dependent package or `--named-addresses` fills in. Flag it only when the build or publish inputs in the change leave it unset or set it to the wrong address
- Named addresses changed in place for an already-published package, silently retargeting every reference

#### Dependency Replacement Consistency (Sui)
- `published-at` or `original-id` drifting between `[dep-replacements.testnet]` and `[dep-replacements.mainnet]`, or between two packages that must link against the same on-chain package. Toolchain-dependent: newer Sui package managers record these elsewhere, so check the project's Sui version first
- A `dep-replacements` entry whose `rev` does not match the corresponding `[dependencies]` entry, so source typechecking and on-chain linkage disagree

#### Upgrade Policy and Metadata
- `upgrade_policy` strengthened to `dep_only` or `immutable` without a stated reason: the policy can only get stronger, so this is irreversible. A weaker policy is rejected on chain and is not a finding
- Package `name` changed in ways that break dependents that resolve by name
