package com.alibaba.opencodereview.idea

import java.util.Properties
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/**
 * `plugin.xml` 声明的文案 key 必须在两份 `OcrBundle` 中都存在。
 *
 * 此项必须自动校验：`<resource-bundle>` 一旦声明，action 的文案即完全由 properties 提供，
 * key 拼写错误或漏掉某一门语言时，IDEA 不会报错——Tools 菜单里会直接显示
 * `action.OpenCodeReview.OpenToolWindow.text` 这种原始 key，或退回英文。
 * 而此项目目前只能通过 `runIde` 人工观察，待发现时插件已经发布出去。
 */
class OcrBundleTest {

    private fun load(name: String): Properties {
        val stream = javaClass.getResourceAsStream("/messages/$name")
            ?: error("资源不在 classpath 上：/messages/$name")
        return stream.use { Properties().apply { load(it.reader(Charsets.UTF_8)) } }
    }

    private val en = load("OcrBundle.properties")
    private val zh = load("OcrBundle_zh_CN.properties")

    private val pluginXml: String by lazy {
        javaClass.getResourceAsStream("/META-INF/plugin.xml")
            ?.use { it.reader(Charsets.UTF_8).readText() }
            ?: error("读不到 plugin.xml")
    }

    @Test
    fun `两份 bundle 的 key 集合完全一致`() {
        assertEquals(emptySet<Any?>(), en.keys - zh.keys, "只有英文有的 key")
        assertEquals(emptySet<Any?>(), zh.keys - en.keys, "只有中文有的 key")
    }

    @Test
    fun `plugin_xml 里每个 action 都有对应的文案 key`() {
        val ids = Regex("""<action\s+id="([^"]+)"""").findAll(pluginXml).map { it.groupValues[1] }.toList()
        assertTrue(ids.isNotEmpty(), "没从 plugin.xml 里正则出 action id，用例本身需要更新")
        ids.forEach { id ->
            assertTrue(en.containsKey("action.$id.text"), "缺 action.$id.text（英文）")
            assertTrue(zh.containsKey("action.$id.text"), "缺 action.$id.text（中文）")
        }
    }

    @Test
    fun `plugin_xml 里的 action 不能自带 text 属性`() {
        // 自带 text= 会覆盖 bundle 里的 key，本地化直接失效且不会有任何提示。
        val actionBlocks = Regex("""<action\b[^>]*>""", RegexOption.DOT_MATCHES_ALL).findAll(pluginXml)
        actionBlocks.forEach { block ->
            assertTrue(!block.value.contains("text="), "action 自带了 text= 属性，会盖掉 OcrBundle：${block.value}")
        }
    }

    @Test
    fun `每个 toolWindow 都有 stripe 标题 key`() {
        val ids = Regex("""<toolWindow\s+id="([^"]+)"""").findAll(pluginXml).map { it.groupValues[1] }.toList()
        assertTrue(ids.isNotEmpty(), "没从 plugin.xml 里正则出 toolWindow id，用例本身需要更新")
        ids.forEach { id ->
            // 平台查找规则：id 中的空格替换为下划线。
            val key = "toolwindow.stripe.${id.replace(' ', '_')}"
            assertTrue(en.containsKey(key), "缺 $key（英文）")
            assertTrue(zh.containsKey(key), "缺 $key（中文）")
        }
    }

    @Test
    fun `文案值都不为空`() {
        listOf("英文" to en, "中文" to zh).forEach { (label, props) ->
            props.forEach { (key, value) ->
                assertTrue(value.toString().isNotBlank(), "$label 的 $key 是空值")
            }
        }
    }
}
