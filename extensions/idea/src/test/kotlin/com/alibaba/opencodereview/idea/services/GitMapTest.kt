// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.services

import com.alibaba.opencodereview.idea.model.FileChange
import com.alibaba.opencodereview.idea.model.FileStatus
import com.alibaba.opencodereview.idea.model.SupportedLocale
import kotlin.test.Test
import kotlin.test.assertEquals

class GitMapTest {

    // ------------------------------------------------------------ mapStatusCode

    @Test
    fun `status mapping covers the five Git codes`() {
        assertEquals(FileStatus.ADDED, mapStatusCode('A'))
        assertEquals(FileStatus.ADDED, mapStatusCode('?'))
        assertEquals(FileStatus.DELETED, mapStatusCode('D'))
        assertEquals(FileStatus.RENAMED, mapStatusCode('R'))
        assertEquals(FileStatus.MODIFIED, mapStatusCode('M'))
    }

    @Test
    fun `unknown status codes fall back to modified`() {
        // Treat Git C (copied), T (type change), and U (unmerged) codes as modified.
        assertEquals(FileStatus.MODIFIED, mapStatusCode('C'))
        assertEquals(FileStatus.MODIFIED, mapStatusCode('T'))
        assertEquals(FileStatus.MODIFIED, mapStatusCode('X'))
    }

    // ------------------------------------------------------------ parseNameStatus

    @Test
    fun `parses tab-separated name-status output`() {
        val out = "M\tsrc/a.kt\nA\tsrc/b.kt\nD\tsrc/c.kt\n"
        assertEquals(
            listOf(
                FileChange("src/a.kt", FileStatus.MODIFIED),
                FileChange("src/b.kt", FileStatus.ADDED),
                FileChange("src/c.kt", FileStatus.DELETED),
            ),
            parseNameStatus(out),
        )
    }

    @Test
    fun `rename lines use the new path`() {
        // R<score> is followed by old and new paths; use the post-change location.
        val out = "R100\tsrc/old.kt\tsrc/new.kt\n"
        assertEquals(listOf(FileChange("src/new.kt", FileStatus.RENAMED)), parseNameStatus(out))
    }

    @Test
    fun `name-status ignores blank lines and lines with missing columns`() {
        val out = "\nM\tsrc/a.kt\n\nonlyonecolumn\n\n"
        assertEquals(listOf(FileChange("src/a.kt", FileStatus.MODIFIED)), parseNameStatus(out))
    }

    @Test
    fun `name-status deduplicates paths and preserves the first occurrence`() {
        val out = "M\tsrc/a.kt\nD\tsrc/a.kt\n"
        assertEquals(listOf(FileChange("src/a.kt", FileStatus.MODIFIED)), parseNameStatus(out))
    }

    // ------------------------------------------------------------ parsePorcelain

    @Test
    fun `porcelain prefers index status over working tree status`() {
        // "AM" is a staged addition modified again afterward; classify it as added.
        assertEquals(
            listOf(FileChange("src/a.kt", FileStatus.ADDED)),
            parsePorcelain("AM src/a.kt\n"),
        )
    }

    @Test
    fun `porcelain uses working tree status when index status is empty`() {
        assertEquals(
            listOf(FileChange("src/a.kt", FileStatus.MODIFIED)),
            parsePorcelain(" M src/a.kt\n"),
        )
    }

    @Test
    fun `porcelain double question marks mean untracked files`() {
        assertEquals(
            listOf(FileChange("src/new.kt", FileStatus.ADDED)),
            parsePorcelain("?? src/new.kt\n"),
        )
    }

    @Test
    fun `porcelain renames use the path after the arrow`() {
        assertEquals(
            listOf(FileChange("b.kt", FileStatus.RENAMED)),
            parsePorcelain("R  a.kt -> b.kt\n"),
        )
    }

    @Test
    fun `porcelain renames use the new path with quoted spaces`() {
        assertEquals(
            listOf(FileChange("your file.kt", FileStatus.RENAMED)),
            parsePorcelain("R  \"my file.kt\" -> \"your file.kt\"\n"),
        )
    }

    @Test
    fun `porcelain renames do not confuse arrows inside filenames`() {
        // Git quotes "a -> b.kt" because it contains spaces and an arrow; result.kt is unquoted.
        // The old indexOf(" -> ") matched the arrow inside the filename and selected the wrong path.
        // Prefer `" -> ` (closing quote and arrow) to locate the actual separator.
        assertEquals(
            listOf(FileChange("result.kt", FileStatus.RENAMED)),
            parsePorcelain("R  \"a -> b.kt\" -> result.kt\n"),
        )
    }

    @Test
    fun `porcelain renames handle unquoted arrows without spaces`() {
        // Git does not quote x->y.kt, which has no spaces; ` -> ` cannot occur inside that filename.
        assertEquals(
            listOf(FileChange("z.kt", FileStatus.RENAMED)),
            parsePorcelain("R  x->y.kt -> z.kt\n"),
        )
    }

