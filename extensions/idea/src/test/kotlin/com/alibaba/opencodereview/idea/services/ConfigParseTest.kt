package com.alibaba.opencodereview.idea.services

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNull
import kotlin.test.assertTrue

class ConfigParseTest {

    @Test
    fun `空内容返回 null`() {
        assertNull(parseConfig(""))
        assertNull(parseConfig("   \n  "))
    }

    @Test
    fun `snake_case 配置文件转成 camelCase 领域模型`() {
        val cfg = parseConfig(
            """
            {
              "provider": "anthropic",
              "model": "claude-opus-4-8",
              "language": "English",
              "providers": {
                "anthropic": {
                  "api_key": "sk-x",
                  "url": "https://api.anthropic.com",
                  "protocol": "anthropic",
                  "model": "claude-opus-4-8",
                  "auth_header": "x-api-key",
                  "models": ["a", "b"]
                }
              },
              "custom_providers": {
                "mine": { "api_key": "k", "url": "http://localhost" }
              },
              "llm": {
                "url": "http://llm",
                "auth_token": "t",
                "model": "m",
                "auth_header": "h"
              }
            }
            """.trimIndent(),
        )!!

        assertEquals("anthropic", cfg.provider)
        assertEquals("English", cfg.language)

        val preset = cfg.providers.getValue("anthropic")
        // 这些断言是字段名错误类缺陷的哨兵：字段名若为 snake_case，前端只会渲染空白。
        assertEquals("sk-x", preset.apiKey)
        assertEquals("x-api-key", preset.authHeader)
        assertEquals(listOf("a", "b"), preset.models)

        val custom = cfg.customProviders.getValue("mine")
        assertEquals("k", custom.apiKey)
        assertEquals("", custom.protocol)

        assertEquals("http://llm", cfg.llm.url)
        assertEquals("t", cfg.llm.authToken)
        assertEquals("h", cfg.llm.authHeader)
    }

    @Test
    fun `缺失字段用空串兜底而不是抛异常`() {
        val cfg = parseConfig("{}")!!
        assertEquals("", cfg.provider)
        assertEquals("", cfg.model)
        assertEquals(emptyMap(), cfg.providers)
        assertEquals(emptyMap(), cfg.customProviders)
        assertEquals("", cfg.llm.url)
    }

    @Test
    fun `language 缺失时默认 Chinese`() {
        assertEquals("Chinese", parseConfig("{}")!!.language)
        assertEquals("Chinese", parseConfig("""{"language": ""}""")!!.language)
        assertEquals("English", parseConfig("""{"language": "English"}""")!!.language)
    }

    @Test
    fun `use_anthropic 只有显式布尔 false 才关闭`() {
        assertTrue(parseConfig("{}")!!.llm.useAnthropic)
        assertTrue(parseConfig("""{"llm": {}}""")!!.llm.useAnthropic)
        assertTrue(parseConfig("""{"llm": {"use_anthropic": true}}""")!!.llm.useAnthropic)
        assertEquals(false, parseConfig("""{"llm": {"use_anthropic": false}}""")!!.llm.useAnthropic)
    }

    @Test
    fun `字符串 false 不算关闭`() {
        // JS 侧 "false" !== false 为真，因此仍视为启用；
        // 若此处使用 booleanOrNull 会误判为 false，导致两端行为分叉。
        assertTrue(parseConfig("""{"llm": {"use_anthropic": "false"}}""")!!.llm.useAnthropic)
    }

    @Test
    fun `models 缺失是 null 而不是空列表`() {
        // null 表示"未配置"，前端将退回 preset 的模型列表；
        // 空列表表示"已配置但无项"，会渲染为空下拉框。
        val entry = parseConfig("""{"providers": {"anthropic": {}}}""")!!.providers.getValue("anthropic")
        assertNull(entry.models)
    }

    @Test
    fun `models 里的非字符串元素被过滤掉`() {
        val entry = parseConfig("""{"providers": {"p": {"models": ["a", 1, null, "b"]}}}""")!!
            .providers.getValue("p")
        assertEquals(listOf("a", "b"), entry.models)
    }

    @Test
    fun `字段类型不对时当作缺失`() {
        val cfg = parseConfig("""{"provider": 123, "model": true, "providers": []}""")!!
        assertEquals("", cfg.provider)
        assertEquals("", cfg.model)
        assertEquals(emptyMap(), cfg.providers)
    }

    @Test
    fun `未知顶层字段不影响解析`() {
        val cfg = parseConfig("""{"provider": "openai", "future_flag": {"deep": [1]}}""")!!
        assertEquals("openai", cfg.provider)
    }

    @Test
    fun `非法 JSON 向上抛出`() {
        // 由 ConfigService.read() 捕获后视为"无配置"，解析层自身不吞异常。
        assertFailsWith<Exception> { parseConfig("{ not json") }
    }

    @Test
    fun `config set 参数顺序固定`() {
        assertEquals(listOf("config", "set", "provider", "openai"), toConfigSetArgs("provider", "openai"))
    }
}
