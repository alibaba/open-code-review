# Evidence: #1439 `**/test_*.py` zero-item regression (related to #1454)

**Date:** 2026-09-20 09:06 CST (Asia/Shanghai)  
**PR:** https://github.com/alibaba/open-code-review/pull/1462  
**Branch:** `fix/issue-1454-zero-item-selection`  
**Fixed binary commit:** `9b14348` (HEAD of this PR)  
**Broken binary commit:** `a003b93` (parent of the fix; still embeds `**/test_*.py` from #1439 / `ff67fed`)

## Scope of this evidence

This document **locally reproduces** the zero-item skip caused by the #1439 default
exclude `**/test_*.py`, using real `ocr review --preview --format json` output.

It does **not** claim that every fleet-wide failure reported in #1454 is this cause.
The issue reporter has not yet posted preview logs (`total_files` / `reviewable_count` /
per-file `exclude_reason`). Other shapes (empty/missing diff input, allowlist, etc.)
remain possible for those runs.

## Binaries

```bash
# Fixed (this PR HEAD)
cd /path/to/open-code-review   # at 9b14348
go build -o /tmp/oh-lanes/ocr-1454 ./cmd/opencodereview
# sha256: c36bd353e3d8da4e2d026177e173d8acf08b49537536b5f5ebc792c79caa58d0

# Broken (pre-fix main tip that still has **/test_*.py)
git worktree add --detach /tmp/oh-lanes/broken-src a003b93
cd /tmp/oh-lanes/broken-src
go build -o /tmp/oh-lanes/ocr-1454-broken ./cmd/opencodereview
# sha256: 42aa923248a9a62b3c40afb5a87e9eef9cc40c8625220e90d00b654a3926348d
```

Confirm pattern delta:

| Commit   | `**/test_*.py` in `default_exclude_patterns.json` |
|----------|---------------------------------------------------|
| `a003b93` (broken) | **present** |
| `9b14348` (fixed)  | **removed** (keeps `*_test.py` / `**/test/**/*_test.py` / `**/tests/**/*_test.py`) |

## Fixtures

Minimal local git repos under `/tmp/oh-lanes/` (not committed). Each has a BASE commit
with only `README.md`, then a HEAD commit with the listed files.

### Fixture 1 — pytest-prefix **test-only** (primary proof)

Path: `/tmp/oh-lanes/ocr-1454-fixture`  
Changed files BASE→HEAD: **only** `tests/test_utils.py`

```
BASE_SHA=cad23d039b8d6c716cd2bdc8355464b2ed6e6ecc
HEAD_SHA=b2d7c030a0c033b5b13dc619cbd2cefa262078b0
```

### Fixture 2 — ordinary **source-only** (control)

Path: `/tmp/oh-lanes/ocr-1454-fixture-source`  
Changed: `app/handler.py` only

### Fixture 3 — **mixed** source + pytest prefix

Path: `/tmp/oh-lanes/ocr-1454-fixture-mixed`  
Changed: `app/handler.py` + `tests/test_utils.py`

## Commands

```bash
FIXED=/tmp/oh-lanes/ocr-1454
BROKEN=/tmp/oh-lanes/ocr-1454-broken

# Fixture 1 (test-only) — BEFORE / AFTER
"$BROKEN" review --preview --repo /tmp/oh-lanes/ocr-1454-fixture \
  --from cad23d039b8d6c716cd2bdc8355464b2ed6e6ecc \
  --to   b2d7c030a0c033b5b13dc619cbd2cefa262078b0 \
  --format json

"$FIXED"  review --preview --repo /tmp/oh-lanes/ocr-1454-fixture \
  --from cad23d039b8d6c716cd2bdc8355464b2ed6e6ecc \
  --to   b2d7c030a0c033b5b13dc619cbd2cefa262078b0 \
  --format json

# Same pattern for fixture-source and fixture-mixed with their BASE/HEAD SHAs.
```

## Before / after table (preview JSON fields)

| Fixture | Binary | `total_files` | `reviewable_count` | `excluded_count` | Per-file `exclude_reason` |
|---------|--------|---------------|--------------------|------------------|---------------------------|
| test-only `tests/test_utils.py` | **broken** `a003b93` | 1 | **0** | 1 | `tests/test_utils.py` → **`default_path`** |
| test-only `tests/test_utils.py` | **fixed** `9b14348` | 1 | **1** | 0 | (none; `will_review: true`) |
| source-only `app/handler.py` | broken | 1 | 1 | 0 | (none) |
| source-only `app/handler.py` | fixed | 1 | 1 | 0 | (none) |
| mixed | broken | 2 | 1 | 1 | `tests/test_utils.py` → `default_path`; `app/handler.py` reviewable |
| mixed | fixed | 2 | 2 | 0 | both `will_review: true` |

## Key JSON excerpts

### Fixture 1 BEFORE (broken) — proves zero-item skip path

```json
{
  "files": [
    {
      "path": "tests/test_utils.py",
      "status": "added",
      "insertions": 5,
      "deletions": 0,
      "will_review": false,
      "exclude_reason": "default_path"
    }
  ],
  "total_insertions": 5,
  "total_deletions": 0,
  "total_files": 1,
  "reviewable_count": 0,
  "excluded_count": 1
}
```

With `reviewable_count: 0`, a full `ocr review` exits 0 with
`Review skipped: no items were selected` — the symptom in #1454 / Qiyuan’s
local test-only reproduction.

### Fixture 1 AFTER (fixed) — selection restored

```json
{
  "files": [
    {
      "path": "tests/test_utils.py",
      "status": "added",
      "insertions": 5,
      "deletions": 0,
      "will_review": true
    }
  ],
  "total_insertions": 5,
  "total_deletions": 0,
  "total_files": 1,
  "reviewable_count": 1,
  "excluded_count": 0
}
```

### Fixture 3 BEFORE (broken) — mixed still reviews source, drops pytest prefix

```json
{
  "files": [
    {
      "path": "app/handler.py",
      "status": "added",
      "insertions": 2,
      "deletions": 0,
      "will_review": true
    },
    {
      "path": "tests/test_utils.py",
      "status": "added",
      "insertions": 2,
      "deletions": 0,
      "will_review": false,
      "exclude_reason": "default_path"
    }
  ],
  "total_files": 2,
  "reviewable_count": 1,
  "excluded_count": 1
}
```

## Unit tests

```bash
go test ./internal/agent/... ./internal/config/allowlist/... ./internal/scan/...
```

Result (2026-09-20 CST):

```
ok  github.com/alibaba/open-code-review/internal/agent   3.711s
ok  github.com/alibaba/open-code-review/internal/config/allowlist  (cached)
ok  github.com/alibaba/open-code-review/internal/scan    0.658s
```

Includes `TestSelectFiles_PythonPRShapes` (pytest prefix stays selected; `*_test.py`
suffix still `default_path`).

## Conclusion

1. On binaries that still embed `**/test_*.py` (#1439), a non-empty **pytest-prefix-only**
   diff selects **zero** items; preview shows `exclude_reason: "default_path"`.
2. After this PR removes that pattern, the same fixture selects the file
   (`reviewable_count: 1`, `will_review: true`).
3. Ordinary source-only PRs are unaffected either side.
4. This is solid proof for the **#1439 regression**. Fleet-wide #1454 still needs
   reporter `ocr review --preview` logs for other failure shapes.