    @Test
    fun `porcelain renames handle an unquoted old path and quoted new path`() {
        // The old path has no special characters and is unquoted; the new path has spaces and is quoted.
        // Use the ` -> ` fallback, which cannot match inside the old path, and unquoteGitPath removes the new path quotes.
        assertEquals(
            listOf(FileChange("new name.kt", FileStatus.RENAMED)),
            parsePorcelain("R  normal.kt -> \"new name.kt\"\n"),
        )
    }

    @Test
    fun `porcelain ignores blank and too-short lines`() {
        assertEquals(emptyList(), parsePorcelain("\nM\n  \n"))
    }

    // ------------------------------------------------------------ Untracked files and merging

    @Test
    fun `parses untracked paths and removes blank lines`() {
        assertEquals(
            listOf("a.kt", "dir/b.kt"),
            parseUntrackedList("a.kt\n\ndir/b.kt\n\n"),
        )
    }

    @Test
    fun `merging prefers tracked status over untracked added status`() {
        val merged = mergeWorkspaceFiles(
            tracked = listOf(FileChange("a.kt", FileStatus.DELETED)),
            untrackedPaths = listOf("a.kt", "b.kt"),
        )
        assertEquals(
            listOf(FileChange("a.kt", FileStatus.DELETED), FileChange("b.kt", FileStatus.ADDED)),
            merged,
        )
    }

    // ------------------------------------------------------------ buildWorkspaceFiles

    @Test
    fun `workspace files prefer diff HEAD results`() {
        val files = buildWorkspaceFiles(
            diffHeadOut = "M\ta.kt\n",
            diffCachedOut = "A\tshould-be-ignored.kt\n",
            untrackedOut = "c.kt\n",
        )
        assertEquals(
            listOf(FileChange("a.kt", FileStatus.MODIFIED), FileChange("c.kt", FileStatus.ADDED)),
            files,
        )
    }

    @Test
    fun `empty diff HEAD falls back to staged changes`() {
        // Before the first commit, HEAD does not exist; only --cached can be inspected.
        val files = buildWorkspaceFiles(
            diffHeadOut = "",
            diffCachedOut = "A\ta.kt\n",
            untrackedOut = "b.kt\n",
        )
        assertEquals(
            listOf(FileChange("a.kt", FileStatus.ADDED), FileChange("b.kt", FileStatus.ADDED)),
            files,
        )
    }

    // ------------------------------------------------------------ parseBranchList

    @Test
    fun `branch lists trim refs prefixes and preserve remote HEAD`() {
        // origin/HEAD is a symbolic ref to the remote default branch. Selecting it means
        // comparing against that default branch; it is valid and must not be filtered out.
        val out = """
            refs/heads/main
            refs/remotes/origin/HEAD
            refs/remotes/origin/dependabot/npm_and_yarn/x
            refs/remotes/origin/main
        """.trimIndent()
        assertEquals(
            listOf("main", "origin/HEAD", "origin/dependabot/npm_and_yarn/x", "origin/main"),
            parseBranchList(out),
        )
    }

    @Test
    fun `branch lists deduplicate and ignore blank lines`() {
        val out = "refs/heads/main\n\nrefs/heads/main\n  \n"
        assertEquals(listOf("main"), parseBranchList(out))
    }

    @Test
    fun `branch lists skip non-branch refs such as tags`() {
        val out = "refs/heads/main\nrefs/tags/v1.0\nrefs/stash\n"
        assertEquals(listOf("main"), parseBranchList(out))
    }

    @Test
    fun `bare HEAD is not a branch`() {
        assertEquals(emptyList(), parseBranchList("HEAD\n"))
    }

    // ------------------------------------------------------------ branchRefCandidates

    @Test
    fun `bare branch names gain an origin candidate`() {
        assertEquals(listOf("feature/x"), branchRefCandidates("feature/x"))
        assertEquals(listOf("dev", "origin/dev"), branchRefCandidates("dev"))
    }

    @Test
    fun `main and master are mutual fallback candidates`() {
        assertEquals(
            listOf("main", "origin/main", "master", "origin/master"),
            branchRefCandidates("main"),
        )
        assertEquals(
            listOf("master", "origin/master", "main", "origin/main"),
            branchRefCandidates("master"),
        )
    }

    // ------------------------------------------------------------ unquoteGitPath

    @Test
    fun `unquoted paths are returned unchanged`() {
        assertEquals("src/a.kt", unquoteGitPath("src/a.kt"))
    }

    @Test
    fun `octal escapes decode to Chinese paths`() {
        // U+4E2D encodes as UTF-8 E4 B8 AD, or octal \344\270\255.
        assertEquals("中", unquoteGitPath("\"\\344\\270\\255\"")) // allow-non-english: fixture verifies UTF-8 Git path decoding
    }

