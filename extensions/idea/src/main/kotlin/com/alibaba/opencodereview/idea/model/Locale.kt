package com.alibaba.opencodereview.idea.model

import com.intellij.DynamicBundle
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

/**
 * 本插件支持的语言枚举。
 * 序列化值须为 `en` / `zh-cn` 字面量：前端以此从词条表中取值，写作 `ZH_CN` 或 `zh-CN` 将使文案退化为 key 本身。
 */
@Serializable
enum class SupportedLocale {
    @SerialName("en") EN,
    @SerialName("zh-cn") ZH_CN,
}

/**
 * 语言判定规则：仅 `zh-cn`（忽略大小写）视为简体中文，
 * `zh-tw` / `zh-hk` 等变体在对应翻译补齐之前，一律回退为英文。
 */
fun resolveLocale(raw: String): SupportedLocale =
    if (raw.lowercase() == "zh-cn") SupportedLocale.ZH_CN else SupportedLocale.EN

/** 转换为 HTML `lang` 属性取值：`zh-cn` 转为 `zh-CN`，其余原样返回。 */
fun SupportedLocale.toHtmlLang(): String = when (this) {
    SupportedLocale.ZH_CN -> "zh-CN"
    SupportedLocale.EN -> "en"
}

/**
 * 返回当前 IDE 界面语言。`DynamicBundle.getLocale()` 反映 IDE 界面语言
 * （安装中文语言包后为 zh-CN），取不到时回退至 JVM 默认区域。不持有 webview locale 的组件可直接调用，无需额外注入。
 */
fun currentIdeLocale(): SupportedLocale {
    val tag = runCatching { DynamicBundle.getLocale().toLanguageTag() }
        .getOrElse { java.util.Locale.getDefault().toLanguageTag() }
    return resolveLocale(tag)
}
