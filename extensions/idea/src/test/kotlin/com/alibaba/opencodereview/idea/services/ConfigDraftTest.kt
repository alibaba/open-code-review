package com.alibaba.opencodereview.idea.services

import com.alibaba.opencodereview.idea.model.ConfigEntry
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull
import kotlin.test.assertTrue

class ConfigDraftTest {

    private fun draft(base: String, vararg entries: Pair<String, String>): RawConfig =
        applyConfigEntries(parseRawConfig(base), entries.map { ConfigEntry(it.first, it.second) })

    private fun RawConfig.str(vararg path: String): String? {
        var cur: Any? = this[path.first()]
        for (key in path.drop(1)) {
            cur = (cur as? JsonObject)?.get(key)
        }
        return (cur as? JsonPrimitive)?.takeIf { it.isString }?.content
    }

    private fun RawConfig.obj(vararg path: String): JsonObject? {
        var cur: Any? = this[path.first()]
        for (key in path.drop(1)) {
            cur = (cur as? JsonObject)?.get(key)
        }
        return cur as? JsonObject
    }

    // ------------------------------------------------------------ provider

    @Test
    fun `切换 provider 会清掉顶层 model`() {
        val d = draft("""{"provider": "openai", "model": "gpt-5.5"}""", "provider" to "anthropic")
        assertEquals("anthropic", d.str("provider"))
        assertEquals("", d.str("model"))
    }

    @Test
    fun `内置 provider 会创建 providers 条目`() {
        val d = draft("{}", "provider" to "dashscope")
        assertEquals(JsonObject(emptyMap()), d.obj("providers", "dashscope"))
        assertNull(d.obj("custom_providers"))
    }

    @Test
    fun `非内置 provider 会创建 custom_providers 条目`() {
        val d = draft("{}", "provider" to "my-gateway")
        assertEquals(JsonObject(emptyMap()), d.obj("custom_providers", "my-gateway"))
        assertNull(d.obj("providers"))
    }

    @Test
    fun `provider 置空不创建任何条目`() {
        val d = draft("{}", "provider" to "")
        assertEquals("", d.str("provider"))
        assertNull(d.obj("providers"))
        assertNull(d.obj("custom_providers"))
    }

    @Test
    fun `已存在的 provider 条目不会被清空`() {
        val d = draft("""{"providers": {"openai": {"api_key": "k"}}}""", "provider" to "openai")
        assertEquals("k", d.str("providers", "openai", "api_key"))
    }

    // ------------------------------------------------------------ model

    @Test
    fun `选了内置 provider 时 model 写进该条目`() {
        val d = draft("""{"provider": "openai"}""", "model" to "gpt-5.5")
        assertEquals("gpt-5.5", d.str("providers", "openai", "model"))
        // 顶层 model 不应被写入——CLI 优先读取 provider 条目中的值。
        assertNull(d.str("model"))
    }

    @Test
    fun `选了自定义 provider 时 model 写进 custom_providers`() {
        val d = draft("""{"provider": "mine"}""", "model" to "m1")
        assertEquals("m1", d.str("custom_providers", "mine", "model"))
    }

    @Test
    fun `没选 provider 时 model 落在顶层`() {
        val d = draft("{}", "model" to "m1")
        assertEquals("m1", d.str("model"))
    }

    @Test
    fun `provider 和 model 同批写入时顺序决定归属`() {
        // setMany 的顺序契约：provider 先生效，model 才能确定写入哪个条目。
        val d = draft("{}", "provider" to "openai", "model" to "gpt-5.5")
        assertEquals("gpt-5.5", d.str("providers", "openai", "model"))
    }

    // ------------------------------------------------------------ providers.<name>.<field>

    @Test
    fun `providers 前缀按名字路由到内置或自定义容器`() {
        val preset = draft("{}", "providers.anthropic.api_key" to "sk-1")
        assertEquals("sk-1", preset.str("providers", "anthropic", "api_key"))

        val custom = draft("{}", "providers.mine.api_key" to "sk-2")
        assertEquals("sk-2", custom.str("custom_providers", "mine", "api_key"))
        assertNull(custom.obj("providers"))
    }

    @Test
    fun `custom_providers 前缀始终写自定义容器`() {
        // 即使名称与内置 provider 重名，显式前缀也不改变路由。
        val d = draft("{}", "custom_providers.anthropic.url" to "http://proxy")
        assertEquals("http://proxy", d.str("custom_providers", "anthropic", "url"))
    }

    @Test
    fun `段数不对的 key 被忽略`() {
        assertEquals(emptyRawConfig(), draft("{}", "providers.anthropic" to "x"))
        assertEquals(emptyRawConfig(), draft("{}", "providers.a.b.c" to "x"))
        assertEquals(emptyRawConfig(), draft("{}", "custom_providers.a" to "x"))
    }

