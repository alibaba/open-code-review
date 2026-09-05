# Nushell Script Review Checklist

Comprehensive checklist for reviewing Nushell scripts. Check items in order of priority.

---

## 1. Security (Critical)

### Code injection

- [ ] No `nu -c $variable` with untrusted input
- [ ] No `source $variable` with runtime paths (must be `const`)
- [ ] No `run` of untrusted `.nu` script paths; `run` targets must be parse-time constants
- [ ] No `^sh -c`, `^bash -c`, or `^cmd.exe /C` with interpolated user input
- [ ] No `run-external` with user-controlled command names

### Path safety

- [ ] Existing user-provided paths validated with strict expansion + `path relative-to`; no string-prefix containment checks
- [ ] No raw `open $user_input` without path traversal guard
- [ ] `..` sequences in user paths detected and rejected
- [ ] Base directory enforcement for file operations

### Credential handling

- [ ] No hardcoded secrets in source code
- [ ] Credentials scoped with `with-env`, not set on `$env` directly
- [ ] Secrets read from files/stdin, not passed as command-line arguments
- [ ] No credentials logged via `print` or written to non-secure files

### Destructive operations

- [ ] `rm` operations validate target path (not `/`, not `$nu.home-dir`)
- [ ] Glob patterns from user input are validated (no unintended expansion)
- [ ] `--depth` limits on `glob` to prevent DoS on large trees

### Temp files

- [ ] Temp files created with built-in `mktemp`, not predictable paths
- [ ] Temp files cleaned up in `try/catch` or equivalent
- [ ] Temp directories use `mktemp --directory`

### Environment safety

- [ ] `$env.PATH` not modifiable by untrusted input
- [ ] Dangerous env vars (`LD_PRELOAD`, `DYLD_INSERT_LIBRARIES`) cleared before running untrusted commands
- [ ] `with-env` used for scoped environment changes in security-sensitive contexts

---

## 2. Correctness

### Type safety

- [ ] All exported commands have type annotations on parameters
- [ ] I/O pipeline signatures (`]: type -> type {`) match actual behavior
- [ ] Complex types use proper syntax: `record<name: string>`, `list<int>`, `table<col: type>`
- [ ] Optional parameters use `?` suffix: `name?: string`
- [ ] Optional params and typed named options without defaults are handled as `oneof<T, nothing>`; boolean switch flags remain `bool`
- [ ] Rest parameters typed: `...args: string`
- [ ] Runtime assignment annotations are valid under Nu 0.114's default `enforce-runtime-annotations`

### Error handling

- [ ] Fallible operations wrapped in `try/catch`
- [ ] External commands checked with `complete` when exit code matters
- [ ] Tests do not assert raw nested-Nu diagnostics without normalizing ANSI, PTY wrapping, and `|` gutters
- [ ] `catch` blocks include meaningful error context (not empty)
- [ ] `finally` used for cleanup side effects, not as the returned value
- [ ] `catch` blocks read `$err.details` for structured diagnostics, not removed `$err.json`
- [ ] Custom errors include `label` with `span` for good error messages
- [ ] No bare `error make {msg: '...'}` without span when metadata is available

### Null safety

- [ ] Optional record fields accessed with `?`: `$rec.field?`
- [ ] `default` used for fallback values: `$val | default 'N/A'`
- [ ] No bare field access on records from external/untrusted sources
- [ ] `$in` captured early with `let` when used multiple times

### Logic correctness

- [ ] `for` not used as final expression (returns null, use `each`)
- [ ] `mut` variables not captured in closures (will error)
- [ ] `source`/`use` paths are `const`, not `let`
- [ ] Long `if`/`else if` chains on one value prefer `match` unless `if` is clearer
- [ ] `if`/`match` expressions have `else`/`_` fallbacks when a non-null output is required
- [ ] `each` not used on single records (use `items` or `transpose`)
- [ ] `parse` gets `lines` first when line-by-line parsing of stream input is intended
- [ ] Correct operator: `>` in non-pipeline context is comparison, not redirect
- [ ] Multiline custom command calls with named flags are one-line or wrapped in parentheses
- [ ] SemVer logic uses `into semver` / `into semver-range` / `semver bump`, not string surgery

### External commands

- [ ] External commands prefixed with `^` when name conflicts with builtins
- [ ] `find` (Nushell builtin) vs `^find` (Unix) distinction maintained
- [ ] `sort` (Nushell builtin) vs `^sort` (Unix) distinction maintained
- [ ] Arguments to external commands separated (not concatenated strings)
- [ ] Format strings passed to external commands use double quotes for simple escapes, or `char tab` / `char nl` when interpolation is needed

---

## 3. Style & Idiom

### Naming

