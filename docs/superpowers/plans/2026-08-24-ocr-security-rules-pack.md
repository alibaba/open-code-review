# OCR Security Rules Pack Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an opt-in, per-language security rules pack to `ocr` plus an `ocr-review-security-lite` command on the skill and plugin surfaces, so security findings appear as line-anchored comments in an ordinary PR review.

**Architecture:** A data-only pack under `examples/security-profile/` loaded through `ocr`'s existing `--rule` custom-rule layer, with `merge_system_rule: true` so security rules are added to the language rule rather than replacing it. A shell script concatenates a shared spine with per-language sink tables into the generated files `ocr` actually reads. No Go source changes.

**Tech Stack:** Markdown rule documents, Bash (build script), JSON (`rule.json`, plugin manifests), TypeScript (opencode plugin), Node's built-in test runner.

**Spec:** `docs/superpowers/specs/2026-08-24-ocr-security-rules-pack-design.md`

## Global Constraints

Every task's requirements implicitly include this section.

- **No Go source changes.** The pack uses mechanisms already in the binary. If a task appears to need a Go change, stop and escalate.
- **SPDX headers** are required on `.go .sh .js .mjs .ts .tsx`. Run `make license-add` after creating any such file. The exact header for `.sh` is:
  ```
  # SPDX-License-Identifier: Apache-2.0
  # Copyright 2026 alibaba/open-code-review Contributors
  ```
- **`testdata/`, `dist/`, `vendor/`, `node_modules/`** are in `IGNORED_PATHS` for both `scripts/verify-license.sh` and `scripts/add-license.sh`. Files under `testdata/` need no header.
- **English-only** is enforced by `make english-check` on non-Markdown source. Markdown is out of scope by extension, so rule content is unaffected.
- **LF line endings.** Run `git add --renormalize .` before committing if unsure.
- **Never name a directory `dist/`.** `.gitignore:6` is an unanchored `dist/`, which matches at any depth. The pack's generated output directory is `rules/`.
- **12 KB hard ceiling** per generated file in `examples/security-profile/rules/`. For reference, `internal/config/rules/rule_docs/go.md` is 10.6 KB and is the largest rule `ocr` injects today.
- **Do not modify `README.md`.** `AGENTS.md` requires syncing every README change into `README.zh-CN.md`, `README.ja-JP.md`, `README.ko-KR.md`, `README.ru-RU.md`. Pack documentation goes elsewhere.
- **Commit messages in English**, conventional-commit prefixes (`feat:`, `docs:`, `test:`, `chore:`), matching the existing log.
- **Default behaviour must not change.** `ocr review` with no `--rule` flag must produce byte-identical prompts to today.

---

### Task 1: Build script, shared spine, and the Go rules file

The vertical slice. Produces a working pack for one language so the mechanism is proven before content is scaled out. Go is first because it lets the pack be dogfooded on this repo's own PRs.

**Files:**
- Create: `examples/security-profile/src/_spine.md`
- Create: `examples/security-profile/src/sinks_go.md`
- Create: `scripts/build-security-rules.sh`
- Generated (committed): `examples/security-profile/rules/security_go.md`, `examples/security-profile/rule.json`

**Interfaces:**
- Consumes: nothing.
- Produces: `scripts/build-security-rules.sh` with two modes — no argument regenerates `examples/security-profile/rules/` and `rule.json` in place; `--check` regenerates into a temp directory and exits non-zero on any drift. Later tasks call `bash scripts/build-security-rules.sh --check`. The language table lives in the script as the `LANGUAGES` array of `name|glob` pairs.

- [ ] **Step 1: Write the shared spine**

Create `examples/security-profile/src/_spine.md`. This is the whole file (outer fence is four backticks because the content itself contains a fenced block):

````markdown
#### Security Review Scope

You are additionally acting as an application security reviewer for this file.
Apply the checks below **in addition to** the mandatory system rules above, never
instead of them. Do not restate a concern the system rules already cover.

Report a security finding only when **all** of the following hold:

1. The vulnerable construct is on a line changed by this diff, or is directly
   reachable from one.
2. Untrusted input can reach it. Name the entry point. If you cannot name one,
   do not report.
3. You can state a concrete attack input that exercises it.

If any of the three fails, stay silent. A speculative security comment is worse
than a missing one: it trains reviewers to skim past this whole category.
Defensive code that merely looks unusual is not a finding.

#### Finding Format

Set `category` to `security`. Open the comment body with the OWASP category and
the ASVS Level 1 requirement, then give three short parts:

```
**A03 Injection** (ASVS 5.3.4)

Attack: <the concrete input an attacker supplies>
Impact: <what they gain>
Fix: <the specific change, not general advice>
```

#### OWASP Top 10 Categories

- **A01 Broken Access Control** — missing or bypassable authorisation on an
  operation that needs it.
- **A02 Cryptographic Failures** — weak algorithm, missing encryption, or
  mishandled key or secret material.
- **A03 Injection** — untrusted input reaching an interpreter: SQL, OS command,
  template, expression, LDAP, or markup.
- **A04 Insecure Design** — a missing control the design requires, such as no
  rate limit on a credential endpoint.
- **A05 Security Misconfiguration** — debug surfaces, permissive defaults, or
  verbose errors reaching production.
- **A06 Vulnerable and Outdated Components** — a dependency pinned to a version
  with a known advisory.
- **A07 Identification and Authentication Failures** — weak session, token, or
  credential handling.
- **A08 Software and Data Integrity Failures** — unverified deserialisation,
  unpinned or unverified supply-chain input.
- **A09 Security Logging and Monitoring Failures** — a security-relevant event
  that is unlogged, or secrets written to logs.
- **A10 Server-Side Request Forgery** — a server-side fetch whose destination is
  attacker-influenced.

#### Severity

- **critical** — remote unauthenticated exploitation, or direct loss of
  credentials, keys, or bulk data.
- **high** — exploitable by an authenticated user across a trust boundary, such
  as horizontal or vertical privilege escalation.
- **medium** — needs an unusual precondition, or the impact is bounded.
- **low** — defence in depth; no exploit path demonstrated.

Do not inflate. A finding you could not write an exploit for is at most medium.
````

- [ ] **Step 2: Write the Go sink table**

