# Contributing to OpenCodeReview

Thank you for your interest in contributing to OpenCodeReview! Every contribution matters — whether it's fixing a typo, reporting a bug, or implementing a new feature.

[简体中文版](CONTRIBUTING.zh-CN.md) | [日本語版](CONTRIBUTING.ja-JP.md) | [한국어](CONTRIBUTING.ko-KR.md) | [Русский](CONTRIBUTING.ru-RU.md)

## Code of Conduct

By participating in this project, you agree to maintain a respectful and inclusive environment. Please be kind and constructive in all interactions.

## Ways to Contribute

There are many ways to contribute beyond writing code:

- **Report bugs** — Found something broken? Open an issue with reproduction steps.
- **Suggest features** — Have an idea for improvement? You can start a conversation in [GitHub Discussions](https://github.com/alibaba/open-code-review/discussions/categories/ideas) or open a [Feature Request](https://github.com/alibaba/open-code-review/issues/new?template=feature_request.yml) issue.
- **Improve documentation** — Fix typos, clarify explanations, or add examples. You can also open a [Documentation Issue](https://github.com/alibaba/open-code-review/issues/new?template=docs_report.yml) to report problems.
- **Review pull requests** — Help us review code from other contributors.
- **Write code** — Fix bugs, add features, or improve performance.

## Getting Started

### Prerequisites

- [Go 1.25+](https://go.dev/dl/)
- [Git](https://git-scm.com/)
- [Make](https://www.gnu.org/software/make/)

### Setup

```bash
# 1. Fork the repository on GitHub

# 2. Clone your fork
git clone https://github.com/<your-username>/open-code-review.git
cd open-code-review

# 3. Add upstream remote (for syncing updates from the main repo)
git remote add upstream https://github.com/alibaba/open-code-review.git

# 4. Build the project
make build

# 5. Run tests
make test
```

If everything passes, you're ready to contribute.

> **Note:** The `upstream` remote is read-only for contributors — it is used to pull the latest changes from the main repository. You cannot push directly to upstream. All contributions must be pushed to your fork (`origin`) and submitted via Pull Request.

## Development Workflow

### Branching

Create a feature branch from `main`:

```bash
git checkout main
git pull upstream main
git checkout -b feat/your-feature-name
```

Use prefixes to indicate the type of change:

| Prefix      | Purpose                               |
| ----------- | ------------------------------------- |
| `feat/`     | New feature                           |
| `fix/`      | Bug fix                               |
| `docs/`     | Documentation only                    |
| `refactor/` | Code refactoring (no behavior change) |
| `test/`     | Adding or updating tests              |
| `chore/`    | Build, CI, or tooling changes         |

### Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/) format:

```
<type>(<scope>): <short summary>

[optional body]
```

Examples:

```
feat(agent): add support for custom tool definitions
fix(llm): handle timeout errors in Anthropic API calls
docs(README): update configuration examples
```

### Code Quality

Before submitting your changes, make sure they pass all checks:

```bash
# Format and lint (Go standard tooling)
go fmt ./...
go vet ./...

# Run tests with race detection
make test

# Build successfully
make build
```

### Project Structure

```
├── cmd/opencodereview/   # CLI entry point
├── internal/
│   ├── agent/            # Review agent logic
│   ├── config/           # Configuration management
│   ├── diff/             # Git diff parsing
│   ├── llm/              # LLM API client (Anthropic & OpenAI)
│   ├── model/            # Data models
│   ├── session/          # Review session management
│   ├── tool/             # Built-in tools (file_read, code_search, etc.)
│   ├── telemetry/        # OpenTelemetry integration
│   └── viewer/           # WebUI session viewer
├── pages/                # WebUI frontend
├── scripts/              # Build & install scripts
└── bin/                  # NPM wrapper
```

## Contributing to Documentation

Documentation is a crucial part of OpenCodeReview. We welcome improvements to README files, inline code comments, configuration examples, and any user-facing text.

### What Counts as a Documentation Contribution

- Fixing typos, grammar errors, or broken links
- Clarifying confusing explanations or adding missing context
- Adding usage examples for commands or configuration options
- Updating outdated content (e.g., after a feature change)
- Translating or improving localized documentation (`README.zh-CN.md`, `README.ja-JP.md`, `README.ko-KR.md`, `README.ru-RU.md`, `CONTRIBUTING.zh-CN.md`, `CONTRIBUTING.ja-JP.md`, `CONTRIBUTING.ko-KR.md`, `CONTRIBUTING.ru-RU.md`)

### Documentation Workflow

1. If you spot an issue but don't plan to fix it yourself, open a [Documentation Issue](https://github.com/alibaba/open-code-review/issues/new?template=docs_report.yml).
2. If you'd like to fix it, fork the repo, make your changes, and submit a PR with the `docs/` branch prefix (e.g., `docs/fix-config-example`).
3. Documentation-only PRs don't require test changes, but please verify that any commands or code snippets you include are accurate.

### Documentation Files

| File                    | Purpose                              |
| ----------------------- | ------------------------------------ |
| `README.md`             | Main project documentation (English) |
| `README.zh-CN.md`       | Chinese translation                  |
| `README.ja-JP.md`       | Japanese translation                 |
| `README.ko-KR.md`       | Korean translation                   |
| `README.ru-RU.md`       | Russian translation                  |
| `CONTRIBUTING.md`       | Contribution guide (English)         |
| `CONTRIBUTING.zh-CN.md` | Contribution guide (Chinese)         |
| `CONTRIBUTING.ja-JP.md` | Contribution guide (Japanese)        |
| `CONTRIBUTING.ko-KR.md` | Contribution guide (Korean)          |
| `CONTRIBUTING.ru-RU.md` | Contribution guide (Russian)         |

## Submitting Changes

### Opening an Issue

Before working on a significant change, please open an issue first to discuss the approach. This prevents duplicate work and ensures your contribution aligns with the project's direction.

When reporting a bug, include:

1. OpenCodeReview version (`ocr version`)
2. OS and architecture
3. Steps to reproduce
4. Expected vs. actual behavior
5. Relevant logs or error messages

### Pull Request Process

1. **Keep PRs focused** — One logical change per PR. If you have multiple independent changes, submit separate PRs.
2. **Write tests** — Add or update tests for any behavior changes.
3. **Update docs** — If your change affects user-facing behavior, update the relevant documentation.
4. **Sign the CLA** — All contributors must sign the Contributor License Agreement before their PR can be merged (see below).
5. **Fill in the PR template** — Describe what your change does and why.

### PR Title Format

Use the same Conventional Commits format as commit messages:

```
feat(agent): add support for custom tool definitions
```

### Review Process

- A maintainer will review your PR, usually within a few business days.
- We may request changes — this is normal and collaborative, not adversarial.
- Once approved, a maintainer will merge your PR.

## Contributor License Agreement (CLA)

We require all contributors to sign the Alibaba Open Source Contributor License Agreement before we can merge your contributions. This ensures that the project can be distributed under its license terms.

When you open your first PR, a CLA bot will post a comment with instructions. Simply follow the link to sign electronically — it only takes a minute.

## First-Time Contributors

New to the project? Look for issues labeled:

- [`good first issue`](https://github.com/alibaba/open-code-review/labels/good%20first%20issue) — Small, well-scoped tasks ideal for getting started.
- [`help wanted`](https://github.com/alibaba/open-code-review/labels/help%20wanted) — Issues where we'd appreciate community help.

Some good areas to start:

- Improving error messages and CLI output
- Writing tests for untested code paths
- Documentation improvements

## Community

- **Bug Reports** — [GitHub Issues](https://github.com/alibaba/open-code-review/issues)
- **Feature Suggestions** — [GitHub Discussions (Ideas)](https://github.com/alibaba/open-code-review/discussions/categories/ideas) or [Feature Request Issue](https://github.com/alibaba/open-code-review/issues/new?template=feature_request.yml)
- **Questions & Help** — If you have any questions about using OpenCodeReview, feel free to ask in [GitHub Discussions](https://github.com/alibaba/open-code-review/discussions)

## License

By contributing to OpenCodeReview, you agree that your contributions will be licensed under the [Apache License 2.0](LICENSE).
