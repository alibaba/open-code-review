# OCR Security Rules Pack and `ocr-review-security-lite`

Status: approved design, not yet implemented
Date: 2026-08-24
Repo: `securityphoenix/open-code-review` (fork of `alibaba/open-code-review`)

## 1. Summary

Bring two existing Phoenix security skills into `ocr`'s pull-request review path as a
**rules pack**: a set of distilled, per-language security review rules loaded through
`ocr`'s existing `--rule` custom-rule layer, plus a new **`ocr-review-security-lite`**
command exposed on the skill and plugin surfaces.

No Go changes. The pack is data plus one shell script, consumed by mechanisms that
already exist in the binary.

Source material:

- `internal-cloude-skills/Security Assessment/security-assessment/SKILL.md` — an
  MCP-driven whole-repo OWASP Top 10 / ASVS L1 assessment.
- `internal-cloude-skills/Security Assessment/Security-Analysis-Agent/*.md` — four
  technology-agnostic backend/frontend security agent and runbook templates
  (2,337 lines) parameterised with `{{PLACEHOLDER}}` tokens.

## 2. Motivation

`ocr review` is diff-scoped, per-file, line-anchored, and costs minutes and cents.
Both source skills are whole-repo, report-shaped, and cost tens of minutes and dollars
(`security-assessment` self-reports $8-10 and 15-20 minutes for a full run). Neither
drops into a PR review as-is.

What is portable differs sharply between the two:

- **`Security-Analysis-Agent` — nearly all of it.** Its grep patterns, attack
  scenarios, and fix patterns are structurally the same artifact as
  `internal/config/rules/rule_docs/*.md`. `go.md` already carries a
  "Security-Sensitive Boundaries" section. This is the high-value half.
- **`security-assessment` — the taxonomy, not the engine.** The `security_assessment`
  MCP tool and its `PHX_LLM_*`-configured Spring service cannot ride inside the `ocr`
  binary. What survives is the OWASP A01-A10 and ASVS L1 mapping, the finding schema
  (attack scenario -> PoC -> remediation), and the severity discipline. The skill's own
  "On-disk fallback" section already concedes this path.

## 3. Goals

- Security findings on changed lines, anchored to a line range, in the same review run.
- Findings tagged with an OWASP category and an ASVS L1 requirement number.
- Works anywhere `ocr` works: CLI, any provider, CI, delegation mode. No MCP, no new
  runtime dependency, no network service.
- Opt-in. Default `ocr review` behaviour is byte-for-byte unchanged.
- A `ocr-review-security-lite` entry point on the skill and plugin surfaces.

## 4. Non-goals

- ASVS compliance percentages, the risk-matrix report, or generated test suites from
  `security-assessment`. Those need whole-repo state that a diff review does not have.
- Replacing the `security-assessment` skill. The pack is the pre-merge tier; the full
  assessment remains the periodic deep tier.
- Any change to Go source, the embedded system rules, or the default review path.
- Upstreaming to `alibaba/open-code-review`. The pack is additive and could be
  proposed later, but that is not this change.

## 5. Decisions and rationale

