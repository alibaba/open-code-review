package com.alibaba.opencodereview.idea.services

import com.alibaba.opencodereview.idea.model.CliResult
import com.alibaba.opencodereview.idea.model.CliRunOptions
import com.alibaba.opencodereview.idea.model.EnvCheckResult
import com.alibaba.opencodereview.idea.model.EnvToolStatus
import com.alibaba.opencodereview.idea.model.HostStrings
import com.alibaba.opencodereview.idea.model.LogLevel
import com.alibaba.opencodereview.idea.model.LogLine
import com.alibaba.opencodereview.idea.model.currentIdeLocale
import com.intellij.openapi.diagnostic.thisLogger
import com.intellij.util.concurrency.AppExecutorUtil
import java.io.File
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicReference

/** CLI 以非 0 退出时抛出，message 已经过 [extractCliError] 提炼，可直接展示给用户。 */
class CliException(message: String) : RuntimeException(message)

/**
 * 所有子进程必经 [ShellEnv]：环境取登录 shell 的、命令名由 `resolveBin` 解析为绝对路径。
 * 裸命令名加继承环境在 GUI 启动的 IDEA 中无法运行。本类方法均为阻塞调用，调用方须在后台线程执行。
 */
class CliService(private val cliPath: String = "ocr") {

    private companion object {
        const val ENV_CACHE_TTL_MS = 5 * 60 * 1000L
        const val PROBE_TIMEOUT_MS = 10_000L
        const val FORCE_KILL_DELAY_MS = 3_000L
        const val NPM_PACKAGE = "@alibaba-group/open-code-review"
    }

    private val current = AtomicReference<Process?>(null)

    @Volatile
    private var envCache: Pair<EnvCheckResult, Long>? = null

    fun invalidateEnvironmentCache() {
        envCache = null
    }

    fun getCachedEnvironment(): EnvCheckResult? {
        val (env, at) = envCache ?: return null
        if (System.currentTimeMillis() - at > ENV_CACHE_TTL_MS) {
            envCache = null
            return null
        }
        return env
    }

    fun isAvailable(): Boolean = checkEnvironment().ocr.ok

    /** node → npm → ocr 顺序探测并短路：前一个不可用时后续直接判失败，避免无意义的等待。 */
    fun checkEnvironment(force: Boolean = false): EnvCheckResult {
        if (!force) getCachedEnvironment()?.let { return it }
        val node = probeCommand("node")
        val npm = if (node.ok) probeCommand("npm") else EnvToolStatus()
        val ocr = if (node.ok && npm.ok) probeCommand(cliPath) else EnvToolStatus()
        val env = EnvCheckResult(node, npm, ocr)
        envCache = env to System.currentTimeMillis()
        return env
    }

    private fun probeCommand(bin: String): EnvToolStatus = runCatching {
        // 参数固定为 --version，可安全套 shell（Windows 上 npm/ocr 是 .cmd，不套 shell 无法执行）。
        val process = ProcessBuilder(ShellEnv.forShell(listOf(ShellEnv.resolveBin(bin), "--version")))
            .withShellEnv()
            .redirectErrorStream(true)
            .start()
        process.outputStream.close()
        // stdout 必须分线程读取。本线程 readText() 会阻塞至进程退出（或管道写满），导致后续
        // waitFor(PROBE_TIMEOUT_MS) 无法执行——探测卡住的 node 会永久挂住整个环境检查。写法与 ShellEnv.capture 一致。
        val out = StringBuilder()
        val reader = Thread({
            runCatching { process.inputStream.bufferedReader().forEachLine { out.appendLine(it) } }
        }, "ocr-probe-$bin").apply { isDaemon = true; start() }
        if (!process.waitFor(PROBE_TIMEOUT_MS, TimeUnit.MILLISECONDS)) {
            process.destroyForcibly()
            return EnvToolStatus()
        }
        reader.join(500)
        if (process.exitValue() != 0) return EnvToolStatus()
        EnvToolStatus(ok = true, version = out.lineSequence().firstOrNull()?.trim()?.takeIf(String::isNotEmpty))
    }.getOrElse { EnvToolStatus() }

