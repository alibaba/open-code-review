package com.alibaba.opencodereview.idea.services

import com.alibaba.opencodereview.idea.model.ConfigEntry
import com.alibaba.opencodereview.idea.model.OcrJson
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonObject

/**
 * 在内存中将 config set 条目合并进原始配置 JSON，不落盘。
 * 配置面板测试连通性依赖此函数：先计算草稿写入临时 HOME、在隔离环境执行测试，用户配置文件全程不变。
 */

/** 原始配置 JSON 的可变表示。值均为不可变的 [JsonElement]，整棵子树通过替换来"修改"。 */
typealias RawConfig = MutableMap<String, JsonElement>

fun emptyRawConfig(): RawConfig = mutableMapOf()

/** 解析配置文件文本；空白或非法 JSON 返回空草稿（调用方语义上等于"当作没有配置"）。 */
fun parseRawConfig(text: String): RawConfig {
    if (text.isBlank()) return emptyRawConfig()
    return runCatching {
        OcrJson.parseToJsonElement(text).jsonObject.toMutableMap()
    }.getOrElse { emptyRawConfig() }
}

/** 序列化为写盘用的两空格缩进 JSON。 */
fun RawConfig.toPrettyJson(): String = PrettyJson.encodeToString(JsonObject.serializer(), JsonObject(this))

private val PrettyJson = kotlinx.serialization.json.Json {
    prettyPrint = true
    prettyPrintIndent = "  "
    encodeDefaults = true
    explicitNulls = false
}

/**
 * 顶层浅拷贝即等价深拷贝：[JsonElement] 不可变，改动都通过 [mutateObj] 整体替换子树，
 * 不存在原地修改嵌套对象的路径，base 不会被写穿。
 */
fun RawConfig.copyDraft(): RawConfig = toMutableMap()

/** 读-改-写一个嵌套对象字段：不存在则视为空对象，修改后整体写回。 */
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
 * 解析 models 字段的用户输入：优先按 JSON 数组解析，失败或不是数组则按逗号分隔。
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
        // 解析失败则退化为逗号分隔，不报错。
    }
    return trimmed.split(',').map { it.trim() }.filter { it.isNotEmpty() }
}

/** 仅识别这 6 个 provider 字段，其余键忽略。 */
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

/** `providers.<name>.<field>`：name 是内置 provider 就写 providers，否则写 custom_providers。 */
private fun RawConfig.setProviderValue(key: String, value: String) {
    val parts = key.split('.')
    if (parts.size != 3) return
    val name = parts[1]
    val field = parts[2]
    if (isPresetProvider(name)) {
        setProviderEntryField("providers", name, field, value)
    } else {
        setCustomProviderField(name, field, value)
    }
}

private fun RawConfig.ensureProviderBucketEntry(value: String) {
    if (isPresetProvider(value)) {
        mutateObj("providers") { m -> if (m[value] !is JsonObject) m[value] = JsonObject(emptyMap()) }
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
            // 切换 provider 时清空顶层 model：旧 model 通常不属于新 provider。
            if (strAt("provider") != value) this["model"] = JsonPrimitive("")
            this["provider"] = JsonPrimitive(value)
            ensureProviderBucketEntry(value)
        }

        // 已选 provider 时，model 写入该 provider 条目而非顶层——CLI 优先读取 provider 条目中的 model，写顶层不生效。
        "model" -> {
            val provider = strAt("provider")
            if (provider.isNotEmpty()) {
                if (isPresetProvider(provider)) {
                    setProviderEntryField("providers", provider, "model", value)
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

/** 在内存中把 config set 条目合并进原始配置（不写磁盘）。 */
fun applyConfigEntries(base: RawConfig, entries: List<ConfigEntry>): RawConfig {
    val draft = base.copyDraft()
    for (entry in entries) {
        draft.setConfigValue(entry.key, entry.value)
    }
    return draft
}