Create `examples/security-profile/src/sinks_go.md`. Every row carries pattern, why, OWASP, ASVS, and fix — no row may omit a column:

```markdown
#### Go Security Sinks

`go.md`'s "Security-Sensitive Boundaries" section already covers `math/rand` for
key, token, and session material. Do not duplicate it.

| Sink | Why it is a finding | OWASP / ASVS | Fix |
|---|---|---|---|
| `fmt.Sprintf` / string concatenation building SQL passed to `db.Query`, `db.Exec`, `QueryRow` | Untrusted value is parsed as SQL | A03 / 5.3.4 | Placeholders (`$1`, `?`) with query arguments |
| `text/template` rendering into an HTTP response | `text/template` does not escape; `html/template` does | A03 / 5.3.3 | `html/template` for any HTML output |
| `exec.Command("sh", "-c", ...)` or any argument built from request input | Shell metacharacters become commands | A03 / 5.3.8 | `exec.Command(bin, args...)` with no shell, arguments passed separately |
| `filepath.Join` / `os.Open` on a request-derived path | `..` traversal escapes the intended root | A01 / 12.3.1 | `filepath.Clean`, then verify the result is still under the root prefix |
| `http.Get` / `http.Post` / `client.Do` with a request-derived URL | Server fetches an attacker-chosen destination | A10 / 5.2.6 | Allowlist scheme and host; reject link-local and private ranges |
| `&http.Client{}` with no `Timeout`, or a `Transport` with no timeouts | A slow peer holds the connection open indefinitely | A04 / 13.2.3 | Set `Timeout`, and `ResponseHeaderTimeout` on the transport |
| `encoding/gob`, `encoding/json` into `interface{}`, or `yaml.Unmarshal` on untrusted bytes | Type confusion or resource exhaustion | A08 / 5.5.1 | Decode into a concrete struct; bound the input size |
| Secrets, tokens, or full request bodies passed to `log.Printf` / `slog` | Credentials persist in log storage | A09 / 7.1.1 | Log an identifier, never the secret |
| `crypto/md5`, `crypto/sha1`, `crypto/des`, or `cipher.NewCBCEncrypter` without an authenticating MAC | Broken or unauthenticated cryptography | A02 / 6.2.5 | SHA-256 or better; AEAD such as `cipher.NewGCM` |
```

- [ ] **Step 3: Write the build script**

Create `scripts/build-security-rules.sh`:

```bash
#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "$REPO_ROOT"

PACK_DIR="examples/security-profile"
SRC_DIR="$PACK_DIR/src"
MAX_BYTES=12288

# name|glob — globs mirror internal/config/rules/system_rules.json so a file the
# system layer routes to python.md the pack routes to security_python.md.
LANGUAGES=(
  "go|**/*.go"
)

CHECK_MODE=false
if [[ "${1:-}" == "--check" ]]; then
  CHECK_MODE=true
elif [[ $# -gt 0 ]]; then
  echo "usage: $0 [--check]" >&2
  exit 2
fi

build_into() {
  local out_dir="$1"
  mkdir -p "$out_dir/rules"

  local entries=()
  for entry in "${LANGUAGES[@]}"; do
    local name="${entry%%|*}"
    local glob="${entry##*|}"
    local sink="$SRC_DIR/sinks_$name.md"

    if [[ ! -f "$sink" ]]; then
      echo "missing sink file: $sink" >&2
      exit 1
    fi

    local target="$out_dir/rules/security_$name.md"
    cat "$SRC_DIR/_spine.md" > "$target"
    printf '\n' >> "$target"
    cat "$sink" >> "$target"

    local size
    size=$(wc -c < "$target" | tr -d ' ')
    if (( size > MAX_BYTES )); then
      echo "FAIL: rules/security_$name.md is $size bytes, over the $MAX_BYTES limit" >&2
      exit 1
    fi

    entries+=("$(printf '    { "path": "%s", "rule": "rules/security_%s.md", "merge_system_rule": true }' "$glob" "$name")")
  done

  {
    echo '{'
    echo '  "rules": ['
    local i
    for i in "${!entries[@]}"; do
      if (( i + 1 < ${#entries[@]} )); then
        echo "${entries[$i]},"
      else
        echo "${entries[$i]}"
      fi
    done
    echo '  ]'
    echo '}'
  } > "$out_dir/rule.json"
}

if $CHECK_MODE; then
  TEMP_DIR="$(mktemp -d)"
  trap 'rm -rf "$TEMP_DIR"' EXIT
  build_into "$TEMP_DIR"
  if ! diff -r "$TEMP_DIR/rules" "$PACK_DIR/rules" >/dev/null 2>&1 \
     || ! diff -q "$TEMP_DIR/rule.json" "$PACK_DIR/rule.json" >/dev/null 2>&1; then
    echo "FAIL: generated security rules are out of date. Run: bash scripts/build-security-rules.sh" >&2
    diff -r "$TEMP_DIR/rules" "$PACK_DIR/rules" || true
    diff -u "$PACK_DIR/rule.json" "$TEMP_DIR/rule.json" || true
    exit 1
  fi
  echo "PASS: generated security rules are up to date"
else
  build_into "$PACK_DIR"
  echo "built $PACK_DIR/rules and $PACK_DIR/rule.json"
fi
```

- [ ] **Step 4: Verify `--check` fails before anything is generated**

The failing test. Run:

```bash
bash scripts/build-security-rules.sh --check
```

Expected: non-zero exit, `FAIL: generated security rules are out of date` (there is no `examples/security-profile/rules/` yet).

- [ ] **Step 5: Generate the pack**

```bash
bash scripts/build-security-rules.sh
```

Expected: `built examples/security-profile/rules and examples/security-profile/rule.json`.
Confirm the size gate is real:

```bash
wc -c examples/security-profile/rules/security_go.md
```

Expected: under 12288.

- [ ] **Step 6: Verify `--check` now passes**

```bash
bash scripts/build-security-rules.sh --check
```

Expected: `PASS: generated security rules are up to date`, exit 0.

- [ ] **Step 7: Verify the generated files are actually trackable**

The whole reason the output directory is `rules/` and not `dist/`:

```bash
git check-ignore -v examples/security-profile/rules/security_go.md; echo "exit=$?"
```

Expected: no output and `exit=1` (not ignored). If this prints a `.gitignore` rule, stop — the directory name is wrong.

