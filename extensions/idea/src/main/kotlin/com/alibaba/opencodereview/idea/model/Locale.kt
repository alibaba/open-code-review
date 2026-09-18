// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.model

import com.intellij.DynamicBundle
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

/**
 * The locales this plugin supports.
 * The serialized values must stay the literals `en` / `zh-cn`: the frontend uses these values to look up
 * entries in its localization table, and writing `ZH_CN` or `zh-CN` would degrade the UI text to the key itself.
 */
@Serializable
enum class SupportedLocale {
    @SerialName("en") EN,
    @SerialName("zh-cn") ZH_CN,
}

/**
 * Locale resolution rule: only `zh-cn` (case-insensitive) counts as Simplified Chinese;
 * variants such as `zh-tw` / `zh-hk` all fall back to English until their translations exist.
 */
fun resolveLocale(raw: String): SupportedLocale =
    if (raw.lowercase() == "zh-cn") SupportedLocale.ZH_CN else SupportedLocale.EN

/** Converts to the HTML `lang` attribute value: `zh-cn` becomes `zh-CN`; everything else is returned as-is. */
fun SupportedLocale.toHtmlLang(): String = when (this) {
    SupportedLocale.ZH_CN -> "zh-CN"
    SupportedLocale.EN -> "en"
}

/**
 * Returns the current IDE UI language. `DynamicBundle.getLocale()` reflects the IDE UI language
 * (zh-CN once the Chinese language pack is installed), falling back to the JVM default locale when unavailable.
 * Components that do not hold a webview locale can call this directly, no injection needed.
 */
fun currentIdeLocale(): SupportedLocale {
    val tag = runCatching { DynamicBundle.getLocale().toLanguageTag() }
        .getOrElse { java.util.Locale.getDefault().toLanguageTag() }
    return resolveLocale(tag)
}
