# Domain Docs

How the engineering skills should consume this repo's domain documentation when exploring the codebase.

## Before exploring, read these

- **`CONTEXT.md`** at the repo root.
- **`CONTEXT-MAP.md`** at the repo root if it exists; read each linked `CONTEXT.md` relevant to the topic.
- **`docs/adr/`**; read ADRs that touch the area you're about to work in.

If any of these files don't exist, proceed silently. The `/domain-modeling` skill creates them lazily when terms or decisions get resolved.

## File structure

This repository uses a single-context layout:

```text
/
├── CONTEXT.md
├── docs/adr/
└── src/
```

## Use the glossary's vocabulary

When output names a domain concept, use the term defined in `CONTEXT.md`. Don't drift to synonyms the glossary explicitly avoids.

If a needed concept isn't in the glossary, reconsider whether it belongs to the project or note the gap for `/domain-modeling`.

## Flag ADR conflicts

If output contradicts an existing ADR, surface the conflict explicitly instead of silently overriding it.
