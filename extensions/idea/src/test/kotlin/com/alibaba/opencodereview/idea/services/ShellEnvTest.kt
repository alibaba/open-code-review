package com.alibaba.opencodereview.idea.services

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

class ShellEnvTest {

    private val delim = "_OCR_ENV_DELIM_"

    @Test
    fun `解析分隔标记之间的 env 输出`() {
        val stdout = """
            some rc file noise
            $delim
            PATH=/usr/local/bin:/usr/bin
            SHELL=/bin/zsh
            $delim
            trailing noise
        """.trimIndent()
        val env = ShellEnv.parseEnvBlock(stdout)
        assertEquals("/usr/local/bin:/usr/bin", env["PATH"])
        assertEquals("/bin/zsh", env["SHELL"])
        assertEquals(2, env.size)
    }

    @Test
    fun `值里含等号时只按第一个等号切分`() {
        val env = ShellEnv.parseEnvBlock("$delim\nFOO=a=b=c\n$delim")
        assertEquals("a=b=c", env["FOO"])
    }

    @Test
    fun `值为空串时保留该键`() {
        val env = ShellEnv.parseEnvBlock("$delim\nEMPTY=\n$delim")
        assertEquals("", env["EMPTY"])
    }

    @Test
    fun `没有等号或以等号开头的行被忽略`() {
        val env = ShellEnv.parseEnvBlock("$delim\nnot an assignment\n=novalue\nOK=1\n$delim")
        assertEquals(mapOf("OK" to "1"), env)
    }

    @Test
    fun `缺少分隔标记时返回空表`() {
        assertTrue(ShellEnv.parseEnvBlock("PATH=/usr/bin").isEmpty())
        assertTrue(ShellEnv.parseEnvBlock("$delim\nPATH=/usr/bin").isEmpty())
        assertTrue(ShellEnv.parseEnvBlock("").isEmpty())
    }

    @Test
    fun `含非法字符的命令名不做 shell 解析`() {
        // 命令名不得被拼入 shell 命令行，否则构成命令注入
        assertEquals("ocr; rm -rf /", ShellEnv.resolveBin("ocr; rm -rf /"))
        assertEquals("ocr\$(whoami)", ShellEnv.resolveBin("ocr\$(whoami)"))
    }

    @Test
    fun `forShell 原样保留命令，只可能在前面加 cmd 前缀`() {
        // 平台相关，因此仅断言与平台无关的两件事：原命令完整保留在末尾，
        // 且附加的前缀只能是 cmd.exe /c（Windows 上 npm/ocr 为 .cmd，必须由 cmd 解释）。
        val command = listOf("/usr/local/bin/npm", "install", "-g", "pkg")
        val wrapped = ShellEnv.forShell(command)
        assertEquals(command, wrapped.takeLast(command.size))
        val prefix = wrapped.dropLast(command.size)
        assertTrue(prefix.isEmpty() || prefix == listOf("cmd.exe", "/c"), "意外的前缀：$prefix")
    }
}
