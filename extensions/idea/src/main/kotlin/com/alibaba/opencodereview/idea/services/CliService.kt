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
import java.io.IOException
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

    /** npm install 独立追踪，与 [current]（ocr review）分开：cancel() 只杀 review，不殃及 install；反之亦然。 */
    private val installProcess = AtomicReference<Process?>(null)

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
            runCatching { process.inputStream.bufferedReader().forEachLine { synchronized(out) { out.appendLine(it) } } }
        }, "ocr-probe-$bin").apply { isDaemon = true; start() }
        if (!process.waitFor(PROBE_TIMEOUT_MS, TimeUnit.MILLISECONDS)) {
            process.destroyForcibly()
            // 进程被强杀后 stdout 管道关闭、reader 将很快 EOF 退出；短 join 避免 daemon 线程在反复环境检查中堆积。
            reader.join(500)
            // 关流与 runRaw/install 的清理一致，避免高频环境检查下 fd 累积到 GC。
            process.closeStreamsQuietly()
            return EnvToolStatus()
        }
        // 有界等待 reader 收完：--version 探测毫秒级结束；极端情况（子进程继承 stdout 管道不 EOF）2s 上限也避免主线程永久挂。
        // 读写 out 均 synchronized，即便超时 reader 仍在写，读 out 也不踩 StringBuilder 跨线程脏读。
        reader.join(2_000)
        // 关流（超时分支已自行关流并返回；此处覆盖正常退出/非零退出路径），避免高频环境检查下 fd 累积到 GC。
        process.closeStreamsQuietly()
        if (process.exitValue() != 0) return EnvToolStatus()
        val version = synchronized(out) { out.lineSequence().firstOrNull()?.trim()?.takeIf(String::isNotEmpty) }
        EnvToolStatus(ok = true, version = version)
    }.getOrElse { EnvToolStatus() }

    /** 全局安装 ocr CLI，逐行回显 npm 日志，按 exit code 返回是否成功。 */
    fun install(onLog: (LogLine) -> Unit): Boolean {
        val args = listOf("install", "-g", NPM_PACKAGE, "--loglevel", "http", "--no-progress")
        onLog(LogLine("$ npm ${args.joinToString(" ")}"))
        return runCatching {
            // 参数均为固定值，同 probeCommand，可套 shell 以便 Windows 上执行 npm.cmd。
            // 先收尾上一个 install（若有）再 start 新的，避免两个 npm install 同时跑争全局 npm 缓存/lockfile。
            installProcess.getAndSet(null)?.let(::killStaleInstall)
            val process = ProcessBuilder(ShellEnv.forShell(listOf(ShellEnv.resolveBin("npm")) + args))
                // 非 TTY 下 npm 仍可能画进度条，强制关闭并去色，否则日志中充斥转义序列。
                .withShellEnv("npm_config_progress" to "false", "npm_config_color" to "false")
                .redirectErrorStream(true)
                .start()
            // 注册紧贴 start（start 到注册是并发盲区，越短越好）；独立槽 installProcess 不碰 current（否则误杀 review）。
            // 正常刚清空过应为 null；并发 install 罕见，若有同样收尾。
            installProcess.getAndSet(process)?.let(::killStaleInstall)
            try {
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
            } finally {
                // 与 runRaw 对齐：异常路径（forEachLine 抛 IOException 等）下进程可能仍存活，强杀 + 关流兜底，避免僵尸与 fd 泄漏。
                if (process.isAlive) process.destroyForcibly()
                process.closeStreamsQuietly()
                installProcess.compareAndSet(process, null)
            }
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
        // 先注册到 current（紧贴 start，盲区最短），再关 stdin：若 close 抛异常，进程已登记、cancel 仍可杀，不致脱管。
        current.getAndSet(process)?.let { stale ->
            if (stale.isAlive) {
                thisLogger().warn("[ocr] 上一个 CLI 进程仍在运行，已终止")
                stale.destroy()
                // 等 SIGTERM 生效，避免新旧 CLI 进程并行读写同一仓库/配置；超时则强杀。
                if (!stale.waitFor(FORCE_KILL_DELAY_MS, TimeUnit.MILLISECONDS)) {
                    thisLogger().warn("[ocr] 上一个 CLI 进程 ${FORCE_KILL_DELAY_MS}ms 未退出，强制终止")
                    stale.destroyForcibly()
                }
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
            process.outputStream.close() // 在 try 内：close 抛 IOException 时 finally 仍会清理已注册的进程，不致脱管。
            val stdout = process.inputStream.bufferedReader().readText()
            val exit = process.waitFor()
            stderrThread.join(2_000)
            if (exit != 0) {
                val text = synchronized(stderr) { stderr.toString() }
                throw CliException(extractCliError(text).ifBlank { "CLI exited with code $exit" })
            }
            return stdout
        } finally {
            // 先强杀、再关流、最后释放 current：杀在前使并发 cancel 看到的是已死进程，且不让新 runRaw 在旧进程仍活时抢占槽位。
            if (process.isAlive) process.destroyForcibly()
            // 异常路径下 stderrThread 可能仍在读 errorStream；先等它收尾再关流，避免打断它丢日志行（成功路径上面已 join，此处幂等）。
            stderrThread.join(2_000)
            process.closeStreamsQuietly()
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

    /** 先 SIGTERM，3 秒后仍存活则 SIGKILL。只取消 review（current 槽）；install 有独立生命周期，不在此处殃及。 */
    fun cancel() {
        // 取走 current 槽里此刻 tracked 的 review 进程并终止。getAndSet 取的是"此刻槽里的那个"——若期间有新 runRaw
        // 接管了 current，被取走的就是新进程。CliService 这层无 session 身份，无法保证杀的恰是"用户想取消的那轮"；
        // 正常流程下 startReview 先 cancel 旧 session 再建新 session，使 current 始终对应当前轮，误杀仅多轮极端竞态下理论存在。
        val process = current.getAndSet(null) ?: return
        if (!process.isAlive) return
        process.destroy()
        AppExecutorUtil.getAppScheduledExecutorService().schedule(
            { if (process.isAlive) process.destroyForcibly() },
            FORCE_KILL_DELAY_MS,
            TimeUnit.MILLISECONDS,
        )
    }

    /** 关掉进程的三路流，吞掉 close() 声明的 IOException，不吞 InterruptedException 等运行期信号。 */
    private fun Process.closeStreamsQuietly() {
        try { inputStream.close() } catch (_: IOException) {}
        try { outputStream.close() } catch (_: IOException) {}
        try { errorStream.close() } catch (_: IOException) {}
    }

    /** 终止并清理上一个 npm install：先 SIGTERM+宽限（与 runRaw 的 stale 处理一致，给 npm 清理机会），再关流。 */
    private fun killStaleInstall(stale: Process) {
        if (stale.isAlive) {
            thisLogger().warn("[ocr] 上一个 npm install 仍在运行，已终止")
            stale.destroy()
            if (!stale.waitFor(FORCE_KILL_DELAY_MS, TimeUnit.MILLISECONDS)) {
                thisLogger().warn("[ocr] 上一个 npm install ${FORCE_KILL_DELAY_MS}ms 内未退出，强制终止")
                stale.destroyForcibly()
            }
        }
        stale.closeStreamsQuietly()
    }

    private fun ProcessBuilder.withShellEnv(vararg extra: Pair<String, String>): ProcessBuilder = apply {
        environment().apply {
            clear()
            putAll(ShellEnv.env())
            extra.forEach { (key, value) -> put(key, value) }
        }
    }
}
