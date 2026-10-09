// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.services

import com.alibaba.opencodereview.idea.model.ConfigEntry
import com.alibaba.opencodereview.idea.model.OcrJson
import com.intellij.openapi.diagnostic.Logger
import kotlinx.serialization.ExperimentalSerializationApi
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonObject
import java.util.Locale

/**
 * Merges `config set` entries into the raw config JSON in memory, without touching disk.
 * The config panel's connectivity test relies on this: it computes the draft into a temporary HOME
 * and runs the test in an isolated environment, so the user's config file stays untouched throughout.
 */

/** A mutable representation of the raw config JSON. Values are immutable [JsonElement]s; a whole subtree is "modified" by replacement. */
typealias RawConfig = MutableMap<String, JsonElement>

fun emptyRawConfig(): RawConfig = mutableMapOf()

/** Parses the config file text; blank input or invalid JSON yields an empty draft (semantically "no configuration" for callers). */
fun parseRawConfig(text: String): RawConfig {
    if (text.isBlank()) return emptyRawConfig()
    return runCatching {
        OcrJson.parseToJsonElement(text).jsonObject.toMutableMap()
    }.getOrElse {
        // JSON parse failures are logged here -- the outer runCatching in readRaw only catches IO errors,
        // so a JSON error swallowed here would never reach it.
        Logger.getInstance("ConfigDraft").warn("[ocr] Failed to parse config JSON, treating as empty", it)
        emptyRawConfig()
    }
}

/** Serializes to two-space-indented JSON for writing to disk. */
fun RawConfig.toPrettyJson(): String = PrettyJson.encodeToString(JsonObject.serializer(), JsonObject(this))

@OptIn(ExperimentalSerializationApi::class)
private val PrettyJson = kotlinx.serialization.json.Json {
    prettyPrint = true
    prettyPrintIndent = "  "
    encodeDefaults = true
    explicitNulls = false
}

/**
 * A shallow copy of the top level is equivalent to a deep copy: [JsonElement]s are immutable, and all
 * changes go through [mutateObj], which replaces whole subtrees. No path mutates a nested object in
 * place, so the base can never be written through.
 */
fun RawConfig.copyDraft(): RawConfig = toMutableMap()

/** Read-modify-write of a nested object field: a missing object is treated as empty, and the whole object is written back after the change. */
private fun RawConfig.mutateObj(key: String, block: (MutableMap<String, JsonElement>) -> Unit) {
    val inner = (this[key] as? JsonObject)?.toMutableMap() ?: mutableMapOf()
    block(inner)
    this[key] = JsonObject(inner)
}

private fun RawConfig.strAt(key: String): String {
    val prim = this[key] as? JsonPrimitive ?: return ""
    return if (prim.isString) prim.content else ""
}

/**
 * Parses the user input for the models field: try a JSON array first, falling back to comma splitting
 * when parsing fails or the value is not an array.
 */
internal fun parseModelList(value: String): List<String> {
    val trimmed = value.trim()
    if (trimmed.isEmpty()) return emptyList()
    if (trimmed.startsWith("[")) {
        val parsed = runCatching { OcrJson.parseToJsonElement(trimmed) }.getOrNull()
        if (parsed is JsonArray) {
            return parsed.mapNotNull { item ->
                (item as? JsonPrimitive)?.takeIf { it.isString }?.content?.takeIf { it.isNotBlank() }
            }
        }
        // On a parse failure, degrade to comma splitting without raising an error.
    }
    return trimmed.split(',').map { it.trim() }.filter { it.isNotEmpty() }
}

/** Only these 6 provider fields are recognized; every other key is ignored. */
private fun applyProviderField(entry: MutableMap<String, JsonElement>, field: String, value: String) {
    when (field) {
        "api_key" -> entry["api_key"] = JsonPrimitive(value)
        "url" -> entry["url"] = JsonPrimitive(value)
        "protocol" -> entry["protocol"] = JsonPrimitive(value)
        "model" -> entry["model"] = JsonPrimitive(value)
        "models" -> entry["models"] = JsonArray(parseModelList(value).map(::JsonPrimitive))
        "auth_header" -> entry["auth_header"] = JsonPrimitive(value)
        else -> Unit
    }
}

