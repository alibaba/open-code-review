package com.alibaba.opencodereview.idea.services

import com.intellij.openapi.diagnostic.thisLogger
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.TimeUnit

private const val DELIM = "_OCR_ENV_DELIM_"
private const val SHELL_TIMEOUT_MS = 5_000L

/**
 * 从 Dock/Spotlight 启动的 IDEA 仅继承精简 PATH，不含 nvm/homebrew/npm 全局 bin。
 * 解法：启动登录交互式 shell 读取真实环境到缓存；再 command -v 将命令名解析为绝对路径。
 * Windows 下 GUI 与终端环境一致，直接使用当前进程环境。
 */
object ShellEnv {
    private val binCache = ConcurrentHashMap<String, String>()

    @Volatile
    private var cachedEnv: Map<String, String>? = null

    private val isWindows: Boolean
        get() = System.getProperty("os.name").orEmpty().startsWith("Windows", ignoreCase = true)

    /** 设了 `OCR_SKIP_SHELL_RESOLVE` 则完全跳过登录 shell 解析（CI / 排障用）。 */
    private val skipResolve: Boolean
        get() = !System.getenv("OCR_SKIP_SHELL_RESOLVE").isNullOrBlank()

    /** 从登录 shell 的 `env` 输出里取两个分隔标记之间的 key=value。 */
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

    /** 当前进程环境叠加登录 shell 环境；解析失败时退回当前进程环境。 */
    fun env(): Map<String, String> {
        cachedEnv?.let { return it }
        val processEnv: Map<String, String> = System.getenv()
        if (isWindows || skipResolve) {
            cachedEnv = processEnv
            return processEnv
        }
        val resolved = runCatching {
            val parsed = parseEnvBlock(capture(listOf(shell(), "-ilc", "echo $DELIM; env; echo $DELIM")))
            if (parsed.isEmpty()) processEnv else processEnv + parsed
        }.getOrElse {
            thisLogger().warn("[ocr] Failed to read login shell env, falling back to process env", it)
            processEnv
        }
        cachedEnv = resolved
        return resolved
    }

    /**
     * 将命令名解析为绝对路径。解析不出来则返回原名，交由 [ProcessBuilder] 在注入的 PATH 中查找。
     * Windows 直接返回原名。
     */
    fun resolveBin(name: String): String {
        if (isWindows || skipResolve) return name
        binCache[name]?.let { return it }
        // 命令名要进入 shell 命令行，先做字符白名单校验，避免路径拼接变成命令注入。
        if (!name.matches(Regex("^[a-zA-Z0-9._/-]+$"))) return name
        val resolved = runCatching {
            val quoted = name.replace("'", "'\\''")
            capture(listOf(shell(), "-ilc", "command -v '$quoted'"))
                .lineSequence()
                .map(String::trim)
                .lastOrNull { it.startsWith("/") }
                ?: name
        }.getOrDefault(name)
        binCache[name] = resolved
        return resolved
    }

    /**
     * 为命令添加 shell 前缀（Windows: cmd.exe /c）。仅用于参数固定的命令（--version 等），
     * 含用户输入的命令不得套 shell。
     */
    fun forShell(command: List<String>): List<String> =
        if (isWindows) listOf("cmd.exe", "/c") + command else command

    /** CLI 安装完毕或用户修改 shell 配置后调用，下次重新探测。 */
    fun invalidate() {
        cachedEnv = null
        binCache.clear()
    }

    private fun shell(): String = System.getenv("SHELL")?.takeIf(String::isNotBlank) ?: "/bin/zsh"

    /**
     * 执行命令收集 stdout，超时强杀。stdin 立即关闭避免交互式 shell 等待输入，
     * stderr 丢弃避免 rc 文件输出污染结果。
     */
    private fun capture(command: List<String>): String {
        val process = ProcessBuilder(command)
            .redirectError(ProcessBuilder.Redirect.DISCARD)
            .start()
        process.outputStream.close()
        val out = StringBuilder()
        val reader = Thread({
            runCatching {
                process.inputStream.bufferedReader().forEachLine { out.appendLine(it) }
            }
        }, "ocr-shell-env").apply { isDaemon = true; start() }
        if (!process.waitFor(SHELL_TIMEOUT_MS, TimeUnit.MILLISECONDS)) {
            process.destroyForcibly()
            reader.join(1000)
            return out.toString()
        }
        reader.join()
        return out.toString()
    }
}
