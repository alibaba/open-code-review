# Security Policy

## Supported Versions

| Version | Supported          |
|---------|--------------------|
| Latest  | :white_check_mark: |
| < Latest | :x:               |

Only the latest released version receives security updates. Users are encouraged to upgrade promptly.

## Reporting a Vulnerability

**Please do NOT report security vulnerabilities through public GitHub issues.**

Instead, use **GitHub Private Vulnerability Reporting** — go to the [Security Advisories](https://github.com/alibaba/open-code-review/security/advisories/new) page and submit a new advisory.

### What to Include

- A description of the vulnerability and its potential impact.
- Step-by-step instructions to reproduce the issue.
- Affected version(s).
- Any suggested fix or mitigation, if available.

## Response Timeline

- **Acknowledgment**: within **3 business days** of receiving your report.
- **Initial Assessment**: within **7 business days**.
- **Fix & Disclosure**: we aim to release a fix within **14 days** for confirmed critical or high-severity issues, coordinating disclosure with the reporter.

## Scope

The following are in scope for security reports:

- Remote code execution or command injection via crafted diffs, configs, or LLM responses.
- Credential or API key leakage through logs, telemetry, or output files.
- Path traversal allowing reads/writes outside the intended working directory.
- Vulnerabilities in dependencies that are exploitable through this project.

Out of scope:

- Issues in third-party LLM providers or APIs.
- Denial-of-service attacks that require local access.
- Social engineering attacks.

## Release Signatures

All release binaries and checksums are signed using [GitHub Artifact Attestations](https://docs.github.com/en/actions/security-for-github-actions/using-artifact-attestations/using-artifact-attestations-to-establish-provenance-for-builds) (Sigstore). Signatures are keyless — backed by GitHub Actions OIDC, with no long-lived private key. Version tags are signed with SSH keys via `git tag -s`.

To verify a downloaded binary:

```bash
gh attestation verify opencodereview-linux-amd64 --repo alibaba/open-code-review
```

To verify a version tag:

```bash
git tag -v v1.6.4
```

## Recognition

We appreciate the security research community's efforts. Reporters who follow responsible disclosure will be credited in the release notes (unless they prefer to remain anonymous).
