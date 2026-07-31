> Favor precision over recall: report only Nix issues that are likely to break evaluation, reproducibility, build isolation, security, or deployment behavior. Do not report formatting that `nixfmt` or project style would handle, and do not require flakes when the repository intentionally uses channels or legacy Nix.

#### Evaluation and Attribute Sets
- Duplicate attribute definitions in the same attrset, or an attribute override that unintentionally replaces a previously defined value in the changed scope.
- Referencing `self`, `super`, `pkgs`, `config`, or function arguments that are not in scope for the changed expression.
- `inherit` statements that reference missing names, or inherit from an attrset that cannot contain the requested attribute.
- Recursive attrsets (`rec`) where a value depends on itself directly or through an obvious cycle.

#### Reproducibility and Pinning
- Fetchers such as `fetchTarball`, `fetchGit`, `fetchurl`, `fetchFromGitHub`, or `builtins.fetch*` without a fixed revision and hash when the source affects a package, module, or deployment output.
- Version strings, source revisions, and hashes that are changed inconsistently, for example a package version bump without the matching source revision/hash update.
- Imports from `<nixpkgs>` or mutable channels in otherwise pinned flake or lockfile-based code, unless existing neighboring code deliberately follows the same pattern.

#### Build and Packaging Correctness
- Derivations that use undeclared build tools or runtime dependencies instead of adding them to `nativeBuildInputs`, `buildInputs`, `propagatedBuildInputs`, or wrapper inputs.
- Phase overrides that drop required default behavior without reintroducing it, such as replacing `installPhase` without installing outputs into `$out`.
- Hardcoded host paths, user home paths, or `/usr/bin` tools inside derivations that should build in the Nix sandbox.
- `meta.mainProgram`, `passthru.tests`, or output names that are changed inconsistently with installed binaries or referenced package attributes.

#### NixOS and Home Manager Modules
- Options used before declaration, options with defaults whose type does not match the declared `types.*`, or renamed options without a compatibility alias or migration path.
- Systemd service, timer, user, group, port, or file path changes that conflict with existing module options or make activation fail.
- Secrets or credentials embedded directly in module defaults, environment variables, scripts, or generated config instead of coming from secret management or protected files.

#### Overlays and Flakes
- Overlay functions with argument order or names swapped (`final`/`prev`, `self`/`super`) causing packages to be pulled from the wrong package set.
- Flake outputs that reference missing inputs, unsupported systems, or package attributes not defined for every advertised system.
- Adding a package/app/check/devShell for one system while the surrounding flake helper expects all systems to expose the same attribute.

#### Review Scope
- Focus on evaluation failures, non-reproducible sources, sandbox escapes, missing dependencies, module activation errors, and secret exposure.
- Do not flag preference-only style choices, attr ordering, or idioms that are consistent with neighboring Nix files.
