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
 * snake_case 与 camelCase 之间的转换边界：
 * `~/.opencodereview/config.json` 是 CLI snake_case 格式（`api_key`/`auth_header`/`custom_providers`/`use_anthropic`），插件内部使用 camelCase。
 * 不要给 [OcrConfig] 加 `@SerialName` 直接反序列化配置文件——那样序列化给前端时字段名仍为 snake_case，前端会读取到 undefined。
 */

/** 取字符串字段，缺失或类型不符一律返回 ""。 */
private fun JsonObject?.strAt(key: String): String {
    val prim = this?.get(key) as? JsonPrimitive ?: return ""
    return if (prim.isString) prim.content else ""
}

private fun parseProviderEntry(raw: JsonElement?): ProviderEntry {
    val obj = raw as? JsonObject ?: return ProviderEntry()
    // models 缺失时保持 null（非空列表）：调用方按"有无此字段"决定是否使用预置模型兜底，
    // 空列表会被当作"该 provider 无任何模型"。
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
 * 解析配置文件内容。空白输入返回 null；JSON 非法会抛异常（由 [ConfigService] 捕获后当作"无配置"）。
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
