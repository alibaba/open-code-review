// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.services

import com.alibaba.opencodereview.idea.model.ConfigEntry
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull
import kotlin.test.assertTrue

class ConfigDraftTest {

    private fun draft(base: String, vararg entries: Pair<String, String>): RawConfig =
        applyConfigEntries(parseRawConfig(base), entries.map { ConfigEntry(it.first, it.second) })

    private fun RawConfig.str(vararg path: String): String? {
        var cur: Any? = this[path.first()]
        for (key in path.drop(1)) {
            cur = (cur as? JsonObject)?.get(key)
        }
        return (cur as? JsonPrimitive)?.takeIf { it.isString }?.content
    }

    private fun RawConfig.obj(vararg path: String): JsonObject? {
        var cur: Any? = this[path.first()]
        for (key in path.drop(1)) {
            cur = (cur as? JsonObject)?.get(key)
        }
        return cur as? JsonObject
    }

    // ------------------------------------------------------------ provider

    @Test
    fun `switching provider clears the top-level model`() {
        val d = draft("""{"provider": "openai", "model": "gpt-5.5"}""", "provider" to "anthropic")
        assertEquals("anthropic", d.str("provider"))
        assertEquals("", d.str("model"))
    }

    @Test
    fun `built-in provider creates a providers entry`() {
        val d = draft("{}", "provider" to "dashscope")
        assertEquals(JsonObject(emptyMap()), d.obj("providers", "dashscope"))
        assertNull(d.obj("custom_providers"))
    }

    @Test
    fun `non-built-in provider creates a custom_providers entry`() {
        val d = draft("{}", "provider" to "my-gateway")
        assertEquals(JsonObject(emptyMap()), d.obj("custom_providers", "my-gateway"))
        assertNull(d.obj("providers"))
    }

    @Test
    fun `empty provider does not create an entry`() {
        val d = draft("{}", "provider" to "")
        assertEquals("", d.str("provider"))
        assertNull(d.obj("providers"))
        assertNull(d.obj("custom_providers"))
    }

    @Test
    fun `existing provider entries are not cleared`() {
        val d = draft("""{"providers": {"openai": {"api_key": "k"}}}""", "provider" to "openai")
        assertEquals("k", d.str("providers", "openai", "api_key"))
    }

    // ------------------------------------------------------------ model

    @Test
    fun `model is written to the selected built-in provider entry`() {
        val d = draft("""{"provider": "openai"}""", "model" to "gpt-5.5")
        assertEquals("gpt-5.5", d.str("providers", "openai", "model"))
        // Do not write the top-level model; the CLI prefers the value in the provider entry.
        assertNull(d.str("model"))
    }

    @Test
    fun `model is written to custom_providers for a custom provider`() {
        val d = draft("""{"provider": "mine"}""", "model" to "m1")
        assertEquals("m1", d.str("custom_providers", "mine", "model"))
    }

    @Test
    fun `model is written at the top level when no provider is selected`() {
        val d = draft("{}", "model" to "m1")
        assertEquals("m1", d.str("model"))
    }

    @Test
    fun `provider and model batch ordering determines the destination`() {
        // setMany ordering contract: apply provider first to determine the destination for model.
        val d = draft("{}", "provider" to "openai", "model" to "gpt-5.5")
        assertEquals("gpt-5.5", d.str("providers", "openai", "model"))
    }

    // ------------------------------------------------------------ providers.<name>.<field>

    @Test
    fun `new generated presets keep configuration in the built-in provider bucket`() {
        for (name in listOf("bedrock", "openai-responses", "xai", "gemini", "ollama-cloud")) {
            val d = draft("{}", "provider" to name, "providers.$name.model" to "saved-model")
            assertEquals(name, d.str("provider"))
            assertEquals("saved-model", d.str("providers", name, "model"), name)
            assertNull(d.obj("custom_providers"), name)
        }
    }

    @Test
    fun `the providers prefix routes by name to built-in or custom containers`() {
        val preset = draft("{}", "providers.anthropic.api_key" to "sk-1")
        assertEquals("sk-1", preset.str("providers", "anthropic", "api_key"))

        val custom = draft("{}", "providers.mine.api_key" to "sk-2")
        assertEquals("sk-2", custom.str("custom_providers", "mine", "api_key"))
        assertNull(custom.obj("providers"))
    }

    @Test
    fun `the custom_providers prefix always writes to the custom container`() {
        // An explicit prefix preserves routing even when the name matches a built-in provider.
        val d = draft("{}", "custom_providers.anthropic.url" to "http://proxy")
        assertEquals("http://proxy", d.str("custom_providers", "anthropic", "url"))
    }

