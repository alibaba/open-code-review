package com.alibaba.opencodereview.idea.review

import com.alibaba.opencodereview.idea.messages.CommentActionKind
import com.alibaba.opencodereview.idea.messages.HostToWebview
import com.alibaba.opencodereview.idea.messages.WebviewToHost
import com.alibaba.opencodereview.idea.model.CliResult
import com.alibaba.opencodereview.idea.model.FileChange
import com.alibaba.opencodereview.idea.model.HostStrings
import com.alibaba.opencodereview.idea.model.LogLine
import com.alibaba.opencodereview.idea.model.ReviewContext
import com.alibaba.opencodereview.idea.model.ReviewMode
import com.alibaba.opencodereview.idea.model.ReviewState
import com.alibaba.opencodereview.idea.model.SupportedLocale
import com.alibaba.opencodereview.idea.model.toReviewContext
import com.alibaba.opencodereview.idea.providers.CommentService
import com.alibaba.opencodereview.idea.services.CliService
import com.alibaba.opencodereview.idea.services.ConfigService
import com.alibaba.opencodereview.idea.services.GitService
import com.alibaba.opencodereview.idea.services.ReviewSession
import com.alibaba.opencodereview.idea.services.SessionCallbacks
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.diagnostic.thisLogger
import com.intellij.openapi.project.Project
import com.intellij.openapi.util.io.FileUtil
import kotlinx.serialization.json.JsonElement
import java.io.File
import java.util.concurrent.atomic.AtomicReference

/**
 * 侧栏消息路由：接收前端 webview 在工作区审查流程中发来的全部消息并分发处理。
 *
 * 本类处理的消息类型见 [WebviewToHost]；配置面板的消息由 [ConfigPanelRouter] 处理，
 * 两者各自只处理归属自身通道的消息类型。
 *
 * 线程约定：凡涉及 git 或 CLI 的处理均提交至公共线程池执行，不在 EDT 上阻塞。
 */