- [ ] **Step 8: Verify `ocr` resolves the pack and merges the system rule**

```bash
go build -o /tmp/ocr ./cmd/opencodereview
/tmp/ocr rules check --rule examples/security-profile/rule.json internal/agent/agent.go
```

Expected output contains all four:
- `Source: Custom (--rule)`
- `Pattern: **/*.go`
- `## System-Specific Rules (Mandatory)` followed by the `go.md` text
- `## User-Specific Rules (Mandatory)` followed by the spine and Go sink text

- [ ] **Step 9: Verify default behaviour is unchanged**

```bash
/tmp/ocr rules check internal/agent/agent.go | head -3
```

Expected: `Source: System built-in`, `Pattern: **/*.go`, and no security content anywhere in the output.

- [ ] **Step 10: Add the license header and run the repo gates**

```bash
make license-add
make license-check
make english-check
```

Expected: all pass. `make license-add` inserts the hash-style SPDX header into `scripts/build-security-rules.sh`.

- [ ] **Step 11: Commit**

```bash
git add --renormalize .
git add scripts/build-security-rules.sh examples/security-profile/
git commit -m "feat(rules): add security rules pack with Go coverage"
```

---

### Task 2: Sink tables for kotlin, java, python, and ts

**Files:**
- Create: `examples/security-profile/src/sinks_kotlin.md`
- Create: `examples/security-profile/src/sinks_java.md`
- Create: `examples/security-profile/src/sinks_python.md`
- Create: `examples/security-profile/src/sinks_ts.md`
- Modify: `scripts/build-security-rules.sh` — the `LANGUAGES` array only
- Generated: four more files under `examples/security-profile/rules/`, and a regenerated `rule.json`

**Interfaces:**
- Consumes: `_spine.md` and the build script from Task 1. Sink files must use the identical five-column table format (`Sink | Why it is a finding | OWASP / ASVS | Fix`) — the spine's Finding Format section refers to it.
- Produces: `rules/security_{kotlin,java,python,ts}.md` and the four matching `rule.json` entries.

- [ ] **Step 1: Extend the language table**

In `scripts/build-security-rules.sh`, replace the `LANGUAGES` array with:

```bash
LANGUAGES=(
  "kotlin|**/*.kt"
  "java|**/*.java"
  "python|**/*.{py,ipynb}"
  "ts|**/*.{ts,js,tsx,jsx}"
  "go|**/*.go"
)
```

Order is significant — `matchProjectRuleEntry` returns the first match. These globs are copied from `internal/config/rules/system_rules.json`; do not invent variants.

- [ ] **Step 2: Verify the build fails on the missing sink files**

```bash
bash scripts/build-security-rules.sh
```

Expected: non-zero exit, `missing sink file: examples/security-profile/src/sinks_kotlin.md`.

- [ ] **Step 3: Write `sinks_kotlin.md`**

Same table format as `sinks_go.md`. Every row needs all five columns. Cover exactly these nine sink families, resolved from the `ADAPTATION MANIFEST` in `Security-Analysis-Agent/security-testing-runbook-backend-generic.md`:

1. `@PreAuthorize` / `@Secured` / `@RolesAllowed` absent on a `@GetMapping`/`@PostMapping`/`@PutMapping`/`@DeleteMapping` that mutates or exposes another user's data — A01 / 4.1.1.
2. `hasRole("ROLE_ADMIN")` — Spring prepends `ROLE_`, so this checks for `ROLE_ROLE_ADMIN` and always fails open where the check is the only gate — A01 / 4.1.3.
3. `${...}` inside a `@Query` annotation or a MyBatis XML mapper (`#{...}` is the safe form) — A03 / 5.3.4.
4. `BeanUtils.copyProperties` or `BeanUtils.populate` from a `@RequestBody` DTO onto an `@Entity` — mass assignment writes fields the caller should not control — A01 / 5.1.2.
5. `SignedJWT.parse` without a following `.verify(...)`, or an accepted `JWSAlgorithm.NONE` — A07 / 3.5.3.
6. `RestTemplate` / `WebClient` called with a URL taken from `@RequestParam` or `@RequestBody` — A10 / 5.2.6.
7. `MessageDigest.getInstance("MD5")` or `"SHA-1"`, and `"DES"`, `"RC4"`, or any `"...ECB..."` cipher transformation — A02 / 6.2.5.
8. `File(...)` or `Paths.get(...)` built from request input without a root-prefix check after normalisation — A01 / 12.3.1.
9. `management.endpoints.web.exposure.include=*` or an `h2-console` enabled in a non-test profile — A05 / 14.1.3.

Worked example of the required row shape:

