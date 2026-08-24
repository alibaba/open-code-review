Parent document: `/CLAUDE.md`
Related documents:
- `ASSURANCE_CASE.md` (primary source for the original 4-boundary diagram — this doc adds two boundaries not in that diagram)
- `docs/architecture/SERVICE_TOPOLOGY.md`
- `docs/ai/MODEL_GUARDRAILS.md`

Read this when:
- You need every ingress/egress point in the system enumerated before a security review or before adding a new integration.

Purpose:
- Enumerate every trust boundary — the four already in `ASSURANCE_CASE.md` plus two this documentation pass surfaced that aren't in its diagram (MCP, delegate/QCA Forward).

Scope:
- Included: all six boundaries, privileged modules, identity/secret boundaries.
- Excluded: the mitigation detail for boundaries 1–4 (fully covered in `ASSURANCE_CASE.md`, cited not repeated).

---

# Trust Boundaries

`ASSURANCE_CASE.md` diagrams four boundaries (Git→CLI, CLI→LLM API, CLI→local output, Browser→Viewer) with full T1–T7 mitigations. This document extends that diagram with two boundaries this pass identified that the existing diagram doesn't cover, and restates the original four at pointer-depth.

## The six boundaries

1. **Git repo → CLI** (`ASSURANCE_CASE.md` boundary 1) — diff content may be adversarial; command injection mitigated by hardcoded `git` subcommands, no shell.
2. **CLI → LLM Provider API** (boundary 2) — HTTPS/TLS enforced, API keys never leave this boundary in logs.
3. **CLI → local filesystem** (boundary 3) — `pathutil.WithinBase()` constrains every agent-tool file write/read to the repo root.
4. **Browser → viewer** (boundary 4) — Host-header allowlist, no authentication (single-user local tool by design).
5. **CLI → MCP server** *(not in `ASSURANCE_CASE.md`'s diagram)* — a **local subprocess** MCP server is fully trusted (runs with the user's full environment, spawned by the user's own config); a **remote** MCP server carries header-injected credentials over a transport with **no enforced HTTPS**, unlike boundary 2. This is a real, documented-here-first asymmetry: the LLM provider path is TLS-enforced, the remote-MCP path is not.
6. **External host agent ↔ `ocr delegate`** *(not in `ASSURANCE_CASE.md`'s diagram)* — OCR hands a deterministic file list and rule text to an external agent and has zero further visibility or control. The most fully worked example, **QCA Forward**, explicitly documents its own gap: the calling agent's Bash access is meant to be read-only during a session, but this is **prompt-enforced only** — OCR provides no sandboxing, and the integration's own docs acknowledge the runtime enforcing it (if anything does) is outside OCR entirely.

## Privileged modules

| Module | Why privileged |
|---|---|
| `internal/pathutil` | The sole path-traversal guard — every file-touching tool depends on it being correct |
| `internal/gitcmd` | The only code path allowed to spawn `git` as a subprocess |
| `internal/llm/keycmd*.go` | Executes an arbitrary user-configured shell command to obtain credentials — by design, but worth flagging as the one place OCR intentionally shells out based on config content |
| `internal/viewer/hostguard.go` + `securityheaders.go` | The entire access-control surface for the one network listener this system opens |
| `internal/session/manifest.go`'s `sanitizeReason` | The only redaction layer between internal error detail and a persisted/rendered artifact |

## Identity and secret boundaries

- **Provider credentials** never cross from `internal/llm` into `internal/session`'s persisted JSONL — only the resolved model/provider *name* is recorded, never the token.
- **Session JSONL is the one place secrets *can* leak** — not credentials, but any secret that happened to be present in reviewed code, since prompt/response content is persisted in full (see `docs/security/SECURITY_MODEL.md`).
- **Resume identity** (`internal/session/resume_identity.go`) is itself a trust mechanism, not just a correctness one — it prevents silently reusing checkpoints across a repo/rule/model change that could otherwise let stale AI output masquerade as current.

## Known gaps / uncertainties:
- Whether config.json's `custom_providers.*.url` for a self-hosted gateway is ever HTTPS-enforced or validated (vs. accepted verbatim) was not directly confirmed.