| # | Decision | Rationale |
|---|---|---|
| D1 | Native `ocr` rules pack, not an agentic skill or a CI-only step | Works in CI, in any host, with any provider; stays line-anchored; no MCP dependency |
| D2 | Zero Go changes; enabled via the existing `--rule` flag | `--rule` is already registered on `review`, `scan`, and `delegate` (`cmd/opencodereview/shared_flags.go:170,188,216`); no code risk, no coverage-gate exposure |
| D3 | Per-language rule files generated from a shared spine | Matches `ocr`'s own model 1:1; each injected file stays small and concrete; only the matching language is paid for |
| D4 | First pack covers kotlin, java, python, ts/js, go | Kotlin/Spring + Angular is the stack the source skills were written against; Go lets the pack be dogfooded on this repo's own PRs |
| D5 | Output directory named `rules/`, not `dist/` | `.gitignore:6` is an unanchored `dist/`, which matches at any depth and would silently untrack the generated pack |
| D6 | Fixtures live under `testdata/` | Go tooling ignores `testdata/` entirely, and `scripts/verify-license.sh` lists it in `IGNORED_PATHS`, so deliberately-vulnerable fixtures need no SPDX header and never enter `go list ./...` |
| D7 | Pack lives in `examples/security-profile/` | `examples/` already holds the CI integrations, keeps the repo root clean, and is the least invasive tree to carry against `upstream/alibaba`. Weakest decision in this design: it is a functional pack, not an example. Alternative was a new top-level `rules/`. |
| D8 | README.md is not modified | `AGENTS.md` requires syncing every README change into four localised READMEs. Pack documentation goes in `examples/security-profile/README.md` and `pages/src/content/docs/en/` instead. |

## 6. How the mechanism works

`composedResolver.Resolve` (`internal/config/rules/system_rules.go:395`) checks the
custom (`--rule`) layer first. When the matched entry sets `merge_system_rule: true`,
`mergeWithSystemRule` (line 437) emits:

```
## System-Specific Rules (Mandatory)

<the matched system rule, e.g. rule_docs/go.md>

---

## User-Specific Rules (Mandatory)

<the pack's security_go.md>
```

So the language rule is preserved and the security rule is added, not substituted.

`resolveRuleEntries` resolves a rule value that looks like a file path (single line, no
spaces, `.md`/`.txt`/`.markdown`) relative to the directory holding `rule.json`, with
absolute paths permitted on the `--rule` layer. `"rule": "rules/security_go.md"`
therefore resolves correctly from any working directory.

**A constraint this imposes:** `matchProjectRuleEntry` returns the *first* matching
entry, and merge only ever adds the *system* rule. A path resolves to exactly one user
file plus the system rule. A shared spine cannot be a third layer — it must be
pre-concatenated into each per-language file. Hence the build script.

**Why the embedded system layer was not used instead:** `SystemRule.Resolve`
(line 132) is strictly first-match-wins with no merge. Adding
`"**/*.go": "security_go.md"` to `system_rules.json` would shadow `go.md` entirely
rather than augment it.

## 7. Architecture

### 7.1 File layout

```
examples/security-profile/
  README.md                    enabling, coverage, limits
  rule.json                    GENERATED, committed
  src/
    _spine.md                  shared: taxonomy, severity, discipline, output contract
    sinks_kotlin.md
    sinks_java.md
    sinks_python.md
    sinks_ts.md
    sinks_go.md
  rules/                       GENERATED, committed - consumed by ocr
    security_kotlin.md
    security_java.md
    security_python.md
    security_ts.md
    security_go.md
  testdata/                    validation fixtures, one dir per language
    kotlin/ java/ python/ ts/ go/
scripts/build-security-rules.sh
```

`rules/` is committed so the pack works from a clean checkout with no build step. The
spine is duplicated into all five outputs; only one is ever injected per file, so the
duplication costs disk, not tokens.

### 7.2 Generated `rule.json`

```json
{
  "rules": [
    { "path": "**/*.kt",              "rule": "rules/security_kotlin.md", "merge_system_rule": true },
    { "path": "**/*.java",            "rule": "rules/security_java.md",   "merge_system_rule": true },
    { "path": "**/*.{py,ipynb}",      "rule": "rules/security_python.md", "merge_system_rule": true },
    { "path": "**/*.{ts,js,tsx,jsx}", "rule": "rules/security_ts.md",     "merge_system_rule": true },
    { "path": "**/*.go",              "rule": "rules/security_go.md",     "merge_system_rule": true }
  ]
}
```

Globs mirror `internal/config/rules/system_rules.json` so a file routed to `python.md`
by the system layer is routed to `security_python.md` by the pack. Order matters
(first match wins) and is fixed by the generator.