```markdown
| `${...}` in `@Query` or a MyBatis XML mapper | `${}` is textual substitution; `#{}` is a bound parameter | A03 / 5.3.4 | Use `#{param}`, or `:param` with `@Param` |
```

- [ ] **Step 4: Write `sinks_java.md`**

The same nine families enumerated in Step 3 (items 1-9) — the Spring, JPA, MyBatis, and Nimbus surfaces are identical on the JVM. Differences to reflect in the wording: Java uses `String.format` and `+` concatenation where Kotlin uses templates, and Java has no null-safety operators, so mention `Objects.requireNonNull` gaps only where they gate a security check. Do not write "same as Kotlin" — write the table out.

- [ ] **Step 5: Write `sinks_python.md`**

Six sink families, resolved from the same backend runbook:

1. f-string or `%`-format or `.format()` building SQL passed to `cursor.execute` / `session.execute` — A03 / 5.3.4.
2. `jinja2.Environment(...)` without `autoescape=True`, or `Markup(...)` on request data — A03 / 5.3.3.
3. `pickle.loads`, `yaml.load` without `SafeLoader`, or `dill` on untrusted bytes — A08 / 5.5.1.
4. `subprocess.run` / `Popen` / `os.system` with `shell=True` or a command string built from input — A03 / 5.3.8.
5. `requests.get` / `httpx.get` / `urllib.request.urlopen` with a request-derived URL — A10 / 5.2.6.
6. `os.path.join` / `open()` / `send_file` on a request-derived path with no root-prefix check after `os.path.realpath` — A01 / 12.3.1.

- [ ] **Step 6: Write `sinks_ts.md`**

Seven sink families, resolved from `Security-Analysis-Agent/security-testing-runbook-frontend-generic.md`:

1. `bypassSecurityTrustHtml` / `bypassSecurityTrustUrl` (Angular), `dangerouslySetInnerHTML` (React), `v-html` (Vue) on any value that is not a literal — A03 / 5.3.3.
2. `innerHTML` / `outerHTML` assignment, including via `nativeElement` — A03 / 5.3.3.
3. `marked(...)` with `{ sanitize: true }` — the option has been a silent no-op since marked v4, so this reads as safe and is not — A03 / 5.3.3.
4. Access tokens or refresh tokens written to `localStorage` or `sessionStorage` — readable by any XSS on the origin — A07 / 3.5.3.
5. `router.navigate` / `location.href` fed from a `returnUrl`, `redirect`, or `next` query parameter without an allowlist — A01 / 5.1.5.
6. `StoreDevtoolsModule` / Redux devtools registered without an `isDevMode()` or `NODE_ENV`/`import.meta.env.PROD` guard — A05 / 14.1.3.
7. An ECharts, Chart.js, or D3 `formatter`/`.html()` callback returning interpolated markup — A03 / 5.3.3.

- [ ] **Step 7: Build and verify the size gate on every file**

```bash
bash scripts/build-security-rules.sh
wc -c examples/security-profile/rules/*.md
```

Expected: five files, each under 12288 bytes. If one is over, trim the sink table — do not raise `MAX_BYTES`.

- [ ] **Step 8: Verify routing for every language**

```bash
go build -o /tmp/ocr ./cmd/opencodereview
for f in a.kt a.java a.py a.ts a.go; do
  echo "--- $f"
  /tmp/ocr rules check --rule examples/security-profile/rule.json "$f" | head -3
done
```

Expected: each prints `Source: Custom (--rule)` and the `Pattern` matching its own language — `**/*.kt`, `**/*.java`, `**/*.{py,ipynb}`, `**/*.{ts,js,tsx,jsx}`, `**/*.go`. A `.ts` file resolving to `**/*.go` means the array order is wrong.

- [ ] **Step 9: Verify `--check` passes and commit**

```bash
bash scripts/build-security-rules.sh --check
git add --renormalize .
git add scripts/build-security-rules.sh examples/security-profile/
git commit -m "feat(rules): add kotlin, java, python and ts security sinks"
```

---

### Task 3: Validation fixtures and the routing harness

**Files:**
- Create: `examples/security-profile/testdata/go/vulnerable.go`
- Create: `examples/security-profile/testdata/go/clean.go`
- Create: `examples/security-profile/testdata/python/vulnerable.py`
- Create: `examples/security-profile/testdata/python/clean.py`
- Create: `examples/security-profile/testdata/ts/vulnerable.ts`
- Create: `examples/security-profile/testdata/ts/clean.ts`
- Create: `examples/security-profile/testdata/kotlin/Vulnerable.kt`
- Create: `examples/security-profile/testdata/java/Vulnerable.java`
- Modify: `scripts/build-security-rules.sh` — add routing assertions to `--check`

**Interfaces:**
- Consumes: `rule.json` from Task 2.
- Produces: `--check` additionally asserts, for one fixture path per language, that `ocr rules check` reports `Source: Custom (--rule)`, the expected pattern, and both merge headers. Later tasks rely on `--check` being the single gate command.

`testdata/` is deliberate: Go tooling ignores it entirely, so `go list ./...`, `go build ./...`, and `make test` never see these files, and both `scripts/verify-license.sh` and `scripts/add-license.sh` list `testdata/` in `IGNORED_PATHS`, so no SPDX headers are needed.

- [ ] **Step 1: Write the Go vulnerable fixture**

Create `examples/security-profile/testdata/go/vulnerable.go`. One function per sink family in `sinks_go.md`, each with a comment naming the OWASP category it should trigger:

```go
package testdata

import (
	"database/sql"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
)

// A03 Injection — request value concatenated into SQL.
func lookup(db *sql.DB, r *http.Request) (*sql.Rows, error) {
	name := r.URL.Query().Get("name")
	return db.Query("SELECT * FROM users WHERE name = '" + name + "'")
}

// A03 Injection — request value reaching a shell.
func archive(r *http.Request) error {
	target := r.URL.Query().Get("target")
	return exec.Command("sh", "-c", "tar czf out.tgz "+target).Run()
}

// A01 Broken Access Control — traversal, no root-prefix check.
func read(r *http.Request) ([]byte, error) {
	return os.ReadFile(filepath.Join("/srv/data", r.URL.Query().Get("f")))
}

// A10 SSRF — attacker chooses the destination.
func fetch(r *http.Request) (*http.Response, error) {
	return http.Get(r.URL.Query().Get("url"))
}
```

- [ ] **Step 2: Write the Go clean fixture**

Create `examples/security-profile/testdata/go/clean.go` — the same four operations done safely. This measures the false-positive rate, so it must be realistic code, not trivially different:

```go
package testdata

import (
	"database/sql"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var client = &http.Client{Timeout: 10 * time.Second}

func lookupSafe(db *sql.DB, r *http.Request) (*sql.Rows, error) {
	return db.Query("SELECT * FROM users WHERE name = $1", r.URL.Query().Get("name"))
}

func archiveSafe(r *http.Request) error {
	return exec.Command("tar", "czf", "out.tgz", r.URL.Query().Get("target")).Run()
}

func readSafe(r *http.Request) ([]byte, error) {
	root := "/srv/data"
	p := filepath.Clean(filepath.Join(root, r.URL.Query().Get("f")))
	if !strings.HasPrefix(p, root+string(os.PathSeparator)) {
		return nil, os.ErrPermission
	}
	return os.ReadFile(p)
}

func fetchAllowlisted(host string) (*http.Response, error) {
	if host != "api.internal.example" {
		return nil, os.ErrPermission
	}
	return client.Get("https://" + host + "/status")
}
```

- [ ] **Step 3: Write the remaining fixtures**

Same shape for python, ts, kotlin, and java: a `vulnerable` file with one function per sink family from that language's table, each commented with its OWASP category, and (for python and ts) a `clean` counterpart. Kotlin and Java get vulnerable fixtures only — the JVM clean cases are covered well enough by the Go and Python pairs, and two more Spring scaffolds is not worth the maintenance.

- [ ] **Step 4: Verify Go tooling ignores the fixtures**

The fixtures do not compile and must never be built:

```bash
go build ./... && go list ./... | grep -c security-profile
```

Expected: `go build` succeeds, and the grep prints `0`.

- [ ] **Step 5: Verify the license gate ignores the fixtures**

```bash
make license-check
```

Expected: pass, with no complaint about the fixture files despite `.go`, `.py`, and `.ts` extensions.

- [ ] **Step 6: Add routing assertions to `--check`**

In `scripts/build-security-rules.sh`, inside the `if $CHECK_MODE` branch after the drift diff passes, append:

```bash
  OCR_BIN="${OCR_BIN:-ocr}"
  if command -v "$OCR_BIN" >/dev/null 2>&1; then
    declare -a ROUTES=(
      "$PACK_DIR/testdata/go/vulnerable.go|**/*.go"
      "$PACK_DIR/testdata/kotlin/Vulnerable.kt|**/*.kt"
      "$PACK_DIR/testdata/java/Vulnerable.java|**/*.java"
      "$PACK_DIR/testdata/python/vulnerable.py|**/*.{py,ipynb}"
      "$PACK_DIR/testdata/ts/vulnerable.ts|**/*.{ts,js,tsx,jsx}"
    )
    for route in "${ROUTES[@]}"; do
      file="${route%%|*}"
      want="${route##*|}"
      out="$("$OCR_BIN" rules check --rule "$PACK_DIR/rule.json" "$file")"
      grep -q "Source: Custom (--rule)" <<<"$out" || { echo "FAIL: $file did not resolve to the custom layer" >&2; exit 1; }
      grep -qF "Pattern: $want" <<<"$out" || { echo "FAIL: $file resolved to the wrong pattern, wanted $want" >&2; exit 1; }
      grep -qF "## System-Specific Rules (Mandatory)" <<<"$out" || { echo "FAIL: $file lost the merged system rule" >&2; exit 1; }
      grep -qF "## User-Specific Rules (Mandatory)" <<<"$out" || { echo "FAIL: $file lost the security rule" >&2; exit 1; }
    done
    echo "PASS: rule routing verified for ${#ROUTES[@]} languages"
  else
    echo "SKIP: rule routing checks (ocr not on PATH; set OCR_BIN to override)"
  fi
