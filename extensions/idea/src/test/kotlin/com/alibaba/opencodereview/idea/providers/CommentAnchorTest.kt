package com.alibaba.opencodereview.idea.providers

import com.alibaba.opencodereview.idea.model.SupportedLocale
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

/** 镜像 commentAnchor 对应用例的输入与期望输出，保持同等覆盖。 */
class CommentAnchorTest {

    private val content = listOf("line1", "for (let i = 0; i <= 30, i++) {", "  console.log(i);", "}", "line5")
        .joinToString("\n")

    @Test
    fun `normalizeLine 去掉 diff 标记`() {
        assertEquals("added", normalizeLine("+added"))
        assertEquals("removed", normalizeLine("-removed"))
    }

    @Test
    fun `splitAndNormalize 跳过空行`() {
        assertEquals(listOf("a", "b"), splitAndNormalize("a\n\n b "))
    }

    @Test
    fun `resolveLinesInContent 行号在范围内直接用`() {
        val result = resolveLinesInContent(content, 2, 2, null)
        assertEquals(ResolvedLines(2, 2, relocated = false), result)
    }

    @Test
    fun `resolveLinesInContent 行号越界时退到 existingCode 重定位`() {
        val code = "for (let i = 0; i <= 30, i++) {"
        val result = resolveLinesInContent(content, 99, 99, code)
        assertEquals(ResolvedLines(2, 2, relocated = true), result)
    }

    @Test
    fun `findLinesByExistingCode 匹配连续非空行`() {
        val found = findLinesByExistingCode(content, "console.log(i);")
        assertEquals(LineSpan(3, 3), found)
    }

    @Test
    fun `行号和 existingCode 都解析不出来时返回 null`() {
        assertNull(resolveLinesInContent(content, 99, 99, null))
        assertNull(resolveLinesInContent(content, 0, 0, null))
    }

    @Test
    fun `existingCode 跨越多行时按整段滑窗匹配`() {
        val result = resolveLinesInContent(content, 99, 99, "for (let i = 0; i <= 30, i++) {\n  console.log(i);")
        assertEquals(ResolvedLines(2, 3, relocated = true), result)
    }

    @Test
    fun `CRLF 换行不影响匹配`() {
        val crlfContent = content.replace("\n", "\r\n")
        val found = findLinesByExistingCode(crlfContent, "console.log(i);")
        assertEquals(LineSpan(3, 3), found)
    }

    @Test
    fun `formatLocateNote 行号变了时点名原始行号`() {
        assertEquals(
            "⚠ 原本第 99 行没能匹配上，改为显示第 2 行。",
            formatLocateNote(99, 2, SupportedLocale.ZH_CN),
        )
        assertEquals(
            "⚠ Line 99 could not be matched, showing line 2 instead.",
            formatLocateNote(99, 2, SupportedLocale.EN),
        )
    }

    @Test
    fun `formatLocateNote 没有原始行号时用通用说明`() {
        assertEquals("⚠ 行号是根据代码内容重新定位的。", formatLocateNote(0, 2, SupportedLocale.ZH_CN))
        assertEquals(
            "⚠ Line number was relocated based on code content.",
            formatLocateNote(0, 2, SupportedLocale.EN),
        )
    }

    // ---------------------------------------------------------------- diff 侧挂载

    private val marks = listOf(
        DiffMark(0, "a.kt", AnchorSide.RIGHT, 10, 12),
        DiffMark(1, "a.kt", AnchorSide.LEFT, 20, 20),
        DiffMark(2, "b.kt", AnchorSide.RIGHT, 30, 35),
    )

    @Test
    fun `selectDiffMarks 只挑路径和侧别都对上的`() {
        assertEquals(listOf(marks[0]), selectDiffMarks("a.kt", AnchorSide.RIGHT, marks))
        assertEquals(listOf(marks[1]), selectDiffMarks("a.kt", AnchorSide.LEFT, marks))
        assertEquals(listOf(marks[2]), selectDiffMarks("b.kt", AnchorSide.RIGHT, marks))
    }

    @Test
    fun `selectDiffMarks 侧别不对就不挂`() {
        // 一次 diff 会同时交出左右两个文档，不区分侧别会导致同一条评论在两侧各挂一个图标。
        assertEquals(emptyList(), selectDiffMarks("b.kt", AnchorSide.LEFT, marks))
        assertEquals(emptyList(), selectDiffMarks("c.kt", AnchorSide.RIGHT, marks))
    }

    @Test
    fun `selectDiffMarks 带着完整范围一起交出去`() {
        // 此用例补上一个真实缺陷：早期 DiffMark 仅有一个 line 字段，多行评论
        // （实测占 CLI 输出的六成以上）在绘制高亮时仅剩起始行，底色仅标记首行。
        val picked = selectDiffMarks("b.kt", AnchorSide.RIGHT, marks).single()
        assertEquals(30, picked.startLine)
        assertEquals(35, picked.endLine)
    }

    @Test
    fun `clampLineRange 把 1-based 闭区间换成 0-based`() {
        assertEquals(0..0, clampLineRange(1, 1, 5))
        assertEquals(0..4, clampLineRange(1, 5, 5))
        assertEquals(4..4, clampLineRange(5, 5, 5))
    }

    @Test
    fun `clampLineRange 多行范围不能塌成单行`() {
        // 此性质此前未被测试覆盖，因此 diff 模式丢失结束行的问题长期未被发现。
        assertEquals(9..11, clampLineRange(10, 12, 100))
    }

    @Test
    fun `clampLineRange 越界不抛也不越界`() {
        // 新增文件的左侧、删除文件的右侧为空串，行数与挂载时计算的不一致；
        // 不做夹紧处理时 getLineStartOffset 会抛出 IndexOutOfBounds，diff 将无法打开。
        assertEquals(4..4, clampLineRange(999, 1000, 5))
        assertEquals(0..4, clampLineRange(0, 999, 5))
        assertEquals(0..0, clampLineRange(-3, -1, 5))
    }

    @Test
    fun `clampLineRange 结束行越界时首行仍然保留`() {
        // 仅结束行超出文档（评论覆盖的代码后半段被删除）时：起始行不应被一并丢弃。
        assertEquals(2..4, clampLineRange(3, 99, 5))
    }

    @Test
    fun `clampLineRange 反序输入收敬成单行`() {
        // CLI 偶尔会输出 end < start。夹紧后必须保证 start <= end，
        // 否则 addRangeHighlighter 收到反向区间会抛出异常。
        assertEquals(4..4, clampLineRange(5, 2, 10))
    }

    @Test
    fun `clampLineRange 空文档不抛异常`() {
        assertEquals(0..0, clampLineRange(7, 9, 0))
    }
}
