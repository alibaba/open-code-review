---
status: accepted
---

# Unlimited default aggregate token budget

The aggregate token budget is unlimited when `--max-tokens-budget` is omitted for both diff reviews and full-file scans. A positive value remains an explicit cap, while an explicit `0` also means unlimited. The 75/150 per-file tool-request round defaults, per-file timeouts, provider request limits, and MCP idle watchdog remain unchanged; the fixed whole-review MCP maximum duration is removed.
