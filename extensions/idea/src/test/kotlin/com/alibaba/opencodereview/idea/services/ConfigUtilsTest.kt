// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.services

import com.alibaba.opencodereview.idea.model.LlmConfig
import com.alibaba.opencodereview.idea.model.OcrConfig
import com.alibaba.opencodereview.idea.model.ProviderEntry
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * [isConfigReady] determines whether the configuration panel skips environment setup.
 * Wrong decisions either repeatedly send configured users back to setup or allow reviews that fail for missing configuration.
 */
class ConfigUtilsTest {

    @Test
    fun `missing configuration is not ready`() {
        assertFalse(isConfigReady(null))
        assertFalse(isConfigReady(OcrConfig()))
    }

    @Test
    fun `preset provider is ready with a model`() {
        // The CLI preset table supplies url / protocol, so they are not required in configuration.
        val config = OcrConfig(
            provider = "kimi",
            providers = mapOf("kimi" to ProviderEntry(model = "kimi-k2", apiKey = "sk-x")),
        )
        assertTrue(isConfigReady(config))
    }

    @Test
    fun `preset provider without a model is not ready`() {
        val config = OcrConfig(
            provider = "kimi",
            providers = mapOf("kimi" to ProviderEntry(apiKey = "sk-x")),
        )
        assertFalse(isConfigReady(config))
    }

    @Test
    fun `preset provider without an entry is not ready`() {
        assertFalse(isConfigReady(OcrConfig(provider = "kimi")))
    }

    @Test
    fun `custom provider requires url protocol and apiKey`() {
        fun custom(entry: ProviderEntry) = OcrConfig(
            provider = "my-llm",
            customProviders = mapOf("my-llm" to entry),
        )

        val full = ProviderEntry(
            model = "m",
            url = "https://x",
            protocol = "openai",
            apiKey = "sk-x",
        )
        assertTrue(isConfigReady(custom(full)))
        assertFalse(isConfigReady(custom(full.copy(url = ""))))
        assertFalse(isConfigReady(custom(full.copy(protocol = ""))))
        assertFalse(isConfigReady(custom(full.copy(apiKey = ""))))
        assertFalse(isConfigReady(custom(full.copy(model = ""))))
    }

    @Test
    fun `custom provider is not looked up in providers`() {
        // Names outside the preset table are checked only in custom_providers; entries in the wrong container are not ready.
        val config = OcrConfig(
            provider = "my-llm",
            providers = mapOf("my-llm" to ProviderEntry(model = "m", url = "u", protocol = "p", apiKey = "k")),
        )
        assertFalse(isConfigReady(config))
    }

    @Test
    fun `without a selected provider readiness uses the three llm fields`() {
        val llm = LlmConfig(url = "https://x", model = "m", authToken = "t")
        assertTrue(isConfigReady(OcrConfig(llm = llm)))
        assertFalse(isConfigReady(OcrConfig(llm = llm.copy(url = ""))))
        assertFalse(isConfigReady(OcrConfig(llm = llm.copy(model = ""))))
        assertFalse(isConfigReady(OcrConfig(llm = llm.copy(authToken = ""))))
    }

    @Test
    fun `a selected provider prevents fallback to llm`() {
        // Complete llm fields cannot compensate for an incomplete provider; the logic is if/else, not or.
        val config = OcrConfig(
            provider = "kimi",
            llm = LlmConfig(url = "https://x", model = "m", authToken = "t"),
        )
        assertFalse(isConfigReady(config))
    }
}