    @Test
    fun `common backslash escapes are decoded`() {
        assertEquals("a\"b", unquoteGitPath("\"a\\\"b\""))
        assertEquals("a\\b", unquoteGitPath("\"a\\\\b\""))
        assertEquals("a\tb", unquoteGitPath("\"a\\tb\""))
    }

    @Test
    fun `invalid octal does not throw`() {
        // JavaScript parseInt("899", 8) returns NaN, whereas Kotlin toInt(8) throws.
        // GitMap therefore accepts only digits 0-7; keep the no-throw guarantee covered here.
        unquoteGitPath("\"\\899\"")
        unquoteGitPath("\"\\9\"")
        unquoteGitPath("\"\\\"")
    }

    // ------------------------------------------------------------ formatRelative

    @Test
    fun `relative time covers every interval`() {
        val now = 1_700_000_000_000L
        fun ago(ms: Long) = formatRelative((now - ms) / 1000, now, SupportedLocale.ZH_CN)

        assertEquals("刚刚", ago(30_000)) // allow-non-english: assertion verifies Chinese UI translations
        assertEquals("5 分钟前", ago(5 * 60_000)) // allow-non-english: assertion verifies Chinese UI translations
        assertEquals("1 小时前", ago(3_600_000)) // allow-non-english: assertion verifies Chinese UI translations
        assertEquals("3 小时前", ago(3 * 3_600_000)) // allow-non-english: assertion verifies Chinese UI translations
        assertEquals("昨天", ago(25 * 3_600_000)) // allow-non-english: assertion verifies Chinese UI translations
        assertEquals("5 天前", ago(5 * 86_400_000L)) // allow-non-english: assertion verifies Chinese UI translations
        assertEquals("2 个月前", ago(70 * 86_400_000L)) // allow-non-english: assertion verifies Chinese UI translations
        assertEquals("2 年前", ago(800 * 86_400_000L)) // allow-non-english: assertion verifies Chinese UI translations
    }

    @Test
    fun `relative time follows the locale`() {
        // Use English for the same intervals to verify that both host i18n dictionaries are complete.
        val now = 1_700_000_000_000L
        fun ago(ms: Long) = formatRelative((now - ms) / 1000, now, SupportedLocale.EN)

        assertEquals("just now", ago(30_000))
        assertEquals("5 minutes ago", ago(5 * 60_000))
        assertEquals("1 hour ago", ago(3_600_000))
        assertEquals("3 hours ago", ago(3 * 3_600_000))
        assertEquals("yesterday", ago(25 * 3_600_000))
        assertEquals("5 days ago", ago(5 * 86_400_000L))
        assertEquals("2 months ago", ago(70 * 86_400_000L))
        assertEquals("2 years ago", ago(800 * 86_400_000L))
    }

    @Test
    fun `missing or invalid timestamps return an empty string`() {
        assertEquals("", formatRelative(null, 1_700_000_000_000L, SupportedLocale.ZH_CN))
        assertEquals("", formatRelative(0, 1_700_000_000_000L, SupportedLocale.ZH_CN))
        assertEquals("", formatRelative(-1, 1_700_000_000_000L, SupportedLocale.ZH_CN))
    }

    @Test
    fun `future timestamps are treated as just now`() {
        // Clock drift or modified commit times must not produce negative values.
        assertEquals("刚刚", formatRelative(1_700_000_060L, 1_700_000_000_000L, SupportedLocale.ZH_CN)) // allow-non-english: assertion verifies Chinese UI translations
    }

    // ------------------------------------------------------------ pinDefaultBranches

    @Test
    fun `origin HEAD and the default branch move to second and third while preserving other order`() {
        val branches = listOf("master", "origin/368-x", "origin/Avasam", "origin/HEAD", "origin/add-once", "origin/master")
        assertEquals(
            listOf("master", "origin/HEAD", "origin/master", "origin/368-x", "origin/Avasam", "origin/add-once"),
            pinDefaultBranches(branches, "origin/master"),
        )
    }

    @Test
    fun `prioritization also works when the default branch is main`() {
        val branches = listOf("main", "origin/HEAD", "origin/chore", "origin/main", "origin/feat")
        assertEquals(
            listOf("main", "origin/HEAD", "origin/main", "origin/chore", "origin/feat"),
            pinDefaultBranches(branches, "origin/main"),
        )
    }

    @Test
    fun `without origin HEAD the original order is preserved`() {
        val branches = listOf("master", "origin/develop", "origin/feature")
        assertEquals(branches, pinDefaultBranches(branches, null))
    }

    @Test
    fun `without the default remote branch the original order is preserved`() {
        val branches = listOf("master", "origin/develop")
        assertEquals(branches, pinDefaultBranches(branches, "origin/master"))
    }
}
