package com.alibaba.opencodereview.idea.jcef

import java.awt.Color
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * 颜色到 CSS 的转换。运行在无 Application 的普通 JUnit 中，因此仅测 [IdeaTheme.css]，
 * 不涉及 `cssVariables()`（后者需要读取 `UIManager`）。
 *
 * 此用例针对一个真实缺陷添加：`css` 的早期实现仅输出 `#rrggbb`，丢弃了 alpha 分量。
 * IntelliJ 主题中 hover 等颜色大量为半透明叠加层——expUI Dark 的
 * `ActionButton.hoverBackground` 即为 `#FFFFFF16`（白色 8.6%）——被压缩为实心色后变为纯白，
 * 而该变量被用作 `.comment-card` 的底色，暗色主题下会渲染为白底配浅灰字。
 * 此情况下不抛异常、不白屏，只能依靠测试拦截。
 */
class IdeaThemeTest {

    @Test
    fun `不透明色输出六位十六进制`() {
        assertEquals("#2b2d30", IdeaTheme.css(Color(0x2B, 0x2D, 0x30)))
        assertEquals("#ffffff", IdeaTheme.css(Color(0xFF, 0xFF, 0xFF)))
        assertEquals("#000000", IdeaTheme.css(Color(0, 0, 0)))
    }

    @Test
    fun `半透明色输出 rgba 且保留透明度`() {
        // expUI Dark 的 ActionButton.hoverBackground 原值。
        val hover = Color(0xFF, 0xFF, 0xFF, 0x16)
        assertEquals("rgba(255, 255, 255, 0.086)", IdeaTheme.css(hover))
    }

    @Test
    fun `半透明色绝不能塌成实心`() {
        // 此即该缺陷的表现形式：白色 8.6% 被压缩为 #ffffff，暗色主题下卡片变为纯白。
        val hover = Color(0xFF, 0xFF, 0xFF, 0x16)
        val css = IdeaTheme.css(hover)
        assertTrue("半透明色被压成了实心：$css", css.startsWith("rgba("))
        assertEquals("#ffffff", IdeaTheme.css(Color(0xFF, 0xFF, 0xFF, 0xFF)))
    }

    @Test
    fun `全透明与几乎不透明都不越界`() {
        assertEquals("rgba(18, 52, 86, 0.000)", IdeaTheme.css(Color(0x12, 0x34, 0x56, 0)))
        // 254 不等于 255，仍应走 rgba 分支，不可因四舍五入得到 1.000 以外的值。
        assertEquals("rgba(18, 52, 86, 0.996)", IdeaTheme.css(Color(0x12, 0x34, 0x56, 254)))
    }

    @Test
    fun `每个变量名都能出现在合法的 CSS 声明里`() {
        // 变量名若包含空格或分号，注入的 :root 会被浏览器整块丢弃——同样属于静默失败。
        IdeaTheme.VARIABLE_NAMES.forEach { name ->
            assertTrue("变量名不合法：$name", Regex("""^--[a-zA-Z0-9-]+$""").matches(name))
        }
    }
}
