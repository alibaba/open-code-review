# Roadmap

This document describes the planned direction for OpenCodeReview over the
next year. It is a living document and will be updated as priorities evolve.

Feedback is welcome via
[GitHub Discussions](https://github.com/alibaba/open-code-review/discussions)
or [Issues](https://github.com/alibaba/open-code-review/issues).

## Current State (Mid-2026)

OpenCodeReview currently provides:

- A CLI tool (`ocr`) for AI-powered code review with deterministic
  engineering and agent hybrid architecture.
- Integration with coding agents: Claude Code (plugin/skill), Codex
  (plugin), and Cursor (plugin).
- A VSCode extension for in-editor code review.
- CI/CD integration (GitHub Actions, GitLab CI, etc.).
- Multi-provider LLM support (OpenAI-compatible, Anthropic, Google Gemini,
  Amazon Bedrock, Azure OpenAI, etc.).
- Review rules engine with per-file pattern matching.
- Multi-language documentation (English, Chinese, Japanese, Korean, Russian).

## Planned — H2 2026

### IDE Plugins

- **JetBrains plugin** — Bring AI code review to IntelliJ IDEA, GoLand,
  PyCharm, and other JetBrains IDEs with the same capabilities as the
  existing VSCode extension.

### MCP Integration

- **Standard MCP server** — Expose OpenCodeReview as a
  [Model Context Protocol](https://modelcontextprotocol.io/) server,
  allowing users to integrate external context tools (documentation
  retrieval, issue trackers, internal knowledge bases) into the review
  process through the standard MCP interface.

### Ultra Mode

- **Higher-recall review mode** — An opt-in mode that trades increased
  token consumption and review time for significantly higher issue recall
  rate. Designed for security-sensitive or high-risk changesets where
  thoroughness is more important than speed.

## Planned — H1 2027

### Domain-Specific Long-Term Memory

- **Persistent review knowledge** — Enable the review engine to accumulate
  domain-specific knowledge over time (recurring patterns, past review
  decisions, project-specific conventions) and apply it to future reviews,
  improving relevance and reducing repeated feedback.

## Not Planned

The following are explicitly out of scope for the foreseeable future:

- **Automated code fixing without human review** — OCR is a review tool,
  not an auto-fix tool. While it can suggest fixes, applying changes always
  requires human approval.
- **General-purpose AI coding assistant** — OCR focuses exclusively on
  code review. Features like code generation, refactoring, or chat-based
  coding assistance are not planned.
- **Self-hosted LLM bundling** — OCR connects to external LLM providers
  but does not bundle or host models itself.
