Parent document: `/CLAUDE.md`
Related documents:
- `docs/architecture/DEPENDENCY_GRAPH.md`
- `docs/architecture/DATA_CONTRACTS.md`
- `docs/development/SAFE_CHANGE_ZONES.md`

Read this when:
- You're about to change a shared type, schema, or core-loop file and need to know who breaks.
- You're reviewing a PR that touches a high-fan-in package.

Purpose:
- Rank modules by change-impact and state, concretely, what breaks — direct and indirect — for the highest-risk ones.

Scope:
- Included: the modules with the widest blast radius, with explicit consumer lists.
- Excluded: how to change them safely (see `docs/development/SAFE_CHANGE_ZONES.md`), general ownership (see `MODULE_OWNERSHIP.md`).

---

# Change Blast Radius

Ranked highest to lowest impact.

## 1. `internal/model/*` — shared DTOs

**Direct consumers**: `internal/agent`, `internal/scan`, `internal/diff`, `internal/llmloop`, `internal/session`, `cmd/opencodereview` (rendering).
**Indirect consumers**: `internal/viewer` (via session JSONL), every CI posting script (JS + Python ×4), the VS Code extension (parses `--format json`).
**What breaks on a careless change**: renaming/removing a `LlmComment` field breaks every renderer and every external CI script simultaneously, with no compiler error outside the Go binary — the JS/Python consumers only fail at runtime, in someone else's pipeline. Changing `Category`/`Severity` enum values is a silent behavioral change for routing logic in `post-review-comments.js` (`route_severity_below`, `route_categories`).

## 2. `internal/session/persist.go` + `manifest.go` — JSONL schema

**Direct consumers**: `internal/session` itself (resume), `internal/viewer` (`store.go`).
**Indirect consumers**: any external tool reading JSONL directly (documented as a supported workflow in `viewer.md`'s "grepping across sessions" note), `session_end.run_manifest`'s embedded object which mirrors the CLI's JSON output exactly.
**What breaks**: adding/removing a record `type` without updating `walkSessionFile`'s best-effort skip logic can silently drop data from `ocr session list` summaries. Changing `Coverage` set semantics without bumping `ocr.run-manifest/v1` breaks the "always-consistent" guarantee the manifest is designed to provide. This is the **single most externally-depended-on schema in the repo** because it is the substrate for resume correctness.

## 3. `internal/llmloop/loop.go` — the shared tool-use loop

**Direct consumers**: `internal/agent` (review) and `internal/scan` both call `Runner.RunPerFile` — **a bug or behavior change here ships to both commands simultaneously**, and the two orchestrators don't have independent test coverage isolating loop behavior from their own filtering/batching logic.
**What breaks**: the five loop-exit conditions (task_done / max rounds / 3 empty rounds / context cancelled / compression failure), the 60%/80% compression thresholds, and per-tool telemetry counters are all defined once here. A change to `MainLoopStop` semantics changes observable behavior in `docs/operations/OBSERVABILITY.md`'s event inventory and `docs/operations/FAILURE_MODES.md`'s failure table simultaneously.

## 4. Embedded prompt templates (`internal/config/template/*.json`, `prompts/*.md`)

**Direct consumers**: every review/scan run, for every provider.
**What breaks**: these are `//go:embed`-baked into the binary — a wording change requires a rebuild and ships to every user on upgrade with **no automated regression test for prompt quality**. This is the highest-risk *silent* change category in the repo: a well-intentioned prompt tweak can degrade comment quality or anchoring reliability repo-wide, and the only detection mechanism is manual (`ocr review --preview` + a real session inspected via `ocr viewer`).

## 5. `internal/config/toolsconfig/tools.json`

**Direct consumers**: every provider's tool-calling surface.
**What breaks**: a malformed schema breaks tool-calling for every model/provider at once (not gracefully degradable — this is the contract the SDK-level function-calling validates against). Removing a tool name without updating `internal/tool`'s registry produces `tool.NotAvailableMsg` at runtime rather than a build error.

## 6. `internal/llm/resolver.go` — endpoint precedence

**Direct consumers**: every command that needs an LLM (`review`, `scan`; not `delegate`/`viewer`).
**What breaks**: this is a **silent behavior change** category — reordering the 4-strategy precedence (config → OCR env → Claude Code env → rc files) changes which credential/model a user's existing setup resolves to, with no error raised (both old and new resolution can be "complete" triples). This is the kind of change that passes all unit tests and only surfaces as "why is it suddenly using the wrong model" in production.

## 7. `internal/llm/providers.go` — provider preset registry

**Lower risk, additive by default.** Adding a new provider entry is safe (pure addition). Changing an *existing* preset's `BaseURL`, `Protocol`, or `EnvVar` silently redirects every user relying on that preset's defaults — same silent-change risk class as #6, scoped to one provider.

## Known gaps / uncertainties:
- No automated test was found (in this pass) that pins prompt *output quality* — only structural/unit tests around the loop mechanics were confirmed to exist. If such a test exists, it should be cited here and the risk downgraded.
