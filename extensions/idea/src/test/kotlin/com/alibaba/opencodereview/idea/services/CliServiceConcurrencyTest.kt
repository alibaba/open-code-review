// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.services

import java.io.File
import java.nio.file.Files
import java.util.concurrent.Executors
import java.util.concurrent.ExecutionException
import java.util.concurrent.TimeUnit
import java.util.concurrent.TimeoutException
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertFailsWith
import kotlin.test.assertTrue

class CliServiceConcurrencyTest {
    @Test
    fun `cancelling a review leaves a concurrent configuration command alive`() {
        withCli { cli, fixture, executor ->
            val ready = File(fixture.parentFile, "ready")
            val configReady = File(fixture.parentFile, "config-ready")
            val release = File(fixture.parentFile, "release")
            val cancellation = CliCancellation()
            val review = executor.submit<String> {
                cli.runRaw(listOf(fixture.path, ready.path, release.path), fixture.parentFile, {}, cancellation = cancellation)
            }
            assertTrue(waitUntil { ready.exists() })
            val config = executor.submit<String> {
                cli.runRaw(listOf(fixture.path, configReady.path, release.path), fixture.parentFile, {})
            }
            assertTrue(waitUntil { configReady.exists() })
            try {
                cancellation.cancel()
                assertFailsWith<ExecutionException> { review.get(10, TimeUnit.SECONDS) }
                assertFalse(config.isDone, "Cancelling review terminated the configuration command")
            } finally {
                release.createNewFile()
            }
            assertEquals("finished", config.get(15, TimeUnit.SECONDS).trim())
        }
    }

    @Test
    fun `a completed configuration command does not lose review cancellation`() {
        withCli { cli, fixture, executor ->
            val ready = File(fixture.parentFile, "ready")
            val release = File(fixture.parentFile, "release")
            val cancellation = CliCancellation()
            val review = executor.submit<String> {
                cli.runRaw(listOf(fixture.path, ready.path, release.path), fixture.parentFile, {}, cancellation = cancellation)
            }
            assertTrue(waitUntil { ready.exists() })
            assertEquals("finished", cli.runRaw(listOf(fixture.path), fixture.parentFile, {}).trim())
            cancellation.cancel()
            assertFailsWith<ExecutionException> { review.get(10, TimeUnit.SECONDS) }
        }
    }

    @Test
    fun `old cancellation and delayed cleanup cannot terminate a new review`() {
        withCli { cli, fixture, executor ->
            val ready = File(fixture.parentFile, "ready")
            val newReady = File(fixture.parentFile, "new-ready")
            val release = File(fixture.parentFile, "release")
            val oldCancellation = CliCancellation()
            val oldReview = executor.submit<String> {
                cli.runRaw(listOf(fixture.path, ready.path, release.path), fixture.parentFile, {}, cancellation = oldCancellation)
            }
            assertTrue(waitUntil { ready.exists() })
            oldCancellation.cancel()
            val newReview = executor.submit<String> {
                cli.runRaw(listOf(fixture.path, newReady.path, release.path), fixture.parentFile, {}, cancellation = CliCancellation())
            }
            assertTrue(waitUntil { newReady.exists() })
            try {
                oldCancellation.cancel()
                assertFailsWith<ExecutionException> { oldReview.get(10, TimeUnit.SECONDS) }
                assertFailsWith<TimeoutException> { newReview.get(4, TimeUnit.SECONDS) }
            } finally {
                release.createNewFile()
            }
            assertEquals("finished", newReview.get(15, TimeUnit.SECONDS).trim())
        }
    }

    @Test
    fun `cancellation before execution prevents process startup`() {
        withCli { cli, fixture, _ ->
            val ready = File(fixture.parentFile, "ready")
            val release = File(fixture.parentFile, "release")
            val cancellation = CliCancellation().apply { cancel() }
            assertFailsWith<java.util.concurrent.CancellationException> {
                cli.runRaw(listOf(fixture.path, ready.path, release.path), fixture.parentFile, {}, cancellation = cancellation)
            }
            assertFalse(ready.exists())
        }
    }

    @Test
    fun `a short command does not terminate an ongoing command`() {
        withCli { cli, fixture, executor ->
            val ready = File(fixture.parentFile, "ready")
            val release = File(fixture.parentFile, "release")
            val running = executor.submit<String> {
                cli.runRaw(listOf(fixture.path, ready.path, release.path), fixture.parentFile, {})
            }
            assertTrue(waitUntil { ready.exists() }, "Long-running command did not start")
            try {
                assertEquals("finished", cli.runRaw(listOf(fixture.path), fixture.parentFile, {}).trim())
                assertFalse(running.isDone, "Starting another command terminated the running command")
            } finally {
                release.createNewFile()
            }
            assertEquals("finished", running.get(15, TimeUnit.SECONDS).trim())
        }
    }

    private fun withCli(block: (CliService, File, java.util.concurrent.ExecutorService) -> Unit) {
        val dir = Files.createTempDirectory("ocr-cli-concurrency-").toFile()
        val fixture = File(dir, "FakeCli.java")
        fixture.writeText(
            """
            import java.nio.file.*;
            class FakeCli {
                public static void main(String[] args) throws Exception {
                    if (args.length == 2) {
                        Files.createFile(Path.of(args[0]));
                        long deadline = System.nanoTime() + 30_000_000_000L;
                        while (!Files.exists(Path.of(args[1])) && System.nanoTime() < deadline) {
                            Thread.sleep(20);
                        }
                    }
                    System.out.println("finished");
                }
            }
            """.trimIndent(),
        )
        val java = File(System.getProperty("java.home"), "bin/java" + if (File.separatorChar == '\\') ".exe" else "")
        val executor = Executors.newCachedThreadPool()
        try {
            block(CliService(java.absolutePath), fixture, executor)
        } finally {
            File(dir, "release").createNewFile()
            executor.shutdown()
            if (!executor.awaitTermination(35, TimeUnit.SECONDS)) executor.shutdownNow()
            dir.deleteRecursively()
        }
    }

    private fun waitUntil(condition: () -> Boolean): Boolean {
        val deadline = System.nanoTime() + TimeUnit.SECONDS.toNanos(15)
        while (System.nanoTime() < deadline) {
            if (condition()) return true
            Thread.sleep(20)
        }
        return condition()
    }
}
