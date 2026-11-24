// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.model

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/**
 * Keep both [HostStrings] tables consistent.
 *
 * Previously, eight shared keys had different Chinese values and seven shared keys were missing.
 * Both problems compiled without errors and had to be found by manual comparison.
 * Compare key sets and placeholders automatically wherever possible.
 */
class HostStringsTest {

    private val tables = HostStrings.tables()
    private val en = tables.getValue(SupportedLocale.EN)
    private val zh = tables.getValue(SupportedLocale.ZH_CN)

    /** The set of `{param}` placeholders. */
    private fun placeholders(template: String): Set<String> =
        Regex("\\{([a-zA-Z]+)}").findAll(template).map { it.groupValues[1] }.toSet()

    @Test
    fun `both locales have identical key sets`() {
        assertEquals(emptySet(), en.keys - zh.keys, "English-only keys (missing Chinese translations)")
        assertEquals(emptySet(), zh.keys - en.keys, "Chinese-only keys (English fallback would return the raw key)")
    }

    @Test
    fun `placeholders for each key match across locales`() {
        val mismatched = en.keys.filter { placeholders(en.getValue(it)) != placeholders(zh.getValue(it)) }
        assertEquals(emptyList(), mismatched, "Placeholder mismatch: switching locales could omit parameters or leave unresolved {xxx}")
    }

    @Test
    fun `all entries are non-empty and use the ext prefix`() {
        en.keys.forEach { key ->
            assertTrue(key.startsWith("ext."), "Host strings must use the 'ext.' prefix rather than the frontend 'view.' or 'cmp.' prefixes: $key")
        }
        (en + zh).forEach { (key, value) -> assertTrue(value.isNotBlank(), "$key has an empty value") }
    }

    @Test
    fun `t substitutes parameters without leaving placeholders`() {
        val rendered = HostStrings.t(SupportedLocale.ZH_CN, "ext.message.missingField", "type" to "setConfig", "field" to "key")
        assertEquals("setConfig 缺少 key", rendered) // allow-non-english: assertion verifies Chinese UI translations
        assertTrue(!rendered.contains('{'), "Unresolved placeholders remain: $rendered")
    }

    @Test
    fun `missing Chinese entries fall back to English then to the key itself`() {
        // Both tables are currently complete, so only the entirely missing key case can be checked here.
        assertEquals("ext.nope.notAKey", HostStrings.t(SupportedLocale.ZH_CN, "ext.nope.notAKey"))
    }

    @Test
    fun `shared entries use the original upstream values`() {
        // Spot-check entries prone to being rewritten; these values must exactly match the shared source.
        assertEquals("模型配置", HostStrings.t(SupportedLocale.ZH_CN, "ext.configPanelTitle")) // allow-non-english: assertion verifies Chinese UI translations
        assertEquals("Model Configuration", HostStrings.t(SupportedLocale.EN, "ext.configPanelTitle"))
        assertEquals("应用失败：代码位置已失效，请刷新后重试。", HostStrings.t(SupportedLocale.ZH_CN, "ext.comment.applyFailedStale")) // allow-non-english: assertion verifies Chinese UI translations
        assertEquals("仅工作区审查模式支持应用建议。", HostStrings.t(SupportedLocale.ZH_CN, "ext.comment.applyWorkspaceOnly")) // allow-non-english: assertion verifies Chinese UI translations
        assertEquals("✗ 安装失败 (exit 2)", HostStrings.t(SupportedLocale.ZH_CN, "ext.cli.installFail", "code" to "2")) // allow-non-english: assertion verifies Chinese UI translations
        assertEquals("✓ 安装完成", HostStrings.t(SupportedLocale.ZH_CN, "ext.cli.installOk")) // allow-non-english: assertion verifies Chinese UI translations
        assertEquals("确定删除自定义 Provider「Foo」？", HostStrings.t(SupportedLocale.ZH_CN, "ext.deleteProviderConfirm", "name" to "Foo")) // allow-non-english: assertion verifies Chinese UI translations
        assertEquals("无法跳转到 a.ts：未能解析行号。", HostStrings.t(SupportedLocale.ZH_CN, "ext.comment.jumpLineUnresolved", "path" to "a.ts")) // allow-non-english: assertion verifies Chinese UI translations
        assertEquals("⏳ [未处理]", HostStrings.t(SupportedLocale.ZH_CN, "ext.comment.pending")) // allow-non-english: assertion verifies Chinese UI translations
        assertEquals("✅ [已应用]", HostStrings.t(SupportedLocale.ZH_CN, "ext.comment.statusApplied")) // allow-non-english: assertion verifies Chinese UI translations
    }

    @Test
    fun `noSuggestion removes Markdown underscores`() {
        // The shared MarkdownString uses underscores for italics; a plain-text popup would display them literally.
        val zhText = HostStrings.t(SupportedLocale.ZH_CN, "ext.comment.noSuggestion")
        assertEquals("💡 无代码建议，请手动处理", zhText) // allow-non-english: assertion verifies Chinese UI translations
        assertTrue(!zhText.startsWith("_") && !zhText.endsWith("_"))
    }
}
