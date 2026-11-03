// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.services

import java.io.File
import java.nio.file.Files
import kotlin.concurrent.thread
import kotlin.test.Test
import kotlin.test.assertTrue

/**
 * cancel() must terminate the whole process tree: ocr is a Node launcher whose child is the actual Go binary.
 * Killing only the direct child can leave its descendants running as orphaned processes.
 */
class CliServiceTreeKillTest {

    private val isWindows = System.getProperty("os.name").orEmpty().startsWith("Windows", ignoreCase = true)

    @Test
    fun `cancel kills the whole process tree including grandchildren`() {
        if (isWindows) return // The bash launcher simulation is meaningful only on POSIX.

        val dir = Files.createTempDirectory("ocr-treekill-test").toFile()
        val pidFile = File(dir, "grandchild.pid")
        val script = File(dir, "fake-ocr.sh")
        // Simulate a launcher that spawns a grandchild, records its PID, and waits.
        script.writeText(
            """
            #!/bin/bash
            sleep 300 &
            echo ${'$'}! > "${pidFile.absolutePath}"
            wait
            """.trimIndent() + "\n",
        )
        check(script.setExecutable(true))

        val grandchildPid: Long
        try {
            val service = CliService(cliPath = script.absolutePath)
            val cancellation = CliCancellation()
            val runner = thread { runCatching { service.runRaw(emptyList(), dir, {}, cancellation = cancellation) } }

            // Wait for the grandchild to start.
            assertTrue(waitFor(pidFile::exists, 5_000), "The fake launcher did not write the grandchild PID in time")
            grandchildPid = pidFile.readText().trim().toLong()
            assertTrue(isAlive(grandchildPid), "The grandchild should be running")

            cancellation.cancel()

            // Allow the 3-second grace period plus scheduling time for cleanup of the captured descendants.
            assertTrue(
                waitFor({ !isAlive(grandchildPid) }, 8_000),
                "The grandchild survived cancel(): orphan process leak",
            )
            runner.join(5_000)
        } finally {
            // Do not leave sleep running even if the test fails.
            pidFile.takeIf { it.exists() }?.readText()?.trim()?.toLongOrNull()?.let { pid ->
                ProcessHandle.of(pid).ifPresent { it.destroyForcibly() }
            }
            dir.deleteRecursively()
        }
    }

    private fun isAlive(pid: Long): Boolean =
        ProcessHandle.of(pid).map { it.isAlive }.orElse(false)

    private fun waitFor(condition: () -> Boolean, timeoutMs: Long): Boolean {
        val deadline = System.currentTimeMillis() + timeoutMs
        while (System.currentTimeMillis() < deadline) {
            if (condition()) return true
            Thread.sleep(50)
        }
        return condition()
    }
}
