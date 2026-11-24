// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.providers

import com.alibaba.opencodereview.idea.model.SupportedLocale
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

/** Mirror the inputs and expected outputs of the corresponding commentAnchor tests to preserve equivalent coverage. */
class CommentAnchorTest {

    private val content = listOf("line1", "for (let i = 0; i <= 30, i++) {", "  console.log(i);", "}", "line5")
        .joinToString("\n")

    @Test
    fun `normalizeLine removes diff markers`() {
        assertEquals("added", normalizeLine("+added"))
        assertEquals("removed", normalizeLine("-removed"))
    }

    @Test
    fun `splitAndNormalize skips blank lines`() {
        assertEquals(listOf("a", "b"), splitAndNormalize("a\n\n b "))
    }

    @Test
    fun `resolveLinesInContent uses in-range line numbers directly`() {
        val result = resolveLinesInContent(content, 2, 2, null)
        assertEquals(ResolvedLines(2, 2, relocated = false), result)
    }

    @Test
    fun `resolveLinesInContent relocates out-of-range lines using existingCode`() {
        val code = "for (let i = 0; i <= 30, i++) {"
        val result = resolveLinesInContent(content, 99, 99, code)
        assertEquals(ResolvedLines(2, 2, relocated = true), result)
    }

    @Test
    fun `findLinesByExistingCode matches consecutive non-empty lines`() {
        val found = findLinesByExistingCode(content, "console.log(i);")
        assertEquals(LineSpan(3, 3), found)
    }

    @Test
    fun `returns null when neither line numbers nor existingCode can be resolved`() {
        assertNull(resolveLinesInContent(content, 99, 99, null))
        assertNull(resolveLinesInContent(content, 0, 0, null))
    }

    @Test
    fun `multiline existingCode uses a sliding window over the entire block`() {
        val result = resolveLinesInContent(content, 99, 99, "for (let i = 0; i <= 30, i++) {\n  console.log(i);")
        assertEquals(ResolvedLines(2, 3, relocated = true), result)
    }

    @Test
    fun `CRLF line endings do not affect matching`() {
        val crlfContent = content.replace("\n", "\r\n")
        val found = findLinesByExistingCode(crlfContent, "console.log(i);")
        assertEquals(LineSpan(3, 3), found)
    }

    @Test
    fun `formatLocateNote names the original line when relocated`() {
        assertEquals(
            "⚠ 原本第 99 行没能匹配上，改为显示第 2 行。", // allow-non-english: assertion verifies Chinese UI translations
            formatLocateNote(99, 2, SupportedLocale.ZH_CN),
        )
        assertEquals(
            "⚠ Line 99 could not be matched, showing line 2 instead.",
            formatLocateNote(99, 2, SupportedLocale.EN),
        )
    }

    @Test
    fun `formatLocateNote uses a generic explanation without an original line`() {
        assertEquals("⚠ 行号是根据代码内容重新定位的。", formatLocateNote(0, 2, SupportedLocale.ZH_CN)) // allow-non-english: assertion verifies Chinese UI translations
        assertEquals(
            "⚠ Line number was relocated based on code content.",
            formatLocateNote(0, 2, SupportedLocale.EN),
        )
    }

    // ---------------------------------------------------------------- Diff-side mounting

    private val marks = listOf(
        DiffMark(0, "a.kt", AnchorSide.RIGHT, 10, 12),
        DiffMark(1, "a.kt", AnchorSide.LEFT, 20, 20),
        DiffMark(2, "b.kt", AnchorSide.RIGHT, 30, 35),
    )

    @Test
    fun `selectDiffMarks selects only matching paths and sides`() {
        assertEquals(listOf(marks[0]), selectDiffMarks("a.kt", AnchorSide.RIGHT, marks))
        assertEquals(listOf(marks[1]), selectDiffMarks("a.kt", AnchorSide.LEFT, marks))
        assertEquals(listOf(marks[2]), selectDiffMarks("b.kt", AnchorSide.RIGHT, marks))
    }

    @Test
    fun `selectDiffMarks excludes the wrong side`() {
        // A diff exposes both documents; ignoring the side would attach an icon on each side for the same comment.
        assertEquals(emptyList(), selectDiffMarks("b.kt", AnchorSide.LEFT, marks))
        assertEquals(emptyList(), selectDiffMarks("c.kt", AnchorSide.RIGHT, marks))
    }

    @Test
    fun `selectDiffMarks preserves the full range`() {
        // Regression coverage: DiffMark originally had only a single line field, so multiline comments
        // (over 60% of observed CLI output) highlighted only their starting line.
        val picked = selectDiffMarks("b.kt", AnchorSide.RIGHT, marks).single()
        assertEquals(30, picked.startLine)
        assertEquals(35, picked.endLine)
    }

    @Test
    fun `clampLineRange converts a 1-based inclusive range to 0-based`() {
        assertEquals(0..0, clampLineRange(1, 1, 5))
        assertEquals(0..4, clampLineRange(1, 5, 5))
        assertEquals(4..4, clampLineRange(5, 5, 5))
    }

    @Test
    fun `clampLineRange preserves multiline ranges`() {
        // This property lacked coverage, allowing diff mode to lose ending lines unnoticed.
        assertEquals(9..11, clampLineRange(10, 12, 100))
    }

    @Test
    fun `clampLineRange handles out-of-bounds input without throwing`() {
        // The left side of an added file and the right side of a deleted file are empty, changing the line count.
        // Without clamping, getLineStartOffset throws IndexOutOfBounds and prevents the diff from opening.
        assertEquals(4..4, clampLineRange(999, 1000, 5))
        assertEquals(0..4, clampLineRange(0, 999, 5))
        assertEquals(0..0, clampLineRange(-3, -1, 5))
    }

    @Test
    fun `clampLineRange preserves the start when only the end is out of bounds`() {
        // If only the ending lines were deleted, the starting line must not be discarded too.
        assertEquals(2..4, clampLineRange(3, 99, 5))
    }

    @Test
    fun `clampLineRange reduces reversed input to a single line`() {
        // The CLI occasionally emits end < start. Clamping must ensure start <= end,
        // otherwise addRangeHighlighter throws on a reversed range.
        assertEquals(4..4, clampLineRange(5, 2, 10))
    }

    @Test
    fun `clampLineRange does not throw for an empty document`() {
        assertEquals(0..0, clampLineRange(7, 9, 0))
    }
}
