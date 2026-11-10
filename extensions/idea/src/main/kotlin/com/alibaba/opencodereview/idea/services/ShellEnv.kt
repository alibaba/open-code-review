// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.services

import com.intellij.openapi.diagnostic.thisLogger
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.TimeUnit

private const val DELIM = "_OCR_ENV_DELIM_"
private const val SHELL_TIMEOUT_MS = 5_000L

/** Command-name whitelist: only alphanumerics . _ / - are allowed, so path concatenation cannot become shell injection. */
private val BIN_NAME_REGEX = Regex("^[a-zA-Z0-9._/-]+$")

/**
 * IDEA launched from Dock/Spotlight inherits only a minimal PATH, without nvm/homebrew/npm global bin.
 * Solution: spawn a login interactive shell to read the real environment into a cache, then resolve
 * command names to absolute paths with `command -v`.
 * On Windows the GUI and terminal environments are identical, so the current process environment is used directly.
 */
object ShellEnv {
    private val binCache = ConcurrentHashMap<String, String>()

    @Volatile
    private var cachedEnv: Map<String, String>? = null

    private val isWindows: Boolean
        get() = System.getProperty("os.name").orEmpty().startsWith("Windows", ignoreCase = true)

    /** When `OCR_SKIP_SHELL_RESOLVE` is set, skip login-shell resolution entirely (for CI / troubleshooting). */
    private val skipResolve: Boolean
        get() = !System.getenv("OCR_SKIP_SHELL_RESOLVE").isNullOrBlank()

    /** Takes the key=value lines between the two delimiter markers in the login shell's `env` output. */
    fun parseEnvBlock(stdout: String): Map<String, String> {
        val start = stdout.indexOf(DELIM)
        val end = stdout.lastIndexOf(DELIM)
        if (start < 0 || end <= start) return emptyMap()
        val block = stdout.substring(start + DELIM.length, end)
        val env = LinkedHashMap<String, String>()
        for (line in block.lineSequence()) {
            val eq = line.indexOf('=')
            if (eq > 0) env[line.substring(0, eq)] = line.substring(eq + 1)
        }
        return env
    }

    /** The current process environment overlaid with the login shell environment; falls back to the process environment when parsing fails. */
    fun env(): Map<String, String> {
        cachedEnv?.let { return it }
        val processEnv: Map<String, String> = System.getenv()
        if (isWindows || skipResolve) {
            cachedEnv = processEnv
            return processEnv
        }
        val resolved = runCatching {
            val cap = capture(listOf(shell(), "-ilc", "echo $DELIM; env; echo $DELIM")) ?: return@runCatching processEnv
            val parsed = parseEnvBlock(cap)
            if (parsed.isEmpty()) processEnv else processEnv + parsed
        }.getOrElse {
            thisLogger().warn("[ocr] Failed to read login shell env, falling back to process env", it)
            processEnv
        }
        cachedEnv = resolved
        return resolved
    }

    /**
     * Resolves a command name to an absolute path. When resolution fails, the original name is returned and left
     * to [ProcessBuilder] to find on the injected PATH.
     * On Windows the original name is returned directly.
     */
    fun resolveBin(name: String): String {
        if (isWindows || skipResolve) return name
        binCache[name]?.let { return it }
        // The command name goes into a shell command line, so validate it against the character whitelist first
        // to keep path concatenation from becoming command injection.
        if (!name.matches(BIN_NAME_REGEX)) return name
        val resolved = runCatching {
            val quoted = name.replace("'", "'\\''")
            val cap = capture(listOf(shell(), "-ilc", "command -v '$quoted'")) ?: return@runCatching null
            cap.lineSequence()
                .map(String::trim)
                .lastOrNull { it.startsWith("/") }
        }.getOrNull()
        val result = resolved ?: name
        // Cache only successful resolutions; timeouts and not-found results are not cached so the next call retries
        // (otherwise one timeout would cache the failure forever, even though a PATH lookup could succeed).
        if (resolved != null) binCache[name] = result
        return result
    }

    /**
     * Adds a shell prefix for a command (Windows: cmd.exe /c). Only for fixed-argument commands (--version and
     * the like); commands that carry user input must never be wrapped in a shell.
     */
    fun forShell(command: List<String>): List<String> =
        if (isWindows) listOf("cmd.exe", "/c") + command else command

    /** Call after a CLI install or a user shell-config change so the next lookup probes afresh. */
    fun invalidate() {
        cachedEnv = null
        binCache.clear()
    }

    private fun shell(): String =
        System.getenv("SHELL")?.takeIf(String::isNotBlank)
            ?: if (System.getProperty("os.name").startsWith("Mac")) "/bin/zsh" else "/bin/bash"

    /** Closes the process's three streams, swallowing exceptions (mirrors CliService.closeStreamsQuietly). */
    private fun Process.closeStreamsQuietly() {
        runCatching { inputStream.close() }
        runCatching { outputStream.close() }
        runCatching { errorStream.close() }
    }

    /**
     * Runs a command and collects stdout, force-killing on timeout. stdin is closed immediately so an interactive
     * shell does not wait for input, and stderr is discarded so rc-file output cannot pollute the result.
     */
    private fun capture(command: List<String>): String? {
        val process = ProcessBuilder(command)
            .redirectError(ProcessBuilder.Redirect.DISCARD)
            .start()
        runCatching { process.outputStream.close() } // stdin closed so an interactive shell does not wait for input; a throw here does not affect the reads below.
        val out = StringBuilder()
        val reader = Thread({
            runCatching { process.inputStream.bufferedReader().forEachLine { synchronized(out) { out.appendLine(it) } } }
        }, "ocr-shell-env").apply { isDaemon = true; start() }
        try {
            if (!process.waitFor(SHELL_TIMEOUT_MS, TimeUnit.MILLISECONDS)) {
                process.destroyForcibly()
                reader.join(1_000) // deliberately not wrapped in runCatching: on InterruptedException the outer catch restores the interrupt flag
                // Return null on timeout instead of handing back partial stdout -- otherwise it would be cached in
                // binCache and poison resolution results long-term.
                return null
            }
            reader.join()
            return synchronized(out) { out.toString() }
        } catch (_: InterruptedException) {
            // Even when interrupted, clean up the started process so it cannot leak unmanaged; re-assert the interrupt status for callers above.
            process.destroyForcibly()
            Thread.currentThread().interrupt()
            return null
        } finally {
            // All three exit paths (timeout/success/interrupt) pass through finally, so fds never leak.
            process.closeStreamsQuietly()
        }
    }
}