```

- [ ] **Step 7: Verify the routing assertions actually fire**

Prove the check is not vacuous by breaking it on purpose:

```bash
go build -o /tmp/ocr ./cmd/opencodereview
cp scripts/build-security-rules.sh /tmp/build-security-rules.sh.bak
python3 - <<'EOF'
p = "scripts/build-security-rules.sh"
s = open(p).read()
open(p, "w").write(s.replace('"go|**/*.go"', '"go|**/*.NOPE"'))
EOF
bash scripts/build-security-rules.sh
OCR_BIN=/tmp/ocr bash scripts/build-security-rules.sh --check
```

Expected: `FAIL: ... resolved to the wrong pattern`. Then restore and rebuild:

```bash
cp /tmp/build-security-rules.sh.bak scripts/build-security-rules.sh
bash scripts/build-security-rules.sh
OCR_BIN=/tmp/ocr bash scripts/build-security-rules.sh --check
```

Expected: `PASS: generated security rules are up to date` and `PASS: rule routing verified for 5 languages`.

- [ ] **Step 8: Commit**

```bash
git add --renormalize .
git add scripts/build-security-rules.sh examples/security-profile/
git commit -m "test(rules): add security fixtures and rule routing checks"
```

---

### Task 4: The canonical security skill and its plugin mirror

**Files:**
- Create: `skills/open-code-review-security/SKILL.md`
- Create: `plugins/open-code-review/skills/open-code-review-security/SKILL.md`
- Modify: `scripts/build-security-rules.sh` — add the skill drift check to `--check`

**Interfaces:**
- Consumes: `examples/security-profile/rule.json`.
- Produces: a skill named `open-code-review-security`, referenced by name from Task 5's command file.

The two copies are deliberate duplicates. `plugins/open-code-review/skills/open-code-review/SKILL.md` carries this banner immediately after the H1, and the new mirror must carry the same one with its own path substituted:

```
This Codex plugin skill intentionally mirrors the canonical skill at
`skills/open-code-review-security/SKILL.md`. Keep both files synchronized when
updating OCR agent instructions; a symlink is avoided because plugin installs
may only materialize the plugin subtree.
```

- [ ] **Step 1: Write the canonical skill**

Create `skills/open-code-review-security/SKILL.md`. Frontmatter matching the sibling skills' shape:

```markdown
---
name: open-code-review-security
description: >
  Runs a diff-scoped security review using the `ocr` CLI with the security
  rules pack. Use when the user asks for a security review of a pull request,
  branch, or working copy, or asks to check changes for OWASP Top 10 issues.
  Produces line-anchored findings tagged with an OWASP category and an ASVS
  Level 1 requirement. This is the pre-merge tier: it reviews only what
  changed, not the whole repository.
license: Apache-2.0
compatibility: >
  Requires the `ocr` CLI and a configured LLM endpoint. Requires the security
  rules pack from alibaba/open-code-review at
  `examples/security-profile/rule.json`.
metadata:
  author: alibaba
  homepage: https://github.com/alibaba/open-code-review
  version: "1.0.0"
---
```

The body must cover, in this order:

1. **Locate the pack.** Search in order: `$OCR_SECURITY_PACK`, then `examples/security-profile/rule.json` relative to the repo root, then `~/.opencodereview/security-profile/rule.json`. If none exist, say so and stop — do not fall back to a plain review and call it a security review.
2. **Run it:**
   ```bash
   ocr review --audience agent --rule "$PACK" --background "<context>" [target flags]
   ```
   Target flags pass through unchanged: none for workspace, `--commit <hash>`, or `--from <ref> --to <ref>`.
3. **Delegation alternative** for hosts with no `ocr` LLM configured — `ocr delegate preview` for the changed-file set, then `ocr delegate rule --rule "$PACK" <paths>` for the merged security checklist, then review with the host's own model. No `ocr` LLM cost.
4. **Report** grouped by OWASP category, then by severity descending, each finding with `path:startLine-endLine`.
5. **Scope statement, required in the output.** State plainly that this reviewed only the changed files, name the count, and that it is not a substitute for a full assessment. Without this, a clean result reads as "the repository is secure", which it does not mean.

- [ ] **Step 2: Create the mirror**

```bash
mkdir -p plugins/open-code-review/skills/open-code-review-security
cp skills/open-code-review-security/SKILL.md \
   plugins/open-code-review/skills/open-code-review-security/SKILL.md
