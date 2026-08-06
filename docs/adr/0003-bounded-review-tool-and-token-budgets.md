---
status: superseded
superseded_by: 0004-unlimited-default-aggregate-token-budget.md
---

# Bounded review tool and token budgets

Diff reviews use 75 tool-request rounds per file and full-file scans use 150, increasing the existing defaults by 2.5x for low-cost models such as `gpt luna`. When `--max-tokens-budget` is omitted, each run uses 2.5 times the estimated aggregate token cost; an explicit positive value overrides that default and an explicit `0` preserves unlimited behavior. Per-file timeouts remain unchanged, and all models share these defaults while timeout failures receive distinct result and telemetry classifications.