    /** 全局安装 ocr CLI，逐行回显 npm 日志，按 exit code 返回是否成功。 */
    fun install(onLog: (LogLine) -> Unit): Boolean {
        val args = listOf("install", "-g", NPM_PACKAGE, "--loglevel", "http", "--no-progress")
        onLog(LogLine("$ npm ${args.joinToString(" ")}"))
        return runCatching {
            // 参数均为固定值，同 probeCommand，可套 shell 以便 Windows 上执行 npm.cmd。
            val process = ProcessBuilder(ShellEnv.forShell(listOf(ShellEnv.resolveBin("npm")) + args))
                // 非 TTY 下 npm 仍可能画进度条，强制关闭并去色，否则日志中充斥转义序列。
                .withShellEnv("npm_config_progress" to "false", "npm_config_color" to "false")
                .redirectErrorStream(true)
                .start()
            process.outputStream.close()
            // npm 用 \r 覆盖行，此处归一为 \n 再逐行输出。
            process.inputStream.bufferedReader().forEachLine { raw ->
                raw.replace('\r', '\n').lineSequence().forEach { line ->
                    if (line.isNotBlank()) onLog(LogLine(line))
                }
            }
            val exit = process.waitFor()
            if (exit == 0) {
                onLog(LogLine(HostStrings.t(currentIdeLocale(), "ext.cli.installOk")))
                invalidateEnvironmentCache()
                // 新装的全局 bin 可能不在已缓存的 PATH 中，须让 shell 环境重新解析一次。
                ShellEnv.invalidate()
            } else {
                onLog(
                    LogLine(
                        HostStrings.t(currentIdeLocale(), "ext.cli.installFail", "code" to exit.toString()),
                        LogLevel.ERROR,
                    ),
                )
            }
            exit == 0
        }.getOrElse {
            onLog(LogLine(it.message ?: it.javaClass.simpleName, LogLevel.ERROR))
            false
        }
    }

    /**
     * 执行任意 CLI 参数：stderr 逐行回调，结束返回 stdout 全文；退出码非 0 抛 CliException。
     * 不走 forShell——args 含用户输入，套 shell 会增加注入面。
     */
    fun runRaw(
        args: List<String>,
        cwd: File,
        onLog: (LogLine) -> Unit,
        envExtra: Map<String, String> = emptyMap(),
    ): String {
        val process = ProcessBuilder(listOf(ShellEnv.resolveBin(cliPath)) + args)
            .directory(cwd)
            .withShellEnv(*envExtra.toList().toTypedArray())
            .start()
        process.outputStream.close()
        // 先接管 current 再清理上一个：直接覆盖会遗漏前一个进程。
        current.getAndSet(process)?.let { stale ->
            if (stale.isAlive) {
                thisLogger().warn("[ocr] 上一个 CLI 进程仍在运行，已终止")
                stale.destroy()
            }
        }
        val stderr = StringBuilder()
        val stderrThread = Thread({
            runCatching {
                process.errorStream.bufferedReader().forEachLine { line ->
                    synchronized(stderr) { stderr.appendLine(line) }
                    parseLogLine(line)?.let(onLog)
                }
            }
        }, "ocr-cli-stderr").apply { isDaemon = true; start() }
        try {
            val stdout = process.inputStream.bufferedReader().readText()
            val exit = process.waitFor()
            stderrThread.join(2_000)
            if (exit != 0) {
                val text = synchronized(stderr) { stderr.toString() }
                throw CliException(extractCliError(text).ifBlank { "CLI exited with code $exit" })
            }
            return stdout
        } finally {
            current.compareAndSet(process, null)
        }
    }

    fun review(opts: CliRunOptions, cwd: File, onLog: (LogLine) -> Unit): CliResult =
        parseCliResult(runRaw(buildReviewArgs(opts), cwd, onLog))

    /**
     * 执行 `ocr llm test`。传入 [home] / [configPath] 时在隔离环境下执行，
     * 使"测试连通性"不会破坏用户真正的 ~/.opencodereview/config.json。
     */
    fun testConnection(home: File? = null, configPath: File? = null): Pair<Boolean, String?> {
        val envExtra = buildMap {
            home?.let {
                put("HOME", it.absolutePath)
                put("USERPROFILE", it.absolutePath)
            }
            configPath?.let { put("OCR_CONFIG_PATH", it.absolutePath) }
        }
        val cwd = File(System.getProperty("user.dir"))
        return runCatching {
            runRaw(listOf("llm", "test"), cwd, {}, envExtra)
            true to null
        }.getOrElse { false to (it.message ?: it.javaClass.simpleName) }
    }

    /** 先 SIGTERM，3 秒后仍存活则 SIGKILL。 */
    fun cancel() {
        val process = current.get() ?: return
        if (!process.isAlive) return
        process.destroy()
        AppExecutorUtil.getAppScheduledExecutorService().schedule(
            { if (process.isAlive) process.destroyForcibly() },
            FORCE_KILL_DELAY_MS,
            TimeUnit.MILLISECONDS,
        )
    }

    private fun ProcessBuilder.withShellEnv(vararg extra: Pair<String, String>): ProcessBuilder = apply {
        environment().apply {
            clear()
            putAll(ShellEnv.env())
            extra.forEach { (key, value) -> put(key, value) }
        }
    }
}
