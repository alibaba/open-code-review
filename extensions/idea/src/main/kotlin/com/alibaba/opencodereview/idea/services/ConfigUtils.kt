// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.services

import com.alibaba.opencodereview.idea.model.OcrConfig
import java.util.Locale

/**
 * Exposes a single function, [isConfigReady].
 * The other helpers (`detectInitialTab`/`describeActiveProvider`/`build*SaveEntries`/`listCustomProviderNames`)
 * are frontend-only; a host-side duplicate implementation would drift as the two copies diverge.
 * [isConfigReady] is the exception: the host-side config panel needs it to compute `skipEnvCheck`,
 * so it must be computable on the host side.
 */

/**
 * Whether the configuration is ready (a review can start directly, with no environment onboarding).
 * With a provider selected: presets need a model, and custom providers also need url/protocol/apiKey.
 * With no provider selected: fall back to the `llm` trio (url/model/authToken).
 */
fun isConfigReady(config: OcrConfig?): Boolean {
    if (config == null) return false

    if (config.provider.isNotBlank()) {
        val preset = isPresetProvider(config.provider)
        // isPresetProvider compares after trim+lowercase(Locale.ROOT), and preset providers are stored under
        // canonically lowercased keys; the preset lookup must therefore use the same normalized key, otherwise
        // a config.provider of "OpenAI" is judged preset yet misses in the lowercase-key map.
        val key = if (preset) config.provider.trim().lowercase(Locale.ROOT) else config.provider
        val entry =
            if (preset) config.providers[key] else config.customProviders[key]
        if (entry == null || entry.model.isBlank()) return false
        if (preset) return true
        return entry.url.isNotBlank() && entry.protocol.isNotBlank() && entry.apiKey.isNotBlank()
    }

    return config.llm.url.isNotBlank() &&
        config.llm.model.isNotBlank() &&
        config.llm.authToken.isNotBlank()
}
