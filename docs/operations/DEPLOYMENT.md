Parent document: `/CLAUDE.md`
Related documents:
- `docs/integrations/EXTERNAL_INTEGRATIONS.md`
- `docs/security/SECURITY_MODEL.md`
- `SECURITY.md` (release-signing detail, primary source)

Read this when:
- You need to understand how `ocr` reaches a user's machine or a CI runner — there is no server deployment to reason about.

Purpose:
- Distribution/release mechanics: environments, install paths, infrastructure assumptions.

Scope:
- Included: npm distribution, GitHub Releases, install scripts, CI runtime environments, docs-site deploy.
- Excluded: CI integration usage contracts (see `docs/integrations/EXTERNAL_INTEGRATIONS.md`).

---

# Deployment

There is no server to deploy. "Deployment" here means: how the static Go binary reaches a machine, and where that machine runs.

## Distribution paths (two, possibly overlapping)

1. **npm**: `npm install -g @alibaba-group/open-code-review`. Root `package.json` declares `optionalDependencies` on 6 per-platform packages (`@alibaba-group/ocr-{darwin,linux,win32}-{arm64,x64}`), which npm resolves to the one matching the current OS/arch.
2. **`postinstall` direct download**: `scripts/install.js` independently downloads the release binary directly from GitHub Releases (`releases/download/v{version}/opencodereview-{os}-{arch}`) plus a `sha256sum.txt`, verifies the checksum, and stages it into `bin/`.

**These appear to be two parallel mechanisms** — whether the `optionalDependencies` packages actually ship a consumed binary, or postinstall's direct download is the sole real path, was not confirmed (`publish-platform.sh` staging behavior wasn't fully traced). Treat this as an open question before relying on either path exclusively for a packaging change.

3. **Standalone installers**: `install.sh` / `install.ps1`, served from `open-codereview.ai/install.sh`, download the same GitHub Release asset directly — independent of npm entirely. `OCR_GITHUB_MIRROR` is supported but **explicitly documented as forfeiting checksum-integrity guarantees** when used — a supply-chain caveat.

## Build & release (`.github/workflows/release.yml`, triggered on `v*` tags)

Cross-compiles for 6 targets (linux/darwin/windows × amd64/arm64), `CGO_ENABLED=0` (static binaries, no C attack surface — also cited in `ASSURANCE_CASE.md`), ldflags embed `Version`/`GitCommit`/`BuildDate`. Release notes auto-generated from commit prefixes (`feat`/`fix`/`refactor`/`docs`/other). `SECURITY.md` documents GitHub Artifact Attestation (Sigstore, keyless, OIDC-backed) and SSH-signed version tags as the release-integrity mechanism — the exact workflow step wiring this in `release.yml` was not independently re-confirmed in this documentation pass.

## CI (build/test, not release) — `.github/workflows/ci.yml`

Linux job runs `self-hosted` inside `golang:1.26.6`: license-header check, GH-Action-pin verification, English-only check, `gofmt -s`, LF line-ending check, `go mod tidy` cleanliness, `go vet`, `govulncheck`, `go test -race -coverprofile` with a **90% coverage gate** (hard fail below), build + CLI smoke test. A separate `windows` job runs natively (no container, no `-race`, no coverage gate — deliberately, since `!windows`-tagged test files legitimately lower coverage there).

## Runtime environments

- **Developer machine**: interactive, config in `~/.opencodereview/`.
- **CI runner**: GitHub Actions (self-hosted, per `action.yml`'s dogfooding via `ocr-review.yml`), GitLab CI, Jenkins (Gerrit example), GitFlic, Codeup, Bitbucket Pipelines — each installs via npm and configures the LLM endpoint via env vars or `ocr config set`, scoped to the ephemeral runner's filesystem.
- **VS Code extension**: spawns the locally-installed `ocr` binary as a subprocess; no separate deployment artifact beyond the extension package itself.

## Docs site deploy (`deploy-pages.yml`)

Builds the Astro site under `pages/` on changes to `pages/**`, `install.sh`, or `install.ps1`; **verifies byte-for-byte** that the deployed copies of `install.sh`/`install.ps1` match the repo copies (`cmp`) — guards specifically against the served installer drifting from what's actually in source control.

## Known gaps / uncertainties:
- Whether the npm `optionalDependencies` platform packages carry an actual binary payload, or whether `postinstall`'s direct download is the only path exercised at install time — unconfirmed, flagged in `CLAUDE.md` known unknowns.
- Exact `release.yml` steps implementing Sigstore attestation / SSH tag signing were not independently re-verified against `SECURITY.md`'s claims.