    @Test
    fun `未知 provider 字段被忽略`() {
        assertEquals(JsonObject(emptyMap()), draft("{}", "providers.openai.nope" to "x").obj("providers", "openai"))
    }

    @Test
    fun `写字段时保留同条目的其他字段`() {
        val d = draft(
            """{"providers": {"openai": {"api_key": "k", "url": "u"}}}""",
            "providers.openai.model" to "gpt-5.5",
        )
        assertEquals("k", d.str("providers", "openai", "api_key"))
        assertEquals("u", d.str("providers", "openai", "url"))
        assertEquals("gpt-5.5", d.str("providers", "openai", "model"))
    }

    // ------------------------------------------------------------ models 列表

    @Test
    fun `models 支持 JSON 数组`() {
        assertEquals(listOf("a", "b"), parseModelList("""["a", "b"]"""))
    }

    @Test
    fun `models 支持逗号分隔并去空白`() {
        assertEquals(listOf("a", "b"), parseModelList(" a , b , "))
    }

    @Test
    fun `非法 JSON 数组回退到逗号分隔`() {
        // 用户输入 "[a,b" 这类不完整数组时不应报错，按逗号切分。
        assertEquals(listOf("[a", "b"), parseModelList("[a,b"))
    }

    @Test
    fun `models 空输入是空列表`() {
        assertEquals(emptyList(), parseModelList("   "))
        assertEquals(emptyList(), parseModelList("[]"))
    }

    @Test
    fun `models 写进条目是 JSON 数组`() {
        val d = draft("{}", "providers.openai.models" to "a,b")
        assertEquals(
            JsonArray(listOf(JsonPrimitive("a"), JsonPrimitive("b"))),
            d.obj("providers", "openai")!!["models"],
        )
    }

    // ------------------------------------------------------------ llm

    @Test
    fun `llm 五个字段都能写`() {
        val d = draft(
            "{}",
            "llm.url" to "http://x",
            "llm.auth_token" to "t",
            "llm.auth_header" to "h",
            "llm.model" to "m",
            "llm.use_anthropic" to "true",
        )
        assertEquals("http://x", d.str("llm", "url"))
        assertEquals("t", d.str("llm", "auth_token"))
        assertEquals("h", d.str("llm", "auth_header"))
        assertEquals("m", d.str("llm", "model"))
        assertEquals(JsonPrimitive(true), d.obj("llm")!!["use_anthropic"])
    }

    @Test
    fun `use_anthropic 只有字面量 true 算开启`() {
        assertEquals(JsonPrimitive(false), draft("{}", "llm.use_anthropic" to "false").obj("llm")!!["use_anthropic"])
        assertEquals(JsonPrimitive(false), draft("{}", "llm.use_anthropic" to "TRUE").obj("llm")!!["use_anthropic"])
        assertEquals(JsonPrimitive(false), draft("{}", "llm.use_anthropic" to "").obj("llm")!!["use_anthropic"])
    }

    // ------------------------------------------------------------ 保真 / 不写穿

    @Test
    fun `未知顶层字段在草稿里被保留`() {
        // 此为选择 JsonObject 树而非 data class 的理由：writeRaw 会整体重写文件，
        // 丢失字段即等于把用户配置中未被识别的键静默删除。
        val d = draft("""{"future_flag": {"deep": [1, 2]}}""", "provider" to "openai")
        assertEquals("""{"deep":[1,2]}""", d.obj("future_flag").toString())
    }

    @Test
    fun `原配置不被写穿`() {
        val base = parseRawConfig("""{"provider": "openai", "providers": {"openai": {"api_key": "k"}}}""")
        applyConfigEntries(base, listOf(ConfigEntry("providers.openai.api_key", "changed")))
        assertEquals("k", base.str("providers", "openai", "api_key"))
        assertEquals("openai", base.str("provider"))
    }

    @Test
    fun `未知顶层 key 被忽略`() {
        assertEquals(emptyRawConfig(), draft("{}", "totally.unknown.key.here" to "x"))
        assertEquals(emptyRawConfig(), draft("{}", "language" to "English"))
    }

    @Test
    fun `非法 JSON 配置文件当作空草稿`() {
        // 与 ConfigService.readRaw() 的兜底一致：解析失败即视为未配置，不应连带影响写操作。
        assertEquals(emptyRawConfig(), parseRawConfig("{ not json"))
        assertEquals(emptyRawConfig(), parseRawConfig(""))
    }

    @Test
    fun `序列化用两空格缩进`() {
        val json = draft("{}", "provider" to "openai").toPrettyJson()
        assertTrue(json.contains("\n  \"provider\""), json)
    }
}
