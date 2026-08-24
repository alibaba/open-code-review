Parent document: `/CLAUDE.md`
Related documents:
- `docs/architecture/SERVICE_TOPOLOGY.md`
- `docs/security/TRUST_BOUNDARIES.md`
- `pages/src/content/docs/en/mcp.md` (user-facing MCP config, not repeated here)

Read this when:
- You're adding, auditing, or debugging any external integration — an LLM provider, MCP server, CI platform, or coding-agent plugin.

Purpose:
- Full inventory of third-party/external touchpoints: what each is used for, its contract, and its failure/fragility risk.

Scope:
- Included: LLM providers, MCP, all CI integrations, coding-agent plugins, VS Code extension.
- Excluded: internal module wiring (`docs/architecture/DEPENDENCY_GRAPH.md`).

---

# External Integrations

## LLM providers

26+ built-in presets across 4 protocols plus arbitrary custom OpenAI/Anthropic-compatible endpoints. Full table: `pages/src/content/docs/en/configuration.md`. Fragility: a preset's `BaseURL`/`Protocol`/`EnvVar` change silently redirects every user relying on defaults (`docs/architecture/CHANGE_BLAST_RADIUS.md` #7). Exactly one provider resolved per run — no fallback.

## MCP servers (client only)

OCR connects **out** to external MCP servers (local subprocess via stdio, or remote via Streamable HTTP) — it never exposes itself as an MCP server. Contract: `mcp_servers.<name>.{command, args, tools, setup, env}` in config; tools merge into the same registry as built-ins, subject to a name-collision policy (built-in always wins; first MCP registration wins otherwise). **Risk**: remote MCP servers get header-injected credentials with no enforced HTTPS (see `docs/security/TRUST_BOUNDARIES.md`); non-text tool-result content types silently degrade to a stub string (`docs/architecture/DATA_CONTRACTS.md`).

## CI platform integrations

| Platform | Mechanism | Poster implementation | Notes |
|---|---|---|---|
| **GitHub Actions** | `action.yml` (canonical composite action) | `scripts/github-actions/post-review-comments.js` (Node) | Sticky-summary, incremental (IoU-based overlap matching), severity/category routing, batched inline comments; `examples/github_actions/` is a thin caller |
| **GitLab CI** | pipeline job installs via npm, configures via env/`ocr config set` | `post_review.py` | Most feature-complete non-GitHub example — independently reimplements retry/backoff tuning and sticky-summary toggling |
| **Gerrit** | Jenkinsfile (Groovy), Jenkins `credentials()` for the token | `post_review.py` | Pins OCR to an explicit version (`@1.7.12`) unlike other examples' floating/`latest` default |
| **GitFlic / Codeup** | Same shape as GitLab | `post_review.py` | — |
| **Bitbucket Pipelines** | Repository variables, hard-fails if unset | none | Simplest integration — no dedicated poster script |

**Two real risks identified in this pass**:
1. **Posting-logic duplication** — the same retry/incremental/routing feature set is reimplemented independently in JS (GitHub) and Python (×4 other platforms). A bug fixed in one does not propagate to the other.
2. **Version-pin inconsistency** — GitHub Action defaults to `ocr_version: latest` (floating), Gerrit's example hardcodes a version, others float — any unpinned pipeline risks a breaking upgrade silently changing review behavior.
3. **SARIF is fully implemented (`cmd/opencodereview/sarif.go`, `--format sarif`, SARIF v2.1.0) but wired into zero shipped integrations** — no example uploads to GitHub code-scanning (`upload-sarif`) despite the capability existing.

## Coding-agent plugins (`plugins/open-code-review/`)

| Plugin | Mechanism |
|---|---|
| Claude Code | Marketplace-installed slash commands (`/open-code-review:review`, `/open-code-review:delegate-review`) shelling out to `ocr` |
| Codex | `.codex-plugin/plugin.json` — callable review skills backed by the local `ocr` CLI |
| Cursor | `.cursor-plugin/plugin.json`, manually installed |
| OpenCode | native plugin (`opencode/open-code-review.ts`) |
| **QCA Forward** | Structurally distinct — delegation mode only, **no OCR LLM configuration at all**. The host model performs the review; OCR supplies deterministic file-selection/rule-resolution. Write/Edit disabled for the session; Bash expected read-only, **prompt-enforced only** (acknowledged gap — see `docs/security/TRUST_BOUNDARIES.md`) |

All four standard plugins require `ocr` pre-installed and configured; QCA Forward is the only one that explicitly forbids `OCR_LLM_*` configuration entirely.

## VS Code extension (`extensions/vscode/`)

Confirmed thin wrapper — `CliService.ts` spawns the `ocr` binary via `child_process.spawn` (resolving `PATH` through an interactive login shell to work around GUI-app PATH issues on macOS/Linux), `GitService.ts` shells out to `git` separately. Not MCP-based; parses CLI stdout/JSON rather than reimplementing review logic.

## Distribution channels

npm (`optionalDependencies` platform packages + independent `postinstall` GitHub-Releases download — see `docs/operations/DEPLOYMENT.md` for the unresolved question of which path is authoritative), GitHub Releases (6 platform binaries, Sigstore-attested per `SECURITY.md`), `install.sh`/`install.ps1` (with an explicit integrity caveat when `OCR_GITHUB_MIRROR` is used).

## Known gaps / uncertainties:
- Whether `plugins/open-code-review/claude-code/commands/*.md` bodies literally shell out to `ocr` vs. some other mechanism was not verified by reading the command bodies directly.
- Relationship between `plugins/open-code-review/qca/` (prompt templates) and `internal/delegate` (Go code) at the implementation level — confirmed as a shared *contract* (schema_version "1" JSON), not confirmed as shared *code*.
