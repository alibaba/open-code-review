// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.services

import java.io.File
import java.nio.file.Files
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertFailsWith
import kotlin.test.assertTrue

class ConfigServiceNpmTest {
    @Test
    fun `npm entry points and native executable resolve and read the same existing configuration`() {
        val native = System.getenv("OCR_TEST_CLI")?.takeIf { it.isNotBlank() } ?: return
        val root = Files.createTempDirectory("ocr idea npm ").toFile().canonicalFile
        val cachedEnv = ShellEnv::class.java.getDeclaredField("cachedEnv").apply { isAccessible = true }
        val previousEnv = cachedEnv.get(ShellEnv)
        val previousQuoting = System.getProperty("jdk.lang.Process.allowAmbiguousCommands")
        try {
            if (File.separatorChar == '\\') System.setProperty("jdk.lang.Process.allowAmbiguousCommands", "false")
            val fixture = File("src/test/fixtures/npm-config-fixture.js").absoluteFile
            val result = captureConfigPathQuery(listOf(ShellEnv.resolveBin("node"), fixture.path, root.path, native),
                root, ShellEnv.env(), 60000)
            val values = Json.parseToJsonElement(result).jsonObject
            fun value(key: String) = values.getValue(key).jsonPrimitive.content
            val windows = File.separatorChar == '\\'
            val prefix = value("prefix")
            val inheritedPath = System.getenv().entries.firstOrNull { it.key.equals("PATH", ignoreCase = true) }?.value.orEmpty()
            val env = System.getenv().filterKeys { !it.equals("PATH", ignoreCase = true) } + mapOf(
                "PATH" to ((if (windows) prefix else File(prefix, "bin").path) + File.pathSeparator + inheritedPath),
                "OCR_HOME" to File(value("configPath")).parent,
                "HOME" to root.path, "USERPROFILE" to root.path,
                "OCR_NO_UPDATE" to "1", "OCR_ENABLE_TELEMETRY" to "1", "OTEL_EXPORTER_OTLP_ENDPOINT" to "",
            )
            cachedEnv.set(ShellEnv, env)
            assertTrue(File(value("launcher")).isFile)
            for (bin in listOf("ocr", value("launcher"), value("native"))) {
                val cli = CliService(bin)
                assertEquals(File(value("configPath")), cli.configPath(root), bin)
                assertEquals("saved-model", ConfigService(cli, root).read()?.model, bin)
            }
        } finally {
            cachedEnv.set(ShellEnv, previousEnv)
            if (previousQuoting == null) System.clearProperty("jdk.lang.Process.allowAmbiguousCommands")
            else System.setProperty("jdk.lang.Process.allowAmbiguousCommands", previousQuoting)
            root.deleteRecursively()
        }
    }

    @Test
    fun `Windows query resolves npm cmd from PATH and confines quoting to fixed arguments`() {
        val root = Files.createTempDirectory("ocr-query-command-").toFile()
        try {
            val launcher = File(root, "ocr.cmd").apply { writeText("@echo off") }
            assertFailsWith<CliException> {
                configPathInvocation("ocr", mapOf("Path" to ".;bin"), windows = true)
            }
            val invocation = configPathInvocation("ocr", mapOf("Path" to root.path), windows = true)
            assertEquals("\"${launcher.path}\"", invocation.env["OCR_CONFIG_PATH_COMMAND"])
            assertEquals(listOf("cmd.exe", "/d", "/v:off", "/s", "/c"), invocation.command.dropLast(1))
            assertEquals("\"%OCR_CONFIG_PATH_COMMAND% config path --json\"", invocation.command.last())
            val special = File(root, "OCR & tools %literal%!.cmd").absolutePath
            val quoted = configPathInvocation(special, emptyMap(), windows = true)
            assertEquals("\"$special\"", quoted.env["OCR_CONFIG_PATH_COMMAND"])
            assertFalse(quoted.command.any { it.contains(special) })
        } finally { root.deleteRecursively() }
    }
}
