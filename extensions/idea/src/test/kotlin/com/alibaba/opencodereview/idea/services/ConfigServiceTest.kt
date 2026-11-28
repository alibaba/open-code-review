// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.services

import com.alibaba.opencodereview.idea.model.HostStrings
import com.alibaba.opencodereview.idea.model.currentIdeLocale
import java.io.File
import org.junit.Rule
import org.junit.rules.TemporaryFolder
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertFalse

class ConfigServiceTest {
    @get:Rule
    val temp = TemporaryFolder()

    private fun deleteLocalProvider(config: String): File {
        val file = temp.newFile("config.json")
        file.writeText(config)
        ConfigService(CliService(), configFile = file).deleteCustomProvider("local")
        return file
    }

    @Test
    fun `deleting the last active provider preserves unrelated settings`() {
        val limits = """{"amounts":[0.1234567890123456789,18446744073709551617]}"""
        val file = deleteLocalProvider(
            """
            {
              "provider": "local", "model": "code", "custom_providers": {"local": {}},
              "language": "English", "mcp_servers": {"docs": {"command": "docs"}},
              "telemetry": {"enabled": false}, "limits": $limits
            }
            """.trimIndent(),
        )
        assertEquals(
            parseRawConfig("""{"language":"English","mcp_servers":{"docs":{"command":"docs"}},"telemetry":{"enabled":false},"limits":$limits}"""),
            parseRawConfig(file.readText()),
        )
    }

    @Test
    fun `an unknown false setting alone keeps the config file`() {
        val file = deleteLocalProvider("""{"custom_providers":{"local":{}},"future_option":false}""")
        assertEquals(parseRawConfig("""{"future_option":false}"""), parseRawConfig(file.readText()))
    }

    @Test
    fun `deleting an inactive provider preserves the current provider and model`() {
        val file = deleteLocalProvider("""{"provider":"openai","model":"current","custom_providers":{"local":{}},"LLM":{"TimeoutSec":90}}""")
        assertEquals(parseRawConfig("""{"provider":"openai","model":"current","LLM":{"TimeoutSec":90}}"""), parseRawConfig(file.readText()))
    }

    @Test
    fun `deleting a provider leaves ambiguous configuration keys untouched`() {
        val config = temp.newFile("config.json")
        val service = ConfigService(CliService(), configFile = config)
        val locale = currentIdeLocale()
        for ((original, message) in listOf(
            """{"custom_providers":{"keep":{}},"custom_providers":{"local":{}}}""" to
                HostStrings.t(locale, "ext.config.invalidProviderJson"),
            """{"Provider":"local","custom_providers":{"local":{}}}""" to
                HostStrings.t(locale, "ext.config.renameKey", "key" to "Provider", "canonical" to "provider"),
        )) {
            config.writeText(original)
            val failure = assertFailsWith<IllegalArgumentException> { service.deleteCustomProvider("local") }
            assertEquals(message, failure.message)
            assertEquals(original, config.readText())
        }
    }

    @Test
    fun `deleting an unnamed provider preserves the top-level model`() {
        val file = temp.newFile("config.json")
        file.writeText("""{"provider":"","model":"saved","Model":"legacy","custom_providers":{"":{},"keep":{}}}""")
        ConfigService(CliService(), configFile = file).deleteCustomProvider("")
        assertEquals(
            parseRawConfig("""{"provider":"","model":"saved","Model":"legacy","custom_providers":{"keep":{}}}"""),
            parseRawConfig(file.readText()),
        )
    }

    @Test
    fun `deleting the last provider removes a config with only empty provider fields`() {
        val file = deleteLocalProvider("""{"provider":"","model":"","providers":{},"llm":{},"custom_providers":{"local":{}}}""")
        assertFalse(file.exists())
    }
}
