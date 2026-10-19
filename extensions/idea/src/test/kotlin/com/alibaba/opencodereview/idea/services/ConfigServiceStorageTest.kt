// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.services

import com.alibaba.opencodereview.idea.model.ConfigEntry
import kotlinx.serialization.json.JsonPrimitive
import java.io.File
import java.nio.file.Files
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

class ConfigServiceStorageTest {
    @Test
    fun `custom and relative storage stays pinned across reads writes and deletes`() {
        for (relative in listOf(false, true)) {
            withStorage(relative) { cli, config, path, root ->
                assertEquals("saved-model", config.read()?.customProviders?.get("custom")?.model)
                val other = File(root, "other-workspace").apply { mkdirs() }
                val fromOtherDirectory = ConfigService(cli, other)
                assertEquals("updated-model", fromOtherDirectory.set("model", "updated-model")?.customProviders?.get("custom")?.model)
                assertEquals(path.parent, cli.runRaw(listOf("check-home"), other, {}).trim())
                assertNull(fromOtherDirectory.deleteCustomProvider("custom"))
                assertFalse(path.exists())
                assertEquals("default-model", parseConfig(File(root, "default-home/.opencodereview/config.json").readText())?.model)
            }
        }
    }

    @Test
    fun `failed batch restores the snapshot in the selected storage`() {
        withStorage { cli, config, path, root ->
            val saved = parseRawConfig(path.readText())
            val other = File(root, "other-workspace").apply { mkdirs() }
            assertEquals("saved-model", config.read()?.customProviders?.get("custom")?.model)
            assertFailsWith<CliException> {
                ConfigService(cli, other).setMany(listOf(ConfigEntry("model", "changed-model"), ConfigEntry("invalid", "value")))
            }
            assertEquals(saved, parseRawConfig(path.readText()))
            assertEquals("saved-model", config.read()?.customProviders?.get("custom")?.model)
        }
    }

    @Test
    fun `draft connection uses temporary OCR_HOME instead of pinned saved config`() {
        withStorage { cli, config, path, root ->
            val saved = path.readText()
            assertEquals("saved-model", config.read()?.customProviders?.get("custom")?.model)
            assertTrue(config.testWithEntries(listOf(ConfigEntry("model", "draft-model"))).first)
            assertEquals(saved, path.readText())
            val testedHome = File(root, "tested-home").readText()
            assertFalse(File(testedHome).exists(), "Temporary draft storage was not removed")
            assertEquals(path.parent, cli.runRaw(listOf("check-home"), root, {}).trim())
        }
    }

    private fun withStorage(
        relative: Boolean = true,
        block: (CliService, ConfigService, File, File) -> Unit,
    ) {
        if (File.separatorChar == '\\') return
        val root = Files.createTempDirectory("ocr-config-storage-").toFile().canonicalFile
        val cwd = File(root, "workspace").apply { mkdirs() }
        val storage = File(cwd, "selected storage").apply { mkdirs() }
        val path = File(storage, "config.json")
        path.writeText("""{"provider":"custom","custom_providers":{"custom":{"model":"saved-model"}}}""")
        val defaultHome = File(root, "default-home")
        File(defaultHome, ".opencodereview").mkdirs()
        File(defaultHome, ".opencodereview/config.json").writeText("""{"model":"default-model"}""")
        val override = if (relative) storage.name else storage.absolutePath
        val fixture = File(root, "fake-ocr.sh")
        fixture.writeText(
            """
            #!/bin/sh
            set -eu
            if [ "${'$'}1 ${'$'}{2-}" = 'config path' ]; then
                [ "${'$'}OCR_HOME" = '$override' ]
                [ "${'$'}PWD" = '${cwd.path}' ]
                printf '%s\n' '{"path":${JsonPrimitive(path.path)}}'
            elif [ "${'$'}1 ${'$'}{2-}" = 'config set' ]; then
                [ "${'$'}OCR_HOME" = '${storage.path}' ]
                if [ "${'$'}3" != 'model' ]; then
                    echo 'Error: invalid config key' >&2
                    exit 1
                fi
                printf '{"provider":"custom","custom_providers":{"custom":{"model":"%s"}}}' "${'$'}4" > "${'$'}OCR_HOME/config.json"
            elif [ "${'$'}1 ${'$'}{2-}" = 'llm test' ]; then
                [ "${'$'}OCR_HOME" = "${'$'}HOME/.opencodereview" ]
                [ "${'$'}USERPROFILE" = "${'$'}HOME" ]
                [ "${'$'}OCR_HOME" != '${storage.path}' ]
                grep -q 'draft-model' "${'$'}OCR_HOME/config.json"
                printf '%s' "${'$'}HOME" > '${File(root, "tested-home").path}'
            elif [ "${'$'}1" = 'check-home' ]; then
                [ "${'$'}OCR_HOME" = '${storage.path}' ]
                printf '%s\n' "${'$'}OCR_HOME"
            else
                exit 1
            fi
            """.trimIndent() + "\n",
        )
        check(fixture.setExecutable(true))
        val cachedEnv = ShellEnv::class.java.getDeclaredField("cachedEnv").apply { isAccessible = true }
        val previousEnv = cachedEnv.get(ShellEnv)
        val previousHome = System.getProperty("user.home")
        try {
            cachedEnv.set(ShellEnv, System.getenv() + mapOf(
                "HOME" to defaultHome.path,
                "USERPROFILE" to defaultHome.path,
                "OCR_HOME" to override,
            ))
            System.setProperty("user.home", defaultHome.path)
            val cli = CliService(fixture.path)
            block(cli, ConfigService(cli, cwd), path, root)
        } finally {
            cachedEnv.set(ShellEnv, previousEnv)
            System.setProperty("user.home", previousHome)
            root.deleteRecursively()
        }
    }
}
