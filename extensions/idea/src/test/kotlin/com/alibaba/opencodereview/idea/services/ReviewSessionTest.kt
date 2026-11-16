// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.services

import com.alibaba.opencodereview.idea.model.CliResult
import com.alibaba.opencodereview.idea.model.CliRunOptions
import com.alibaba.opencodereview.idea.model.LogLine
import com.alibaba.opencodereview.idea.model.ReviewComment
import com.alibaba.opencodereview.idea.model.ReviewState
import java.io.File
import org.junit.Rule
import org.junit.rules.TemporaryFolder
import kotlin.test.Test
import kotlin.test.assertEquals

class ReviewSessionTest {
    @get:Rule
    val temp = TemporaryFolder()

    private fun runFailedCli(stdout: String, exitCode: Int = 1): CliResult? {
        val dir = temp.newFolder()
        val windows = File.separatorChar == '\\'
        val script = File(dir, if (windows) "ocr.cmd" else "ocr")
        script.writeText(if (windows) {
            "@echo off\r\ntype result.json\r\necho Error: provider timed out 1>&2\r\nexit /b $exitCode\r\n"
        } else {
            "#!/bin/sh\ncat result.json\necho 'Error: provider timed out' >&2\nexit $exitCode\n"
        })
        if (!windows) check(script.setExecutable(true))
        File(dir, "result.json").writeText(stdout)
        var terminal: Pair<ReviewState, String?>? = null
        var delivered: CliResult? = null
        ReviewSession(CliService(script.absolutePath), dir).run(CliRunOptions(), object : SessionCallbacks {
            override fun onState(state: ReviewState, error: String?) { terminal = state to error }
            override fun onLog(line: LogLine) {}
            override fun onDone(result: CliResult) { delivered = result }
        })
        assertEquals(ReviewState.FAILED to "provider timed out", terminal, stdout)
        return delivered
    }

    @Test
    fun `nonzero partial review reports failure and delivers its findings`() {
        val result = runFailedCli(
            """{"status":"partial","comments":[{"path":"a.kt","content":"Missing return","start_line":2,"end_line":2}]}""",
        )
        assertEquals("partial", result?.status)
        assertEquals(ReviewComment(path = "a.kt", content = "Missing return", startLine = 2, endLine = 2), result?.comments?.single())
    }

    @Test
    fun `other nonzero exits keep the CLI error and do not deliver findings`() {
        for ((stdout, exitCode) in listOf(
            "" to 1, "invalid JSON" to 1, """{"status":"partial"}""" to 1,
            """{"status":"partial","comments":[{}]}""" to 1,
            """{"status":"success","comments":[]}""" to 1, """{"status":"partial","comments":[]}""" to 2,
        )) {
            assertEquals(null, runFailedCli(stdout, exitCode), stdout)
        }
    }

    @Test
    fun `incomplete and unknown results cannot report a clean review`() {
        for (status in listOf("partial", "failed", "", "future_status")) {
            assertEquals(ReviewState.FAILED, resultToState(result(status)), status)
        }
    }

    @Test
    fun `complete results and partial findings remain visible`() {
        assertEquals(ReviewState.EMPTY, resultToState(result("complete")))
        assertEquals(ReviewState.DONE, resultToState(result("partial", comments = 1)))
    }

    private fun result(status: String, comments: Int = 0) =
        CliResult(status = status, comments = List(comments) { ReviewComment(path = "a.ts") })

    @Test
    fun `comments result in done`() {
        assertEquals(ReviewState.DONE, resultToState(result("success", comments = 1)))
    }

    @Test
    fun `success without comments results in empty`() {
        assertEquals(ReviewState.EMPTY, resultToState(result("success")))
    }

    @Test
    fun `skipped without comments results in empty`() {
        assertEquals(ReviewState.EMPTY, resultToState(result("skipped")))
    }

    @Test
    fun `completed_with_errors without comments results in failed`() {
        assertEquals(ReviewState.FAILED, resultToState(result("completed_with_errors")))
    }

    @Test
    fun `completed_with_errors with comments still results in done`() {
        // Comments take precedence: check comments before status.
        assertEquals(ReviewState.DONE, resultToState(result("completed_with_errors", comments = 1)))
    }
}
