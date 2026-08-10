# Agent Guidelines for open-code-review

This file provides instructions for AI coding assistants working on this project.

## Project Overview

open-code-review (`ocr`) is an AI-powered code review CLI tool written in Go (module: `github.com/alibaba/open-code-review`).

## Git Commit Notes

- Before committing, conduct a code review by running:
  ```
  ocr review --audience agent --background "briefly summarize the background requirements"
  ```
- Commit messages must be written in English.

## License Headers

- Every source file (`.go`, `.sh`, `.js`, `.mjs`, `.ts`, `.tsx`) must have an SPDX license header.
- After creating new files, run `make license-add` to add the header automatically.

## Code Style

- After writing code, run `make check` to format and check the code.
- `make check` runs: license check, `go mod tidy`, `gofmt -s -w .`, and `go vet`.

## Testing

- Run unit tests with `make test`, not `go test` directly.
- `make test` sets `LC_ALL=C` to ensure git outputs English messages.
- When writing or modifying code, add necessary unit tests to maintain coverage. The project enforces a 90% coverage threshold via `make coverage`.

## README

- When modifying README.md, always sync the changes to all localized versions:
  - README.zh-CN.md
  - README.ja-JP.md
  - README.ko-KR.md
  - README.ru-RU.md