No `include`/`exclude` keys. Adding either would make the pack hijack file filtering
for the whole run via `buildFileFilter`, which takes the highest-priority layer that
sets anything — a surprising side effect for a rules pack.

### 7.3 `_spine.md` content contract

From `security-assessment`, the taxonomy half. Five things, deliberately not more:

1. **OWASP A01-A10** with one-line definitions, and the instruction to prefix each
   finding with its category and the ASVS L1 requirement number.
2. **Severity calibration** mapped onto `model.LlmComment.Severity`
   (`critical|high|medium|low`, `internal/model/review.go:20`), and the instruction to
   set `Category: "security"` (line 18).
3. **Finding shape** from the source skill's schema: attack scenario -> concrete PoC
   input -> remediation.
4. **Diff-scope discipline.** Load-bearing. Both sources are whole-repo tools; `ocr` is
   diff-scoped. The spine states: report only on changed lines and their reachable
   impact. `go.md` opens with the same constraint, so this matches house style.
5. **False-positive discipline.** Also load-bearing.
   `internal/config/template/prompts/review_filter_task_system.md` is deliberately
   permissive — it drops a comment only when the diff *proves* it wrong, and explicitly
   approves on "suspicious", "I cannot verify this", and "the flagged code looks fine
   to me". The filter will therefore not catch security false positives. Precision must
   be enforced at generation time, in the spine, or pack noise ships straight to the PR.

The spine header records the source commit SHA of the `internal-cloude-skills`
runbooks it was distilled from, so drift is detectable.

### 7.4 `sinks_*.md` content contract

From `Security-Analysis-Agent`, the knowledge half. Each file resolves that runbook's
`ADAPTATION MANIFEST` for one language: what was `{{PARAM_UNSAFE_PATTERN}}` becomes,
for Kotlin, `${...}` in `@Query` and MyBatis XML mappers.

- **kotlin, java** — `@PreAuthorize`/`@Secured` gaps and the `ROLE_` double-prefix
  trap; `${}` in `@Query` and MyBatis mappers; `BeanUtils.copyProperties` onto
  `@Entity` (mass assignment); Nimbus JWT `alg:none` and unverified parse;
  `RestTemplate`/`WebClient` on request-derived URLs (SSRF); MD5/SHA-1/DES/ECB;
  `Paths.get()`/`File()` traversal; wildcard actuator exposure.
- **python** — f-string and `%`-format SQL; `jinja2` autoescape disabled; `pickle` and
  `yaml.load` without `SafeLoader`; `subprocess(shell=True)`; `requests` on user URLs;
  `os.path.join` traversal.
- **ts** — `bypassSecurityTrustHtml` / `dangerouslySetInnerHTML` / `v-html`;
  `innerHTML` writes; `marked({sanitize:true})`, a no-op since v4; auth tokens in
  `localStorage`; `returnUrl` open redirect; NgRx devtools unguarded in production;
  ECharts `formatter` innerHTML sinks.
- **go** — `fmt.Sprintf` into SQL; `text/template` where `html/template` belongs;
  `exec.Command` with request input; `filepath.Join` traversal; `http.Get` on user
  URLs; missing client timeouts.

The Go sinks overlap `go.md`'s existing "Security-Sensitive Boundaries" section
(including its `math/rand` note at line 63). The spine instructs the model not to
restate what the merged system rule already covers.

**Size ceiling: 12 KB per generated file**, a hard failure in the build script.
`go.md` at 10.6 KB is the largest rule `ocr` injects today; staying in that band keeps
the cost profile familiar.

### 7.5 `scripts/build-security-rules.sh`

Deterministic, no dependencies beyond coreutils. Responsibilities:

1. Concatenate `src/_spine.md` + `src/sinks_<lang>.md` -> `rules/security_<lang>.md`.
2. Regenerate `rule.json` from a language->glob table held in the script.
3. Fail if any generated file exceeds 12 KB.
4. `--check` mode: regenerate into a temp dir and diff against the committed
   `rules/` and `rule.json`, failing on drift. This is what CI runs.

