# Phoenix Security Scan Integration — Design

**Date:** 2026-08-24
**Status:** Approved for planning
**Repos touched:** `open-code-review-phx` (primary), `agent-code-analyzer-r2` (Phoenix, critical path)

## 1. Problem

`ocr` reviews git diffs with an LLM tool-use agent. Phoenix (`agent-code-analyzer-r2`)
runs deterministic security scanners over the same code. Today they are unrelated
processes: a PR gets two independent comment streams, two verdicts, and no
cross-referencing. Scanner findings arrive without the reasoning that would tell a
reviewer whether they matter in this diff; the review agent works blind to what the
scanners already found.

This design makes `ocr` the orchestrator: it pulls Phoenix SAST and SCA findings into
the review, splits them by confidence, adjudicates the uncertain ones with an LLM
triage pass that can investigate via Phoenix's graph tools, and emits one merged
output with one verdict.

## 2. Decisions (settled during brainstorming)

| Decision | Choice | Rationale |
|---|---|---|
| Orchestrator | `ocr` drives Phoenix | One PR comment stream, one exit code, one session JSONL. |
| Agent's job | Triage + enrich, split by confidence | High-confidence findings pass through with a fix; uncertain ones get adjudicated. |
| SCA scoping | Manifest delta + reachability gate | Only PR-introduced/bumped components; reachability decides pass-through vs triage. |
| Transport | Deterministic prefetch **and** MCP investigation | Prefetch guarantees coverage; MCP lets the triage agent drill into a finding. |
| Divergence | Vendor-neutral core + Phoenix adapter | Keeps upstream rebases cheap; lets other scanners plug in. |
| Gating | `ocr` owns verdict and comment | Phoenix `PASS/WARN/BLOCK` is an input to `ocr` policy, never silently dropped. |
| Pipeline shape | Dedicated triage stage (not per-file injection) | Reachability and cross-file taint are per-finding investigations, not side-thoughts while reading one file. |

## 3. Current-state findings

### 3.1 What Phoenix offers today

| Surface | SAST | SCA |
|---|---|---|
| `phoenix pr-scan` CLI (`cli/.../PrScanCommand.kt`) → SARIF | Yes — diff-aware, tiered (`RULES_ONLY`, `RULES_AI_VALIDATION`, `AI_SAST`, `RULES_AI_SAST`), verdict `PASS/WARN/BLOCK` | No |
| REST `/api/v1/external/pr-scan/{resolve,execute,:id,:id/sarif}` | Yes | No |
| REST `/api/v1/external/sca/{workspaceId}/scan` (`ExternalScaController.kt`) | — | Whole-workspace only; no diff awareness, no CLI or MCP wrapper |
| MCP `/api/v1/external/mcp` (47 tools) | `sast_scan`, `sast_findings`, `sast_finding_context`, `sast_export_sarif`, `sast_mark_false_positive`, `search_rules`, plus graph tools `entry_points`, `impact`, `context`, `query`, `processes` | **Zero SCA tools** |
| GitHub Action `github-action/action.yml` | Standalone; posts its own PR comment | No |

`PrScanCommand` exit codes: `0` = PASS/CLEAN/WARN, `1` = BLOCK, `2` = error.

### 3.2 What `ocr` already provides

- **Remote MCP client with custom headers** — `internal/mcp/client.go`,
  wired at `cmd/opencodereview/review_cmd.go:485`. Per-server tool allowlist.
  Phoenix's MCP server is reachable today with config alone.
- `--background` / `--background-file` → `{{requirement_background}}` in prompts.
- `--rule` (ordered path→rule JSON), `--tools` (tools config), `--format sarif`.
- Shared `internal/llmloop.Runner` already driven by two consumers (`review`, `scan`) —
  a third consumer is an established pattern, not a new one.
- `model.LlmComment` carries `Category` and `Severity` but has **no** provenance,
  rule id, CVE, purl, or fingerprint field. Every comment is currently "the LLM said so".

### 3.3 The two gaps

1. SCA has no diff-scoped path and no MCP tool anywhere in Phoenix.
2. `ocr` has no concept of a third-party finding.

## 4. Architecture

### 4.1 New packages — vendor-neutral core

**`internal/findings/`**

