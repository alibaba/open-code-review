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
- Verify line endings. Line endings must be LF, not CRLF. Run `git add --renormalize .` to correct line endings and commit them. New binary files must have their extensions added to .gitattributes. 

## License Headers

- Every source file (`.go`, `.sh`, `.js`, `.mjs`, `.ts`, `.tsx`) must have an SPDX license header.
- After creating new files, run `make license-add` to add the header automatically.

## Code Style

- After writing code, run `make check` to format and check the code.
- `make check` runs: license check, English-only check, `go mod tidy`, `gofmt -s -w .`, and `go vet`.
- Source files must not contain unapproved non-English text in comments, identifiers or strings. Translated prose belongs in `README.<locale>.md`, `pages/src/content/docs/<locale>/` or an i18n table. `make english-check` enforces this in CI. It flags every letter outside ASCII, whichever the writing system — Han, kana, Hangul, Cyrillic, Greek, Arabic, and equally the diacritics that spell German, French or Vietnamese — plus combining accents and fullwidth punctuation (`：`, `（`), which is easy to leave behind in an otherwise English sentence. Symbols and emoji (`─ → ≥ ✅`) pass, since they are not letters. One gap remains: another language spelled entirely in ASCII (`Loeschen der Datei`, or a romanised transcription) takes a dictionary to spot, so it stays a matter for review.
- When non-English text is intentional (an encoding fixture, a language-switcher label), append an `allow-non-english: <reason>` marker comment to that line rather than widening the allowlist in `scripts/verify-english-only.go`.

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