Needs an SPDX header (`AGENTS.md` requires one on `.sh`); `make license-add` supplies
it. Markdown is out of scope for `make english-check` by extension, so the rule content
is unaffected by that gate.

## 8. What "lite" means

`ocr-review-security-lite` is the pre-merge tier, defined by three constraints against
the full `security-assessment` skill:

| | security-lite | security-assessment |
|---|---|---|
| Scope | changed files in the diff | whole repo |
| Depth | pattern and sink review of changed lines plus reachable impact | 24 checks, LIGHT/STANDARD/DEEP |
| Engine | `ocr` + the rules pack, no MCP | `security_assessment` MCP, `PHX_LLM_*` service |
| Output | line-anchored review comments | report, ASVS %, risk matrix, generated tests |
| Cost | one ordinary `ocr` review | $8-10, 15-20 min |

The command is a thin wrapper. It runs:

```bash
ocr review --audience agent \
  --rule <path>/examples/security-profile/rule.json \
  --background "<context>" [target flags]
```

and then filters and reports. It does not invent a new review mode.

## 9. Skill and plugin surfaces

### 9.1 The mirroring rule

`plugins/open-code-review/skills/*/SKILL.md` are deliberate duplicates of the canonical
`skills/*/SKILL.md`, each carrying a banner stating that a symlink is avoided because
plugin installs may only materialise the plugin subtree. Any new skill must be authored
in `skills/` and mirrored into `plugins/open-code-review/skills/` with that banner.
There is no existing drift check for this; the build script's `--check` mode will be
extended to cover it.

### 9.2 Command naming

Claude Code namespaces plugin commands, and the existing files are `review.md` and
`delegate-review.md` with no `ocr-` prefix. The opencode plugin registers flat command
names and already uses `ocr-review`. So:

| Surface | Name | Invocation |
|---|---|---|
| Claude Code plugin command | `review-security-lite.md` | `/open-code-review:review-security-lite` |
| opencode plugin command | `ocr-review-security-lite` | `/ocr-review-security-lite` |
| Canonical skill | `open-code-review-security` | skill name |

Single-line change if the literal `ocr-review-security-lite` is wanted in Claude Code
too; flagged as an open decision in section 12.

### 9.3 Surface inventory

**New:**

- `skills/open-code-review-security/SKILL.md` — canonical. Locates the pack, runs
  `ocr review --rule`, reports by OWASP category and severity. Documents delegation
  mode as an alternative (`ocr delegate rule --rule <pack>` returns the merged
  security checklist to the host agent, no `ocr` LLM cost).
- `plugins/open-code-review/skills/open-code-review-security/SKILL.md` — mirror.
- `plugins/open-code-review/claude-code/commands/review-security-lite.md`.

**Modified:**

- `plugins/open-code-review/opencode/open-code-review.ts` — add `rule?: string` to
  `ReviewInput` and `reviewArgs`, a `pushValue(args, "--rule", input.rule)` line in
  `buildReviewArgs`, and a `config.command["ocr-review-security-lite"]` template.
  Already carries an SPDX header; `.ts` is scanned by `make english-check`.
- `plugins/open-code-review/opencode/test/open-code-review.test.mjs` — cover the new
  argument and command.
- `plugins/open-code-review/README.md` — document the new command alongside
  `/open-code-review:review` and `/open-code-review:delegate-review`.
- `plugins/open-code-review/claude-code/.claude-plugin/plugin.json`,
  `.cursor-plugin/plugin.json`, `.codex-plugin/plugin.json`,
  `.claude-plugin/marketplace.json` — version bumps; add a `security` keyword where a
  keyword list exists.
- `pages/src/content/docs/en/review-rules.md` — document the pack.
- `DOC_INDEX.md` — route to the pack README.

