// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.jcef

import com.alibaba.opencodereview.idea.model.HostStrings
import com.alibaba.opencodereview.idea.model.SupportedLocale
import com.alibaba.opencodereview.idea.model.toHtmlLang

/**
 * Assembles the HTML handed to JCEF. The JavaScript is inlined rather than referenced with
 * `<script src>` because loadHTML() has no base URL.
 * The lang attribute is kept: the page's :lang() selectors and font fallback depend on it.
 */
object WebviewHtml {

    /** Id of the `<style>` carrying the injected `--vscode-*` variables; a theme switch locates it
     *  by this. */
    internal const val THEME_STYLE_ID = "ocr-theme"

    /** Sidebar page. [bridgeScript] is produced by `JBCefJSQuery.inject("json")`. */
    fun sidebar(locale: SupportedLocale, bridgeScript: String): String =
        page("/webview/webview.js", locale, bridgeScript)

    /** Config panel page. */
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
     * Pure string assembly that touches no UI, so it can be unit-tested directly.
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
        // The theme variables get a style element of their own with a fixed id: a theme switch
        // replaces only its textContent, leaving the layout rules below untouched. The id comes from
        // THEME_STYLE_ID, the same constant Kotlin uses to build the JS selector, so renaming the id
        // cannot leave the selector behind.
        append("<style id=\"").append(THEME_STYLE_ID).append("\">\n")
        append(themeCss).append('\n')
        append("</style>\n<style>\n")
        append("html, body { margin: 0; padding: 0; height: 100%; overflow: hidden; }\n")
        append("#root { height: 100%; }\n")
        append("</style>\n</head>\n<body>\n")
        append("<div id=\"root\"></div>\n")
        // The bridge must be ready before the bundle: the page registers __ocrReceive as soon as it
        // loads, and __ocrPost must already exist by the time the page sends its ready message.
        append("<script>\nwindow.__ocrPost = function (json) { ")
        append(escapeForInlineScript(bridgeScript))
        append(" };\n</script>\n")
        append("<script>\n").append(escapeForInlineScript(bundleJs)).append("\n</script>\n")
        append("</body>\n</html>\n")
    }

    /**
     * A `</script` inside an inline script reads as the script's end tag to the HTML parser, and the
     * rest of the bundle is then rendered as body text. Minifier output is not stable, so escape
     * unconditionally; `<\/script` is equivalent to the original inside JS strings and regexes.
     *
     * **Note**: only the `</script>` sequence is handled, with no HTML entity escaping; callers must
     * pass trusted JS (from the plugin's own build output).
     */
    internal fun escapeForInlineScript(js: String): String =
        js.replace("</script", "<\\/script", ignoreCase = true)

    private fun readResource(path: String): String? {
        // The path must be absolute on the classpath (start with /): a relative path resolves against
        // this class's package and fails silently, leaving only the fallback page and a bug that is
        // hard to trace.
        require(path.startsWith("/")) { "Resource path must be absolute: $path" }
        // runCatching: an IOException from readText (an I/O fault) yields null, which triggers
        // page()'s missingBundle fallback instead of letting the exception escape and break JCEF
        // assembly.
        // Strips a leading UTF-8 BOM (U+FEFF); otherwise the inline JS starts with a BOM and some
        // engines fail to parse it.
        return runCatching {
            javaClass.getResourceAsStream(path)?.bufferedReader(Charsets.UTF_8)?.use { it.readText() }
        }.getOrNull()?.removePrefix("\uFEFF")
    }

    /**
     * The bundle is missing from the jar — almost always because the frontend was not built.
     * The copy goes through [HostStrings] and must not be hardcoded Chinese: on an English IDE a
     * Chinese explanation is one the user can neither read nor act on.
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