- `model.go` — the `ExternalFinding` DTO:

  | Field | Notes |
  |---|---|
  | `ID`, `Fingerprint` | `Fingerprint` is the stable dedup/resume key: `sha256(source, rule_id, path, normalized_snippet)`. Deliberately **not** line-based, so a shifted line does not create a new finding. |
  | `Source`, `RuleID`, `Kind` | `Kind` ∈ `sast \| sca \| secret \| iac`. |
  | `Path`, `StartLine`, `EndLine` | Repo-relative, resolved through `internal/pathutil`. |
  | `Message`, `Severity` | `Severity` ∈ `critical \| high \| medium \| low`, matching `LlmComment`. |
  | `Confidence` | `high \| medium \| low \| unknown`. |
  | `CWE`, `CVE`, `PURL` | Optional; `CVE`/`PURL` populated for `Kind == sca`. |
  | `Reachability` | `reachable \| unreachable \| unknown`. **Three-state, never boolean.** |
  | `KEV`, `Malware`, `ExploitEvidence` | Blue intel, three-state where absence ≠ negative. |
  | `Evidence` | Ordered flow steps (file/line/description) for taint paths. |
  | `Raw` | Opaque vendor blob, round-tripped into session JSONL for traceability. |

- `sarif.go` — SARIF 2.1.0 **ingestion** (`[]ExternalFinding` from a SARIF run). `ocr`
  currently only emits SARIF (`cmd/opencodereview/sarif.go`); this is the inverse and
  must tolerate partial/vendor-extended documents.
- `provider.go` — `Provider` interface:
  `Fetch(ctx, ScanRequest) (Result, error)`, where `ScanRequest` carries repo dir,
  base/head refs, changed file list, and changed-manifest list; `Result` carries
  findings plus an optional upstream verdict.
- `policy.go` — the confidence split (see §4.3).
- `dedup.go` — fingerprint dedup, then line-proximity merge against `LlmComment`s.

**`internal/findings/providers/sarif/`** — file provider backing `--findings <file>`.
The always-available baseline; makes Semgrep, Trivy, and Snyk work with no Phoenix at all.
This is also the fixture surface for every test that does not need a live Phoenix.

**`internal/triage/`** — the new stage. Structurally a sibling of `internal/scan`:
builds units of work (finding clusters grouped by fingerprint affinity — same rule and
file, or same CVE), drives the shared `llmloop.Runner`, owns its own prompts and tool set.

### 4.2 New package — fork-specific adapter

**`internal/findings/providers/phoenix/`** — implements `Provider`:

- SAST: `POST /pr-scan/resolve` → `POST /pr-scan/execute` → poll `GET /pr-scan/{id}`
  → `GET /pr-scan/{id}/sarif`, then SARIF ingestion via the shared `sarif.go`.
- SCA: manifest delta → `POST /sca/pr-delta` (new, see §5) → CVEs + reachability.
- Maps Blue intel (KEV, malware, exploit evidence) onto `Confidence`.
- Carries the Phoenix `finalVerdict` through as `Result.UpstreamVerdict`.

### 4.3 Confidence policy

Per finding, exactly one disposition:

| Disposition | Condition | Behaviour |
|---|---|---|
| `pass-through` | `Severity ∈ {critical, high}` **and** (`KEV` **or** `Confidence == high` **or** `Reachability == reachable`) | Reported verbatim. The agent may only **add** a fix suggestion and an explanation. It may not dismiss. |
| `triage` | Lands on a changed line, not pass-through | Adjudicated by the triage stage. |
| `drop` | Outside the diff | Recorded in the session JSONL, not reviewed, not reported. |

`Reachability == unknown` routes to `triage`, never to `pass-through` and never to `drop`.

**Fail-closed invariant.** "We did not check" must never resolve as "we checked and it is
clean". This is enforced by the three-state fields and asserted by a dedicated provider
test. Phoenix's own changelog records this defect class three times (6.117.10, 6.118.1,
6.118.2); the design treats it as a known trap, not a hypothetical.

### 4.4 Runtime flow

```
ocr review --security
  1. resolve endpoint -> load diffs -> 5-gate file filter          [unchanged]
  2. PREFETCH (deterministic; runs concurrently with 4a)
       SAST: pr-scan resolve -> execute -> poll -> SARIF
       SCA:  manifest delta from diff -> purls -> /sca/pr-delta -> CVEs
             + per-CVE reachability probe
       -> []ExternalFinding, fingerprinted, mapped onto changed lines
  3. POLICY SPLIT -> pass-through | triage | drop
  4a. REVIEW  (existing per-file loop)
        + a one-line findings digest per file, so the review agent does not
          redundantly re-flag what the scanner already caught
  4b. TRIAGE  (new per-finding loop; joins after 2 and 4a)
        tools: file_read, file_read_diff, code_search
               + Phoenix MCP: sast_finding_context, impact, entry_points,
                 query, sca_finding_context
        emits: finding_verdict{confirmed|dismissed|uncertain, rationale, fix?}
  5. ENRICH pass-through findings — fix suggestion only, no adjudication
  6. MERGE + DEDUP — fingerprint first, then line-proximity vs LLM comments
  7. review-filter pass — scanner-provenance comments are exempt
  8. GATE — verdict = f(confirmed findings, policy, Phoenix verdict)
  9. persist JSONL -> render (text | json | sarif)
```

