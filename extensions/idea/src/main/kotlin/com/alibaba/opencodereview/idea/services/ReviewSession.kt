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
 * 一次审查对应一个 session，状态仅在 session 内（`cancelled` 标记），
 * 不做跨 session 持久化——webview 重建后由前端重新请求。[run] 为阻塞调用，调用方须在后台线程执行。
 */
class ReviewSession(private val cli: CliService, private val cwd: File) {

    private val cancellation = CliCancellation()

    @Volatile
    private var cancelled = false
    /** Only one caller can cancel this session and publish its cancellation state. */
    private val cancelEntered = AtomicBoolean(false)

    fun run(opts: CliRunOptions, cb: SessionCallbacks) {
        // 不复位 cancelled：若 cancel() 在 run 被调度后、真正执行前到达，复位会抹掉这次取消。
        if (cancelled) { // run 启动前已被取消（被调度但尚未跑）：直接 CANCELLED，不先发 RUNNING 制造多余状态跳变
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
            // 只接 Exception：OOM/LinkageError 等 Error 不在此吞，让其上抛，避免掩盖致命问题。
            // ProcessCanceledException 是 IntelliJ 取消信号，不吞。
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