```

Then insert the four-line banner from this task's preamble into the copy, immediately after the H1 heading and before the first body paragraph — matching exactly how `plugins/open-code-review/skills/open-code-review/SKILL.md` is laid out.

- [ ] **Step 3: Add the drift check**

In `scripts/build-security-rules.sh`, inside the `if $CHECK_MODE` branch, append:

```bash
  for skill in open-code-review open-code-review-delegate open-code-review-security; do
    canonical="skills/$skill/SKILL.md"
    mirror="plugins/open-code-review/skills/$skill/SKILL.md"
    [[ -f "$canonical" && -f "$mirror" ]] || { echo "FAIL: missing skill file for $skill" >&2; exit 1; }
    # The mirror adds a banner; every other line must match the canonical file.
    if ! diff <(grep -vF "intentionally mirrors the canonical skill" "$mirror" \
                 | grep -vF "Keep both files synchronized" \
                 | grep -vF "a symlink is avoided because plugin installs" \
                 | grep -vF "materialize the plugin subtree." \
                 | grep -vF "may only materialize the plugin subtree." \
                 | grep -vF "updating OCR delegation instructions; a symlink is avoided because plugin" \
                 | sed '/^$/N;/^\n$/D') \
                <(sed '/^$/N;/^\n$/D' "$canonical") >/dev/null; then
      echo "FAIL: $mirror has drifted from $canonical" >&2
      exit 1
    fi
  done
  echo "PASS: skill mirrors match their canonical files"
```

If this proves brittle against the existing two skills' banner wording, simplify it: compare only the frontmatter block and the set of `##` headings between each pair, and say so in a comment. A check that is skipped because it false-alarms is worse than a narrower one that holds.

- [ ] **Step 4: Verify the drift check fails on real drift**

```bash
echo "drifted line" >> plugins/open-code-review/skills/open-code-review-security/SKILL.md
bash scripts/build-security-rules.sh --check
```

Expected: `FAIL: ... has drifted from ...`. Then revert:

```bash
git checkout plugins/open-code-review/skills/open-code-review-security/SKILL.md 2>/dev/null \
  || sed -i.bak '$d' plugins/open-code-review/skills/open-code-review-security/SKILL.md && rm -f plugins/open-code-review/skills/open-code-review-security/SKILL.md.bak
bash scripts/build-security-rules.sh --check
```

Expected: `PASS: skill mirrors match their canonical files`.

- [ ] **Step 5: Commit**

```bash
git add --renormalize .
git add skills/ plugins/open-code-review/skills/ scripts/build-security-rules.sh
git commit -m "feat(skills): add open-code-review-security skill and mirror check"
```

---

### Task 5: The Claude Code plugin command

**Files:**
- Create: `plugins/open-code-review/claude-code/commands/review-security-lite.md`
- Modify: `plugins/open-code-review/claude-code/.claude-plugin/plugin.json:5`
- Modify: `.claude-plugin/marketplace.json:13`

**Interfaces:**
- Consumes: the `open-code-review-security` skill from Task 4 and `examples/security-profile/rule.json`.
- Produces: the slash command `/open-code-review:review-security-lite`.

Naming: Claude Code namespaces plugin commands by plugin, and the existing files are `review.md` and `delegate-review.md` with no `ocr-` prefix, so the file is `review-security-lite.md`. The opencode surface in Task 6 uses the flat literal `ocr-review-security-lite`, matching its own existing `ocr-review`. If the literal name is wanted here too, rename this one file; nothing else changes.

- [ ] **Step 1: Write the command**

Create `plugins/open-code-review/claude-code/commands/review-security-lite.md`, modelled on the existing `review.md`:

```markdown
---
description: Run a diff-scoped OWASP security review with OpenCodeReview and the security rules pack.
---

Run a security-focused code review over the current changes using OpenCodeReview
(OCR) with the security rules pack. This is the pre-merge tier: it reviews only
what changed, not the whole repository.

## Workflow

### Step 1: Locate the Security Rules Pack

Resolve the pack path in this order and use the first that exists:

1. `$OCR_SECURITY_PACK`
2. `examples/security-profile/rule.json`, relative to the repository root
3. `~/.opencodereview/security-profile/rule.json`

If none exists, tell the user the pack is missing and stop. Do not fall back to
a plain `ocr review` and report it as a security review.

### Step 2: Run the Review

```bash
ocr review --audience agent --rule "$PACK" [user-args]
```

- Default (no user arguments): reviews staged, unstaged, and untracked changes.
- If the user provides `--commit` or `-c`: pass through as-is.
- If the user provides `--from` and `--to`: pass through as-is.
- (Optional) `--background "requirement context"` to describe the change.
- Capture full stdout. Set a 5-minute timeout.
- If `ocr` is not found, install it: `npm i -g @alibaba-group/open-code-review`.

### Step 3: Filter

Each finding carries an OWASP category and an ASVS requirement. Keep a finding
only when the diff shows an untrusted input actually reaching the flagged
construct. Discard anything where you cannot name the entry point — the rules
pack is pattern-driven and OCR's review filter is deliberately permissive, so
this is where precision comes from.

### Step 4: Report

Group by OWASP category, then by severity descending. Give each finding as
`path:startLine-endLine` with the attack, impact, and fix.

End with the scope statement: how many files were reviewed, that only changed
files were covered, and that this is not a substitute for a full security
assessment.
```

- [ ] **Step 2: Bump the plugin version**

In `plugins/open-code-review/claude-code/.claude-plugin/plugin.json`, change `"version": "1.0.0"` to `"1.1.0"`. In `.claude-plugin/marketplace.json`, change the `open-code-review` entry's `"version": "1.0.0"` to `"1.1.0"`.

- [ ] **Step 3: Verify both manifests still parse**

```bash
python3 -m json.tool plugins/open-code-review/claude-code/.claude-plugin/plugin.json > /dev/null
python3 -m json.tool .claude-plugin/marketplace.json > /dev/null
echo "both parse"
```

