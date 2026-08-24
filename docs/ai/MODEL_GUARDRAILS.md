Parent document: `/CLAUDE.md`
Related documents:
- `docs/security/TRUST_BOUNDARIES.md`
- `ASSURANCE_CASE.md` (primary source — this doc extends it with AI-specific detail, does not replace it)
- `docs/architecture/DATA_CONTRACTS.md`

Read this when:
- You're evaluating whether a change weakens a safety mechanism, or auditing what happens when the model misbehaves.

Purpose:
- Prompt-injection protection, output validation, and sensitive-data handling — specifically the AI-facing guardrails layered on top of the general security controls in `ASSURANCE_CASE.md`.

Scope:
- Included: guardrails that exist because the system feeds untrusted content (diffs, files, MCP tool results) to an LLM and acts on its output.
- Excluded: general command-injection/path-traversal mitigations already fully documented in `ASSURANCE_CASE.md` (cited, not repeated).

---

# Model Guardrails

## Prompt injection

**The primary guardrail is prompt-level, not technical.** `MAIN_TASK`'s system prompt's **Strict Focus Rules** instruct the model that context-gathering tools (`file_read`, `file_read_diff`, `file_find`, `code_search`) are read-only understanding aids — any issue observed through them, in a file outside the current diff, must never become a `code_comment`. **This is enforced by instruction, not by code** — nothing in `internal/tool` or `internal/llmloop` technically prevents the model from emitting a comment about a file it read via a context tool. A sufficiently adversarial or confused model can violate this; the `REVIEW_FILTER_TASK` post-pass (itself AI, not a validator) is the only second check, and it's a quality filter, not a security control (see `docs/ai/AI_SYSTEM_MAP.md`'s note on "AI checking AI").

**Diff/file content is never sanitized or escaped before being embedded into the prompt.** A crafted diff or file containing instruction-like text is passed to the model verbatim as the `{{diff}}` placeholder — this is the classic indirect-prompt-injection surface for any code-review tool. Mitigating factors: `code_comment`'s output is constrained to a narrow schema (comment text + optional suggestion, anchored to a snippet that must exist in the diff), so the practical blast radius of a successful injection is "the model writes a misleading comment," not arbitrary action — the model has no tool that executes code or mutates files.

## Output validation

- **Tool-call schema validation** happens at the SDK/function-calling layer (JSON Schema in `tools.json`) — malformed tool-call arguments are rejected before reaching OCR's handlers.
- **Line-anchoring validation** is the sliding-window matcher: `existing_code` must match diff content (whitespace-insensitive) before a comment gets real line numbers; failure degrades to `RE_LOCATION_TASK` (a second AI call) or `StartLine=0` (explicit "unanchored, human must locate" signal) — never a fabricated line number.
- **`REVIEW_FILTER_TASK`** removes comments the model itself judges provably incorrect after seeing the full diff again — a quality pass, not a safety boundary.
- **Path validation for tool-driven file access** is `pathutil.WithinBase()`, applied before and after symlink resolution (`internal/pathutil/path.go`, cited in `ASSURANCE_CASE.md` T3) — this is the guardrail that matters if the model is tricked into requesting a path outside the repo via `file_read`.

## MCP tool-result handling

MCP tool call results are flattened to text; non-text content types degrade to a `[unsupported content type: %T]` stub rather than being silently dropped or crashing — see `docs/architecture/DATA_CONTRACTS.md`. There is no additional sanitization of MCP server output before it enters the model's context — an MCP server is a trusted extension point (the user configured it), not treated as adversarial input.

## Hallucination mitigation

Structural, not statistical: the line-anchoring requirement forces every comment to reference text that provably exists in the diff (or explicitly admit it couldn't be located). There is no confidence scoring or self-consistency checking beyond `REVIEW_FILTER_TASK`.

## Privacy / sensitive data handling

- **API keys**: never logged, never in telemetry, never in session JSONL (only the resolved endpoint's model/provider name is recorded, not the token).
- **Prompt/response content**: session JSONL persists it **in full** on local disk (`llm_request`/`llm_response` records) — this is by design for debuggability via `ocr viewer`, but means any secret accidentally present in a diff is now durably stored under `~/.opencodereview/`. Telemetry deliberately never carries this content.
- **`content_logging` config key is dead** for the config-file path — only `OCR_CONTENT_LOGGING` (env var) reaches the actual read site; `~/.opencodereview/config.json`'s `content_logging: true` is silently ignored. This contradicts the "reserved" framing in `pages/.../telemetry.md` and is a real gap worth fixing or re-documenting.
- **Manifest failure/waive reasons** pass through `sanitizeReason` (strips URL userinfo, bearer/basic tokens, `key: value` credential patterns, control chars, 500-rune cap) — explicitly documented as a defense-in-depth floor, not a substitute for caller-side redaction (it does not strip absolute paths or raw request bodies).

## QCA Forward's acknowledged gap

The QCA Forward delegation integration expects the host agent's Bash access to be read-only during a review session — but this is **prompt-enforced only**; OCR provides no sandboxing of the calling agent's tool access, and the integration's own documentation states this explicitly ("unless the QCA runtime applies a stricter command policy"). This is the one guardrail in the system that is entirely outside OCR's control.

## Known gaps / uncertainties:
- Whether any length/content pre-filter exists on `{{diff}}` beyond the token-budget skip (which is a cost control, not a safety control) was not found — flag as absent unless evidence surfaces otherwise.
