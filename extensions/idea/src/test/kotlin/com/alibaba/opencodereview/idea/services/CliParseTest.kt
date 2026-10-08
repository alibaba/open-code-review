// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.services

import com.alibaba.opencodereview.idea.model.CliRunOptions
import com.alibaba.opencodereview.idea.model.LogLevel
import com.alibaba.opencodereview.idea.model.ReviewMode
import com.alibaba.opencodereview.idea.model.ReviewState
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNull
import kotlin.test.assertTrue

class CliParseTest {

    @Test
    fun `unrelated JSON cannot become a clean review`() {
        for (output in listOf("{}", """{"event":"finished"}""", """{"status":"success"}""")) {
            assertFailsWith<IllegalArgumentException>(output) { parseCliResult(output) }
        }
    }

    @Test
    fun `trailing JSON does not replace actual findings`() {
        val output = """{"status":"success","comments":[{"path":"a.kt","content":"A finding"}]}""" +
            "\n" + """{"event":"finished"}"""
        assertEquals("A finding", parseCliResult(output).comments.single().content)
    }

    @Test
    fun `malformed review cannot fall back to a trailing log object`() {
        val output = """{"status":"success","comments":"broken"}""" + "\n{}"
        assertFailsWith<IllegalArgumentException> { parseCliResult(output) }
    }

    @Test
    fun `multiple review results are rejected as ambiguous`() {
        val result = """{"status":"success","comments":[]}"""
        assertFailsWith<IllegalArgumentException> { parseCliResult("$result\n$result") }
    }

    @Test
    fun `JSON string braces and escapes do not split results`() {
        val output = """{"event":"starting"}
            {"status":"complete","comments":[{"path":"a.kt","content":"Use {x} and \"quoted\" text"}]}
            {"event":"finished"}
        """.trimIndent()
        assertEquals("Use {x} and \"quoted\" text", parseCliResult(output).comments.single().content)
    }

    @Test
    fun `null comments from Go are a valid empty list`() {
        assertEquals(emptyList(), parseCliResult("""{"status":"complete","comments":null}""").comments)
    }

    @Test
    fun `invalid status and malformed comments are rejected`() {
        for (output in listOf(
            """{"status":"","comments":[]}""",
            """{"status":"future_status","comments":[]}""",
            """{"status":42,"comments":[]}""",
            """{"status":"success","comments":[{}]}""",
            """{"status":"success","comments":[{"path":"a.kt","content":""}]}""",
            """[{"status":"success","comments":[]}]""",
            """{"status":"success","summary":{"files_reviewed":1}""",
        )) {
            assertFailsWith<IllegalArgumentException>(output) { parseCliResult(output) }
        }
    }

    @Test
    fun `workspace mode has no ref arguments`() {
        assertEquals(
            listOf("review", "--format", "json"),
            buildReviewArgs(CliRunOptions(mode = ReviewMode.WORKSPACE)),
        )
    }

    @Test
    fun `branch mode includes from and to`() {
        assertEquals(
            listOf("review", "--from", "main", "--to", "dev", "--format", "json"),
            buildReviewArgs(CliRunOptions(mode = ReviewMode.BRANCH, from = "main", to = "dev")),
        )
    }

    @Test
    fun `commit mode includes commit`() {
        assertEquals(
            listOf("review", "--commit", "abc1234", "--format", "json"),
            buildReviewArgs(CliRunOptions(mode = ReviewMode.COMMIT, commit = "abc1234")),
        )
    }

    @Test
    fun `blank from and customPrompt are omitted from arguments`() {
        val args = buildReviewArgs(
            CliRunOptions(mode = ReviewMode.BRANCH, from = "   ", to = "dev", customPrompt = "  "),
        )
        assertEquals(listOf("review", "--to", "dev", "--format", "json"), args)
    }

    @Test
    fun `customPrompt and concurrency are appended at the end`() {
        val args = buildReviewArgs(
            CliRunOptions(mode = ReviewMode.WORKSPACE, customPrompt = "  只看安全问题  ", concurrency = 4), // allow-non-english: fixture verifies Unicode CLI input and output
        )
        assertEquals(
            listOf("review", "--format", "json", "--background", "只看安全问题", "--concurrency", "4"), // allow-non-english: fixture verifies Unicode CLI input and output
            args,
        )
    }

