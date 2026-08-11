# Custom Review Rules

## Resolution Priority (High → Low)

1. `--rule <path>` flag
2. `<repo>/.opencodereview/rule.json` (Project-level)
3. `~/.opencodereview/rule.json` (Global-level)
4. Built-in system default rules

## Rule File Format

```json
{
  "rules": [
    {
      "path": "**/*.java",
      "rule": "All new methods must validate required parameters for null",
      "merge_system_rule": true
    },
    {
      "path": "**/*.ts",
      "rule": "rules/typescript-security.md"
    }
  ],
  "include": ["src/**"],
  "exclude": ["**/*.gen.*", "vendor/**"]
}
```

- `rule` value: inline string OR file reference (`.md`, `.txt`, `.markdown`; max 512 KB)
- `merge_system_rule: true`: merges with matching system rules (instead of replacing them)
- `include`/`exclude`: top-level file filtering (gitignore-style glob, supports `**` and `{a,b}`)

## Debugging Rule Matching

```bash
ocr rules check src/main/java/com/example/Foo.java
```

Displays effective rules, source hierarchy (Custom/Project/Global/System), and matched patterns.
