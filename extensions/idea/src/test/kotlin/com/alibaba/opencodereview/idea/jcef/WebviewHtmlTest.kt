// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.jcef

import com.alibaba.opencodereview.idea.FrontendSources
import com.alibaba.opencodereview.idea.model.SupportedLocale
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Consistency checks for HTML assembly and theme variables.
 *
 * Both failure modes leave the UI visible but incorrect:
 * missing variable values inherit defaults, often producing black text on black in dark themes;
 * unescaped scripts expose source text on the page. Neither throws an exception, so tests must catch them.
 */
class WebviewHtmlTest {

    private val varRegex = Regex("""--vscode-[A-Za-z0-9-]+""")

    @Test
    fun `the host provides every vscode variable used by the frontend`() {
        val css = FrontendSources.readAllText("src/webview", ".css", ".tsx", ".ts")
        val used = varRegex.findAll(css).map { it.value }.toSortedSet()

        assertTrue(
            "No --vscode-* variables matched in frontend source; update this test",
            used.size >= 20,
        )
        val missing = used - IdeaTheme.VARIABLE_NAMES
        assertTrue(
            "Frontend variables missing from IdeaTheme: $missing\n" +
                "Add the corresponding IDEA color lookups to IdeaTheme.SPEC to supply these colors.",
            missing.isEmpty(),
        )
    }

    @Test
    fun `IdeaTheme has no variables unused by the frontend`() {
        // Unused variables can misleadingly suggest that the frontend uses a configured color.
        val css = FrontendSources.readAllText("src/webview", ".css", ".tsx", ".ts")
        val used = varRegex.findAll(css).map { it.value }.toSet()
        val extra = IdeaTheme.VARIABLE_NAMES - used
        assertTrue("IdeaTheme variables no longer used by the frontend can be removed: $extra", extra.isEmpty())
    }

    @Test
    fun `closing script tags in inline scripts are escaped`() {
        // Without escaping, the HTML parser ends the script here and renders the rest of the bundle as page text.
        val js = """var s = "</script>"; var t = '</SCRIPT >';"""
        val escaped = WebviewHtml.escapeForInlineScript(js)
        assertFalse(escaped.contains("</script", ignoreCase = true))
        assertTrue(escaped.contains("<\\/script"))
        // Escape only this sequence; preserve all other characters.
        assertEquals(js.length + 2, escaped.length)
    }

    @Test
    fun `the assembled page has a complete structure`() {
        val html = WebviewHtml.buildPage(
            lang = "zh-CN",
            themeCss = ":root { --vscode-foreground: #bbbbbb; }",
            bridgeScript = "window.cefQuery({request: json});",
            bundleJs = "console.log('bundle');",
        )
        assertTrue(html.startsWith("<!DOCTYPE html>"))
        assertTrue(html.contains("<html lang=\"zh-CN\">"))
        // The frontend mount point is required; without it the entire page is empty.
        assertTrue(html.contains("<div id=\"root\"></div>"))
        assertTrue(html.contains("--vscode-foreground"))
        assertTrue(html.contains("window.__ocrPost = function (json) {"))
        assertTrue(html.contains("window.cefQuery({request: json});"))
        assertTrue(html.contains("console.log('bundle');"))
        // The bridge script must precede the bundle, which may post 'ready' as soon as it executes.
        assertTrue(html.indexOf("__ocrPost") < html.indexOf("console.log('bundle')"))
    }

    @Test
    fun `theme variables have a separate style element with an id`() {
        // OcrWebview.applyTheme finds this style by id and replaces its entire textContent when the theme changes.
        // If layout rules share that style, each theme change removes the html/body height settings,
        // collapsing the panel height to zero. Separate style elements are therefore a required invariant.
        val html = WebviewHtml.buildPage(
            lang = "en",
            themeCss = ":root { --vscode-foreground: #bbbbbb; }",
            bridgeScript = "",
            bundleJs = "",
        )
        val open = "<style id=\"${WebviewHtml.THEME_STYLE_ID}\">"
        assertTrue("Theme style is missing its id", html.contains(open))

        val themeBlock = html.substringAfter(open).substringBefore("</style>")
        assertTrue("Theme variables are missing from the style with an id", themeBlock.contains("--vscode-foreground"))
        assertFalse("Layout rules are inside the theme style and would be lost on theme changes", themeBlock.contains("#root"))
        assertFalse(themeBlock.contains("html, body"))
        // Keep layout rules in a subsequent style element so theme changes do not remove them.
        assertTrue(html.contains("#root { height: 100%; }"))
    }

    @Test
    fun `the missing bundle page follows the locale`() {
        val zh = WebviewHtml.missingBundle("/webview/webview.js", SupportedLocale.ZH_CN)
        assertTrue(zh.contains("<html lang=\"zh-CN\">"))
        assertTrue(zh.contains("前端资源缺失")) // allow-non-english: assertion verifies Chinese UI translations
        assertTrue(zh.contains("<code>/webview/webview.js</code>"))

        val en = WebviewHtml.missingBundle("/webview/webview.js", SupportedLocale.EN)
        assertTrue(en.contains("<html lang=\"en\">"))
        assertTrue(en.contains("Frontend assets missing"))
        // English IDEs must not show Chinese here; this page helps users who cannot read Chinese troubleshoot problems.
        assertTrue("The English page still contains Chinese characters", en.none { it.code in 0x4E00..0x9FFF })
    }

    @Test
    fun `the variable list can be read without a UI environment`() {
        // VARIABLE_NAMES reads only SPEC keys, without calling color lookup lambdas.
        // Calling those lambdas would throw in these plain JUnit tests, which have no Application.
        assertTrue(IdeaTheme.VARIABLE_NAMES.contains("--vscode-foreground"))
        assertTrue(IdeaTheme.VARIABLE_NAMES.all { it.startsWith("--vscode-") })
    }
}