class SidebarRouter(
    private val project: Project,
    private val cli: CliService,
    private val config: ConfigService,
    private val git: GitService,
    private val comments: CommentService,
    private val locale: () -> SupportedLocale,
    private val post: (HostToWebview) -> Unit,
    /** 打开配置面板。宿主侧的面板实现由 [ReviewProjectService] 注入。 */
    private val openConfigPanel: (JsonElement?) -> Unit,
) {

    private val session = AtomicReference<ReviewSession?>(null)

    fun handle(msg: WebviewToHost) {
        when (msg) {
            WebviewToHost.Ready -> background { sendInit() }

            is WebviewToHost.GetGitState -> background {
                post(HostToWebview.GitStateChanged(git.getState(msg.mode)))
            }

            is WebviewToHost.GetModeFiles -> background { sendModeFiles(msg) }

            is WebviewToHost.OpenFileDiff -> background {
                // openDiff 内部会自行切换到 EDT 打开差异视图，此处只负责读取内容。
                git.openDiff(msg.path, msg.status, msg.toReviewContext())
            }

            is WebviewToHost.StartReview -> background { startReview(msg) }

            WebviewToHost.CancelReview -> {
                // 在调用线程捕获目标 session：background(executeOnPooledThread) 与 StartReview 无序，
                // 若先进 background 再 StartReview 抢跑，session.get() 会取到新 session 误杀；捕获 target 即锁定旧目标。
                val target = session.get() ?: return
                background {
                    target.cancel { state ->
                        // 已被新一轮接管则不投递旧轮状态，避免 RUNNING(new)→CANCELLED(old) 乱序。
                        if (session.get() === target) post(HostToWebview.StateChange(state))
                    }
                }
            }

            WebviewToHost.GetConfig -> background { post(HostToWebview.Config(config.read())) }

            // jumpTo 在 diff 模式下需读取 git 引用中的文件内容，属于阻塞式进程调用；
            // 不能在 JCEF 消息回调线程执行，否则该线程阻塞会导致前端后续消息排队、界面无响应。
            is WebviewToHost.JumpToComment -> background { comments.jumpTo(msg.index) }

            // apply 同理：实际写入在 EDT 上完成，但需先解析路径、判断只读状态。
            is WebviewToHost.CommentAction -> background {
                when (msg.action) {
                    CommentActionKind.APPLY -> comments.apply(msg.index)
                    CommentActionKind.DISCARD -> comments.discard(msg.index)
                    CommentActionKind.FALSE_POSITIVE -> comments.falsePositive(msg.index)
                }
            }

            is WebviewToHost.OpenConfigPanel -> openConfigPanel(msg.focus)

            is WebviewToHost.Malformed -> {
                thisLogger().warn("[ocr] Sidebar received invalid message: ${msg.reason}")
                post(HostToWebview.StateChange(ReviewState.FAILED, msg.reason))
            }

            // 配置面板的消息不应出现在侧栏通道；无法识别的类型可能来自更高版本的前端，直接忽略。
            else -> thisLogger().debug("[ocr] Sidebar ignoring message: $msg")
        }
    }

    /**
     * 响应前端的 `ready` 消息，返回初始化所需的三项数据。
     *
     * 三字段必须齐全：缺 `config` 时前端会判定为未配置、停留在配置视图；
     * 缺 `gitState` 时模式选择器没有可选分支与文件。
     */
    private fun sendInit() {
        post(HostToWebview.Init(config.read(), git.getState(ReviewMode.WORKSPACE), locale()))
    }

    private fun sendModeFiles(msg: WebviewToHost.GetModeFiles) {
        // workspace 模式不在此处理：工作区文件清单由初始化时下发的 gitState 提供。
        val files: List<FileChange> = when {
            msg.mode == ReviewMode.BRANCH && msg.from != null && msg.to != null ->
                git.getBranchDiff(msg.from, msg.to)

            msg.mode == ReviewMode.COMMIT && msg.commit != null ->
                git.getCommitFiles(msg.commit)

            else -> emptyList()
        }
        post(HostToWebview.ModeFiles(msg.mode, files))
    }

    private fun startReview(msg: WebviewToHost.StartReview) {
        val cwd = reviewCwd()
        if (cwd == null) {
            post(HostToWebview.StateChange(ReviewState.FAILED, HostStrings.t(locale(), "ext.review.noProjectDir")))
            return
        }

        val options = msg.options
        val context = options.toReviewContext()
        // 先终止可能仍在运行的上一轮：否则其 onState/onLog/onDone 回调会继续投递过时消息，与新一轮结果交错。
        session.getAndSet(null)?.cancel { }
        val current = ReviewSession(cli, cwd)
        session.set(current)
        // 再清除上一轮的行标记，避免新旧两轮的高亮在同一文件上叠加。
        comments.clear()

        current.run(options, object : SessionCallbacks {
            // 身份校验：cancel() 只杀进程不摘回调，旧 run 仍可能在别的线程回调 onState 等；
            // 若本 session 已被新一轮替换（session.get() !== current），丢弃过时回调，避免 RUNNING(new)→CANCELLED(old) 乱序。
            override fun onState(state: ReviewState, error: String?) {
                if (session.get() !== current) return
                post(HostToWebview.StateChange(state, error))
            }

            override fun onLog(line: LogLine) {
                if (session.get() !== current) return
                post(HostToWebview.Log(line))
            }

            override fun onDone(result: CliResult) {
                if (session.get() !== current) return
                if (result.comments.isNotEmpty()) {
                    // 预先计算本次审查涉及的文件状态，供评论定位时区分"文件已删除"与"行号无法匹配"两种情况。
                    git.prepareReviewFileStatus(context)
                    comments.show(result.comments, context)
                }
                post(HostToWebview.ReviewDone(result))
            }
        })
    }

    /**
     * 确定审查使用的工作目录。
     *
     * 优先取项目根目录；当项目根不在 git 仓库内时回退到仓库根，
     * 否则 CLI 在 workspace 模式下无法获取 diff。
     */
    private fun reviewCwd(): File? {
        val base = project.basePath?.let(::File)?.takeIf { it.isDirectory }
        val root = git.repoRoot()
        return when {
            base == null -> root
            root == null -> base
            FileUtil.isAncestor(root, base, false) -> base
            else -> root
        }
    }

    private fun background(block: () -> Unit) {
        ApplicationManager.getApplication().executeOnPooledThread {
            if (project.isDisposed) return@executeOnPooledThread
            runCatching(block).onFailure { thisLogger().warn("[ocr] Sidebar message handling failed", it) }
        }
    }

    fun cancelActiveSession() {
        session.getAndSet(null)?.cancel { }
    }
}

/** `openFileDiff` 消息里的模式与引用字段，形状与 [ReviewContext] 相同。 */
private fun WebviewToHost.OpenFileDiff.toReviewContext() = ReviewContext(mode, from, to, commit)
