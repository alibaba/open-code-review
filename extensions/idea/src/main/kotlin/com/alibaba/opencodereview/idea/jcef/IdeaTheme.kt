package com.alibaba.opencodereview.idea.jcef

import com.intellij.openapi.editor.colors.EditorColorsManager
import java.awt.Color
import java.util.Locale
import javax.swing.UIManager

/**
 * 将当前 IDEA 主题映射为页面所用的 `--vscode-*` CSS 变量集合。
 * 从 UIManager 与编辑器配色实时取值，Darcula/Light/第三方主题均随之同步。
 *
 * 两条硬性规则：
 * 一、alpha 必须保留。IntelliJ 主题存在大量半透明叠加层，压缩为 #rrggbb 会得到实心纯白。
 * 二、颜色仅取自 LaF。前景与背景一律取 UIManager，避免「暗色界面搭配亮色编辑器配色」时拼出白底浅灰。
 */
object IdeaTheme {

    /** 变量名 -> 取值。使用惰性 lambda，使 VARIABLE_NAMES 读取全部键时不触碰 UI API，可在纯 JUnit 环境运行。 */
    private val SPEC: List<Pair<String, () -> String>> = listOf(
        // ---------------------------------------------------------- 文字
        "--vscode-foreground" to { css(ui("Label.foreground", fallback = LIGHT_TEXT)) },
        // 次要说明文字，使用频率最高（卡片副标题、行号、token 统计等）。
        "--vscode-descriptionForeground" to {
            css(ui("Label.infoForeground", "Component.infoForeground", fallback = MUTED_TEXT))
        },
        "--vscode-disabledForeground" to {
            css(ui("Label.disabledForeground", "Component.disabledForeground", fallback = MUTED_TEXT))
        },
        "--vscode-errorForeground" to { css(ui("Label.errorForeground", fallback = ERROR_TEXT)) },
        "--vscode-textLink-foreground" to {
            css(ui("Link.activeForeground", "Component.linkForeground", fallback = LINK))
        },

        // ---------------------------------------------------------- 背景
        // 侧栏整体底色，取 IDEA 工具窗背景（Panel.background）。
        "--vscode-sideBar-background" to { css(ui("Panel.background", fallback = PANEL_BG)) },
        "--vscode-sideBarSectionHeader-background" to {
            css(ui("ToolWindow.Header.background", "Panel.background", fallback = PANEL_BG))
        },
        // 日志面板底色，应比侧栏底色更深一层。
        // 不取编辑器配色 defaultBackground：若从 EditorColorsManager 取值，在暗色界面搭配亮色编辑器配色时，会得到白底，而字色取自 LaF，导致不可读。
        "--vscode-editor-background" to {
            css(ui("EditorPane.background", "TextArea.background", "TextField.background", fallback = INPUT_BG))
        },
        "--vscode-editor-selectionBackground" to {
            css(ui("TextField.selectionBackground", "List.selectionBackground", fallback = SELECTION))
        },
        "--vscode-input-background" to { css(ui("TextField.background", fallback = INPUT_BG)) },
        "--vscode-dropdown-background" to {
            css(ui("ComboBox.background", "TextField.background", fallback = INPUT_BG))
        },
        "--vscode-button-secondaryBackground" to { css(ui("Button.background", fallback = PANEL_BG)) },

        // ---------------------------------------------------------- 列表与悬停
        "--vscode-list-activeSelectionBackground" to {
            css(ui("List.selectionBackground", fallback = SELECTION))
        },
        "--vscode-list-inactiveSelectionBackground" to {
            css(ui("List.selectionInactiveBackground", "List.selectionBackground", fallback = SELECTION))
        },
        "--vscode-list-hoverBackground" to {
            css(ui("List.hoverBackground", "ActionButton.hoverBackground", fallback = HOVER_BG))
        },
        "--vscode-toolbar-hoverBackground" to {
            css(ui("ActionButton.hoverBackground", "List.hoverBackground", fallback = HOVER_BG))
        },

        // ---------------------------------------------------------- 边框
        "--vscode-widget-border" to { css(ui("Component.borderColor", fallback = BORDER)) },
        "--vscode-input-border" to { css(ui("Component.borderColor", fallback = BORDER)) },
        "--vscode-button-border" to {
            css(ui("Button.startBorderColor", "Component.borderColor", fallback = BORDER))
        },

        // ---------------------------------------------------------- 角标
        "--vscode-badge-background" to {
            css(ui("Counter.background", "List.selectionBackground", fallback = SELECTION))
        },
        "--vscode-badge-foreground" to {
            css(ui("Counter.foreground", "List.selectionForeground", fallback = LIGHT_TEXT))
        },

        // ---------------------------------------------------------- 滚动条
        // 滚动条底色：IDEA 无对应 UIManager 键，沿用 VS Code 做法，以前景色叠加透明度生成。
        "--vscode-scrollbarSlider-background" to {
            rgba(ui("Label.foreground", fallback = LIGHT_TEXT), 0.25)
        },
        "--vscode-scrollbarSlider-hoverBackground" to {
            rgba(ui("Label.foreground", fallback = LIGHT_TEXT), 0.40)
        },

        // ---------------------------------------------------------- 字体
        "--vscode-font-family" to { cssFontStack(UIManager.getFont("Label.font")?.family, "sans-serif") },
        "--vscode-editor-font-family" to {
            cssFontStack(runCatching { scheme().editorFontName }.getOrNull(), "monospace")
        },
    )