**Considered and deferred:** `plugins/open-code-review/qca/system-prompt.md`. A
security variant there is straightforward but QCA Forward is not a surface this fork
exercises; deferred rather than written blind.

## 10. PR integration payoff

`cmd/opencodereview/sarif.go:235` maps `Category -> ruleId`, so `--format sarif` output
uploads to GitHub Code Scanning and lands as native PR annotations. Every security
finding collapses to the single `ruleId: "security"`, so Code Scanning cannot group by
OWASP category. Promoting `ruleId` to `security/A03` is a small Go follow-up, out of
scope here and recorded in section 12.

## 11. Validation

The 90% coverage gate is a Go gate and does not apply to a data pack, and `CLAUDE.md`
is explicit that prompt wording has no automated regression. So the strategy is tiered:

**Deterministic, CI-enforceable:**

- `scripts/build-security-rules.sh --check` — generated output matches committed
  output; no file exceeds 12 KB; the two skill copies have not drifted.
- `ocr rules check --rule examples/security-profile/rule.json testdata/go/x.go` per
  language — asserts `Source: Custom (--rule)`, the expected `Pattern`, and that the
  merge header `## System-Specific Rules (Mandatory)` is present in the output.
- `rule.json` parses and every referenced file exists.

**Manual, pre-merge:**

- `testdata/<lang>/` fixtures — deliberately vulnerable files, at least one per OWASP
  category the pack claims to cover. Reviewed with `--format json` and checked by hand.
  These live in `testdata/` precisely so `go list ./...` and `verify-license.sh` skip
  them.
- A clean-file counterpart per language to observe the false-positive rate.

**The measurement that decides whether the pack ships:**

- **A/B on a real PR.** The same diff reviewed with and without `--rule`, diffing
  findings and token cost. This answers the two questions that matter: how much real
  security signal was added, and how much correctness signal was displaced. Easy to
  skip under time pressure, and the design depends on it not being skipped.

## 12. Risks and open questions

| Risk | Mitigation |
|---|---|
| Signal dilution — up to 12 KB of security prompt added to every file review may crowd out correctness findings | The reason the pack is opt-in rather than default, and the reason for the A/B |
| False positives — the review filter is deliberately permissive and will not catch them | FP discipline enforced in the spine at generation time |
| Prompt bloat and cost | Per-language slicing plus the 12 KB hard ceiling |
| Resume invalidation — rule text feeds `rule_config_sha256` in the run manifest, so enabling the pack makes existing checkpoints unusable | Documented in the pack README; expected behaviour, not a defect |
| Upstream drift — sinks are hand-distilled with no automated link back to the runbooks | Source commit SHA recorded in the spine header |
| Skill-copy drift between `skills/` and `plugins/.../skills/` | Covered by `--check` mode |

**Open question — provenance. Needs an answer before anything is pushed.** The source
runbooks live in `internal-cloude-skills`, a private repo. `origin` here is
`securityphoenix/open-code-review`, a public fork of an Apache-2.0 project. Publishing
distilled derivatives of internal material is not a call this design can make. If they
cannot be published, the identical design works with the pack kept in a separate
private repo and referenced by an absolute `--rule` path; nothing in the architecture
changes, only the location of `examples/security-profile/`.

**Open decision — command naming.** Section 9.2 uses `review-security-lite` in Claude
Code for consistency with `review` and `delegate-review`. One-line change to the literal
`ocr-review-security-lite`.

**Recorded follow-ups, out of scope:**

- SARIF `ruleId` granularity (`security/A03` instead of `security`) — Go change.
- Extending the pack to IaC and manifests (`terraform`, `github_workflows`,
  `package.json`, `pom.xml`) for OWASP A05, A06, and A08 coverage that app-code rules
  structurally cannot reach.
- Promoting the pack from a `--rule` file to a first-class `--security` overlay layer
  in the resolver, once the content has proven itself.
