#### Composer Manifest Review Principles
> Focus on newly introduced correctness, reproducibility, security, and deployment defects. Inspect source usage, CI, containers, deployment configuration, and nearby workspace manifests before claiming a dependency or platform incompatibility. Do not turn preferences about exact pins versus compatible ranges into findings.

#### Dependency Constraints and Resolution
- Wildcard constraints such as `*`, unconstrained `dev-*` branches, or mutable VCS references introduced without a committed, current lock file where application builds must be reproducible, or in a reusable library where consumers resolve dependencies themselves. Compatible version ranges are normal for libraries and should not be flagged by default.
- A changed constraint that unintentionally permits an incompatible major version, excludes the repository's supported range, or conflicts with another direct requirement.
- The same package declared inconsistently across `require` and `require-dev`, or a production package available only through development dependencies.
- A newly used package or mandatory PHP extension absent from `require`, causing clean production installs to fail.
- Do not report a known vulnerability without reliable advisory evidence applicable to the resolved version range.

#### PHP and Platform Compatibility
- The `php` constraint contradicts syntax or APIs used by the changed code, the framework's supported range, or the runtime configured in CI and deployment.
- A required native extension missing from `ext-*` requirements, or an extension requirement made mandatory even though the code has a working optional fallback.
- `config.platform` masking a runtime or extension mismatch that will occur in production. Confirm the actual deployment platform before reporting.
- Composer or plugin API requirements incompatible with the Composer version used by CI, containers, or release tooling.

#### Autoloading and Package Layout
- Incorrect PSR-4 namespace prefixes or paths, overlapping prefixes that resolve the wrong class, or moved classes left unreachable by autoload configuration.
- Production classes placed only in `autoload-dev`, or test-only helpers exposed through production autoloading when that changes packaged behavior.
- `autoload.files` additions that execute side effects on every Composer bootstrap or rely on an unsafe initialization order.
- Classmap, exclusion, or files entries left stale after directories are moved or renamed.

#### Scripts and Plugin Execution
- Lifecycle scripts that run destructive commands, interpolate untrusted environment values into a shell, require interactive input in CI, or invoke tools not available from declared dependencies.
- Composer scripts that recursively invoke Composer or make production installation depend on development-only packages or local state.
- A newly required Composer plugin without an intentional `config.allow-plugins` decision, or wildcard/broad authorization that permits unexpected plugin code to execute during install or update.
- Do not flag scripts or plugins solely because they execute code; establish a concrete unsafe command, trust-boundary change, or installation failure.

#### Repositories and Supply Chain
- `secure-http` disabled, plaintext repository URLs, embedded credentials, or newly introduced package sources without appropriate integrity and access controls.
- Repository priority or canonical settings that can cause a private/public package to resolve from an unintended source.
- `package` or VCS repositories pointing to mutable or unverifiable artifacts where reproducible source selection is required.
- Secrets, tokens, or private repository credentials exposed in committed manifest data. Report an internal URL only when the manifest is publicly distributed and the URL itself reveals sensitive infrastructure information.

#### Stability, Package Semantics, and Release Metadata
- `minimum-stability` weakened so unrelated development packages can enter resolution, especially without `prefer-stable`; verify whether a narrowly constrained development dependency would suffice.
- Incorrect `replace`, `provide`, or `conflict` declarations that can make Composer omit a required implementation or accept an incompatible package.
- Changes to `type`, `bin`, installer paths, archive include/exclude rules, or framework `extra` metadata that break installation or packaging.
- Published packages missing or invalid required metadata only when the repository is actually distributed as a package; do not apply publishing requirements to private applications.
