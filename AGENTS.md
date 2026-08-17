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

- After writing code, run `make check`. It formats and tidies in place, so there is no need to run `gofmt` or `go vet` separately.
- **Source files are written in English** — comments, identifiers and strings alike. `make english-check` enforces this in CI. It flags any letter outside ASCII, whichever the writing system (Han, kana, Hangul, Cyrillic, and equally the diacritics that spell German or Vietnamese), plus combining accents and fullwidth punctuation (`：`, `（`), which is easy to leave behind in an otherwise English sentence. Symbols and emoji (`─ → ≥ ✅`) pass, since they are not letters. Prose spelled entirely in ASCII (`Loeschen der Datei`, or a romanised transcription) takes a dictionary to spot and stays a matter for review.
- **Translated prose has its own homes, none of them scanned.** `README.<locale>.md` and `CONTRIBUTING.<locale>.md` (`zh-CN`, `ja-JP`, `ko-KR`, `ru-RU`); the doc pages under `pages/src/content/docs/<locale>/` (`en`, `zh`, `ja`, `ru`, Markdown throughout); and the UI copy tables in `pages/src/i18n/<locale>.ts`. Markdown is out of scope by extension, so translations go there freely. The i18n tables are `.ts` and would be scanned, so they are exempt by prefix instead — translated UI strings belong in those tables rather than inline in a component.
- **Two escape hatches for the exceptional case, narrower one preferred.** Append an `allow-non-english: <reason>` marker comment to the offending line — the right choice for a handful of lines, such as an encoding fixture or a language-switcher label, and it leaves the rest of the file protected. Only for a whole tree that is inherently non-English, add a prefix to `allowedPrefixes` in `scripts/verify-english-only.go`; it currently holds just `pages/src/i18n/` and `extensions/vscode/`, the latter temporary until the extension's Chinese comments are translated.

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
