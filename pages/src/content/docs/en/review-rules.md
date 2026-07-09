---
title: Review Rules
sidebar:
  order: 7
---

Rules tell OCR **what to focus on** when reviewing each file. They live
in JSON files at three layers, plus an embedded system default that ships
with the binary.

## Priority chain

OCR resolves rules using a **four-layer priority chain**. For each file
path, the layers are tried in order; the first matching pattern wins.

| Priority | Source | Path | Notes |
|---|---|---|---|
| 1 (highest) | `--rule` flag | user-specified | CLI override; always wins when supplied. |
| 2 | Project config | `<repoDir>/.opencodereview/rule.json` | Per-project rules — safe to commit. |
| 3 | Global config | `~/.opencodereview/rule.json` | User-wide preferences. |
| 4 (lowest) | System default | embedded `system_rules.json` | Built-in rules covering common languages. |

If a higher-priority layer's file doesn't exist, it's silently skipped —
not an error. So a project that never adds `.opencodereview/rule.json`
just falls through to the global / system layers.

The system layer is **always** present (it ships in the binary), so there
is always *some* rule resolved.

## Rule file format (layers 1–3)

```json
{
  "include": ["src/**/*.{ts,tsx}", "src/**/*.go"],
  "exclude": ["**/*.test.ts", "**/generated/**"],
  "rules": [
    {
      "path": "src/api/**/*.go",
      "rule": "All exported handlers must validate request bodies before use."
    },
    {
      "path": "**/*mapper*.xml",
      "rule": "Check SQL for injection risks, parameter errors, and missing closing tags."
    }
  ]
}
```

Three independent fields:

- `include` — optional. Glob patterns that *bypass* built-in default
  exclude patterns (test-file exclusions — see below). It is not a
  whitelist: files not matching any `include` pattern still proceed
  through the `unsupported_ext` and `default_path` checks and may still
  be reviewed.
- `exclude` — optional. Glob patterns for files OCR must *not* review.
  Highest precedence within the filter.
- `rules` — array of `{path, rule}` entries, evaluated **in declaration
  order**. The first `path` whose glob matches the file determines the
  prompt OCR sends to the model for that file.

### Glob features

