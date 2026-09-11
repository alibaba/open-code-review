package com.alibaba.opencodereview.idea.jcef

import com.alibaba.opencodereview.idea.FrontendSources
import com.alibaba.opencodereview.idea.model.SupportedLocale
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * HTML 装配与主题变量的一致性检查。
 *
 * 上述两项出现问题时表现均为"界面仍可见，但不正确"：
 * 变量缺值会导致某块颜色变为继承的默认值（深色主题下常表现为黑底黑字）；
 * 脚本未转义会导致页面上出现大段源码文本。这两种情况都不会抛异常，只能依靠测试拦截。
 */
class WebviewHtmlTest {

    private val varRegex = Regex("""--vscode-[A-Za-z0-9-]+""")

    @Test
    fun `前端用到的每个 vscode 变量宿主都给了值`() {
        val css = FrontendSources.readAllText("src/webview", ".css", ".tsx", ".ts")
        val used = varRegex.findAll(css).map { it.value }.toSortedSet()

        assertTrue(
            "没能从前端源码里正则出任何 --vscode-* 变量，用例本身需要更新",
            used.size >= 20,
        )
        val missing = used - IdeaTheme.VARIABLE_NAMES
        assertTrue(
            "前端用到了 IdeaTheme 没提供的变量：$missing\n" +
                "去 IdeaTheme.SPEC 里补上对应的 IDEA 取色，否则这些地方在 IDEA 里会没有颜色。",
            missing.isEmpty(),
        )
    }

    @Test
    fun `IdeaTheme 没有前端不用的多余变量`() {
        // 多余变量不会导致错误，但会使人误以为某个颜色已生效而实际并未生效。
        val css = FrontendSources.readAllText("src/webview", ".css", ".tsx", ".ts")
        val used = varRegex.findAll(css).map { it.value }.toSet()
        val extra = IdeaTheme.VARIABLE_NAMES - used
        assertTrue("IdeaTheme 里有前端不再使用的变量，可以删掉：$extra", extra.isEmpty())
    }

    @Test
    fun `内联脚本里的 script 结束标记被转义`() {
        // 未转义时 HTML 解析器会在此处截断脚本，后半段 bundle 内容会变为页面正文。
        val js = """var s = "</script>"; var t = '</SCRIPT >';"""
        val escaped = WebviewHtml.escapeForInlineScript(js)
        assertFalse(escaped.contains("</script", ignoreCase = true))
        assertTrue(escaped.contains("<\\/script"))
        // 转义仅处理该序列，其余字符原样保留。
        assertEquals(js.length + 2, escaped.length)
    }

    @Test
    fun `装配出的页面结构完整`() {
        val html = WebviewHtml.buildPage(
            lang = "zh-CN",
            themeCss = ":root { --vscode-foreground: #bbbbbb; }",
            bridgeScript = "window.cefQuery({request: json});",
            bundleJs = "console.log('bundle');",
        )
        assertTrue(html.startsWith("<!DOCTYPE html>"))
        assertTrue(html.contains("<html lang=\"zh-CN\">"))
        // 前端渲染所需的挂载点，缺失则整个页面为空。
        assertTrue(html.contains("<div id=\"root\"></div>"))
        assertTrue(html.contains("--vscode-foreground"))
        assertTrue(html.contains("window.__ocrPost = function (json) {"))
        assertTrue(html.contains("window.cefQuery({request: json});"))
        assertTrue(html.contains("console.log('bundle');"))
        // 桥脚本必须位于 bundle 之前：bundle 一旦执行即可能 post 'ready'。
        assertTrue(html.indexOf("__ocrPost") < html.indexOf("console.log('bundle')"))
    }

    @Test
    fun `主题变量单独占一个带 id 的 style`() {
        // OcrWebview.applyTheme 切换主题时按 id 定位此 style 并整体替换 textContent。
        // 若布局规则也挤在同一个 style 中，每次切换主题后 html/body 的高度设置即会丢失，
        // 表现为整个面板高度塌缩为 0——因此此处的"拆分为两个 style"是必须固化的不变量。
        val html = WebviewHtml.buildPage(
            lang = "en",
            themeCss = ":root { --vscode-foreground: #bbbbbb; }",
            bridgeScript = "",
            bundleJs = "",
        )
        val open = "<style id=\"${WebviewHtml.THEME_STYLE_ID}\">"
        assertTrue("主题 style 没有带 id", html.contains(open))

        val themeBlock = html.substringAfter(open).substringBefore("</style>")
        assertTrue("主题变量不在带 id 的 style 里", themeBlock.contains("--vscode-foreground"))
        assertFalse("布局规则挤进了主题 style，换主题会把它冲掉", themeBlock.contains("#root"))
        assertFalse(themeBlock.contains("html, body"))
        // 布局规则需放在后续另一个 style 中，避免随主题切换一同丢失。
        assertTrue(html.contains("#root { height: 100%; }"))
    }

    @Test
    fun `bundle 缺失页跟着 locale 走`() {
        val zh = WebviewHtml.missingBundle("/webview/webview.js", SupportedLocale.ZH_CN)
        assertTrue(zh.contains("<html lang=\"zh-CN\">"))
        assertTrue(zh.contains("前端资源缺失"))
        assertTrue(zh.contains("<code>/webview/webview.js</code>"))

        val en = WebviewHtml.missingBundle("/webview/webview.js", SupportedLocale.EN)
        assertTrue(en.contains("<html lang=\"en\">"))
        assertTrue(en.contains("Frontend assets missing"))
        // 英文 IDE 中不应出现中文——此页面用于协助无法阅读中文的用户排查问题。
        assertTrue("英文页里还有中文字符", en.none { it.code in 0x4E00..0x9FFF })
    }

    @Test
    fun `变量清单不需要 UI 环境就能读`() {
        // VARIABLE_NAMES 仅取 SPEC 的键，不调用取色 lambda——
        // 此用例运行在无 Application 的普通 JUnit 中，若实际调用会抛出异常。
        assertTrue(IdeaTheme.VARIABLE_NAMES.contains("--vscode-foreground"))
        assertTrue(IdeaTheme.VARIABLE_NAMES.all { it.startsWith("--vscode-") })
    }
}
