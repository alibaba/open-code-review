---
status: accepted
---

# Unlimited default aggregate token budget

The aggregate token budget is unlimited when `--max-tokens-budget` is omitted for both diff reviews and full-file scans. A positive value remains an explicit cap, while an explicit `0` also means unlimited. The 75/150 per-file tool-request round defaults, per-file timeouts, provider request limits, and MCP idle watchdog base safety behavior remain in place; request-period pause behavior is defined by [ADR 0007](0007-pause-idle-watchdog-during-llm-requests.md). The fixed whole-review MCP maximum duration is removed.
