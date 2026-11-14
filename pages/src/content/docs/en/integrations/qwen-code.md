---
title: Qwen Code
sidebar:
  order: 3
---

Run OCR in Qwen Code using the repository's existing Claude Code marketplace
plugin. The installer converts the plugin and loads its review commands.

These instructions were verified with Qwen Code 0.27.2 on Windows. Compatibility
with the separate [Qwen Code 1.0 Preview](https://github.com/QwenLM/qwen-code/releases/tag/v1.0.0-preview)
release has not been verified.

## Prerequisites

Install Qwen Code and Git 2.41 or later, then install the OCR CLI:

```bash
npm install -g @alibaba-group/open-code-review
```

## Install

Run these commands in your terminal:

```bash
qwen extensions install alibaba/open-code-review:open-code-review
qwen extensions list
```

Start a new Qwen Code session after installing from the CLI. The plugin reuses
the [bundled command prompts](https://github.com/alibaba/open-code-review/tree/main/plugins/open-code-review/claude-code/commands).

## Run a review

Configure and test the LLM that OCR will call:

```bash
ocr config provider
ocr config model
ocr llm test
```

With Qwen Code's bundled `/review` enabled, invoke OCR with:

```text
/open-code-review.review
/open-code-review.review --from main --to feature
/open-code-review.review --commit abc123
```

Qwen Code keeps its bundled `/review` and prefixes the conflicting OCR command
with `open-code-review.`. If the bundled review skill is disabled, OCR can use
`/review` instead. Check `/help` in your session for the loaded command names.

## Delegation mode

To review with Qwen Code's own model and authentication, use:

```text
/delegate-review
/delegate-review --from main --to feature
/delegate-review --commit abc123
```

OCR runs `ocr delegate preview` and `ocr delegate rule` to select files and resolve
rules; Qwen Code performs the review. This mode requires the OCR CLI but no
separate OCR LLM configuration.

If another command already uses `/delegate-review`, Qwen Code registers the OCR
command as `/open-code-review.delegate-review`.

Both bundled commands can automatically apply fixes for findings they accept.

## Marketplace discovery

Add this repository as a source to browse the plugin in the Discover tab:

```bash
qwen extensions sources add alibaba/open-code-review
```

Run `/extensions` inside Qwen Code and open Discover.

## See also

- [Delegation Mode](../delegate/)
- [Configuration](../configuration/)
- [Qwen Code extension documentation](https://qwenlm.github.io/qwen-code-docs/en/users/extension/introduction/)
