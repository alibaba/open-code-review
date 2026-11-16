// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.services

import java.io.File
import java.nio.file.Files
import java.util.concurrent.TimeUnit
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertFalse
import kotlin.test.assertTrue
import kotlin.test.assertNull

class ConfigPathQueryTest {
    @Test
    fun `old CLI is distinct from missing config and upgrade failures never change saved data`() {
        if (File.separatorChar == '\\') return
        val root = Files.createTempDirectory("ocr-old-cli-").toFile()
        try {
            val saved = File(root, "config.json").apply { writeText("""{"model":"saved-model"}""") }
            val fixture = File(root, "ocr").apply {
                writeText("""
                    #!/bin/sh
                    if [ "${'$'}1" = '--version' ]; then echo v1.0.0; exit 0; fi
                    echo 'Error: unknown command path for ocr config' >&2
                    exit 1
                """.trimIndent() + "\n")
                check(setExecutable(true))
            }
            val cli = CliService(fixture.path)
            assertEquals("v1.0.0", captureConfigPathQuery(listOf(fixture.path, "--version"), root, System.getenv(), 3000).trim())
            val failure = assertFailsWith<CliUpgradeRequiredException> { cli.configPath(root) }
            val status = configPathFailure(failure)
            assertTrue(status.upgradeRequired)
            assertTrue(status.message.orEmpty().contains("npm install -g"))
            val config = ConfigService(cli, root)
            assertFailsWith<CliUpgradeRequiredException> { config.read() }
            assertFailsWith<CliUpgradeRequiredException> { config.set("model", "wrong-model") }
            assertEquals("""{"model":"saved-model"}""", saved.readText())
            fixture.writeText("#!/bin/sh\nprintf '%s' '{\"path\":\"${File(root, "missing.json").path}\"}'\n")
            assertNull(ConfigService(CliService(fixture.path), root).read())
        } finally { root.deleteRecursively() }
    }

    @Test
    fun `hung query kills its launcher and child`() = withFixture { root, command ->
        val error = assertFailsWith<CliException> {
            captureConfigPathQuery(command + "hang", root, System.getenv(), 1500)
        }
        assertTrue(error.message.orEmpty().contains("timed out"))
        val pid = File(root, "child.pid").readText().trim().toLong()
        val deadline = System.nanoTime() + TimeUnit.SECONDS.toNanos(3)
        while (ProcessHandle.of(pid).map { it.isAlive }.orElse(false) && System.nanoTime() < deadline) Thread.sleep(20)
        assertFalse(ProcessHandle.of(pid).map { it.isAlive }.orElse(false))
    }

    @Test
    fun `failed and oversized queries do not poison later successful queries`() = withFixture { root, command ->
        val failed = assertFailsWith<CliException> { captureConfigPathQuery(command + "fail", root, System.getenv(), 3000) }
        assertFalse(configPathFailure(failed).upgradeRequired)
        assertFailsWith<CliException> { captureConfigPathQuery(command + "large", root, System.getenv(), 3000) }
        assertEquals("result", captureConfigPathQuery(command + "ok", root, System.getenv(), 3000))
    }

    @Test
    fun `concurrent callers share an asynchronous resolution and cache success`() {
        if (File.separatorChar == '\\') return
        val root = Files.createTempDirectory("ocr-query-sharing-").toFile()
        try {
            val fixture = File(root, "ocr")
            val path = File(root, "config.json")
            fixture.writeText("""
                #!/bin/sh
                echo query >> '${File(root, "calls").path}'
                sleep 0.3
                printf '%s' '{"path":"${path.path}"}'
            """.trimIndent() + "\n")
            check(fixture.setExecutable(true))
            val cli = CliService(fixture.path)
            val first = cli.configPathAsync(root)
            val second = cli.configPathAsync(root)
            assertFalse(first.isDone)
            assertEquals(path, first.get(5, TimeUnit.SECONDS))
            assertEquals(path, second.get(5, TimeUnit.SECONDS))
            assertEquals(path, cli.configPath(root))
            assertEquals(listOf("query"), File(root, "calls").readLines())
        } finally { root.deleteRecursively() }
    }

    private fun withFixture(block: (File, List<String>) -> Unit) {
        val root = Files.createTempDirectory("ocr-query-process-").toFile()
        try {
            val source = File(root, "QueryFixture.java")
            source.writeText("""
                // SPDX-License-Identifier: Apache-2.0
                import java.nio.file.*;
                public class QueryFixture {
                    public static void main(String[] args) throws Exception {
                        if (args[0].equals("hang")) {
                            Process child = new ProcessBuilder(ProcessHandle.current().info().command().orElseThrow(), "-cp", System.getProperty("java.class.path"), "QueryFixture", "child").start();
                            Files.writeString(Path.of("child.pid"), Long.toString(child.pid()));
                            Thread.sleep(60000);
                        } else if (args[0].equals("child")) Thread.sleep(60000);
                        else if (args[0].equals("large")) System.out.print("x".repeat(70000));
                        else if (args[0].equals("fail")) { System.err.print("Error: unavailable"); System.exit(1); }
                        else System.out.print("result");
                    }
                }
            """.trimIndent())
            val javaHome = System.getenv("JAVA_HOME") ?: System.getProperty("java.home")
            val suffix = if (File.separatorChar == '\\') ".exe" else ""
            val compiler = ProcessBuilder(File(javaHome, "bin/javac$suffix").path, source.path).inheritIO().start()
            assertTrue(compiler.waitFor(30, TimeUnit.SECONDS))
            assertEquals(0, compiler.exitValue())
            val java = File(javaHome, "bin/java$suffix").path
            block(root, listOf(java, "-cp", root.path, "QueryFixture"))
        } finally {
            File(root, "child.pid").takeIf { it.exists() }?.readText()?.trim()?.toLongOrNull()?.let { pid ->
                ProcessHandle.of(pid).ifPresent { it.destroyForcibly() }
            }
            root.deleteRecursively()
        }
    }
}
