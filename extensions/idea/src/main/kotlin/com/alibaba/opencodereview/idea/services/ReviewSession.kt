package com.alibaba.opencodereview.idea.services

import com.alibaba.opencodereview.idea.model.CliResult
import com.alibaba.opencodereview.idea.model.CliRunOptions
import com.alibaba.opencodereview.idea.model.LogLevel
import com.alibaba.opencodereview.idea.model.LogLine
import com.alibaba.opencodereview.idea.model.ReviewState
import com.intellij.openapi.progress.ProcessCanceledException
import java.io.File
import java.util.concurrent.atomic.AtomicBoolean

/** 有评论则 done，无评论但 CLI 报错则 failed，否则 empty。 */
fun resultToState(result: CliResult): ReviewState = when {
    result.comments.isNotEmpty() -> ReviewState.DONE
    result.status == "completed_with_errors" -> ReviewState.FAILED
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

    @Volatile
    private var cancelled = false
    /** cancel 入口的原子锁：两个并发 cancel() 只有一个能通过 CAS，避免重复 cli.cancel()/onState(CANCELLED)。 */
    private val cancelEntered = AtomicBoolean(false)

    fun run(opts: CliRunOptions, cb: SessionCallbacks) {
        // 不复位 cancelled：若 cancel() 在 run 被调度后、真正执行前到达，复位会抹掉这次取消。
        if (cancelled) { // run 启动前已被取消（被调度但尚未跑）：直接 CANCELLED，不先发 RUNNING 制造多余状态跳变
            cb.onState(ReviewState.CANCELLED)
            return
        }
        cb.onState(ReviewState.RUNNING)
        try {
            val result = cli.review(opts, cwd, cb::onLog)
            if (cancelled) {
                cb.onState(ReviewState.CANCELLED)
                return
            }
            cb.onState(resultToState(result))
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
        // 原子进入：并发 cancel 只有一个通过 CAS，避免重复 cli.cancel()/onState。
        if (!cancelEntered.compareAndSet(false, true)) return
        cancelled = true
        cli.cancel()
        onState(ReviewState.CANCELLED)
    }
}
