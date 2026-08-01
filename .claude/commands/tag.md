# /tag

Create an annotated git tag with an auto-generated summary of changes since the last tag.

## Arguments

- `$ARGUMENTS`: Optional version number (e.g., `v1.2.0`). If omitted, auto-increment the patch version from the latest tag.

## Workflow

### Step 1: Determine version

1. Get the latest tag: `git describe --tags --abbrev=0`
2. If `$ARGUMENTS` is provided and non-empty, use it as the new version.
3. Otherwise, auto-increment the patch number (e.g., `v1.1.5` → `v1.1.6`).

### Step 2: Generate summary

1. Get the commit range: from the latest tag to HEAD.
2. Run `git log <latest-tag>..HEAD --oneline` to list all commits in this range.
3. Summarize the changes into a brief tag message (1-3 lines, similar to a commit message style). Focus on what was implemented/fixed, not individual commits.

### Step 3: Create tag

Run:

```bash
git tag -s <new-version> -m "<summary>"
```

Report the created tag and its message to the user.
