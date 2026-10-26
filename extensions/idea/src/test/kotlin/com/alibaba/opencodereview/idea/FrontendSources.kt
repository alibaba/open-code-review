// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea

import java.io.File

/**
 * Locate `frontend/` so host/frontend consistency tests can read the frontend source.
 *
 * These tests catch missing host updates after frontend additions such as providers or theme variables.
 * Such drift degrades the UI silently: no error or blank screen, just missing content or colors that are hard to spot manually.
 * Build-time checks are needed to detect this drift.
 */
object FrontendSources {

    /** Project root. Gradle tests run from it; other runners may not, so search one level up as a fallback. */
    private val projectRoot: File by lazy {
        var dir: File? = File(System.getProperty("user.dir")).absoluteFile
        while (dir != null) {
            if (File(dir, "frontend/src/shared").isDirectory) return@lazy dir
            dir = dir.parentFile
        }
        error("Cannot find frontend/ after searching upward from ${System.getProperty("user.dir")}")
    }

    val frontendDir: File get() = File(projectRoot, "frontend")

    fun file(relative: String): File = File(frontendDir, relative).also {
        check(it.isFile) { "Frontend file does not exist: $it (has the frontend layout changed?)" }
    }

    /** Recursively read all source files in a subdirectory and combine them for regex scanning. */
    fun readAllText(relativeDir: String, vararg extensions: String): String {
        val dir = File(frontendDir, relativeDir)
        check(dir.isDirectory) { "Frontend directory does not exist: $dir" }
        return dir.walkTopDown()
            .filter { it.isFile && extensions.any { ext -> it.name.endsWith(ext) } }
            .joinToString("\n") { it.readText() }
    }
}
