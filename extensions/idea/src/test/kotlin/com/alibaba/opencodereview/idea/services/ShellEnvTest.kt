// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.services

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

class ShellEnvTest {

    private val delim = "_OCR_ENV_DELIM_"

    @Test
    fun `parses env output between delimiter markers`() {
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
    fun `splits at the first equals sign when values contain equals signs`() {
        val env = ShellEnv.parseEnvBlock("$delim\nFOO=a=b=c\n$delim")
        assertEquals("a=b=c", env["FOO"])
    }

    @Test
    fun `empty string values preserve their keys`() {
        val env = ShellEnv.parseEnvBlock("$delim\nEMPTY=\n$delim")
        assertEquals("", env["EMPTY"])
    }

    @Test
    fun `lines without equals signs or starting with equals are ignored`() {
        val env = ShellEnv.parseEnvBlock("$delim\nnot an assignment\n=novalue\nOK=1\n$delim")
        assertEquals(mapOf("OK" to "1"), env)
    }

    @Test
    fun `missing delimiter markers return an empty map`() {
        assertTrue(ShellEnv.parseEnvBlock("PATH=/usr/bin").isEmpty())
        assertTrue(ShellEnv.parseEnvBlock("$delim\nPATH=/usr/bin").isEmpty())
        assertTrue(ShellEnv.parseEnvBlock("").isEmpty())
    }

    @Test
    fun `command names with invalid characters skip shell resolution`() {
        // Never interpolate invalid command names into a shell command, which would allow command injection.
        assertEquals("ocr; rm -rf /", ShellEnv.resolveBin("ocr; rm -rf /"))
        assertEquals("ocr\$(whoami)", ShellEnv.resolveBin("ocr\$(whoami)"))
    }

    @Test
    fun `forShell preserves the command and may only prepend cmd`() {
        // Assert only the platform-independent properties: preserve the entire original command at the end,
        // and allow only a cmd.exe /c prefix, needed to interpret npm/ocr .cmd files on Windows.
        val command = listOf("/usr/local/bin/npm", "install", "-g", "pkg")
        val wrapped = ShellEnv.forShell(command)
        assertEquals(command, wrapped.takeLast(command.size))
        val prefix = wrapped.dropLast(command.size)
        assertTrue(prefix.isEmpty() || prefix == listOf("cmd.exe", "/c"), "Unexpected prefix: $prefix")
    }
}
