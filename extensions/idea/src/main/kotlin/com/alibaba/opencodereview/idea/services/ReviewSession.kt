// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.services

import com.alibaba.opencodereview.idea.model.CliResult
import com.alibaba.opencodereview.idea.model.CliRunOptions
import com.alibaba.opencodereview.idea.model.LogLevel
import com.alibaba.opencodereview.idea.model.LogLine
import com.alibaba.opencodereview.idea.model.ReviewState
import com.intellij.openapi.progress.ProcessCanceledException
import java.io.File
import java.util.concurrent.atomic.AtomicBoolean

internal val supportedReviewStatuses = setOf(
    "success", "complete", "completed_with_warnings", "completed_with_errors", "partial", "failed", "skipped",
)

/** Only recognized, non-failing terminal results can report an empty review. */
fun resultToState(result: CliResult): ReviewState = when {
    result.status !in supportedReviewStatuses -> ReviewState.FAILED
    result.comments.isNotEmpty() -> ReviewState.DONE
    result.status in setOf("completed_with_errors", "partial", "failed") -> ReviewState.FAILED
    else -> ReviewState.EMPTY
}

interface SessionCallbacks {
    fun onState(state: ReviewState, error: String? = null)
    fun onLog(line: LogLine)
    fun onDone(result: CliResult)
}

/**
 * One review corresponds to one session; state lives only inside the session (the `cancelled` flag),
 * with no cross-session persistence. After the webview is recreated the frontend simply requests again.
 * [run] is a blocking call; callers must run it on a background thread.
 */
class ReviewSession(private val cli: CliService, private val cwd: File) {

    private val cancellation = CliCancellation()

    @Volatile
    private var cancelled = false
    /** Only one caller can cancel this session and publish its cancellation state. */
    private val cancelEntered = AtomicBoolean(false)

    fun run(opts: CliRunOptions, cb: SessionCallbacks) {
        // Do not reset cancelled: if cancel() arrives after run is scheduled but before it actually executes,
        // resetting here would erase that cancellation.
        if (cancelled) { // cancelled before run started (scheduled but not yet executing): report CANCELLED directly instead of emitting RUNNING first, which would add a spurious state transition
            cb.onState(ReviewState.CANCELLED)
            return
        }
        cb.onState(ReviewState.RUNNING)
        try {
            val result = cli.review(opts, cwd, cb::onLog, cancellation)
            if (cancelled) {
                cb.onState(ReviewState.CANCELLED)
                return
            }
            val state = resultToState(result)
            val incomplete = result.status in setOf("completed_with_errors", "partial", "failed")
            val error = if (state == ReviewState.FAILED || incomplete) {
                result.message?.takeIf(String::isNotBlank) ?: "Review did not complete successfully (${result.status})"
            } else null
            if (error != null) cb.onLog(LogLine("[ocr] $error", LogLevel.ERROR))
            cb.onState(state, error.takeIf { state == ReviewState.FAILED })
            cb.onDone(result)
        } catch (error: Exception) {
            // Catch Exception only: Errors such as OOM/LinkageError are not swallowed here -- they propagate
            // so fatal problems are not masked.
            // ProcessCanceledException is IntelliJ's cancellation signal; do not swallow it.
            if (error is ProcessCanceledException) throw error
            if (cancelled) {
                cb.onState(ReviewState.CANCELLED)
            } else {
                val message = error.message ?: error.javaClass.simpleName
                cb.onLog(LogLine("[ocr] $message", LogLevel.ERROR))
                cb.onState(ReviewState.FAILED, message)
            }
        }
    }

    fun cancel(onState: (ReviewState) -> Unit) {
        // The cancellation handle belongs to this session, including before run() starts.
        if (!cancelEntered.compareAndSet(false, true)) return
        cancelled = true
        cancellation.cancel()
        onState(ReviewState.CANCELLED)
    }
}
