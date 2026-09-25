// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.services

import com.alibaba.opencodereview.idea.FrontendSources
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Consistency checks for the preset provider list.
 *
 * The host needs only names to route providers to `providers` or `custom_providers`,
 * while the frontend uses the full preset table generated from Go. Both name sets must match:
 * if the frontend adds a built-in provider without a host update, it is treated as custom,
 * written to the wrong container, and silently ignored by the CLI, which reports no configured model during review.
 */
class ProvidersTest {

    @Test
    fun `preset provider names exactly match the frontend provider table`() {
        val ts = FrontendSources.file("src/shared/providers.generated.ts").readText()
        val declaration = "export const PROVIDER_PRESETS: OcrProviderPreset[] = "
        assertTrue("Missing generated provider declaration", ts.contains(declaration))
        val json = ts.substringAfter(declaration).trim().removeSuffix(";")
        val fromFrontend = Json.parseToJsonElement(json).jsonArray
            .map { it.jsonObject.getValue("name").jsonPrimitive.content }.toSortedSet()

        assertTrue(
            "Expected the generated catalog to contain the built-in providers",
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
        for (name in presetProviderNames()) {
            assertTrue(isPresetProvider(name))
            assertTrue(isPresetProvider(name.uppercase(java.util.Locale.ROOT)))
            assertTrue(isPresetProvider("  $name  "))
        }
        assertFalse(isPresetProvider("my-own-llm"))
        assertFalse(isPresetProvider(""))
    }
}
