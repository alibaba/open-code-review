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
    fun `workspace 模式不带 ref 参数`() {
        assertEquals(
            listOf("review", "--format", "json"),
            buildReviewArgs(CliRunOptions(mode = ReviewMode.WORKSPACE)),
        )
    }

    @Test
    fun `branch 模式带 from 和 to`() {
        assertEquals(
            listOf("review", "--from", "main", "--to", "dev", "--format", "json"),
            buildReviewArgs(CliRunOptions(mode = ReviewMode.BRANCH, from = "main", to = "dev")),
        )
    }

    @Test
    fun `commit 模式带 commit`() {
        assertEquals(
            listOf("review", "--commit", "abc1234", "--format", "json"),
            buildReviewArgs(CliRunOptions(mode = ReviewMode.COMMIT, commit = "abc1234")),
        )
    }

    @Test
    fun `空白的 from 和 customPrompt 不进参数表`() {
        val args = buildReviewArgs(
            CliRunOptions(mode = ReviewMode.BRANCH, from = "   ", to = "dev", customPrompt = "  "),
        )
        assertEquals(listOf("review", "--to", "dev", "--format", "json"), args)
    }

    @Test
    fun `customPrompt 和 concurrency 追加在末尾`() {
        val args = buildReviewArgs(
            CliRunOptions(mode = ReviewMode.WORKSPACE, customPrompt = "  只看安全问题  ", concurrency = 4),
        )
        assertEquals(
            listOf("review", "--format", "json", "--background", "只看安全问题", "--concurrency", "4"),
            args,
        )
    }

    @Test
    fun `snake_case 的 CLI 输出转换成 camelCase 领域模型`() {
        val result = parseCliResult(
            """
            {
              "status": "success",
              "comments": [
                {
                  "path": "src/a.kt",
                  "content": "问题描述",
                  "suggestion_code": "val a = 1",
                  "existing_code": "val a = 2",
                  "start_line": 10,
                  "end_line": 12,
                  "thinking": "推理过程"
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
        assertEquals("推理过程", comment.thinking)
        assertEquals("b.bin", result.warnings.single().file)
        assertEquals(3, result.summary?.filesReviewed)
        assertEquals(900, result.summary?.totalTokens)
        assertEquals(700, result.summary?.inputTokens)
        assertEquals("1.2s", result.summary?.elapsed)
    }

    @Test
    fun `空字符串的可选字段归一成 null`() {
        val result = parseCliResult(
            """{"status":"success","comments":[{"path":"a","content":"c","suggestion_code":"","existing_code":"","thinking":""}]}""",
        )
        val comment = result.comments.single()
        assertNull(comment.suggestionCode)
        assertNull(comment.existingCode)
        assertNull(comment.thinking)
    }

    @Test
    fun `缺失行号回落到 0 哨兵值`() {
        val result = parseCliResult("""{"status":"success","comments":[{"path":"a","content":"c"}]}""")
        assertEquals(0, result.comments.single().startLine)
        assertEquals(0, result.comments.single().endLine)
    }

    @Test
    fun `CLI 未知字段不影响解析`() {
        val result = parseCliResult("""{"status":"success","future_field":1,"comments":[]}""")
        assertEquals("success", result.status)
    }

    @Test
    fun `JSON 前后的日志噪声会被裁掉`() {
        val result = parseCliResult("reviewing 3 files...\n{\"status\":\"skipped\",\"comments\":[]}\ndone\n")
        assertEquals("skipped", result.status)
    }

    @Test
    fun `前置日志里混进花括号也能找到真正的 JSON`() {
        val result = parseCliResult(
            "config: {legacy: true}\n{\"status\":\"success\",\"comments\":[]}\n",
        )
        assertEquals("success", result.status)
    }

    @Test
    fun `没有 JSON 时抛异常`() {
        assertFailsWith<IllegalArgumentException> { parseCliResult("nothing here") }
    }

    @Test
    fun `extractCliError 优先取最后一条 error 行`() {
        val stderr = """
            error: first failure
            some noise
            error: real failure
            trailing noise
        """.trimIndent()
        assertEquals("real failure", extractCliError(stderr))
    }

    @Test
    fun `extractCliError 没有 error 行时取最后一行非空内容`() {
        assertEquals("last line", extractCliError("first\n\nlast line\n\n"))
    }

    @Test
    fun `extractCliError 空输入返回空串`() {
        assertEquals("", extractCliError("   \n\n"))
    }

    @Test
    fun `parseLogLine 识别 warn 级别`() {
        assertEquals(LogLevel.WARN, parseLogLine("retrying request 2/3")?.level)
        assertEquals(LogLevel.WARN, parseLogLine("WARNING: model fallback")?.level)
        assertEquals(LogLevel.INFO, parseLogLine("reviewing src/a.kt")?.level)
    }

    @Test
    fun `parseLogLine 丢弃空行并裁掉行尾空白`() {
        assertNull(parseLogLine("   "))
        assertEquals("text", parseLogLine("text   \t")?.text)
    }

    @Test
    fun `resultToState 有评论就是 done`() {
        val result = parseCliResult("""{"status":"success","comments":[{"path":"a","content":"c"}]}""")
        assertEquals(ReviewState.DONE, resultToState(result))
    }

    @Test
    fun `resultToState 无评论且 completed_with_errors 是 failed`() {
        val result = parseCliResult("""{"status":"completed_with_errors","comments":[]}""")
        assertEquals(ReviewState.FAILED, resultToState(result))
    }

    @Test
    fun `resultToState 无评论且无错误是 empty`() {
        val result = parseCliResult("""{"status":"success","comments":[]}""")
        assertEquals(ReviewState.EMPTY, resultToState(result))
    }

    @Test
    fun `有评论时即使 completed_with_errors 也是 done`() {
        val result = parseCliResult(
            """{"status":"completed_with_errors","comments":[{"path":"a","content":"c"}]}""",
        )
        assertTrue(resultToState(result) == ReviewState.DONE)
    }
}
