#### Move Package Manifest Hygiene
- Git dependency resolution that is not fixed. On Sui a branch or tag `rev` is pinned to a commit in `Move.lock`, so flag a `Move.lock` that is missing, ignored by git, or repinned in the same change without a stated reason, not the branch itself. On Aptos, prefer a pinned commit for third-party dependencies, but verify against the project's toolchain before reporting. A framework dependency on its release branch (`framework/mainnet` on Sui, `mainnet` on Aptos) is the documented convention and is not a finding by itself
- Framework dependencies that should share one revision pinned to different revisions within the same environment, mixing incompatible versions of the same source tree
- `local =` paths that are absolute or reach outside the repository, which do not resolve on another machine or in CI
- Dependencies used only by tests placed in `[dependencies]` instead of `[dev-dependencies]`, which is only included in dev and test modes

#### Addresses and Publication
- Do not flag an `[addresses]` entry of `"_"` by itself: it is a documented placeholder that a dependent package or `--named-addresses` fills in. Flag it only when the build or publish inputs in the change leave it unset or set it to the wrong address
- Named addresses changed in place for an already-published package, silently retargeting every reference

#### Dependency Replacement Consistency (Sui)
- A `[dep-replacements.<env>]` entry replaces the default dependency for that environment, so the effective dependency is the replacement when present and the `[dependencies]` entry otherwise. Do not flag a replacement `rev`, `published-at` or `original-id` for differing from the default dependency or from another environment: testnet and mainnet packages are published independently and legitimately differ
- Flag only a mismatch inside one environment: the effective dependency source (its `rev` or `local` path) does not correspond to the package that environment's `published-at` or `original-id` points to, or two packages in that environment that must link against the same on-chain package record different IDs. Toolchain-dependent: newer Sui package managers record publication data elsewhere (such as `Published.toml`), so check the project's Sui version first, and report only with both sides of the mismatch visible in the change

#### Upgrade Policy and Metadata
- `upgrade_policy` strengthened to `dep_only` or `immutable` without a stated reason: the policy can only get stronger, so this is irreversible. A weaker policy is rejected on chain and is not a finding
- Package `name` changed in ways that break dependents that resolve by name
