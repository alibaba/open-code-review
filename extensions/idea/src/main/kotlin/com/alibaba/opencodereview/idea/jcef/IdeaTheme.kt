// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.jcef

import com.intellij.openapi.editor.colors.EditorColorsManager
import java.awt.Color
import java.util.Locale
import javax.swing.UIManager

/**
 * Maps the current IDEA theme onto the set of `--vscode-*` CSS variables the page uses.
 * Values are read live from UIManager and the editor color scheme, so Darcula, Light and
 * third-party themes all follow along.
 *
 * Two hard rules:
 * 1. Alpha must be kept. IntelliJ themes use many translucent overlays, and collapsing them to
 *    #rrggbb yields opaque white.
 * 2. Colors come from the LaF only. Foreground and background always come from UIManager, so a
 *    dark UI paired with a light editor scheme does not end up light-grey on white.
 */
object IdeaTheme {

    /** Variable name -> value. The lazy lambdas let VARIABLE_NAMES read every key without touching
     *  the UI API, so this runs in a plain JUnit environment. */
    private val SPEC: List<Pair<String, () -> String>> = listOf(
        // ---------------------------------------------------------- Text
        "--vscode-foreground" to { css(ui("Label.foreground", fallback = LIGHT_TEXT)) },
        // Secondary text, used most often (card subtitles, line numbers, token counts, ...).
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
        // Base color of the whole sidebar, taken from the IDEA tool window background
        // (Panel.background).
        "--vscode-sideBar-background" to { css(ui("Panel.background", fallback = PANEL_BG)) },
        "--vscode-sideBarSectionHeader-background" to {
            css(ui("ToolWindow.Header.background", "Panel.background", fallback = PANEL_BG))
        },
        // Log panel background, one shade darker than the sidebar background.
        // Deliberately not the editor scheme's defaultBackground: taking it from
        // EditorColorsManager gives white under a dark UI with a light editor scheme, while the
        // text color comes from the LaF — unreadable.
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

        // ---------------------------------------------------------- Scrollbars
        // Scrollbar thumb: IDEA has no matching UIManager key, so this follows the VS Code
        // approach and builds it from the foreground color overlaid with alpha.
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

    /** Every variable name the page can use. Touches no UI API, so it can be read in a plain
     *  JUnit environment. */
    val VARIABLE_NAMES: Set<String> = SPEC.map { it.first }.toSet()

    /** Builds the injected `:root { ... }`. A value that cannot be read falls back to a default
     *  color; this never throws. */
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

    /** Used only for the editor font name — colors always go through [ui], see rule 2 above. */
    private fun scheme() = EditorColorsManager.getInstance().globalScheme

    /** Converts to a CSS color; translucent colors are emitted as rgba(). */
    internal fun css(color: Color): String =
        if (color.alpha == 255) {
            String.format(Locale.ROOT, "#%02x%02x%02x", color.red, color.green, color.blue)
        } else {
            rgba(color, 1.0)
        }

    /**
     * Emits `rgba()`, multiplying [extraAlpha] by the color's own alpha (not overriding it: the
     * scrollbar variables are "foreground plus a layer of transparency", and the foreground may
     * already be translucent, so overriding would come out more opaque than the theme intends).
     */
    private fun rgba(color: Color, extraAlpha: Double): String {
        // Clamped to [0,1]: a bogus extraAlpha or foreground alpha must not push the rgba channel
        // out of range and invalidate the CSS declaration.
        val alpha = ((color.alpha / 255.0) * extraAlpha).coerceIn(0.0, 1.0)
        // Locale.ROOT is required: locales such as German use a comma as the decimal separator, and
        // `0,086` would invalidate the CSS declaration.
        val formatted = String.format(Locale.ROOT, "%.3f", alpha)
        return "rgba(${color.red}, ${color.green}, ${color.blue}, $formatted)"
    }

    /** Characters to strip from a font name: they would break the `:root{}` structure
     *  (`}`, `;`, `\`, newline) or close the enclosing `<style>` block early (`<`, which guards
     *  against a `</style` injection). */
    private val FONT_NAME_INVALID = Regex("[\"\\\\{};\\n\\r<]")

    /** A font name may contain spaces (e.g. "JetBrains Mono") and must be quoted, or the browser
     *  discards the whole declaration. */
    private fun cssFontStack(family: String?, generic: String): String {
        val name = family?.takeIf { it.isNotBlank() } ?: return generic
        // The font name comes from UIManager and is not under our control, so every value passes
        // through this layer before it is concatenated into the CSS.
        val sanitized = name.replace(FONT_NAME_INVALID, "")
        return "\"$sanitized\", $generic"
    }

    // Fallback colors (approximating Darcula). Used only when the LaF is not installed at all, so a
    // single null value cannot make a whole region transparent.
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