    @Test
    fun `snake_case CLI output becomes a camelCase domain model`() {
        val content = "问题描述" // allow-non-english: fixture verifies Unicode CLI output
        val thinking = "推理过程" // allow-non-english: fixture verifies Unicode CLI output
        val result = parseCliResult(
            """
            {
              "status": "success",
              "comments": [
                {
                  "path": "src/a.kt",
                  "content": "$content",
                  "suggestion_code": "val a = 1",
                  "existing_code": "val a = 2",
                  "start_line": 10,
                  "end_line": 12,
                  "thinking": "$thinking"
                }
              ],
              "warnings": [{ "type": "skip", "file": "b.bin", "message": "binary" }],
              "summary": {
                "files_reviewed": 3,
                "comments": 1,
                "total_tokens": 900,
                "input_tokens": 700,
                "output_tokens": 200,
                "elapsed": "1.2s"
              }
            }
            """.trimIndent(),
        )

        assertEquals("success", result.status)
        val comment = result.comments.single()
        assertEquals("src/a.kt", comment.path)
        assertEquals("val a = 1", comment.suggestionCode)
        assertEquals("val a = 2", comment.existingCode)
        assertEquals(10, comment.startLine)
        assertEquals(12, comment.endLine)
        assertEquals("推理过程", comment.thinking) // allow-non-english: fixture verifies Unicode CLI input and output
        assertEquals("b.bin", result.warnings.single().file)
        assertEquals(3, result.summary?.filesReviewed)
        assertEquals(900, result.summary?.totalTokens)
        assertEquals(700, result.summary?.inputTokens)
        assertEquals("1.2s", result.summary?.elapsed)
    }

    @Test
    fun `empty optional strings are normalized to null`() {
        val result = parseCliResult(
            """{"status":"success","comments":[{"path":"a","content":"c","suggestion_code":"","existing_code":"","thinking":""}]}""",
        )
        val comment = result.comments.single()
        assertNull(comment.suggestionCode)
        assertNull(comment.existingCode)
        assertNull(comment.thinking)
    }

    @Test
    fun `missing line numbers fall back to the zero sentinel`() {
        val result = parseCliResult("""{"status":"success","comments":[{"path":"a","content":"c"}]}""")
        assertEquals(0, result.comments.single().startLine)
        assertEquals(0, result.comments.single().endLine)
    }

    @Test
    fun `unknown CLI fields do not affect parsing`() {
        val result = parseCliResult("""{"status":"success","future_field":1,"comments":[]}""")
        assertEquals("success", result.status)
    }

    @Test
    fun `log noise before and after JSON is trimmed`() {
        val result = parseCliResult("reviewing 3 files...\n{\"status\":\"skipped\",\"comments\":[]}\ndone\n")
        assertEquals("skipped", result.status)
    }

    @Test
    fun `finds the real JSON when preceding logs contain braces`() {
        val result = parseCliResult(
            "config: {legacy: true}\n{\"status\":\"success\",\"comments\":[]}\n",
        )
        assertEquals("success", result.status)
    }

    @Test
    fun `throws when no JSON is present`() {
        assertFailsWith<IllegalArgumentException> { parseCliResult("nothing here") }
    }

    @Test
    fun `extractCliError prefers the last error line`() {
        val stderr = """
            error: first failure
            some noise
            error: real failure
            trailing noise
        """.trimIndent()
        assertEquals("real failure", extractCliError(stderr))
    }

    @Test
    fun `extractCliError uses the last non-empty line when no error line exists`() {
        assertEquals("last line", extractCliError("first\n\nlast line\n\n"))
    }

    @Test
    fun `extractCliError returns an empty string for empty input`() {
        assertEquals("", extractCliError("   \n\n"))
    }

    @Test
    fun `parseLogLine recognizes the warn level`() {
        assertEquals(LogLevel.WARN, parseLogLine("retrying request 2/3")?.level)
        assertEquals(LogLevel.WARN, parseLogLine("WARNING: model fallback")?.level)
        assertEquals(LogLevel.INFO, parseLogLine("reviewing src/a.kt")?.level)
    }

    @Test
    fun `parseLogLine drops blank lines and trims trailing whitespace`() {
        assertNull(parseLogLine("   "))
        assertEquals("text", parseLogLine("text   \t")?.text)
    }

    @Test
    fun `resultToState returns done when comments exist`() {
        val result = parseCliResult("""{"status":"success","comments":[{"path":"a","content":"c"}]}""")
        assertEquals(ReviewState.DONE, resultToState(result))
    }

    @Test
    fun `resultToState returns failed for completed_with_errors without comments`() {
        val result = parseCliResult("""{"status":"completed_with_errors","comments":[]}""")
        assertEquals(ReviewState.FAILED, resultToState(result))
    }

    @Test
    fun `resultToState returns empty without comments or errors`() {
        val result = parseCliResult("""{"status":"success","comments":[]}""")
        assertEquals(ReviewState.EMPTY, resultToState(result))
    }

    @Test
    fun `completed_with_errors still returns done when comments exist`() {
        val result = parseCliResult(
            """{"status":"completed_with_errors","comments":[{"path":"a","content":"c"}]}""",
        )
        assertTrue(resultToState(result) == ReviewState.DONE)
    }
}
