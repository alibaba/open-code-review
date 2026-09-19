// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.jcef

import com.intellij.openapi.editor.colors.EditorColorsManager
import java.awt.Color
import java.util.Locale
import javax.swing.UIManager

/**
 * Maps the current IDEA theme to the set of `--vscode-*` CSS variables the page consumes.
 * Values are read live from UIManager and the editor color scheme, so Darcula/light/third-party
 * themes are all synchronized automatically.
 *
 * Two strict rules:
 * 1. Alpha must be preserved. IntelliJ themes contain many semi-transparent overlay layers;
 *    collapsing them to #rrggbb yields solid pure white.
 * 2. Colors come only from the LaF. Both foreground and background are read from UIManager,
 *    avoiding a "dark UI with a light editor scheme" combination that would produce light gray on white.
 */
object IdeaTheme {

    /** Variable name -> value. Lazy lambdas keep VARIABLE_NAMES from touching any UI API when reading the keys, so it works in a plain JUnit environment. */
    private val SPEC: List<Pair<String, () -> String>> = listOf(
        // ---------------------------------------------------------- Text
        "--vscode-foreground" to { css(ui("Label.foreground", fallback = LIGHT_TEXT)) },
        // Secondary description text, the most frequently used text (card subtitles, line numbers, token counts, ...).
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

        // ---------------------------------------------------------- Background
        // Overall sidebar background color, from the IDEA tool-window background (Panel.background).
        "--vscode-sideBar-background" to { css(ui("Panel.background", fallback = PANEL_BG)) },
        "--vscode-sideBarSectionHeader-background" to {
            css(ui("ToolWindow.Header.background", "Panel.background", fallback = PANEL_BG))
        },
        // Log panel background color, should be one shade darker than the sidebar background.
        // Deliberately not the editor scheme's defaultBackground: reading it from EditorColorsManager yields a
        // white background under a dark UI with a light editor scheme, while the text color comes from the LaF -- unreadable.
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

        // ---------------------------------------------------------- Lists and hover
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

        // ---------------------------------------------------------- Borders
        "--vscode-widget-border" to { css(ui("Component.borderColor", fallback = BORDER)) },
        "--vscode-input-border" to { css(ui("Component.borderColor", fallback = BORDER)) },
        "--vscode-button-border" to {
            css(ui("Button.startBorderColor", "Component.borderColor", fallback = BORDER))
        },

        // ---------------------------------------------------------- Badges
        "--vscode-badge-background" to {
            css(ui("Counter.background", "List.selectionBackground", fallback = SELECTION))
        },
        "--vscode-badge-foreground" to {
            css(ui("Counter.foreground", "List.selectionForeground", fallback = LIGHT_TEXT))
        },

        // ---------------------------------------------------------- Scrollbar
        // Scrollbar slider background: IDEA has no matching UIManager key, so follow VS Code's approach and layer transparency over the foreground color.
        "--vscode-scrollbarSlider-background" to {
            rgba(ui("Label.foreground", fallback = LIGHT_TEXT), 0.25)
        },
        "--vscode-scrollbarSlider-hoverBackground" to {
            rgba(ui("Label.foreground", fallback = LIGHT_TEXT), 0.40)
        },

        // ---------------------------------------------------------- Fonts
        "--vscode-font-family" to { cssFontStack(UIManager.getFont("Label.font")?.family, "sans-serif") },
        "--vscode-editor-font-family" to {
            cssFontStack(runCatching { scheme().editorFontName }.getOrNull(), "monospace")
        },
    )

    /** All variable names the page can use. Do not touch the UI API, so it can be read in a plain JUnit environment. */
    val VARIABLE_NAMES: Set<String> = SPEC.map { it.first }.toSet()

    /** Generates the `:root { ... }` block for injection. Falls back to the fallback colors when a value cannot be read, and never throws errors. */
    fun cssVariables(): String = buildString {
        append(":root {\n")
        SPEC.forEach { (name, provider) ->
            val value = runCatching(provider).getOrElse { "inherit" }
            append("  ").append(name).append(": ").append(value).append(";\n")
        }
        append("}")
    }

    // ------------------------------------------------------------ Color lookup

    private fun ui(vararg keys: String, fallback: Color): Color =
        keys.firstNotNullOfOrNull { runCatching { UIManager.getColor(it) }.getOrNull() } ?: fallback

    /** Only for reading the editor font name -- colors always go through [ui]; see rule 2 in the class doc. */
    private fun scheme() = EditorColorsManager.getInstance().globalScheme

    /** Converts to a CSS color, emitting rgba() for semi-transparent values. */
    internal fun css(color: Color): String =
        if (color.alpha == 255) {
            String.format(Locale.ROOT, "#%02x%02x%02x", color.red, color.green, color.blue)
        } else {
            rgba(color, 1.0)
        }

    /**
     * Emits `rgba()`; [extraAlpha] multiplies the color's own alpha rather than overriding it (the scrollbar
     * variables are "foreground layered with transparency", and the foreground may itself be semi-transparent --
     * overriding would render more opaque than the theme intends).
     */
    private fun rgba(color: Color, extraAlpha: Double): String {
        // Clamp to [0,1]: if extraAlpha or the foreground alpha is out of range, keep the rgba channel value from
        // going out of bounds and invalidating the CSS declaration.
        val alpha = ((color.alpha / 255.0) * extraAlpha).coerceIn(0.0, 1.0)
        // Locale.ROOT is mandatory: in locales such as German the decimal separator is a comma, and `0,086` would invalidate the CSS declaration.
        val formatted = String.format(Locale.ROOT, "%.3f", alpha)
        return "rgba(${color.red}, ${color.green}, ${color.blue}, $formatted)"
    }

    /** Characters to strip from font names: they would break the `:root{}` structure (`} ; \ newline`) or close the outer `<style>` block early (`<`, guarding against `</style` injection). */
    private val FONT_NAME_INVALID = Regex("[\"\\\\{};\\n\\r<]")

    /** Font names may contain spaces ("JetBrains Mono"), so they must be quoted; otherwise the browser drops the whole declaration. */
    private fun cssFontStack(family: String?, generic: String): String {
        val name = family?.takeIf { it.isNotBlank() } ?: return generic
        // Font names come from UIManager and are uncontrolled input, so always sanitize here before splicing into CSS.
        val sanitized = name.replace(FONT_NAME_INVALID, "")
        return "\"$sanitized\", $generic"
    }

    // Fallback colors (Darcula approximations). Used only when the LaF is entirely unavailable, so the page never
    // goes fully transparent because of a single null value.
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