    @Test
    fun `keys with the wrong number of segments are ignored`() {
        assertEquals(emptyRawConfig(), draft("{}", "providers.anthropic" to "x"))
        assertEquals(emptyRawConfig(), draft("{}", "providers.a.b.c" to "x"))
        assertEquals(emptyRawConfig(), draft("{}", "custom_providers.a" to "x"))
    }

    @Test
    fun `unknown provider fields are ignored`() {
        assertEquals(JsonObject(emptyMap()), draft("{}", "providers.openai.nope" to "x").obj("providers", "openai"))
    }

    @Test
    fun `writing a field preserves other fields in the same entry`() {
        val d = draft(
            """{"providers": {"openai": {"api_key": "k", "url": "u"}}}""",
            "providers.openai.model" to "gpt-5.5",
        )
        assertEquals("k", d.str("providers", "openai", "api_key"))
        assertEquals("u", d.str("providers", "openai", "url"))
        assertEquals("gpt-5.5", d.str("providers", "openai", "model"))
    }

    // ------------------------------------------------------------ Model lists

    @Test
    fun `models supports JSON arrays`() {
        assertEquals(listOf("a", "b"), parseModelList("""["a", "b"]"""))
    }

    @Test
    fun `models supports comma-separated values and trims whitespace`() {
        assertEquals(listOf("a", "b"), parseModelList(" a , b , "))
    }

    @Test
    fun `invalid JSON arrays fall back to comma splitting`() {
        // Incomplete arrays such as "[a,b" should split on commas instead of failing.
        assertEquals(listOf("[a", "b"), parseModelList("[a,b"))
    }

    @Test
    fun `empty models input becomes an empty list`() {
        assertEquals(emptyList(), parseModelList("   "))
        assertEquals(emptyList(), parseModelList("[]"))
    }

    @Test
    fun `models is stored in the entry as a JSON array`() {
        val d = draft("{}", "providers.openai.models" to "a,b")
        assertEquals(
            JsonArray(listOf(JsonPrimitive("a"), JsonPrimitive("b"))),
            d.obj("providers", "openai")!!["models"],
        )
    }

    // ------------------------------------------------------------ llm

    @Test
    fun `all five llm fields can be written`() {
        val d = draft(
            "{}",
            "llm.url" to "http://x",
            "llm.auth_token" to "t",
            "llm.auth_header" to "h",
            "llm.model" to "m",
            "llm.use_anthropic" to "true",
        )
        assertEquals("http://x", d.str("llm", "url"))
        assertEquals("t", d.str("llm", "auth_token"))
        assertEquals("h", d.str("llm", "auth_header"))
        assertEquals("m", d.str("llm", "model"))
        assertEquals(JsonPrimitive(true), d.obj("llm")!!["use_anthropic"])
    }

    @Test
    fun `use_anthropic is enabled only by the true literal`() {
        assertEquals(JsonPrimitive(false), draft("{}", "llm.use_anthropic" to "false").obj("llm")!!["use_anthropic"])
        assertEquals(JsonPrimitive(false), draft("{}", "llm.use_anthropic" to "TRUE").obj("llm")!!["use_anthropic"])
        assertEquals(JsonPrimitive(false), draft("{}", "llm.use_anthropic" to "").obj("llm")!!["use_anthropic"])
    }

    // ------------------------------------------------------------ Preservation and isolation

    @Test
    fun `unknown top-level fields are preserved in the draft`() {
        // A JsonObject tree preserves unknown fields when writeRaw rewrites the entire file.
        // Dropping those fields would silently delete unrecognized keys from the user configuration.
        val d = draft("""{"future_flag": {"deep": [1, 2]}}""", "provider" to "openai")
        assertEquals("""{"deep":[1,2]}""", d.obj("future_flag").toString())
    }

    @Test
    fun `the original configuration is not mutated`() {
        val base = parseRawConfig("""{"provider": "openai", "providers": {"openai": {"api_key": "k"}}}""")
        applyConfigEntries(base, listOf(ConfigEntry("providers.openai.api_key", "changed")))
        assertEquals("k", base.str("providers", "openai", "api_key"))
        assertEquals("openai", base.str("provider"))
    }

    @Test
    fun `unknown top-level keys are ignored`() {
        assertEquals(emptyRawConfig(), draft("{}", "totally.unknown.key.here" to "x"))
        assertEquals(emptyRawConfig(), draft("{}", "language" to "English"))
    }

    @Test
    fun `invalid JSON configuration becomes an empty draft`() {
        // Match ConfigService.readRaw(): parse failures mean unconfigured and must not prevent subsequent writes.
        assertEquals(emptyRawConfig(), parseRawConfig("{ not json"))
        assertEquals(emptyRawConfig(), parseRawConfig(""))
    }

    @Test
    fun `serialization uses two-space indentation`() {
        val json = draft("{}", "provider" to "openai").toPrettyJson()
        assertTrue(json.contains("\n  \"provider\""), json)
    }
}
