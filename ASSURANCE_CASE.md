# Security Assurance Case

This document provides a security assurance case for Open Code Review (OCR), justifying that security requirements are met through secure design principles and countermeasures against common implementation weaknesses.

## Threat Model

### System Description

OCR is a CLI tool that:

1. Reads git diff output from a local repository.
2. Sends code diffs to a configured LLM provider (OpenAI, Anthropic, etc.) via HTTPS.
3. Receives review comments from the LLM and presents them to the user.
4. Optionally serves a local web viewer for browsing review session history.

### Actors

| Actor | Trust Level |
|-------|-------------|
| Local user | Trusted — invokes the CLI with full control over configuration |
| LLM provider API | Semi-trusted — responses are validated before use |
| Git repository | Semi-trusted — diffs may contain adversarial content |
| Network | Untrusted — all communication uses TLS |
| Web browser (viewer) | Untrusted — may be exploited via DNS rebinding |

### Trust Boundaries

```
┌──────────────────────────────────────────────────┐
│  User Machine (Trusted Zone)                     │
│                                                  │
│  ┌──────────┐    ┌──────────┐    ┌────────────┐  │
│  │ Git Repo │───▶│ OCR CLI  │───▶│ Local File │  │
│  │ (diffs)  │    │ (core)   │    │  (output)  │  │
│  └──────────┘    └────┬─────┘    └────────────┘  │
│        Trust ────────▶│◀──────── Trust            │
│        Boundary 1     │         Boundary 3        │
│                       │                           │
│                  ┌────┴─────┐                     │
│                  │  Viewer  │◀── Trust Boundary 4  │
│                  │ (HTTP)   │    (Browser access)  │
│                  └──────────┘                     │
└───────────────────────┼───────────────────────────┘
           Trust ──────▶│◀────── Boundary 2
                        │
              ┌─────────┴──────────┐
              │  LLM Provider API  │
              │  (HTTPS only)      │
              └────────────────────┘
```

1. **Git → CLI**: Diff content may contain crafted payloads. Parsed with strict format validation.
2. **CLI → LLM API**: API keys transmitted over HTTPS only. Responses validated before use.
3. **CLI → Local output**: File writes constrained to the working directory.
4. **Browser → Viewer**: Host-header allowlist enforces access control; blocks DNS rebinding.

### Threat Summary

| ID | Threat | Boundary | Mitigation |
|----|--------|----------|------------|
| T1 | Command injection via crafted diff content | 1 | All external commands are `git` only, with hardcoded subcommands; no shell expansion; `--end-of-options` used to prevent flag injection |
| T2 | API key leakage | 2 | Keys read from environment variables only; never logged, written to output files, or transmitted beyond the configured LLM endpoint |
| T3 | Path traversal via LLM-suggested file paths | 3 | `pathutil.WithinBase()` validates all file paths against the repository root, both before and after symlink resolution |
| T4 | DNS rebinding against local viewer | 4 | Host-header allowlist rejects requests from non-loopback origins; configurable via `OCR_VIEWER_ALLOWED_HOSTS` |
| T5 | Man-in-the-middle on API communication | 2 | Go's `net/http` enforces TLS 1.2+ with full certificate verification by default; `InsecureSkipVerify` is never set |
| T6 | Malicious LLM response | 2 | JSON schema validation on response structure; line number bounds checking against actual diff ranges |
| T7 | Dependency vulnerabilities | All | `govulncheck` runs in CI; Dependabot monitors for updates; `go.sum` provides integrity verification |

## Secure Design Principles

The following analysis maps [Saltzer & Schroeder's design principles](https://ieeexplore.ieee.org/document/1451869) to the project's implementation.

| Principle | How Applied |
|-----------|-------------|
| **Least privilege** | `CGO_ENABLED=0` eliminates C library attack surface. The CLI requires no elevated permissions. No network listeners except the opt-in viewer. |
| **Fail-safe defaults** | API keys must be explicitly provided via environment variables. The viewer binds to localhost by default; non-loopback hosts require explicit allowlisting. |
| **Complete mediation** | Every viewer HTTP request is checked against the host allowlist (`internal/viewer/hostguard.go`). Every file path from agent tools is validated against the repository root (`internal/tool/filereader.go:98`, `internal/pathutil/path.go`). |
| **Economy of mechanism** | External process execution is limited to `git` with hardcoded subcommands — no shell invocation, no arbitrary command execution. |
| **Open design** | Fully open-source (Apache-2.0). Security relies on TLS, not obscurity. |
| **Separation of privilege** | API authentication (keys) is separated from configuration (files). The viewer's host guard is a distinct middleware layer. |
| **Least common mechanism** | Each review session writes to its own JSONL file. No shared state between sessions. |
| **Psychological acceptability** | Security defaults (HTTPS, localhost binding, host allowlist) require no user configuration. Overrides (`OCR_VIEWER_ALLOWED_HOSTS`) are explicit and documented. |

## Countermeasures Against Common Weaknesses

The following maps [OWASP Top 10](https://owasp.org/www-project-top-ten/) and [CWE/SANS Top 25](https://cwe.mitre.org/top25/) categories to the project.

| Weakness | Applicability | Countermeasure |
|----------|---------------|----------------|
| **A03:2021 Injection** (CWE-78 OS Command Injection) | All `exec.Command` calls use `git` with explicit argument lists — no shell interpolation. `--end-of-options` prevents flag injection. | Mitigated |
| **A01:2021 Broken Access Control** (CWE-22 Path Traversal) | Agent file-read tool validates paths with `pathutil.WithinBase()` before and after symlink resolution (`internal/tool/filereader.go:91-112`). | Mitigated |
| **A02:2021 Cryptographic Failures** | All API communication uses HTTPS/TLS 1.2+. Go's default TLS configuration is used without weakening. `InsecureSkipVerify` is never set. | Mitigated |
| **A07:2021 Auth Failures** (CWE-798 Hard-coded Credentials) | API keys are read exclusively from environment variables, never embedded in code or config files, never logged. | Mitigated |
| **A05:2021 Security Misconfiguration** | Secure defaults: localhost-only viewer, HTTPS-only API calls, `CGO_ENABLED=0`. The `go vet` and `govulncheck` tools run in CI. | Mitigated |
| **A06:2021 Vulnerable Components** (CWE-1104) | Dependabot monitors Go modules and GitHub Actions. `govulncheck` runs on every push/PR. Dependencies are locked via `go.sum`. | Mitigated |
| **A08:2021 Software Integrity** | Release binaries include SHA-256 checksums. Release tags are cryptographically signed (SSH). `CGO_ENABLED=0` produces static binaries with no external shared library dependencies. | Mitigated |
| **A09:2021 Logging Failures** | API keys and sensitive headers are excluded from all log output and telemetry. | Mitigated |
| **A10:2021 SSRF** | The CLI only makes outbound requests to user-configured LLM API endpoints. The viewer does not make outbound requests. | Not applicable |
| **CWE-416 Use After Free / CWE-787 Out-of-bounds Write** | Go is a memory-safe language. `CGO_ENABLED=0` eliminates C memory risks. Race detector (`-race`) runs in CI. | Not applicable (memory-safe language) |

## Automated Verification

| Check | Tool | When |
|-------|------|------|
| Static analysis | `go vet` | Every push and PR (CI) |
| Known vulnerability scan | `govulncheck` | Every push and PR (CI) |
| Data race detection | `go test -race` | Every push and PR (CI) |
| Dependency monitoring | Dependabot | Continuous |
| Build integrity | `CGO_ENABLED=0`, `go.sum` checksums | Every build |
