package com.alibaba.opencodereview.idea.jcef

import com.alibaba.opencodereview.idea.model.HostStrings
import com.alibaba.opencodereview.idea.model.SupportedLocale
import com.alibaba.opencodereview.idea.model.toHtmlLang

/**
 * 装配提交至 JCEF 的 HTML。JS 采用内联而非 `<script src>`：loadHTML() 无 base URL。
 * lang 属性保留，页面 :lang() 选择器与字体回退依赖该属性。
 */
object WebviewHtml {

    /** 注入 `--vscode-*` 变量的 `<style>` 的 id，切换主题时据此定位。 */
    internal const val THEME_STYLE_ID = "ocr-theme"

    /** 侧栏页面。[bridgeScript] 由 `JBCefJSQuery.inject("json")` 生成。 */
    fun sidebar(locale: SupportedLocale, bridgeScript: String): String =
        page("/webview/webview.js", locale, bridgeScript)

    /** 配置面板页面。 */
    fun configPanel(locale: SupportedLocale, bridgeScript: String): String =
        page("/webview/configPanel.js", locale, bridgeScript)

    private fun page(bundleResource: String, locale: SupportedLocale, bridgeScript: String): String {
        val bundle = readResource(bundleResource)
            ?: return missingBundle(bundleResource, locale)
        return buildPage(
            lang = locale.toHtmlLang(),
            themeCss = IdeaTheme.cssVariables(),
            bridgeScript = bridgeScript,
            bundleJs = bundle,
        )
    }

    /**
     * 纯字符串拼装，不触碰 UI，便于直接单测。
     */
    internal fun buildPage(
        lang: String,
        themeCss: String,
        bridgeScript: String,
        bundleJs: String,
    ): String = buildString {
        append("<!DOCTYPE html>\n")
        append("<html lang=\"").append(lang).append("\">\n")
        append("<head>\n<meta charset=\"UTF-8\">\n")
        // 主题变量单独放一个 style、id 固定：切换主题时仅替换它的 textContent，下方布局规则不被覆盖；
        // id 由 THEME_STYLE_ID 统一提供，Kotlin 侧拼 JS 选择器用同一常量，避免改 id 漏改选择器。
        append("<style id=\"").append(THEME_STYLE_ID).append("\">\n")
        append(themeCss).append('\n')
        append("</style>\n<style>\n")
        append("html, body { margin: 0; padding: 0; height: 100%; overflow: hidden; }\n")
        append("#root { height: 100%; }\n")
        append("</style>\n</head>\n<body>\n")
        append("<div id=\"root\"></div>\n")
        // 桥必须在 bundle 之前就绪：页面加载后即注册 __ocrReceive，
        // 当页面发起 ready 消息时 __ocrPost 必须已存在。
        append("<script>\nwindow.__ocrPost = function (json) { ")
        append(escapeForInlineScript(bridgeScript))
        append(" };\n</script>\n")
        append("<script>\n").append(escapeForInlineScript(bundleJs)).append("\n</script>\n")
        append("</body>\n</html>\n")
    }

    /**
     * 内联脚本中出现 `</script` 会被 HTML 解析器当作脚本结束标记，bundle 的后半段会被当作正文渲染。
     * minifier 输出不稳定，无条件转义；`<\/script` 在 JS 字符串与正则中与原文等价。
     */
    internal fun escapeForInlineScript(js: String): String =
        js.replace("</script", "<\\/script", ignoreCase = true)

    private fun readResource(path: String): String? =
        javaClass.getResourceAsStream(path)?.bufferedReader(Charsets.UTF_8)?.use { it.readText() }

    /**
     * bundle 不在 jar 内（几乎仅因前端未构建一种原因）。文案走 [HostStrings]，不能写死中文：
     * 英文 IDE 下出现中文说明，用户既读不懂也无法修复。
     */
    internal fun missingBundle(resource: String, locale: SupportedLocale): String = """
        <!DOCTYPE html>
        <html lang="${locale.toHtmlLang()}"><head><meta charset="UTF-8"></head>
        <body style="font-family: sans-serif; padding: 16px; line-height: 1.6">
        <h3>${HostStrings.t(locale, "ext.webview.missingBundleTitle")}</h3>
        <p>${HostStrings.t(locale, "ext.webview.missingBundleBody", "resource" to resource)}</p>
        <p>${HostStrings.t(locale, "ext.webview.missingBundleHint")}</p>
        <pre>cd frontend &amp;&amp; npm install &amp;&amp; npm run build</pre>
        <p>${HostStrings.t(locale, "ext.webview.missingBundleAlt")}</p>
        </body></html>
    """.trimIndent()
}
