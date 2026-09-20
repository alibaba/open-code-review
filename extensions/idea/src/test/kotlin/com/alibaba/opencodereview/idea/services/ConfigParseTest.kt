// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.services

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNull
import kotlin.test.assertTrue

class ConfigParseTest {

    @Test
    fun `empty content returns null`() {
        assertNull(parseConfig(""))
        assertNull(parseConfig("   \n  "))
    }

    @Test
    fun `snake_case configuration becomes a camelCase domain model`() {
        val cfg = parseConfig(
            """
            {
              "provider": "anthropic",
              "model": "claude-opus-4-8",
              "language": "English",
              "providers": {
                "anthropic": {
                  "api_key": "sk-x",
                  "url": "https://api.anthropic.com",
                  "protocol": "anthropic",
                  "model": "claude-opus-4-8",
                  "auth_header": "x-api-key",
                  "models": ["a", "b"]
                }
              },
              "custom_providers": {
                "mine": { "api_key": "k", "url": "http://localhost" }
              },
              "llm": {
                "url": "http://llm",
                "auth_token": "t",
                "model": "m",
                "auth_header": "h"
              }
            }
            """.trimIndent(),
        )!!

        assertEquals("anthropic", cfg.provider)
        assertEquals("English", cfg.language)

        val preset = cfg.providers.getValue("anthropic")
        // Guard against incorrect field names: snake_case names would silently leave the frontend blank.
        assertEquals("sk-x", preset.apiKey)
        assertEquals("x-api-key", preset.authHeader)
        assertEquals(listOf("a", "b"), preset.models)

        val custom = cfg.customProviders.getValue("mine")
        assertEquals("k", custom.apiKey)
        assertEquals("", custom.protocol)

        assertEquals("http://llm", cfg.llm.url)
        assertEquals("t", cfg.llm.authToken)
        assertEquals("h", cfg.llm.authHeader)
    }

    @Test
    fun `missing fields fall back to empty strings without throwing`() {
        val cfg = parseConfig("{}")!!
        assertEquals("", cfg.provider)
        assertEquals("", cfg.model)
        assertEquals(emptyMap(), cfg.providers)
        assertEquals(emptyMap(), cfg.customProviders)
        assertEquals("", cfg.llm.url)
    }

    @Test
    fun `missing language defaults to Chinese`() {
        assertEquals("Chinese", parseConfig("{}")!!.language)
        assertEquals("Chinese", parseConfig("""{"language": ""}""")!!.language)
        assertEquals("English", parseConfig("""{"language": "English"}""")!!.language)
    }

    @Test
    fun `use_anthropic is disabled only by an explicit boolean false`() {
        assertTrue(parseConfig("{}")!!.llm.useAnthropic)
        assertTrue(parseConfig("""{"llm": {}}""")!!.llm.useAnthropic)
        assertTrue(parseConfig("""{"llm": {"use_anthropic": true}}""")!!.llm.useAnthropic)
        assertEquals(false, parseConfig("""{"llm": {"use_anthropic": false}}""")!!.llm.useAnthropic)
    }

    @Test
    fun `string false does not disable use_anthropic`() {
        // In JavaScript, "false" !== false is true, so the option stays enabled.
        // Using booleanOrNull here would incorrectly disable it and diverge from frontend behavior.
        assertTrue(parseConfig("""{"llm": {"use_anthropic": "false"}}""")!!.llm.useAnthropic)
    }

    @Test
    fun `missing models is null rather than an empty list`() {
        // Null means unconfigured, so the frontend falls back to preset models.
        // An empty list means configured with no entries and renders an empty dropdown.
        val entry = parseConfig("""{"providers": {"anthropic": {}}}""")!!.providers.getValue("anthropic")
        assertNull(entry.models)
    }

    @Test
    fun `non-string elements in models are filtered out`() {
        val entry = parseConfig("""{"providers": {"p": {"models": ["a", 1, null, "b"]}}}""")!!
            .providers.getValue("p")
        assertEquals(listOf("a", "b"), entry.models)
    }

    @Test
    fun `incorrect field types are treated as missing`() {
        val cfg = parseConfig("""{"provider": 123, "model": true, "providers": []}""")!!
        assertEquals("", cfg.provider)
        assertEquals("", cfg.model)
        assertEquals(emptyMap(), cfg.providers)
    }

    @Test
    fun `unknown top-level fields do not affect parsing`() {
        val cfg = parseConfig("""{"provider": "openai", "future_flag": {"deep": [1]}}""")!!
        assertEquals("openai", cfg.provider)
    }

    @Test
    fun `invalid JSON propagates an exception`() {
        // ConfigService.read() catches this as unconfigured; the parser itself does not swallow the exception.
        assertFailsWith<Exception> { parseConfig("{ not json") }
    }

    @Test
    fun `config set argument order is fixed`() {
        assertEquals(listOf("config", "set", "provider", "openai"), toConfigSetArgs("provider", "openai"))
    }
}