Expected: `both parse`.

- [ ] **Step 4: Verify the command file has valid frontmatter**

```bash
head -3 plugins/open-code-review/claude-code/commands/review-security-lite.md
```

Expected: `---`, a `description:` line, `---` — matching `review.md` and `delegate-review.md`.

- [ ] **Step 5: Commit**

```bash
git add --renormalize .
git add plugins/open-code-review/claude-code/ .claude-plugin/marketplace.json
git commit -m "feat(plugin): add review-security-lite command for Claude Code"
```

---

### Task 6: The opencode plugin

**Files:**
- Modify: `plugins/open-code-review/opencode/open-code-review.ts` — `ReviewInput` (line 7-20), `buildReviewArgs` (line 65-102), `reviewArgs` (line 279-301), `config` hook (line 316-334)
- Modify: `plugins/open-code-review/opencode/test/open-code-review.test.mjs`

**Interfaces:**
- Consumes: nothing from earlier tasks at runtime; the command template references the pack path by convention.
- Produces: an optional `rule` argument on the `ocr_review` tool, and the `ocr-review-security-lite` command.

This is the only task touching a file that `make english-check` scans and that must compile. It already carries an SPDX header.

- [ ] **Step 1: Write the failing tests**

Append to `plugins/open-code-review/opencode/test/open-code-review.test.mjs`:

```javascript
test("ocr_review forwards the rule path to OCR", async () => {
  await withFakeOcr(
    "console.log(JSON.stringify({status:'success', argv:process.argv.slice(2)}))",
    async (worktree) => {
      const { hooks } = await loadPlugin(worktree)
      const output = await hooks.tool.ocr_review.execute(
        { rule: "examples/security-profile/rule.json" },
        toolContext(worktree),
      )
      assert.deepEqual(JSON.parse(output).argv, [
        "review",
        "--audience",
        "agent",
        "--format",
        "json",
        "--repo",
        worktree,
        "--rule",
        "examples/security-profile/rule.json",
      ])
    },
  )
})

test("plugin registers the security-lite command and preserves a user override", async () => {
  const { hooks } = await loadPlugin("/tmp/project")
  const config = {
    command: {
      "ocr-review-security-lite": {
        template: "Keep my custom security command.",
      },
    },
  }
  await hooks.config(config)
  assert.equal(
    config.command["ocr-review-security-lite"].template,
    "Keep my custom security command.",
  )

  const fresh = {}
  await hooks.config(fresh)
  assert.match(fresh.command["ocr-review-security-lite"].template, /rule/)
  assert.match(fresh.command["ocr-review-security-lite"].template, /security-profile/)
})
```

- [ ] **Step 2: Run the tests and confirm they fail**

```bash
cd plugins/open-code-review/opencode && npm install && npm test
```

Expected: FAIL. The first test's argv lacks `--rule`; the second throws on `fresh.command["ocr-review-security-lite"]` being undefined.

- [ ] **Step 3: Add the `rule` field to `ReviewInput`**

In `open-code-review.ts`, add to the `ReviewInput` interface after `exclude?: string`:

```typescript
  rule?: string
```

- [ ] **Step 4: Forward the flag in `buildReviewArgs`**

Immediately after the `pushValue(args, "--max-git-procs", input.maxGitProcesses)` line:

```typescript
  pushValue(args, "--rule", input.rule)
```

Appending at the end of the `pushValue` chain keeps every existing `assert.deepEqual` argv assertion valid, since `--rule` appears only when supplied.

- [ ] **Step 5: Declare the argument in the tool schema**

In the `reviewArgs` object, after the `exclude` entry:

```typescript
  rule: optionalString(
    "Path to a custom rule JSON file, such as the security rules pack at examples/security-profile/rule.json.",
  ),
```

- [ ] **Step 6: Register the command**

In the `config` hook, after the `config.command["ocr-review"] ??= {...}` block:

```typescript
      config.command["ocr-review-security-lite"] ??= {
        description: "Diff-scoped OWASP security review with OpenCodeReview",
        template:
          "Use the ocr_review tool with the rule argument set to the security rules pack " +
          "(examples/security-profile/rule.json, or $OCR_SECURITY_PACK if set) to run a " +
          "security review of the requested target. " +
          "Treat the following text as review intent, target details, and business context: $ARGUMENTS. " +
          "If no target is specified, review the current workspace changes. " +
          "Keep only findings where an untrusted input demonstrably reaches the flagged construct. " +
          "Group findings by OWASP category, then by severity, with exact file and line references. " +
          "End by stating that only changed files were reviewed.",
      }
```

The `??=` matters: it preserves a user's own command of the same name, which is what the second test asserts.

- [ ] **Step 7: Run the tests and confirm they pass**

```bash
cd plugins/open-code-review/opencode && npm run check
```

Expected: typecheck clean, all tests pass including the two new ones and every pre-existing one.

- [ ] **Step 8: Run the repo gates**

```bash
make english-check
make license-check
```

Expected: both pass. The `.ts` file is scanned by both.

- [ ] **Step 9: Commit**

```bash
git add --renormalize .
git add plugins/open-code-review/opencode/
git commit -m "feat(opencode): add rule argument and ocr-review-security-lite command"
```

---

### Task 7: Documentation and remaining manifests

**Files:**
- Create: `examples/security-profile/README.md`
- Modify: `plugins/open-code-review/README.md:29-33`
- Modify: `plugins/open-code-review/.cursor-plugin/plugin.json`, `plugins/open-code-review/.codex-plugin/plugin.json`
- Modify: `pages/src/content/docs/en/review-rules.md`
- Modify: `DOC_INDEX.md`

**Interfaces:**
- Consumes: everything from Tasks 1-6.
- Produces: no code interface.

`README.md` at the repo root is deliberately untouched — changing it obliges four localised README syncs under `AGENTS.md`.

- [ ] **Step 1: Write the pack README**

Create `examples/security-profile/README.md` covering, in this order:

1. **What it is** — an opt-in per-language security rules overlay, loaded via `--rule`, merged on top of the built-in language rules rather than replacing them.
2. **Enabling it:**
   ```bash
   ocr review --rule examples/security-profile/rule.json --from main --to my-branch
   ```
