package com.alibaba.opencodereview.idea.services

import java.util.Locale

/**
 * 内置 provider 名字集合。
 * 仅复制名字不复制 baseUrl/models/protocol 表：宿主侧只需 [isPresetProvider]（决定写入 `providers` 还是 `custom_providers`），
 * 复制整表会造成第二份数据源，两边分叉漂移。
 * 漂移由 `ProvidersTest` 拦截：该测试从前端共享源码中正则提取 `name:` 字段进行比对。
 */
private val PRESET_PROVIDER_NAMES: Set<String> = setOf(
    "anthropic",
    "openai",
    "dashscope",
    "dashscope-tokenplan",
    "volcengine",
    "deepseek",
    "tencent-tokenhub",
    "hy-tokenplan",
    "kimi",
    "z-ai",
    "z-ai-coding",
    "mimo",
    "minimax",
    "minimax-cn",
    "baidu-qianfan",
)

/** 按 trim + 小写比对；用 Locale.ROOT 避免 Turkish 等 locale 下 'I'→'ı' 之类意外。 */
fun isPresetProvider(name: String): Boolean =
    PRESET_PROVIDER_NAMES.contains(name.trim().lowercase(Locale.ROOT))

/** 供单测/诊断用的只读视图。返回防御性拷贝，避免外部 mutate 内部集合。 */
fun presetProviderNames(): Set<String> = PRESET_PROVIDER_NAMES.toSet()