**Concurrency:** steps 2 and 4a run concurrently and join before 4b. `pr-scan` polls at
2s intervals with a 300s default timeout; serialising it ahead of the review would be
pure dead wall-clock.

**Degradation:** if the provider fails or times out, the review completes normally and
the run is marked `security: degraded` with the reason. A security prefetch failure must
not fail an otherwise-valid review — but it must also never render as a clean security pass.

### 4.5 Changes to existing code

| File | Change |
|---|---|
| `internal/model/review.go` | `LlmComment` gains `Provenance` (`llm \| scanner \| scanner-confirmed`), `Source`, `RuleID`, `CVE`, `Fingerprint`, `Verdict`. |
| `internal/config/template/prompts/` | New `triage_task_system.md`, `triage_task_user.md`. |
| `internal/config/toolsconfig/tools.json` | New `finding_verdict` tool alongside `code_comment`. |
| `internal/session/persist.go`, `manifest.go` | New JSONL record types for findings and verdicts. |
| `cmd/opencodereview/review_cmd.go` | Wire prefetch, policy, triage, merge. |
| `cmd/opencodereview/sarif.go` | Emit provenance and rule id in the SARIF output. |
| `cmd/opencodereview/shared_flags.go` | New flags (§4.6). |

**Schema-version obligation.** Per `CLAUDE.md` §10, new JSONL record shapes require
bumping `ocr.run-manifest/v1`. The viewer (`internal/viewer/`), resume logic, and every
CI posting script key off it. This is a hard requirement, not a nicety.

**Resume interaction.** `review` gates checkpoint reuse on a strict identity hash
(repo / diff-range / rule-config / provider-model). The security profile and policy
configuration must be folded into that hash — otherwise a `--security` rerun would
reuse a non-security checkpoint and silently report no findings.

### 4.6 CLI surface

```
ocr review --findings <file.sarif>    # vendor-neutral; any SARIF-emitting scanner
ocr review --security                 # Phoenix prefetch + triage + security rules
ocr review --security --no-triage     # prefetch + pass-through only, single pass
ocr triage --findings <file.sarif>    # standalone triage of an existing SARIF
```

`ocr triage` runs steps 2 (from file), 3, 4b, 5, 6, 8, and 9 of §4.4 — everything except
the per-file review loop. It emits the same merged comment output as `ocr review`
(`text | json | sarif`) and the same exit-code semantics, so it is a drop-in for CI
pipelines that already run a scanner and want adjudication without a full review.

Config keys: `security.provider`, `security.policy.*`, `security.phoenix.{api_url,
api_token_env, workspace_id}`, plus existing `mcp_servers.phoenix.*`.

Credentials are read from env (`PHOENIX_API_TOKEN`), never from the config file, and
never written to session JSONL.

## 5. Phoenix-side work (critical path)

SCA is the blocker. None of this exists today.

1. **`POST /api/v1/external/sca/pr-delta`** — body: repo, base/head refs, changed
   manifest paths (or before/after manifest contents). Returns per-purl CVE list with
   Blue intel and a reachability verdict.
2. **Manifest-delta resolution** — reuse `sbom-worker`'s build-file resolution, run
   against two trees, diff the resolved component sets. Distinguish added / bumped /
   unchanged; only added and bumped are in scope.
3. **Per-CVE reachability** — `sca/reachability` exists but is batched per repo. Needs a
   query keyed to the changed code that returns a three-state verdict per CVE.
4. **MCP tools `sca_pr_delta` and `sca_finding_context`** — so the triage agent can
   investigate an SCA finding the way it can a SAST one.
5. *(optional)* **`phoenix sca-scan --pr` CLI wrapper** — makes CI debuggable and gives
   the integration a non-HTTP fallback path.

Items 1–3 are required for the reachability gate. Item 4 is required for triage quality;
without it, SCA findings can only be adjudicated from the diff and the CVE metadata.

## 6. Distribution surfaces

The security variant must reach every surface the existing review reaches.

