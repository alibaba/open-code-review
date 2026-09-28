// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.jcef

import java.awt.Color
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Color-to-CSS conversion. These plain JUnit tests have no Application, so they test only [IdeaTheme.css],
 * not `cssVariables()`, which reads `UIManager`.
 *
 * Regression coverage: an earlier `css` implementation emitted only `#rrggbb` and discarded alpha.
 * IntelliJ themes often use translucent overlays for hover colors. In expUI Dark,
 * `ActionButton.hoverBackground` is `#FFFFFF16` (8.6% white), which became solid white when alpha was discarded.
 * That variable supplies the `.comment-card` background, producing light gray text on white in a dark theme.
 * There is no exception or blank screen, so tests must catch this regression.
 */
class IdeaThemeTest {

    @Test
    fun `opaque colors use six-digit hexadecimal`() {
        assertEquals("#2b2d30", IdeaTheme.css(Color(0x2B, 0x2D, 0x30)))
        assertEquals("#ffffff", IdeaTheme.css(Color(0xFF, 0xFF, 0xFF)))
        assertEquals("#000000", IdeaTheme.css(Color(0, 0, 0)))
    }

    @Test
    fun `translucent colors use rgba and preserve alpha`() {
        // The original ActionButton.hoverBackground value in expUI Dark.
        val hover = Color(0xFF, 0xFF, 0xFF, 0x16)
        assertEquals("rgba(255, 255, 255, 0.086)", IdeaTheme.css(hover))
    }

    @Test
    fun `translucent colors never become opaque`() {
        // The regression turned 8.6% white into #ffffff, making dark-theme cards solid white.
        val hover = Color(0xFF, 0xFF, 0xFF, 0x16)
        val css = IdeaTheme.css(hover)
        assertTrue("Translucent color became opaque: $css", css.startsWith("rgba("))
        assertEquals("#ffffff", IdeaTheme.css(Color(0xFF, 0xFF, 0xFF, 0xFF)))
    }

    @Test
    fun `fully transparent and nearly opaque colors stay in bounds`() {
        assertEquals("rgba(18, 52, 86, 0.000)", IdeaTheme.css(Color(0x12, 0x34, 0x56, 0)))
        // 254 is not 255 and must still use rgba; rounding must not produce values above 1.000.
        assertEquals("rgba(18, 52, 86, 0.996)", IdeaTheme.css(Color(0x12, 0x34, 0x56, 254)))
    }

    @Test
    fun `every variable name is valid in a CSS declaration`() {
        // Spaces or semicolons in variable names can make the browser discard the injected :root block silently.
        IdeaTheme.VARIABLE_NAMES.forEach { name ->
            assertTrue("Invalid variable name: $name", Regex("""^--[a-zA-Z0-9-]+$""").matches(name))
        }
    }
}
