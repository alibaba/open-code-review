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
- **Extended Language Support**: Native rule resolution provides 35 built-in rulesets (covering Java, TS/JS, Go, Python, C/C++, Rust, Kotlin, PHP, Julia, Terraform, Bicep, Nix, Haskell, Nim, ArkTS, Astro, Protobuf, GraphQL, Prisma, FreeMarker, manifest files, etc.) and natively supports code reviews across 88 allowed file extensions including Swift, MATLAB, Objective-C (`.m`), R, Jupyter Notebook (`.ipynb`), Zig, Elm, Thrift, and Cap'n Proto.

## Debugging Rule Matching

```bash
ocr rules check src/main/java/com/example/Foo.java
ocr rules check --rule /path/to/custom_rule.json src/main/java/com/example/Foo.java
```

Displays effective rules, source hierarchy (Custom/Project/Global/System), and matched patterns. Supports `--repo <path>` and `--rule <path>`.

## Behavioral Details

- **Case-insensitive matching**: `path` patterns and matched file paths are compared in lowercase.
- **include/exclude take effect at a single level**: only the highest-priority layer that configures include/exclude is used (Custom > Project > Global); layers are not merged.
- **File references are heuristic**: only a value that is a single line, contains no spaces, and has a supported extension (`.md`/`.txt`/`.markdown`) is treated as a file path; inline text containing spaces is treated as inline. Project-level rules resolve relative to the repository root (cannot escape it); custom rules (`--rule`) and global rules resolve relative to the directory containing the `rule.json` file. On read failure (missing/oversized/unsupported extension), the rule is emptied with a WARNING rather than an error.
