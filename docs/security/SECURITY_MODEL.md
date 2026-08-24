Parent document: `/CLAUDE.md`
Related documents:
- `ASSURANCE_CASE.md` (primary source — full threat model, Saltzer & Schroeder mapping, OWASP/CWE countermeasure table; this doc summarizes and adds AI-specific/integration findings from this documentation pass)
- `SECURITY.md` (vulnerability reporting, release signing)
- `docs/security/TRUST_BOUNDARIES.md`
- `docs/ai/MODEL_GUARDRAILS.md`

Read this when:
- You need the auth/secrets/trust summary before a security review — then go to `ASSURANCE_CASE.md` for the full threat table.

Purpose:
- Summarize the auth model, secrets handling, and trust assumptions; point to `ASSURANCE_CASE.md` as the canonical detailed source rather than duplicating its threat table.

Scope:
- Included: auth model, secrets handling, trust assumptions, this-pass's additions to the existing assurance case.
- Excluded: the full T1–T7 threat table and OWASP mapping (already authoritative in `ASSURANCE_CASE.md` — read it directly).

---

# Security Model

`ASSURANCE_CASE.md` is the authoritative, already-comprehensive security document for this repo — its threat model (T1–T7), Saltzer & Schroeder principle mapping, and OWASP/CWE countermeasure table are not reproduced here. This doc summarizes the auth/secrets/trust picture and adds findings from this documentation pass that extend, rather than duplicate, that source.

## Authentication model

**There is no authentication anywhere in this system's own surfaces.** It's a local single-user CLI tool:
- The CLI itself requires no login — it runs with the invoking user's OS permissions.
- The viewer has no auth — its perimeter is the Host-header allowlist (`docs/security/TRUST_BOUNDARIES.md`), not credentials.
- Delegate mode has no auth — it's local deterministic computation.

**LLM provider authentication** (the one place credentials matter): static `api_key` in config, `api_key_cmd`/`auth_token_cmd` (external secret-manager command), or a built-in provider's environment-variable fallback. Bedrock uses ambient AWS credential chain (SigV4), never an API key. Precedence: static key > `_cmd` (warning if both set) > env fallback (presets only).

## Secrets handling

- API keys/tokens: never logged, never in telemetry, never in `code_comment` output, `config.json` written with `0600` permissions.
- `api_key_cmd` execution is deliberately deferred to the last possible moment in endpoint resolution, specifically to avoid triggering an unwanted secret-manager prompt (1Password/Touch ID) on a config that will fail validation anyway.
- MCP server credentials (`mcp_servers.<name>.env`, remote header auth) are expanded from `$ENV_VAR` at connection time — **no enforced HTTPS check exists for remote MCP servers**, unlike the LLM provider path (see `docs/security/TRUST_BOUNDARIES.md`). This is a gap this documentation pass identified that isn't in `ASSURANCE_CASE.md`'s current threat table.
- Manifest failure/waive reasons pass through `sanitizeReason` (redacts URL userinfo, bearer/basic tokens, `key: value` credential patterns) — explicitly a defense-in-depth floor, not comprehensive (doesn't strip paths or raw bodies).
- **Session JSONL stores full prompt/response content on local disk, unencrypted, indefinitely** (until manually deleted). This is a deliberate debuggability trade-off, not an oversight — `viewer.md` documents mitigation options (periodic deletion, transient `HOME` in CI). Treat any secret present in a reviewed diff as now durably stored locally.

## Trust assumptions

| Actor | Trust level | Source |
|---|---|---|
| Local user | Trusted, full control | `ASSURANCE_CASE.md` |
| LLM provider API | Semi-trusted, response validated | `ASSURANCE_CASE.md` |
| Git repository content | Semi-trusted, diffs may be adversarial | `ASSURANCE_CASE.md`; extended in `docs/ai/MODEL_GUARDRAILS.md` (prompt-injection surface) |
| MCP server (local subprocess) | Fully trusted — user-configured, runs with full environment | new in this pass |
| MCP server (remote) | Semi-trusted, but with a weaker transport guarantee than the LLM path | new in this pass |
| External host agent (delegate mode / QCA Forward) | Trusted with OCR's file-selection output, but OCR has **no visibility or control** over what that agent then does | new in this pass |

## Known gaps / uncertainties:
- Whether `ASSURANCE_CASE.md`'s threat table should be formally extended with a "T8: MCP remote server credential exposure over unenforced-TLS" entry is a documentation decision for the maintainers, not made unilaterally here.