OCR uses [`bmatcuk/doublestar/v4`](https://pkg.go.dev/github.com/bmatcuk/doublestar/v4)
for matching:

- `*` — match any characters except `/`.
- `**` — match across directory boundaries (`src/**/*.go` covers any
  depth).
- `{a,b,c}` — brace expansion. `*.{ts,tsx,js,jsx}` is expanded to four
  patterns and matched in turn.
- `?` — match a single character.
- `[abc]` — character class.

> Patterns are matched **case-insensitively** (file path is lowercased
> before matching). When in doubt, use `ocr rules check <path>` to confirm.

## How files are filtered

The filter is a five-gate algorithm in
[`internal/agent/preview.go`](https://github.com/alibaba/open-code-review/blob/main/internal/agent/preview.go).
For each diff, OCR asks:

1. **`binary`** — Is the file binary? Excluded.
2. **`user_exclude`** — Does the path match any user `exclude` pattern?
   Excluded.
3. **`user_include`** — If the user defined `include`, does the path
   match? If yes, **kept immediately** (bypasses the `unsupported_ext`
   and `default_path` gates below).
4. **`unsupported_ext`** — Is the file extension in the
   [allowlist](https://github.com/alibaba/open-code-review/blob/main/internal/config/allowlist/supported_file_types.json)?
   Excluded if not.
5. **`default_path`** — Does the path match a built-in test-file exclude
   pattern (`**/*_test.go`, `**/*.test.{js,jsx,ts,tsx}`, `**/*_spec.rb`,
   …)? Excluded.

Files that survive all five gates are sent to the LLM. A `deleted`
reason (not a gate — it's computed separately in `Preview()`) marks
files whose new path is `/dev/null`; there's no new content to review.
Use `ocr review --preview` to print the result of this filter without
spending a token.

### Default path exclusions

The built-in exclude list (see
[`internal/config/allowlist/default_exclude_patterns.json`](https://github.com/alibaba/open-code-review/blob/main/internal/config/allowlist/default_exclude_patterns.json))
matches test-file patterns:

- `**/*_test.go`
- `**/src/test/java/**/*.java`
- `**/src/test/**/*.kt`
- `**/*.test.{js,jsx,ts,tsx}`
- `**/*.spec.{js,jsx,ts,tsx}`
- `**/__tests__/**`
- `**/test/**/*_test.py`
- `**/tests/**/*_test.py`
- `**/*_test.py`
- `**/*_spec.rb`
- `**/spec/**/*_spec.rb`
- `**/*Test.java`
- `**/*Tests.java`
- `**/*_test.rs`
- `**/oh_modules/**`
- `**/*.test.ets`

Noisy-directory filtering (`vendor/`, `node_modules/`, `target/`, …)
happens earlier, at the diff level in
[`internal/diff/git.go`](https://github.com/alibaba/open-code-review/blob/main/internal/diff/git.go),
before the per-file filter runs.

To **review** a file that matches one of these test-file patterns, add
it to the user `include` list — that overrides the default-path gate.

## Rule resolution per file

After filtering decides a file *will* be reviewed, OCR picks the rule
text the agent should follow:

1. Try `--rule` (custom) layer in declaration order.
2. Try `<repo>/.opencodereview/rule.json` in declaration order.
3. Try `~/.opencodereview/rule.json` in declaration order.
4. Fall back to the embedded system rule layer.

The embedded `system_rules.json` ships with these patterns (in order):

| Pattern | Rule doc |
|---|---|
| `**/*.properties` | `properties.md` — i18n / configuration files. |
| `**/*{mapper,dao}*.xml` | `mapper_dao_xml.md` — MyBatis-style mapper SQL. |
| `**/pom.xml` | `pom_xml.md` — Maven dependencies. |
| `**/build.gradle` | `build_gradle.md` — Gradle dependencies. |
| `**/package.json` | `package_json.md` — NPM dependencies / scripts. |
| `**/Cargo.toml` | `cargo_toml.md` — Rust manifest. |
| `**/*.{json,json5}` | `json.md` — generic JSON (also matches `.json5`). |
| `.github/workflows/**/*.{yaml,yml}` | `github_workflows.md` — GitHub Actions workflow YAML. |
| `.github/**/*.{yaml,yml}` | `github_config.md` — other `.github` config YAML. |
| `**/*.{yaml,yml}` | `yaml.md` |
| `**/*.java` | `java.md` |
| `**/*.ets` | `arkts.md` — ArkTS / HarmonyOS. |
| `**/*.{ts,js,tsx,jsx}` | `ts_js_tsx_jsx.md` |
| `**/*.{kt}` | `kotlin.md` |
| `**/*.rs` | `rust.md` |
| `**/*.{cpp,cc,hpp}` | `cpp.md` |
| `**/*.c` | `c.md` |
| *(fallback)* | `default.md` |

The resolved rule body becomes the `{{system_rule}}` placeholder in the
plan and main task prompts.

## Inspecting which rule wins: `ocr rules check`

```bash
$ ocr rules check src/main/java/com/example/UserService.java
File: src/main/java/com/example/UserService.java
Source: System built-in
Pattern: **/*.java
Rule:
────────────────────────────────────────
…contents of java.md…
────────────────────────────────────────
```

```bash
$ ocr rules check --rule custom.json src/main/resources/mapper/UserMapper.xml
File: src/main/resources/mapper/UserMapper.xml
Source: Custom (--rule)
Pattern: **/*mapper*.xml
Rule:
────────────────────────────────────────
…contents of your custom rule…
────────────────────────────────────────
```

Use this whenever a rule isn't behaving the way you expected — it tells
you the **layer** and the **pattern** that won.

## Recipes

### Project-level: enforce a coding standard

Save as `<repo>/.opencodereview/rule.json` and commit:

```json
{
  "rules": [
    {
      "path": "src/api/**/*.go",
      "rule": "Every public handler must `defer tx.Rollback()` immediately after starting a transaction."
    },
    {
      "path": "**/*mapper*.xml",
      "rule": "Check SQL for injection risks, missing parameter binding, and unclosed XML tags."
    }
  ]
}
```

### Project-level: skip generated code, focus on src

```json
{
  "include": ["src/**/*.{ts,tsx,js,jsx}"],
  "exclude": ["**/*.gen.ts", "**/generated/**"]
}
```

With `include` set, files inside `src/` are kept even if they'd otherwise
be dropped by a built-in default exclude pattern (e.g., a test file).
Files outside `src/` still go through the normal ext / default checks —
`include` is a bypass, not a whitelist.

### Per-PR override

```bash
ocr review --rule ./.review-rules-only-for-this-pr.json
```

Bypasses both the project and global layers — handy when a single PR
needs a totally different review checklist (e.g., security-only review).

### Global personal preferences

Put them at `~/.opencodereview/rule.json` so every repo on your machine
inherits them:

```json
{
  "rules": [
    {
      "path": "**/*.{ts,tsx,js,jsx}",
      "rule": "Always check for unhandled promise rejections; warn on `// eslint-disable` without a reason comment."
    }
  ]
}
```

## See Also

- [CLI Reference](../cli-reference/) — `ocr review --rule`, `--preview`, and `ocr rules check`.
- [Configuration](../configuration/) — config file locations and the layered resolution chain.
- [Architecture](../architecture/) — how the resolved rule feeds the agent prompt.
