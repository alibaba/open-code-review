package com.alibaba.opencodereview.idea.model

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/**
 * 守住 [HostStrings] 两张表的一致性。
 *
 * 此组用例的来由：先前词条表中 8 个 key 沿用了外部共享词条的 key 名，值却为重写的中文，
 * 另有 7 个外部共享中存在的 key 整条遗漏；上述两种问题均不会导致编译失败，只能依靠人工逐条比对发现。
 * key 集合与占位符可自动比对的，即自动比对。
 */
class HostStringsTest {

    private val tables = HostStrings.tables()
    private val en = tables.getValue(SupportedLocale.EN)
    private val zh = tables.getValue(SupportedLocale.ZH_CN)

    /** `{param}` 形式的占位符集合。 */
    private fun placeholders(template: String): Set<String> =
        Regex("\\{([a-zA-Z]+)}").findAll(template).map { it.groupValues[1] }.toSet()

    @Test
    fun `两种语言的 key 集合完全一致`() {
        assertEquals(emptySet(), en.keys - zh.keys, "只有英文有的 key（漏了中文翻译）")
        assertEquals(emptySet(), zh.keys - en.keys, "只有中文有的 key（英文兜底会退成 key 原文）")
    }

    @Test
    fun `同一个 key 的占位符在两种语言里一致`() {
        val mismatched = en.keys.filter { placeholders(en.getValue(it)) != placeholders(zh.getValue(it)) }
        assertEquals(emptyList(), mismatched, "占位符对不上——换语言之后会漏参数或留下未替换的 {xxx}")
    }

    @Test
    fun `所有词条都非空且都以 ext 前缀开头`() {
        en.keys.forEach { key ->
            assertTrue(key.startsWith("ext."), "宿主侧词条必须用 ext. 前缀，与前端的 view./cmp. 分开：$key")
        }
        (en + zh).forEach { (key, value) -> assertTrue(value.isNotBlank(), "$key 的值是空的") }
    }

    @Test
    fun `t 会做 param 替换且不留下未替换的占位符`() {
        val rendered = HostStrings.t(SupportedLocale.ZH_CN, "ext.message.missingField", "type" to "setConfig", "field" to "key")
        assertEquals("setConfig 缺少 key", rendered)
        assertTrue(!rendered.contains('{'), "还有没替换掉的占位符：$rendered")
    }

    @Test
    fun `中文缺词条时退到英文，两边都没有就返回 key 本身`() {
        // 当前两张表已齐，因此此处仅能验证"完全不存在的 key"这一档。
        assertEquals("ext.nope.notAKey", HostStrings.t(SupportedLocale.ZH_CN, "ext.nope.notAKey"))
    }

    @Test
    fun `上游已有的词条用的是上游原值`() {
        // 抽查最容易被改写的几条——这些值必须逐字等于外部共享词条源中的对应值。
        assertEquals("模型配置", HostStrings.t(SupportedLocale.ZH_CN, "ext.configPanelTitle"))
        assertEquals("Model Configuration", HostStrings.t(SupportedLocale.EN, "ext.configPanelTitle"))
        assertEquals("应用失败：代码位置已失效，请刷新后重试。", HostStrings.t(SupportedLocale.ZH_CN, "ext.comment.applyFailedStale"))
        assertEquals("仅工作区审查模式支持应用建议。", HostStrings.t(SupportedLocale.ZH_CN, "ext.comment.applyWorkspaceOnly"))
        assertEquals("✗ 安装失败 (exit 2)", HostStrings.t(SupportedLocale.ZH_CN, "ext.cli.installFail", "code" to "2"))
        assertEquals("✓ 安装完成", HostStrings.t(SupportedLocale.ZH_CN, "ext.cli.installOk"))
        assertEquals("确定删除自定义 Provider「Foo」？", HostStrings.t(SupportedLocale.ZH_CN, "ext.deleteProviderConfirm", "name" to "Foo"))
        assertEquals("无法跳转到 a.ts：未能解析行号。", HostStrings.t(SupportedLocale.ZH_CN, "ext.comment.jumpLineUnresolved", "path" to "a.ts"))
        assertEquals("⏳ [未处理]", HostStrings.t(SupportedLocale.ZH_CN, "ext.comment.pending"))
        assertEquals("✅ [已应用]", HostStrings.t(SupportedLocale.ZH_CN, "ext.comment.statusApplied"))
    }

    @Test
    fun `noSuggestion 去掉了 markdown 的下划线`() {
        // 外部共享中为 MarkdownString，值用一对 `_` 包裹以表示斜体；此处为纯文本 popup，包裹会原样显示。
        val zhText = HostStrings.t(SupportedLocale.ZH_CN, "ext.comment.noSuggestion")
        assertEquals("💡 无代码建议，请手动处理", zhText)
        assertTrue(!zhText.startsWith("_") && !zhText.endsWith("_"))
    }
}
