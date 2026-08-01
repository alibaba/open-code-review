#### Cargo Manifest Hygiene
- Avoid introducing wildcard dependency versions such as `*`; use an explicit compatible version requirement
- Avoid unpinned `git` dependencies in production crates unless a `rev`, `tag`, or documented policy makes the source reproducible
- Keep dependencies in the narrowest appropriate section: `dependencies`, `dev-dependencies`, `build-dependencies`, or target-specific dependencies
- Prefer workspace-managed versions and features in multi-crate repositories when the surrounding manifest already uses workspace inheritance

#### Edition, MSRV, and Resolver
- New packages should declare an explicit `edition`
- Library crates should declare `rust-version` when the repository has a minimum supported Rust version policy
- Workspaces using feature resolver v2 or newer should avoid accidentally falling back to legacy feature unification

#### Feature Flags
- Features should be additive and should not disable behavior in dependent crates
- Optional dependencies should be exposed through intentional feature names rather than leaking internal dependency names when that would become public API
- Default features should stay small for libraries; avoid enabling heavy optional integrations by default without a clear reason

#### Release and Metadata
- Published crates should include accurate `license` or `license-file`, `repository`, `description`, and relevant include/exclude settings
- Avoid accidentally packaging generated artifacts, credentials, local paths, test fixtures with secrets, or large binary assets
