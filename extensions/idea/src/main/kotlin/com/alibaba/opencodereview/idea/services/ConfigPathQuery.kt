// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.services

import java.io.File
import java.nio.file.Files
import java.util.concurrent.TimeUnit
import com.alibaba.opencodereview.idea.model.ConfigPathStatus

class CliUpgradeRequiredException : CliException(
    "This OCR CLI does not support config path --json. Upgrade it with: npm install -g @alibaba-group/open-code-review",
)

fun configPathFailure(error: Throwable): ConfigPathStatus = ConfigPathStatus(
    message = error.message ?: error.javaClass.simpleName,
    upgradeRequired = error is CliUpgradeRequiredException,
)

internal data class ConfigPathInvocation(val command: List<String>, val env: Map<String, String>)

internal fun configPathInvocation(
    bin: String, env: Map<String, String>,
    windows: Boolean = System.getProperty("os.name").orEmpty().startsWith("Windows", ignoreCase = true),
): ConfigPathInvocation {
    require(bin.none { it == '"' || it == '\r' || it == '\n' }) { "Invalid OCR executable path" }
    val executable = resolveConfigPathExecutable(bin, env, windows)
    if (!windows) return ConfigPathInvocation(listOf(executable, "config", "path", "--json"), env)
    // Keep quotes in the environment value so strict JDK launchers see no embedded quotes in the command argument.
    return ConfigPathInvocation(
        listOf("cmd.exe", "/d", "/v:off", "/s", "/c", "\"%OCR_CONFIG_PATH_COMMAND% config path --json\""),
        env.filterKeys { !it.equals("OCR_CONFIG_PATH_COMMAND", ignoreCase = true) } + ("OCR_CONFIG_PATH_COMMAND" to "\"$executable\""),
    )
}

private fun resolveConfigPathExecutable(bin: String, env: Map<String, String>, windows: Boolean): String {
    if (File(bin).isAbsolute) return bin
    val names = if (windows && '.' !in bin) listOf("$bin.exe", "$bin.cmd", "$bin.bat", "$bin.com", bin) else listOf(bin)
    if ('/' in bin || '\\' in bin || ':' in bin) throw CliException("OCR executable path must be absolute")
    return env.entries.firstOrNull { it.key.equals("PATH", ignoreCase = windows) }?.value.orEmpty()
        .split(if (windows) ';' else File.pathSeparatorChar)
        .asSequence().map(::File).filter { it.isAbsolute }.flatMap { folder ->
            names.asSequence().map { File(folder, it) }
        }.firstOrNull { it.isFile && (windows || it.canExecute()) }?.absolutePath
        ?: throw CliException("OCR executable not found on PATH")
}

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
            val message = extractCliError(read(stderr)).ifBlank { "Failed to resolve the OCR config path" }
            if (Regex("unknown (command|flag)|flag provided but not defined", RegexOption.IGNORE_CASE).containsMatchIn(message)) {
                throw CliUpgradeRequiredException()
            }
            throw CliException(message)
        }
        return read(stdout)
    } finally {
        process?.let { child ->
            val descendants = runCatching { child.toHandle().descendants().toList() }.getOrDefault(emptyList())
            runCatching { child.destroyForcibly() }
            descendants.forEach { runCatching { it.destroyForcibly() } }
            val cleanupDeadline = System.nanoTime() + TimeUnit.SECONDS.toNanos(1)
            runCatching { child.waitFor(1000, TimeUnit.MILLISECONDS) }
            descendants.forEach { descendant ->
                val remaining = cleanupDeadline - System.nanoTime()
                if (remaining > 0) runCatching { descendant.onExit().get(remaining, TimeUnit.NANOSECONDS) }
            }
            runCatching { child.inputStream.close() }
            runCatching { child.outputStream.close() }
            runCatching { child.errorStream.close() }
        }
        // Windows can retain redirected handles briefly; cleanup must not replace the query result or error.
        for (path in listOf(stdout, stderr)) {
            runCatching { Files.deleteIfExists(path) }.onFailure { path.toFile().deleteOnExit() }
        }
    }
}
