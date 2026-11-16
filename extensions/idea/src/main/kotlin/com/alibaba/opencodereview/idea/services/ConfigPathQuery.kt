// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.services

import java.io.File
import java.nio.file.Files
import java.util.concurrent.TimeUnit

internal fun captureConfigPathQuery(
    command: List<String>, cwd: File, env: Map<String, String>, timeoutMs: Long,
): String {
    if (timeoutMs <= 0) throw CliException("OCR config path query timed out")
    val stdout = Files.createTempFile("ocr-path-", ".out")
    val stderr = Files.createTempFile("ocr-path-", ".err")
    var process: Process? = null
    try {
        val builder = ProcessBuilder(command).directory(cwd)
            .redirectOutput(stdout.toFile()).redirectError(stderr.toFile())
        builder.environment().apply { clear(); putAll(env) }
        process = builder.start()
        process.outputStream.close()
        val deadline = System.nanoTime() + TimeUnit.MILLISECONDS.toNanos(timeoutMs)
        while (true) {
            if (Files.size(stdout) > 65536 || Files.size(stderr) > 65536) {
                throw CliException("OCR config path response is too large")
            }
            val remaining = TimeUnit.NANOSECONDS.toMillis(deadline - System.nanoTime())
            if (remaining <= 0) throw CliException("OCR config path query timed out")
            if (process.waitFor(minOf(remaining, 50), TimeUnit.MILLISECONDS)) break
        }
        fun read(path: java.nio.file.Path): String {
            val data = Files.newInputStream(path).use { it.readNBytes(65537) }
            if (data.size > 65536) throw CliException("OCR config path response is too large")
            return data.toString(Charsets.UTF_8)
        }
        if (process.exitValue() != 0) {
            throw CliException(extractCliError(read(stderr)).ifBlank { "Failed to resolve the OCR config path" })
        }
        return read(stdout)
    } finally {
        process?.let { child ->
            val descendants = runCatching { child.toHandle().descendants().toList() }.getOrDefault(emptyList())
            child.destroyForcibly()
            descendants.forEach { runCatching { it.destroyForcibly() } }
            runCatching { child.waitFor(1000, TimeUnit.MILLISECONDS) }
            runCatching { child.inputStream.close() }
            runCatching { child.outputStream.close() }
            runCatching { child.errorStream.close() }
        }
        Files.deleteIfExists(stdout)
        Files.deleteIfExists(stderr)
    }
}