- [ ] Commands: `kebab-case` (`fetch-user`, not `fetchUser` or `fetch_user`)
- [ ] Variables/params: `snake_case` (`$user_id`, not `$userId`)
- [ ] Env vars: `SCREAMING_SNAKE_CASE` (`$env.APP_VERSION`)
- [ ] Flags: `kebab-case` (`--output-dir`, not `--output_dir`)
- [ ] Full words preferred (`$user_name`, not `$usr_nm`)

### String format priority

- [ ] Bare words in arrays: `[foo bar]` not `["foo" "bar"]`
- [ ] Single quotes for simple strings: `'hello'` not `"hello"`
- [ ] Single-quoted interpolation preferred: `$'val: ($x)'` not `$"val: ($x)"`
- [ ] Strings needing a **literal** `(` use `$"...\(..."`, never `$'...'` — in
      `$'...'` the paren always opens an expression and `\(` does not escape it
- [ ] Double quotes only when escape sequences needed: `"\n"`, `"\t"`
- [ ] Raw strings for regex: `r#'pattern'#`
- [ ] `str uppercase` / `str lowercase` used instead of deprecated `str upcase` / `str downcase`

### Pipeline & functional style

- [ ] Pipelines preferred over imperative loops
- [ ] `$items | get price | math sum` instead of `mut total; for ...`
- [ ] `ls | where size > 1mb` instead of manual filtering
- [ ] `enumerate` instead of manual index counters
- [ ] `reduce` instead of `mut` accumulator + `for`
- [ ] `match` preferred for multi-branch dispatch on a single value

### Formatting

- [ ] No space before `|params|` in closures: `{|x| ...}` not `{ |x| ...}`
- [ ] Spaces around pipe: `cmd | cmd` not `cmd|cmd`
- [ ] Commas omitted in lists: `[1 2 3]` not `[1, 2, 3]`
- [ ] One space after `:` in records: `{x: 1}` not `{x:1}`
- [ ] Multi-line format for expressions >80 chars

### Documentation

- [ ] Exported commands have `#` comment above `def`
- [ ] Parameter descriptions as inline `#` comments
- [ ] `@example` attributes for non-trivial commands
- [ ] `@category` for organization when applicable

### Modules

- [ ] Only necessary definitions are `export`-ed
- [ ] `export def main` used when command matches module name
- [ ] Submodules are re-exported explicitly when callers need the submodule namespace in Nu 0.114+
- [ ] Private helpers are not exported
- [ ] `export-env` for environment setup blocks

---

## 4. Performance

### Parallelism

- [ ] `par-each` used for I/O-bound work (file reads, HTTP requests)
- [ ] `par-each` used for CPU-bound work (data processing)
- [ ] `--threads` specified when controlling concurrency matters
- [ ] `each` used only when order matters or list is tiny

### Streaming & memory

- [ ] `each --flatten` for streaming nested results
- [ ] Large files not loaded entirely when streaming suffices
- [ ] `lines` + pipeline for line-by-line processing of large files
- [ ] `peek` used for stream metadata/sample inspection instead of collecting
- [ ] `first N` / `take while` to limit processing early

### Caching & computation

- [ ] Expensive results cached in `let` bindings, not recomputed
- [ ] `glob` with `--depth` to avoid scanning huge trees
- [ ] Built-in commands preferred over external for small data
- [ ] External tools (`^rg`, `^jq`) used for large-scale operations

---

## 5. Robustness

### Input validation

- [ ] Parameter types annotated (catches misuse at parse time)
- [ ] Range/value checks at function entry for critical params
- [ ] User-facing commands validate inputs before processing
- [ ] Consistent return types (don't mix null and value unexpectedly)

### File operations

- [ ] `path exists` checked before `open` when file may not exist
- [ ] `save --force` used intentionally (overwrites without warning)
- [ ] File encoding handled appropriately (`open --raw` for binary)
- [ ] `from xlsx` / `from ods` output handled as a record of sheet tables
- [ ] Spreadsheet imports use `--noheaders`, `--first-row`, or `--prefer-integers` instead of removed `--header-row`
- [ ] `mkdir -v` / `mv -v` / `rm -v` outputs treated as tables, not parsed text
- [ ] `rm` / `mkdir` partial-success behavior considered when multiple paths are passed

### Process management

- [ ] Long-running external processes have timeouts or cancellation
- [ ] Background jobs (`job spawn`) tracked and cleaned up
- [ ] Exit codes checked for critical external commands

---

## Review Workflow

1. **Skim the entire script** — Understand purpose, entry points, data flow
2. **Security pass** — Check Section 1 items systematically
3. **Correctness pass** — Verify types, error handling, null safety
4. **Style pass** — Naming, strings, formatting, documentation
5. **Performance pass** — Parallelism, streaming, caching opportunities
6. **Robustness pass** — Input validation, file safety, process management
7. **Summarize findings** — Group by severity, highlight security issues first