3. **What it covers** — the five languages, and the OWASP categories reachable per language.
4. **What it does not cover** — IaC and dependency manifests, so A05, A06, and A08 are largely out of reach; no ASVS compliance percentage; no whole-repo state.
5. **The resume caveat, stated plainly.** Rule text feeds `rule_config_sha256` in the run manifest, so turning the pack on or off invalidates existing resume checkpoints. Expected behaviour, not a bug.
6. **Editing it** — change `src/`, never `rules/`; run `bash scripts/build-security-rules.sh`; `--check` is the gate.
7. **Relationship to the full assessment** — this is the pre-merge tier; the whole-repo assessment remains the periodic deep tier.

- [ ] **Step 2: Document the command in the plugin README**

In `plugins/open-code-review/README.md`, extend the Claude Code section sentence that currently reads "This installs the `/open-code-review:review` and `/open-code-review:delegate-review` slash commands." to also name `/open-code-review:review-security-lite`, with one sentence on what it does.

- [ ] **Step 3: Update the remaining manifests**

In `plugins/open-code-review/.cursor-plugin/plugin.json` and `plugins/open-code-review/.codex-plugin/plugin.json`: bump `"version"` to `"1.1.0"` and add `"security"` to the existing `keywords` array in each.

- [ ] **Step 4: Verify every manifest still parses**

```bash
for f in plugins/open-code-review/.cursor-plugin/plugin.json \
         plugins/open-code-review/.codex-plugin/plugin.json \
         plugins/open-code-review/claude-code/.claude-plugin/plugin.json \
         .claude-plugin/marketplace.json \
         examples/security-profile/rule.json; do
  python3 -m json.tool "$f" > /dev/null && echo "ok $f"
done
```

Expected: five `ok` lines.

- [ ] **Step 5: Document the pack in the user docs**

In `pages/src/content/docs/en/review-rules.md`, add a section on the security rules pack: how the custom `--rule` layer and `merge_system_rule` combine, the enabling command, and a link to `examples/security-profile/README.md`. Match the file's existing heading depth and tone.

- [ ] **Step 6: Route it from the doc index**

Add a `DOC_INDEX.md` entry pointing at `examples/security-profile/README.md`, in the style of the existing rows.

- [ ] **Step 7: Run the full gate set**

```bash
make check
bash scripts/build-security-rules.sh --check
```

Expected: `check passed`, plus all four `PASS:` lines from the build script.

- [ ] **Step 8: Commit**

```bash
git add --renormalize .
git add examples/security-profile/README.md plugins/ pages/ DOC_INDEX.md
git commit -m "docs(rules): document the security rules pack and security-lite command"
```

---

### Task 8: A/B validation on a real diff

The measurement the design depends on. Everything before this proves the pack loads; this is the only step that shows whether it is worth loading. Producing the numbers is the deliverable, whatever they say.

**Files:**
- Create: `docs/superpowers/plans/2026-08-24-ocr-security-rules-pack-ab-results.md`

**Interfaces:**
- Consumes: the complete pack and a configured `ocr` LLM endpoint.
- Produces: a recorded recommendation — ship, revise, or drop.

- [ ] **Step 1: Pick the target**

Choose a real merged PR from this repo touching at least five `.go` files, ideally one with security-relevant surface (I/O, subprocess, or HTTP). Record its refs.

- [ ] **Step 2: Baseline run**

```bash
ocr review --audience agent --format json --from <base> --to <head> \
  > /tmp/ab-baseline.json
```

- [ ] **Step 3: Pack run**

```bash
ocr review --audience agent --format json --from <base> --to <head> \
  --rule examples/security-profile/rule.json > /tmp/ab-pack.json
```

- [ ] **Step 4: Compare**

```bash
for f in /tmp/ab-baseline.json /tmp/ab-pack.json; do
  echo "--- $f"
  python3 -c "
import json,sys,collections
d=json.load(open('$f'))
c=d if isinstance(d,list) else d.get('comments',[])
print('total', len(c))
print(collections.Counter(x.get('category','none') for x in c))
print(collections.Counter(x.get('severity','none') for x in c))
"
done
```

- [ ] **Step 5: Read every added and dropped finding by hand**

The counts alone decide nothing. For each finding present in the pack run but not the baseline: is it real? For each present in the baseline but not the pack run: was a correctness finding displaced? Displacement is the failure mode that matters and it will not show up in a count.

- [ ] **Step 6: Run the fixtures**

```bash
ocr review --audience agent --format json --rule examples/security-profile/rule.json \
  --repo . --preview
```

Then review `examples/security-profile/testdata/go/vulnerable.go` and `clean.go` as a diff. Expected: findings on each commented sink in `vulnerable.go`; ideally none on `clean.go`. Record the actual counts, including false positives.

- [ ] **Step 7: Write up the results**

Create `docs/superpowers/plans/2026-08-24-ocr-security-rules-pack-ab-results.md` with: the target PR, both runs' totals by category and severity, the hand-read verdict on added and dropped findings, the fixture true/false positive counts, and a one-line recommendation. If security signal was added without displacing correctness findings, ship. If correctness findings were displaced, the spine's scope discipline needs tightening before this goes further — say so plainly rather than shipping on the count.

- [ ] **Step 8: Commit**

```bash
git add docs/superpowers/plans/2026-08-24-ocr-security-rules-pack-ab-results.md
git commit -m "docs(rules): record security rules pack A/B validation results"
```

---

## Deferred, recorded in the spec

Not in this plan, do not implement:

- SARIF `ruleId` granularity (`security/A03` instead of the current flat `security`) — requires a Go change to `cmd/opencodereview/sarif.go:235`.
- IaC and manifest coverage (`terraform`, `github_workflows`, `package.json`, `pom.xml`) for OWASP A05, A06, and A08.
- Promoting the pack from a `--rule` file to a first-class `--security` resolver overlay.
- A security variant of `plugins/open-code-review/qca/system-prompt.md`.

## Blocking question

**Provenance, unresolved.** The source runbooks live in `internal-cloude-skills`, a private repo. `origin` here is `securityphoenix/open-code-review`, a public fork. Whether distilled derivatives of internal material may be published is not a call this plan can make. Confirm before pushing any branch. If the answer is no, the identical plan works with `examples/security-profile/` relocated to a private repository and referenced by absolute path — only Task 7's documentation paths change.
