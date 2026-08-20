package com.alibaba.opencodereview.idea.services

import com.alibaba.opencodereview.idea.model.OcrConfig

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

    if (config.provider.isNotEmpty()) {
        val preset = isPresetProvider(config.provider)
        val entry =
            if (preset) config.providers[config.provider] else config.customProviders[config.provider]
        if (entry == null || entry.model.isEmpty()) return false
        if (preset) return true
        return entry.url.isNotEmpty() && entry.protocol.isNotEmpty() && entry.apiKey.isNotEmpty()
    }

    return config.llm.url.isNotEmpty() &&
        config.llm.model.isNotEmpty() &&
        config.llm.authToken.isNotEmpty()
}