    /** 页面可用的全部变量名集合。不触碰任何 UI API，可在纯 JUnit 环境读取。 */
    val VARIABLE_NAMES: Set<String> = SPEC.map { it.first }.toSet()

    /** 生成注入用的 `:root { ... }`。取不到某个值时使用兜底色，绝不抛出异常。 */
    fun cssVariables(): String = buildString {
        append(":root {\n")
        SPEC.forEach { (name, provider) ->
            val value = runCatching(provider).getOrElse { "inherit" }
            append("  ").append(name).append(": ").append(value).append(";\n")
        }
        append("}")
    }

    // ------------------------------------------------------------ 取色

    private fun ui(vararg keys: String, fallback: Color): Color =
        keys.firstNotNullOfOrNull { runCatching { UIManager.getColor(it) }.getOrNull() } ?: fallback

    /** 仅用于取编辑器字体名——颜色一律走 [ui]，参见类注释规则二。 */
    private fun scheme() = EditorColorsManager.getInstance().globalScheme

    /** 转换为 CSS 颜色，半透明时输出 rgba()。 */
    internal fun css(color: Color): String =
        if (color.alpha == 255) {
            String.format(Locale.ROOT, "#%02x%02x%02x", color.red, color.green, color.blue)
        } else {
            rgba(color, 1.0)
        }

    /**
     * 输出 `rgba()`，[extraAlpha] 与颜色自带 alpha 相乘（非覆盖：滚动条变量为「前景色叠加一层透明度」，
     * 前景本身可能已半透明，直接覆盖会比主题原意更实）。
     */
    private fun rgba(color: Color, extraAlpha: Double): String {
        val alpha = (color.alpha / 255.0) * extraAlpha
        // 必须使用 Locale.ROOT：德语等 locale 小数点为逗号，`0,086` 会导致 CSS 声明失效。
        val formatted = String.format(Locale.ROOT, "%.3f", alpha)
        return "rgba(${color.red}, ${color.green}, ${color.blue}, $formatted)"
    }

    /** 字体名可能含空格（如 "JetBrains Mono"），必须加引号，否则整条声明会被浏览器丢弃。 */
    private fun cssFontStack(family: String?, generic: String): String {
        val name = family?.takeIf { it.isNotBlank() } ?: return generic
        return "\"${name.replace("\"", "")}\", $generic"
    }

    // 兜底色（Darcula 近似）。仅在 LaF 完全未装配时使用，保证页面不会因单个 null 值而整块透明。
    private val LIGHT_TEXT = Color(0xBB, 0xBB, 0xBB)
    private val MUTED_TEXT = Color(0x80, 0x80, 0x80)
    private val ERROR_TEXT = Color(0xFF, 0x52, 0x61)
    private val LINK = Color(0x35, 0x92, 0xC4)
    private val PANEL_BG = Color(0x3C, 0x3F, 0x41)
    private val INPUT_BG = Color(0x45, 0x49, 0x4A)
    private val SELECTION = Color(0x2F, 0x65, 0xCA)
    private val HOVER_BG = Color(0x4C, 0x50, 0x52)
    private val BORDER = Color(0x64, 0x64, 0x64)
}
