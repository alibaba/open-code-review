package com.alibaba.opencodereview.idea.services

import java.io.File
import java.nio.file.Files
import kotlin.concurrent.thread
import kotlin.test.Test
import kotlin.test.assertTrue

/**
 * cancel() 必须终止整棵进程树：ocr 是 Node launcher，真正的 Go 二进制是它的子进程。
 * 只杀直接子进程会把孙进程托管给系统根进程，变成杀不到的孤儿（bug 复现见 commit message）。
 */
class CliServiceTreeKillTest {

    private val isWindows = System.getProperty("os.name").orEmpty().startsWith("Windows", ignoreCase = true)

    @Test
    fun `cancel kills the whole process tree including grandchildren`() {
        if (isWindows) return // 依赖 bash 脚本模拟 launcher，仅 POSIX 有意义

        val dir = Files.createTempDirectory("ocr-treekill-test").toFile()
        val pidFile = File(dir, "grandchild.pid")
        val script = File(dir, "fake-ocr.sh")
        // 模拟 launcher 行为：spawn 一个孙进程并记录其 pid，自己 wait 住
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
            val runner = thread { runCatching { service.runRaw(emptyList(), dir, {}) } }

            // 等孙进程起来
            assertTrue(waitFor(pidFile::exists, 5_000), "fake launcher 未及时写出孙进程 pid")
            grandchildPid = pidFile.readText().trim().toLong()
            assertTrue(isAlive(grandchildPid), "孙进程应在运行中")

            service.cancel()

            // 优雅宽限 3s + 调度余量；修复前孙进程会永远存活（孤儿），修复后被快照追杀
            assertTrue(
                waitFor({ !isAlive(grandchildPid) }, 8_000),
                "cancel() 后孙进程仍存活：孤儿进程泄漏",
            )
            runner.join(5_000)
        } finally {
            // 测试失败也不把 sleep 留在系统里
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
