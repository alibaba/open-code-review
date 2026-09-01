package com.alibaba.opencodereview.idea.services

import com.alibaba.opencodereview.idea.model.LlmConfig
import com.alibaba.opencodereview.idea.model.OcrConfig
import com.alibaba.opencodereview.idea.model.ProviderEntry
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * [isConfigReady] 决定配置面板是否跳过环境检查引导。
 * 判定错误的后果：用户配置已齐却仍被反复拉回引导页，或反之直接进入主界面后审查失败。
 */
class ConfigUtilsTest {

    @Test
    fun `没有配置就是没准备好`() {
        assertFalse(isConfigReady(null))
        assertFalse(isConfigReady(OcrConfig()))
    }

    @Test
    fun `预置 provider 只要有 model 就算齐`() {
        // 预置 provider 的 url / protocol 由 CLI 内置表提供，配置中无需填写。
        val config = OcrConfig(
            provider = "kimi",
            providers = mapOf("kimi" to ProviderEntry(model = "kimi-k2", apiKey = "sk-x")),
        )
        assertTrue(isConfigReady(config))
    }

    @Test
    fun `预置 provider 缺 model 不算齐`() {
        val config = OcrConfig(
            provider = "kimi",
            providers = mapOf("kimi" to ProviderEntry(apiKey = "sk-x")),
        )
        assertFalse(isConfigReady(config))
    }

    @Test
    fun `预置 provider 压根没有条目不算齐`() {
        assertFalse(isConfigReady(OcrConfig(provider = "kimi")))
    }

    @Test
    fun `自定义 provider 要 url protocol apiKey 全齐`() {
        fun custom(entry: ProviderEntry) = OcrConfig(
            provider = "my-llm",
            customProviders = mapOf("my-llm" to entry),
        )

        val full = ProviderEntry(
            model = "m",
            url = "https://x",
            protocol = "openai",
            apiKey = "sk-x",
        )
        assertTrue(isConfigReady(custom(full)))
        assertFalse(isConfigReady(custom(full.copy(url = ""))))
        assertFalse(isConfigReady(custom(full.copy(protocol = ""))))
        assertFalse(isConfigReady(custom(full.copy(apiKey = ""))))
        assertFalse(isConfigReady(custom(full.copy(model = ""))))
    }

    @Test
    fun `自定义 provider 不会去 providers 桶里找`() {
        // 名称不在预置表中时仅检查 custom_providers；放入错误的桶必须判定为未就绪。
        val config = OcrConfig(
            provider = "my-llm",
            providers = mapOf("my-llm" to ProviderEntry(model = "m", url = "u", protocol = "p", apiKey = "k")),
        )
        assertFalse(isConfigReady(config))
    }

    @Test
    fun `没选 provider 时退回 llm 三件套`() {
        val llm = LlmConfig(url = "https://x", model = "m", authToken = "t")
        assertTrue(isConfigReady(OcrConfig(llm = llm)))
        assertFalse(isConfigReady(OcrConfig(llm = llm.copy(url = ""))))
        assertFalse(isConfigReady(OcrConfig(llm = llm.copy(model = ""))))
        assertFalse(isConfigReady(OcrConfig(llm = llm.copy(authToken = ""))))
    }

    @Test
    fun `选了 provider 就不再看 llm`() {
        // llm 字段即使填写完整，provider 侧不齐仍视为未就绪——判定逻辑为 if/else 而非 or。
        val config = OcrConfig(
            provider = "kimi",
            llm = LlmConfig(url = "https://x", model = "m", authToken = "t"),
        )
        assertFalse(isConfigReady(config))
    }
}
