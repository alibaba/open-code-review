# Security Policy

## Supported Versions

| Version | Supported          |
|---------|--------------------|
| Latest  | :white_check_mark: |
| < Latest | :x:               |

Only the latest released version receives security updates. Users are encouraged to upgrade promptly.

## Reporting a Vulnerability

**Please do NOT report security vulnerabilities through public GitHub issues.**

Instead, use one of the following channels:

1. **GitHub Private Vulnerability Reporting** — go to the [Security Advisories](https://github.com/alibaba/open-code-review/security/advisories/new) page and submit a new advisory.
2. **Email** — send a report to **open-code-review-security@alibabacloud.com** with the details below.

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

## Recognition

We appreciate the security research community's efforts. Reporters who follow responsible disclosure will be credited in the release notes (unless they prefer to remain anonymous).
