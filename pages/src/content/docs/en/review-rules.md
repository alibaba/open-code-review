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

| Priority    | Source         | Path                                  | Notes                                     |
| ----------- | -------------- | ------------------------------------- | ----------------------------------------- |
| 1 (highest) | `--rule` flag  | user-specified                        | CLI override; always wins when supplied.  |
| 2           | Project config | `<repoDir>/.opencodereview/rule.json` | Per-project rules — safe to commit.       |
| 3           | Global config  | `~/.opencodereview/rule.json`         | User-wide preferences.                    |
| 4 (lowest)  | System default | embedded `system_rules.json`          | Built-in rules covering common languages. |

If a higher-priority layer's file doesn't exist, it's silently skipped —
not an error. So a project that never adds `.opencodereview/rule.json`
just falls through to the global / system layers.

The system layer is **always** present (it ships in the binary), so there
is always _some_ rule resolved.

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

- `include` — optional. Glob patterns that _bypass_ built-in default
  exclude patterns (test-file exclusions — see below). It is not a
  whitelist: files not matching any `include` pattern still proceed
  through the `unsupported_ext` and `default_path` checks and may still
  be reviewed.
- `exclude` — optional. Glob patterns for files OCR must _not_ review.
  Highest precedence among user-configured filters.
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

The filter is a six-gate algorithm in
[`internal/agent/selection.go`](https://github.com/alibaba/open-code-review/blob/main/internal/agent/selection.go).
For each diff, OCR asks:

1. **`binary`** — Is the file binary? Excluded.
2. **`secret_exclude`** — Does either path match a built-in secret-path
   protection? Excluded. This protection runs before user rules and cannot
   be overridden by an `include` pattern; the patterns are listed under
   [Built-in secret paths](#built-in-secret-paths) below.

3. **`user_exclude`** — Does the path match any user `exclude` pattern?
   Excluded.
4. **`user_include`** — If the user defined `include`, does the path
   match? If yes, **kept immediately** (bypasses the `unsupported_ext`
   and `default_path` gates below).
5. **`unsupported_ext`** — Is the file extension in the
   [allowlist](https://github.com/alibaba/open-code-review/blob/main/internal/config/allowlist/supported_file_types.json)?
   Excluded if not.
6. **`default_path`** — Does the path match a built-in exclude pattern?
   Excluded. These cover test files (`**/*_test.go`,
   `**/*.test.{js,jsx,ts,tsx}`, `**/*_spec.rb`, …) and dependency or
   build-output directories (`**/node_modules/**`, `**/vendor/**`,
   `**/target/**`, …).

Files that survive all six gates are sent to the LLM, unless the diff
alone exceeds 80% of `max_tokens`: `selectFiles` applies that ceiling
after the gates and excludes the file as `too_large`. It also marks a
file whose new path is `/dev/null` as `deleted`; there's no new content
to review. Use `ocr review --preview` to print the result of this filter
without spending a token.

### Built-in secret paths

The built-in secret paths are not reviewed (see
[`internal/config/allowlist/default_secret_patterns.json`](https://github.com/alibaba/open-code-review/blob/main/internal/config/allowlist/default_secret_patterns.json)):

- `**/.ssh/**`
- `**/id_rsa`
- `**/id_dsa`
- `**/id_ecdsa`
- `**/id_ed25519`
- `**/.netrc`
- `**/_netrc`
- `**/.npmrc`
- `**/.pypirc`
- `**/.dockercfg`

`.env` and any `.env.*` variant is likewise treated as a secret path, except the templates `.env.example`, `.env.sample`, and `.env.template`.

### Default path exclusions

The built-in exclude list (see
[`internal/config/allowlist/default_exclude_patterns.json`](https://github.com/alibaba/open-code-review/blob/main/internal/config/allowlist/default_exclude_patterns.json))
covers two groups. Test files across languages, plus test fixtures,
snapshots, and generated code:

- `**/*_test.go`
- `**/src/test/java/**/*.java`
- `**/src/test/**/*.{kt,kts}`
- `**/*Test.fs`
- `**/*.test.{js,jsx,ts,tsx}`
- `**/*.spec.{js,jsx,ts,tsx}`
- `**/__tests__/**`
- `**/test/**/*_test.py`
- `**/tests/**/*_test.py`
- `**/*_test.py`
- `**/test_*.py`
- `**/*_spec.rb`
- `**/spec/**/*_spec.rb`
- `**/*Test.java`
- `**/*Tests.java`
- `**/*_test.rs`
- `**/oh_modules/**`
- `**/*.test.ets`
- `**/test/**/*.jl`
- `**/test/**/*.hs`
- `**/*Spec.hs`
- `**/test/**/*.lhs`
- `**/*Spec.lhs`
- `**/tests/**/*.nim`
- `**/tests/**/*.R`
- `**/__snapshots__/**`
- `**/*.snap`
- `**/testdata/**`
- `**/fixtures/**`
- `**/.ipynb_checkpoints/**`
- `**/*.generated.*`
- `**/*.gen.go`
- `**/*.pb.go`
- `**/*.pb.cc`
- `**/*.pb.h`
- `**/*Test.swift`
- `**/*Tests.swift`
- `**/Tests/**/*.swift`
- `**/tests/**/*.elm`
- `**/vendor/**/*.{jsonnet,libsonnet}`
- `**/test/**/*.zig`
- `**/*_test.zig`
- `**/kitex_gen/**/*.go`
- `**/*.capnp.h`
- `**/*.capnp.go`
- `**/*.capnp.ts`
- `**/*_capnp.rs`
- `**/*_capnp.py`
- `**/test/**/*.ml`
- `**/tb_*.{v,sv,vhd,vhdl}`
- `**/*_tb.{v,sv,vhd,vhdl}`
- `lib/**/*.sol`
- `**/*.t.sol`
- `**/test/**/*.sol`
- `**/tests/**/*.sol`
- `**/test/**/*.vy`
- `**/tests/**/*.vy`

…and dependency or build-output directories:

- `**/node_modules/**`
- `**/bower_components/**`
- `**/vendor/**`
- `**/target/**`
- `**/dist/**`
- `**/__pycache__/**`, `**/.venv/**`, `**/site-packages/**`
- `**/Pods/**`, `**/Carthage/**`
- `**/.next/**`, `**/.nuxt/**`, `**/.gradle/**`, `**/.terraform/**`, …

`**/build/**` and `**/bin/**` are deliberately absent: projects keep
hand-written sources in both.

The same noisy directories are also filtered earlier, at the diff level in
[`internal/diff/git.go`](https://github.com/alibaba/open-code-review/blob/main/internal/diff/git.go).
That list matches by path prefix, so it catches only a directory at the
**repository root**. `vendor/pkg/x.go` never reaches the per-file filter and
is reported as `provider_directory`; `api/vendor/pkg/x.go` does reach it and
is reported as `default_path`. Only the second can be brought back by an
`include` rule.

To **review** a file that matches one of these patterns, add
it to the user `include` list — that overrides the default-path gate.

## Rule resolution per file

After filtering decides a file _will_ be reviewed, OCR picks the rule
text the agent should follow:

1. Try `--rule` (custom) layer in declaration order.
2. Try `<repo>/.opencodereview/rule.json` in declaration order.
3. Try `~/.opencodereview/rule.json` in declaration order.
4. Fall back to the embedded system rule layer.

Selected embedded `system_rules.json` patterns are shown below in relative
matching order:

| Pattern                             | Rule doc                                                                                              |
| ----------------------------------- | ----------------------------------------------------------------------------------------------------- |
| `**/*.properties`                   | `properties.md` — i18n / configuration files.                                                         |
| `**/*{mapper,dao}*.xml`             | `mapper_dao_xml.md` — MyBatis-style mapper SQL.                                                       |
| `**/pom.xml`                        | `pom_xml.md` — Maven dependencies.                                                                    |
| `**/build.gradle`                   | `build_gradle.md` — Gradle dependencies.                                                              |
| `**/package.json`                   | `package_json.md` — NPM dependencies / scripts.                                                       |
| `**/Cargo.toml`                     | `cargo_toml.md` — Rust manifest.                                                                      |
| `**/composer.json`                  | `composer_json.md` — Composer dependencies, autoloading, scripts, plugins, and package configuration. |
| `**/*.{json,json5}`                 | `json.md` — generic JSON (also matches `.json5`).                                                     |
| `.github/workflows/**/*.{yaml,yml}` | `github_workflows.md` — GitHub Actions workflow YAML.                                                 |
| `.github/**/*.{yaml,yml}`           | `github_config.md` — other `.github` config YAML.                                                     |
| `**/*.{yaml,yml}`                   | `yaml.md`                                                                                             |
| `**/*.java`                         | `java.md`                                                                                             |
| `**/*.go`                           | `go.md` — Go source.                                                                                  |
| `**/*.{ftl,ftlh,ftlx}`              | `freemarker.md` — FreeMarker templates (SSTI / XSS / null handling).                                  |
| `**/*.{hbs,mustache}`               | `handlebars_mustache.md` — Handlebars and Mustache templates.                                         |
| `**/*.ets`                          | `arkts.md` — ArkTS / HarmonyOS.                                                                       |
| `**/*.astro`                        | `astro.md` — Astro components and islands.                                                            |
| `**/*.{ts,js,tsx,jsx,mjs,cjs}`      | `ts_js_tsx_jsx.md`                                                                                    |
| `**/*.{kt,kts}`                     | `kotlin.md`                                                                                           |
| `**/*.{fs,fsi,fsx}`                 | `fsharp.md` — F# implementation, signature, and script files.                                         |
| `**/*.rs`                           | `rust.md`                                                                                             |
| `**/*.R`                            | `r.md`                                                                                                |
| `**/*.{cpp,cc,cxx,hpp,hxx}`         | `cpp.md`                                                                                              |
| `**/*.c`                            | `c.md`                                                                                                |
| `**/*.{py,pyi,ipynb}`               | `python.md` — Python source.                                                                          |
| `**/*.{php,phtml}`                  | `php.md` — PHP source and PHP templates.                                                              |
| `**/*.proto`                        | `protobuf.md` — Protocol Buffers wire compatibility.                                                  |
| `**/*.po`                           | `po.md` — gettext translation source catalogs.                                                        |
| `**/*.pot`                          | `pot.md` — gettext template files.                                                                    |
| `**/*.{graphql,gql}`                | `graphql.md` — GraphQL schema and operations.                                                         |
| `**/*.prisma`                       | `prisma.md` — Prisma schema.                                                                          |
| `**/*.jl`                           | `julia.md` — Julia source.                                                                            |
| `**/*.{tf,hcl,tfvars}`              | `terraform.md` — Terraform / HCL.                                                                     |
| `**/*.bicep`                        | `bicep.md` — Bicep (Azure) templates.                                                                 |
| `**/*.elm`                          | `elm.md` - Elm source.                                                                                |
| `**/*.{jsonnet,libsonnet}`          | `jsonnet.md` — Jsonnet configuration templates and libraries.                                         |
| `**/*.thrift`                       | `thrift.md` — Apache Thrift IDL wire compatibility.                                                   |
| `**/*.capnp`                        | `capnp.md` — Cap'n Proto schema wire compatibility.                                                   |
| `**/*.{v,sv,vh}`                    | `verilog.md` — Verilog and SystemVerilog RTL.                                                         |
| `**/*.{vhd,vhdl}`                   | `vhdl.md` — VHDL RTL.                                                                                 |
| `**/*.m`                            | `matlab.md` (or `objc.md` via [content sniffing](#content-sniffing-for-m-files))                      |
| `**/*.mm`                           | `objc.md` — Objective-C++ source.                                                                     |
| `**/*.sol`                          | `solidity.md` — Solidity smart contracts.                                                             |
| `**/*.vy`                           | `vyper.md` — Vyper smart contracts.                                                                   |
| `**/*.rego`                         | `rego.md` — Rego policy (OPA).                                                                        |
| _(fallback)_                        | `default.md`                                                                                          |

The resolved rule body becomes the `{{system_rule}}` placeholder in the
plan and main task prompts.

### Content sniffing for `.m` files

`.m` is shared by MATLAB and Objective-C. OCR peeks at the file's first
non-blank line to disambiguate: if it looks like Objective-C (e.g. `#import`,
`@implementation`, a C-style comment), `objc.md` is used instead of
`matlab.md`. When the content cannot be read, resolution falls back to
`matlab.md`.

> **Stability note.** The sniff heuristic may change between OCR versions. If
> you need deterministic `.m` routing, set an explicit project-level rule for
> your `.m` paths — project rules always outrank the system layer.

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
