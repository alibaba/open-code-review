// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.services

import com.alibaba.opencodereview.idea.model.LlmConfig
import com.alibaba.opencodereview.idea.model.OcrConfig
import com.alibaba.opencodereview.idea.model.OcrJson
import com.alibaba.opencodereview.idea.model.ProviderEntry
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonObject

/**
 * The conversion boundary between snake_case and camelCase:
 * `~/.opencodereview/config.json` is the CLI's snake_case format (`api_key`/`auth_header`/`custom_providers`/`use_anthropic`)
 * while the plugin works with camelCase internally.
 * Do not add `@SerialName` to [OcrConfig] and deserialize the file directly: fields serialized to the frontend
 * would keep their snake_case names, and the frontend would read undefined.
 */

/** Reads a string field; returns "" whenever the field is missing or of the wrong type. */
private fun JsonObject?.strAt(key: String): String {
    val prim = this?.get(key) as? JsonPrimitive ?: return ""
    return if (prim.isString) prim.content else ""
}

private fun parseProviderEntry(raw: JsonElement?): ProviderEntry {
    val obj = raw as? JsonObject ?: return ProviderEntry()
    // A missing models field stays null (not an empty list): callers use the field's presence to decide
    // whether to fall back to the preset models, while an empty list reads as "this provider has no models at all".
    val models = (obj["models"] as? kotlinx.serialization.json.JsonArray)?.mapNotNull {
        (it as? JsonPrimitive)?.takeIf { p -> p.isString }?.content
    }
    return ProviderEntry(
        apiKey = obj.strAt("api_key"),
        url = obj.strAt("url"),
        protocol = obj.strAt("protocol"),
        model = obj.strAt("model"),
        models = models,
        authHeader = obj.strAt("auth_header"),
    )
}

private fun parseProviderMap(raw: JsonElement?): Map<String, ProviderEntry> {
    val obj = raw as? JsonObject ?: return emptyMap()
    return obj.mapValues { (_, entry) -> parseProviderEntry(entry) }
}

/**
 * Parses the config file content. Blank input returns null; invalid JSON throws
 * (caught by [ConfigService] and treated as "no configuration").
 */
fun parseConfig(raw: String): OcrConfig? {
    if (raw.isBlank()) return null
    val j = OcrJson.parseToJsonElement(raw).jsonObject
    val llm = j["llm"] as? JsonObject
    return OcrConfig(
        provider = j.strAt("provider"),
        model = j.strAt("model"),
        providers = parseProviderMap(j["providers"]),
        customProviders = parseProviderMap(j["custom_providers"]),
        llm = LlmConfig(
            url = llm.strAt("url"),
            authToken = llm.strAt("auth_token"),
            model = llm.strAt("model"),
            useAnthropic = !isExplicitFalse(llm?.get("use_anthropic")),
            authHeader = llm.strAt("auth_header"),
        ),
        language = j.strAt("language").ifEmpty { "Chinese" },
    )
}

private fun isExplicitFalse(element: JsonElement?): Boolean {
    val prim = element as? JsonPrimitive ?: return false
    return !prim.isString && prim.content == "false"
}

fun toConfigSetArgs(key: String, value: String): List<String> = listOf("config", "set", key, value)