| File | Action |
|---|---|
| `skills/open-code-review-security-phx/SKILL.md` | **New** — security review skill. |
| `plugins/open-code-review/skills/open-code-review-security-phx/SKILL.md` | **New** — mirrored copy. |
| `plugins/open-code-review/claude-code/commands/ocr-review-security-phx.md` | **New** — the `/ocr-review-security-phx` command. |
| `plugins/open-code-review/claude-code/.claude-plugin/plugin.json` | Version bump; `commands: ./commands` auto-discovers the new file. |
| `.claude-plugin/marketplace.json` | Version bump and description update. |
| `plugins/open-code-review/.codex-plugin/plugin.json` | Add security entries to `defaultPrompt`. |
| `plugins/open-code-review/.cursor-plugin/plugin.json` | Keywords and version bump. |
| `plugins/open-code-review/opencode/open-code-review.ts` | Add `security?: boolean` and `findings?: string` to the input type; push `--security` / `--findings`. Extend `test/open-code-review.test.mjs`. |
| `plugins/open-code-review/qca/` | Optional — delegate-mode security variant of `system-prompt.md`. |

### 6.1 Skill-tree drift

`skills/` and `plugins/open-code-review/skills/` are **not** symlinks, and their two
existing skills already differ (`diff -rq` reports both `SKILL.md` files as differing).
Adding a third pair triples the drift surface.

This design adds a `make sync-skills` target that copies `skills/` into
`plugins/open-code-review/skills/`, plus a CI check that fails when the two trees
diverge. Reconciling the two *existing* pairs is in scope — the check cannot be
introduced while it is already red.

### 6.2 Naming

The user specified the command name `ocr-review-security-phx`. Skills follow the same
suffix (`open-code-review-security-phx`) to keep fork-specific assets visually distinct
from the upstream `open-code-review` and `open-code-review-delegate` assets.

## 7. Testing

Per `AGENTS.md`: `make check`, `make test`, 90% coverage gate.

| Area | Approach |
|---|---|
| SARIF ingestion | Golden fixtures: real `phoenix pr-scan` output, plus Semgrep and Trivy documents, plus a truncated/malformed document that must error rather than yield an empty clean result. |
| Policy split | Table-driven over the (severity × KEV × reachability × confidence) matrix, including every `unknown` combination. |
| Fail-closed invariant | Dedicated test: a provider response with absent reachability must produce `unknown` and route to `triage` — never `pass-through`, never `drop`. |
| Triage stage | Driven against the existing `internal/tool` stub; asserts verdict extraction, rationale persistence, and per-finding tool-round capping. |
| Phoenix provider | `httptest` server replaying recorded resolve / execute / poll / sarif exchanges, including a poll timeout and a `FAILED` job status. |
| Merge / dedup | An LLM comment and a scanner finding colliding on one line; a finding whose line shifted between base and head. |
| Session round-trip | New record types survive write → read → resume, with the bumped `schema_version` asserted. |
| Plugin surfaces | Skill-tree sync check; `opencode` plugin argument-construction tests for the new flags. |

Prompt wording has no automated regression (`CLAUDE.md` §10). The two new prompts require
manual verification against a real session with `ocr review --preview` before merge.

## 8. Risks

| Risk | Mitigation |
|---|---|
| Second LLM pass raises token cost. | Triage shares `--max-tokens-budget` with review; per-finding tool-round cap; `--no-triage` escape hatch. |
| Prefetch latency (300s default `pr-scan` timeout). | Run concurrently with the review stage; join before triage; degrade honestly on timeout. |
| Credential surface — REST provider needs an API token and workspace id in CI. | Env-only (`PHOENIX_API_TOKEN`); documented in the action and skill; never persisted to session JSONL. |
| Prompt regression with no automated gate. | Manual verification requirement recorded as a merge blocker. |
| Reachability gate is the highest-value and least-certain component. | Three-state everywhere; fail-closed test; the gate can be disabled by policy without disabling SCA. |
| Skill-tree drift. | `make sync-skills` plus a CI divergence check; existing drift reconciled first. |
| Phoenix API is unversioned for the new SCA endpoint. | Provider pins an explicit contract and fails loudly on shape mismatch rather than degrading to empty results. |

## 9. Out of scope

- Container, IaC, and DAST scanners. The `Kind` field anticipates them; no provider work here.
- Posting to GitLab / Gerrit / GitFlic / Codeup. The existing posting scripts consume the
  session JSONL and inherit the merged output for free.
- Unifying the two CI-comment-posting implementations (`CLAUDE.md` §11 known unknown).
- Cutting over Phoenix's `sbom-worker` to be authoritative.

## 10. Known gaps

- Whether Phoenix's per-CVE reachability can answer at PR latency is unverified. If it
  cannot, the gate degrades to `unknown` for every SCA finding, which routes everything
  to triage — correct, but more expensive than intended.
- The triage clustering heuristic (group by rule+file, or by CVE) is a first guess and
  will need tuning against real PR volume.
- Whether triage should be able to run against a *stale* SARIF (scanner run at a
  different commit than the review target) is unresolved. The plan should decide whether
  to reject it, or accept it with the line-shift tolerance the fingerprint already provides.
