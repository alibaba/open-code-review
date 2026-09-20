// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.services

import com.alibaba.opencodereview.idea.FrontendSources
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Consistency checks for the preset provider list.
 *
 * The host needs only names to route providers to `providers` or `custom_providers`,
 * while the frontend maintains the full preset table. Both name sets must match:
 * if the frontend adds a built-in provider without a host update, it is treated as custom,
 * written to the wrong container, and silently ignored by the CLI, which reports no configured model during review.
 */
class ProvidersTest {

    /** Each frontend preset has the form `name: 'xxx',`. */
    private val nameRegex = Regex("""\bname:\s*'([^']+)'""")

    @Test
    fun `preset provider names exactly match the frontend provider table`() {
        val ts = FrontendSources.file("src/shared/providers.ts").readText()
        val fromFrontend = nameRegex.findAll(ts).map { it.groupValues[1] }.toSortedSet()

        assertTrue(
            "No names matched in providers.ts; upstream syntax changed and this test needs updating",
            fromFrontend.size >= 10,
        )
        assertEquals(
            "Preset provider lists differ between host and frontend. Differences: " +
                "frontend-only: ${fromFrontend - presetProviderNames()}; " +
                "host-only: ${presetProviderNames() - fromFrontend}",
            fromFrontend,
            presetProviderNames().toSortedSet(),
        )
    }

    @Test
    fun `isPresetProvider ignores case and surrounding whitespace`() {
        val any = presetProviderNames().first()
        assertTrue(isPresetProvider(any))
        assertTrue(isPresetProvider(any.uppercase()))
        assertTrue(isPresetProvider("  $any  "))
        assertFalse(isPresetProvider("my-own-llm"))
        assertFalse(isPresetProvider(""))
    }
}
