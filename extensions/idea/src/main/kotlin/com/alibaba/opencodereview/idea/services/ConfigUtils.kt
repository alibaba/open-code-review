package com.alibaba.opencodereview.idea.services

import com.alibaba.opencodereview.idea.model.OcrConfig
import java.util.Locale

/**
 * 仅提供 [isConfigReady] 一个函数。
 * 其余辅助函数（`detectInitialTab`/`describeActiveProvider`/`build*SaveEntries`/`listCustomProviderNames`）仅前端使用，
 * 宿主重复实现会因两套代码分叉而漂移。[isConfigReady] 例外：宿主侧配置面板用它计算 `skipEnvCheck`，须在宿主侧可计算。
 */

/**
 * 配置是否可用（可直接开始审查、无需走环境引导）。
 * 选了 provider：预置须有 model；自定义还须 url/protocol/apiKey。未选 provider：退回 `llm` 三件套（url/model/authToken）。
 */
fun isConfigReady(config: OcrConfig?): Boolean {
    if (config == null) return false

    if (config.provider.isNotBlank()) {
        val preset = isPresetProvider(config.provider)
        // isPresetProvider 按 trim+lowercase(Locale.ROOT) 比对，预置 providers 也按规范小写键存储；
        // 故预置查找须用同一归一化键，否则 config.provider 为 "OpenAI" 时会被判为预置却在小写键 map 里查不到。
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