private fun RawConfig.setProviderEntryField(
    bucket: String,
    name: String,
    field: String,
    value: String,
) {
    mutateObj(bucket) { bucketMap ->
        val entry = (bucketMap[name] as? JsonObject)?.toMutableMap() ?: mutableMapOf()
        applyProviderField(entry, field, value)
        bucketMap[name] = JsonObject(entry)
    }
}

private fun RawConfig.setCustomProviderField(name: String, field: String, value: String) =
    setProviderEntryField("custom_providers", name, field, value)

/** `providers.<name>.<field>`: if name is a built-in provider, write to providers; otherwise write to custom_providers. */
private fun RawConfig.setProviderValue(key: String, value: String) {
    val parts = key.split('.')
    if (parts.size != 3) return
    val name = parts[1]
    val field = parts[2]
    if (isPresetProvider(name)) {
        // Consistent with isPresetProvider / ConfigUtils: preset keys are stored canonically lowercased, so "OpenAI"
        // is not judged preset yet written under a capitalized key that later lookups would miss.
        setProviderEntryField("providers", name.trim().lowercase(Locale.ROOT), field, value)
    } else {
        setCustomProviderField(name, field, value)
    }
}

private fun RawConfig.ensureProviderBucketEntry(value: String) {
    if (isPresetProvider(value)) {
        val key = value.trim().lowercase(Locale.ROOT)
        mutateObj("providers") { m -> if (m[key] !is JsonObject) m[key] = JsonObject(emptyMap()) }
    } else if (value.isNotEmpty()) {
        mutateObj("custom_providers") { m -> if (m[value] !is JsonObject) m[value] = JsonObject(emptyMap()) }
    }
}

private fun RawConfig.setLlmField(field: String, element: JsonElement) {
    mutateObj("llm") { it[field] = element }
}

private fun RawConfig.setConfigValue(key: String, value: String) {
    if (key.startsWith("providers.")) {
        setProviderValue(key, value)
        return
    }
    if (key.startsWith("custom_providers.")) {
        val parts = key.split('.')
        if (parts.size == 3) setCustomProviderField(parts[1], parts[2], value)
        return
    }

    when (key) {
        "provider" -> {
            // Preset providers are stored canonically lowercased, matching the providers bucket keys; custom providers keep the user's original name.
            val providerName = if (isPresetProvider(value)) value.trim().lowercase(Locale.ROOT) else value
            // Clear the top-level model when the provider changes: the old model usually does not belong to the new provider
            // (compare normalized names so a case difference alone is not mistaken for a switch).
            if (strAt("provider") != providerName) this["model"] = JsonPrimitive("")
            this["provider"] = JsonPrimitive(providerName)
            ensureProviderBucketEntry(value)
        }

        // With a provider selected, the model is written into that provider's entry instead of the top level --
        // the CLI prefers the model inside the provider entry and ignores the top-level one.
        "model" -> {
            val provider = strAt("provider")
            if (provider.isNotEmpty()) {
                if (isPresetProvider(provider)) {
                    setProviderEntryField("providers", provider.trim().lowercase(Locale.ROOT), "model", value)
                } else {
                    setCustomProviderField(provider, "model", value)
                }
            } else {
                this["model"] = JsonPrimitive(value)
            }
        }

        "llm.url" -> setLlmField("url", JsonPrimitive(value))
        "llm.auth_token" -> setLlmField("auth_token", JsonPrimitive(value))
        "llm.auth_header" -> setLlmField("auth_header", JsonPrimitive(value))
        "llm.model" -> setLlmField("model", JsonPrimitive(value))
        "llm.use_anthropic" -> setLlmField("use_anthropic", JsonPrimitive(value == "true"))
        else -> Unit
    }
}

/** Merges the config set entries into the raw config in memory (nothing is written to disk). */
fun applyConfigEntries(base: RawConfig, entries: List<ConfigEntry>): RawConfig {
    val draft = base.copyDraft()
    for (entry in entries) {
        draft.setConfigValue(entry.key, entry.value)
    }
    return draft
}
