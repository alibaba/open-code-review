---
title: Native Plugins
sidebar:
  order: 8
---

Native plugins package additional review rules in a local directory. OCR copies
the directory into a user-wide snapshot; enabled plugins contribute to both
`ocr review` and `ocr scan` across repositories. Package files are read as data.

## Install and inspect

From an OCR source checkout, install the bundled example:

```bash
ocr plugin install ./examples/ocr-plugins/review-checklist
ocr plugin list
ocr plugin show review-checklist
ocr plugin enable review-checklist
ocr rules check src/main.go
```

Installation defaults to disabled. Add `--enable` to install and enable in one
command. `ocr rules check` prints each matching plugin ID, version, rule-pack
path and pattern, followed by the combined rule text. All plugin subcommands
accept `--json`, for example `ocr plugin list --json`.

Inspect rule text before enabling: it becomes part of the model's review prompt.
The install source must be a local directory containing `ocr-plugin.json`.
URLs, GitHub repository references, and ZIP or tar.gz archives are not accepted.

## Package format

The example has three files:

```text
review-checklist/
  ocr-plugin.json
  rules.json
  checklist.md
```

`ocr-plugin.json`:

```json
{
  "schema_version": 1,
  "id": "review-checklist",
  "name": "Review Checklist",
  "version": "1.0.0",
  "description": "Additional review checks for error handling and resource ownership.",
  "rules": ["rules.json"],
  "requires": {"features": ["rules"]}
}
```

| Field | Contract |
|---|---|
| `schema_version` | Required; must be `1`. |
| `id` | Required; 1–64 lowercase ASCII letters, digits, `.`, `_`, `-`; starts and ends with a letter or digit. Windows reserved names are rejected. |
| `name`, `version` | Required, nonblank strings. |
| `description` | Optional description shown by `ocr plugin show`. |
| `rules` | Required, nonempty ordered list of distinct package-relative `.json` paths. |
| `requires.ocr_min_version` | Optional full semantic version, for example `1.0.0`; checked against the OCR build version. |
| `requires.features` | Optional list of required capabilities; the supported value is `rules`. |
| `metadata` | Optional JSON object retained as opaque data. |

Unknown manifest fields and duplicate JSON keys are rejected. Compatibility is
checked at install, enable, and when loading enabled plugins. A `dev` build cannot satisfy
`ocr_min_version`; omit that requirement for packages used with development builds.

`rules.json`:

```json
{
  "rules": [
    {
      "path": "**/*.{go,js,mjs,ts,tsx,py}",
      "rule": "checklist.md"
    }
  ]
}
```

`checklist.md` contains the review instructions:

```text
Check error propagation, cleanup, and resource ownership.
Report only concrete failure paths in the changed code.
Verify the relevant caller and explain the trigger and user-visible consequence.
```

`rule` also accepts inline text. A Markdown reference is resolved relative to its
rule-pack file and must remain inside the package. Use forward slashes in paths.
Rule packs accept review rules; configure `include` and `exclude` in project or
user rules instead. Nonempty plugin `include` or `exclude` lists are rejected.

## How rules combine

OCR first resolves the existing priority chain: `--rule`, project, global, then
system rules. It then appends enabled plugins in ascending plugin-ID order and
each manifest's `rules` order. Within each rule pack, the first matching entry
contributes. Every matching pack can therefore add instructions for the same file.

Plugin rules are always additive. `merge_system_rule` has no effect in a plugin
pack. Existing file filters still determine which files reach review. See
[Review Rules](../review-rules/) for patterns, filtering and the base priority chain.

## Disable, replace and remove

```bash
ocr plugin disable review-checklist
ocr plugin remove review-checklist
ocr plugin install ./examples/ocr-plugins/review-checklist --enable
```

Remove an installed ID before installing its replacement. Editing the original
source directory takes effect after reinstalling. State is recorded in
`~/.opencodereview/plugins/state.json`; snapshots live under `plugins/packages/`.
Remove unregisters the plugin and retains snapshots for reviews already running.

Resume identity includes enabled manifests and package content. Reinstalling
identical content preserves that identity; changing content invalidates reuse
of results from the previous plugin configuration.

Packages are limited to 64 MiB and 4,096 entries, including directories. The
manifest limit is 1 MiB; each rule-pack JSON is limited to 512 KiB when loaded.
Symlinks, special files, case-conflicting names, Windows reserved filenames and
paths escaping the package are rejected. Keep snapshots unchanged and reinstall
from the source directory to apply edits.

See [CLI Reference](../cli-reference/#ocr-plugin) for the command summary.
