---
status: accepted
---

# Bilingual skill docs use locale-suffixed sibling files

The repository carries Traditional Chinese skill documentation alongside the
English source so the runtime instructions remain stable while readers can use
the locale they prefer.

## Decision

Keep English instructions in `SKILL.md` as the source of truth and add the
Traditional Chinese translation as `SKILL.zh.md`. Copy `name:` byte-for-byte,
translate `description:`, and leave commands, paths, URLs, and code unchanged.

## Consequences

- Translating a skill does not change its invocation key or runtime behavior.
- Structural changes to `SKILL.md` require the `.zh.md` sibling to be updated.
- The translation uses standard written Traditional Chinese and excludes
  Cantonese colloquial markers.
