// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea

import java.util.Properties
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/**
 * Every string key declared in `plugin.xml` must exist in both `OcrBundle` files.
 *
 * Automate this check: once `<resource-bundle>` is declared, action labels come entirely from properties.
 * Misspelled keys or missing translations do not cause IDEA errors; the Tools menu instead shows
 * a raw key such as `action.OpenCodeReview.OpenToolWindow.text`, or falls back to English.
 * Relying on manual `runIde` checks can let these problems go unnoticed until after release.
 */
class OcrBundleTest {

    private fun load(name: String): Properties {
        val stream = javaClass.getResourceAsStream("/messages/$name")
            ?: error("Resource is not on the classpath: /messages/$name")
        return stream.use { Properties().apply { load(it.reader(Charsets.UTF_8)) } }
    }

    private val en = load("OcrBundle.properties")
    private val zh = load("OcrBundle_zh_CN.properties")

    private val pluginXml: String by lazy {
        javaClass.getResourceAsStream("/META-INF/plugin.xml")
            ?.use { it.reader(Charsets.UTF_8).readText() }
            ?: error("Cannot read plugin.xml")
    }

    @Test
    fun `both bundles have identical key sets`() {
        assertEquals(emptySet<Any?>(), en.keys - zh.keys, "English-only keys")
        assertEquals(emptySet<Any?>(), zh.keys - en.keys, "Chinese-only keys")
    }

    @Test
    fun `every action in the plugin descriptor has a corresponding string key`() {
        val ids = Regex("""<action\s+id="([^"]+)"""").findAll(pluginXml).map { it.groupValues[1] }.toList()
        assertTrue(ids.isNotEmpty(), "No action ids matched in plugin.xml; update this test")
        ids.forEach { id ->
            assertTrue(en.containsKey("action.$id.text"), "Missing action.$id.text (English)")
            assertTrue(zh.containsKey("action.$id.text"), "Missing action.$id.text (Chinese)")
        }
    }

    @Test
    fun `actions in the plugin descriptor must not have a text attribute`() {
        // A text= attribute overrides the bundle key, silently disabling localization.
        val actionBlocks = Regex("""<action\b[^>]*>""", RegexOption.DOT_MATCHES_ALL).findAll(pluginXml)
        actionBlocks.forEach { block ->
            assertTrue(!block.value.contains("text="), "The action text= attribute overrides OcrBundle: ${block.value}")
        }
    }

    @Test
    fun `every toolWindow has a stripe title key`() {
        val ids = Regex("""<toolWindow\s+id="([^"]+)"""").findAll(pluginXml).map { it.groupValues[1] }.toList()
        assertTrue(ids.isNotEmpty(), "No toolWindow ids matched in plugin.xml; update this test")
        ids.forEach { id ->
            // The platform lookup replaces spaces in the id with underscores.
            val key = "toolwindow.stripe.${id.replace(' ', '_')}"
            assertTrue(en.containsKey(key), "Missing $key (English)")
            assertTrue(zh.containsKey(key), "Missing $key (Chinese)")
        }
    }

    @Test
    fun `all string values are non-empty`() {
        listOf("English" to en, "Chinese" to zh).forEach { (label, props) ->
            props.forEach { (key, value) ->
                assertTrue(value.toString().isNotBlank(), "$key has an empty value in $label")
            }
        }
    }
}
