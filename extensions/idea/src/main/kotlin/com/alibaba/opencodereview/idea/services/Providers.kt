// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.services

import java.util.Locale

/**
 * Names of the built-in providers.
 * Only the names are copied, not the baseUrl/models/protocol tables: the host side only needs
 * [isPresetProvider] (to decide whether a value is written to `providers` or `custom_providers`),
 * and duplicating the whole tables would create a second data source that drifts apart on the two sides.
 * Drift is caught by `ProvidersTest`, which reads the generated shared frontend catalog
 * and compares its provider names with this set.
 */
private val PRESET_PROVIDER_NAMES: Set<String> = setOf(
    "anthropic",
    "baidu-qianfan",
    "bedrock",
    "dashscope",
    "dashscope-tokenplan",
    "deepseek",
    "edenai",
    "gemini",
    "hy-tokenplan",
    "iflytek",
    "kimi",
    "kimi-global",
    "litellm",
    "mimo",
    "minimax",
    "minimax-cn",
    "mistral",
    "novita",
    "ollama-cloud",
    "openai",
    "openai-responses",
    "siliconflow",
    "siliconflow-cn",
    "tencent-tokenhub",
    "volcengine",
    "xai",
    "z-ai",
    "z-ai-coding",
)

/** Compares after trim + lowercase; Locale.ROOT avoids surprises such as the Turkish-locale 'I' lowercasing to a dotless i. */
fun isPresetProvider(name: String): Boolean =
    PRESET_PROVIDER_NAMES.contains(name.trim().lowercase(Locale.ROOT))

/** Read-only view for tests and diagnostics. Returns a defensive copy so callers cannot mutate the backing set. */
fun presetProviderNames(): Set<String> = PRESET_PROVIDER_NAMES.toSet()
