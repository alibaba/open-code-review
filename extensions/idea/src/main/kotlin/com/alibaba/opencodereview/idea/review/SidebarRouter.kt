// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

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
 * Sidebar message router: receives and dispatches every message the frontend webview sends during the
 * workspace review flow.
 *
 * The message types this class handles are listed in [WebviewToHost]; config-panel messages are handled by
 * [ConfigPanelRouter], and each handles only the types belonging to its own channel.
 *
 * Threading contract: anything involving git or the CLI is submitted to the shared pooled thread and never
 * blocks the EDT.
 */
class SidebarRouter(
    private val project: Project,
    private val cli: CliService,
    private val config: ConfigService,
    private val git: GitService,
    private val comments: CommentService,
    private val locale: () -> SupportedLocale,
    private val post: (HostToWebview) -> Unit,
    /** Opens the config panel. The host-side panel implementation is injected by [ReviewProjectService]. */
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
                // openDiff switches to the EDT itself to open the diff viewer; this side only reads the content.
                git.openDiff(msg.path, msg.status, msg.toReviewContext())
            }

            is WebviewToHost.StartReview -> background { startReview(msg) }

            WebviewToHost.CancelReview -> {
                // Capture the target session on the calling thread: background(executeOnPooledThread) and StartReview
                // are unordered -- if the background body ran after a new StartReview raced ahead, session.get() would
                // return the new session and kill it by mistake; capturing target locks in the old one.
                val target = session.get() ?: return
                background {
                    target.cancel { state ->
                        // If a new round has taken over, do not deliver the old round's state, avoiding out-of-order RUNNING(new) -> CANCELLED(old).
                        if (session.get() === target) post(HostToWebview.StateChange(state))
                    }
                }
            }

            WebviewToHost.GetConfig -> background { post(HostToWebview.Config(config.read())) }

            // In diff mode jumpTo reads file content from git refs -- a blocking process call;
            // it must not run on the JCEF message-callback thread, where blocking would queue up the frontend's
            // subsequent messages and freeze the UI.
            is WebviewToHost.JumpToComment -> background { comments.jumpTo(msg.index) }

            // Same for apply: the actual write happens on the EDT, but the path must be resolved and read-only status checked first.
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
                // Send FAILED only while a review is running, so normal states such as IDLE/DONE are not overwritten
                if (session.get() != null) {
                    session.getAndSet(null)
                    post(HostToWebview.StateChange(ReviewState.FAILED, msg.reason))
                }
            }

            // Config-panel messages must not appear on the sidebar channel; unrecognized types may come from a newer frontend and are ignored.
            else -> thisLogger().debug("[ocr] Sidebar ignoring message: $msg")
        }
    }

    /**
     * Answers the frontend's `ready` message with the three pieces of data initialization needs.
     *
     * All three fields are mandatory: without `config` the frontend decides it is unconfigured and stays on the
     * config view; without `gitState` the mode selector has no branches or files to offer.
     */
    private fun sendInit() {
        post(HostToWebview.Init(config.read(), git.getState(ReviewMode.WORKSPACE), locale()))
    }

    private fun sendModeFiles(msg: WebviewToHost.GetModeFiles) {
        // Workspace mode is not handled here: the workspace file list is delivered at init via gitState.
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
        val current = ReviewSession(cli, cwd)
        // Replace atomically so concurrent starts cannot lose an uncancelled session.
        session.getAndSet(current)?.cancel { }
        // Then clear the previous round's line markers so highlights from the old and new rounds do not stack on the same file.
        comments.clear()

        current.run(options, object : SessionCallbacks {
            // Identity checks: cancel() only kills the process without unregistering callbacks, so an old run can
            // still fire onState and friends from another thread;
            // if this session was replaced by a new round (session.get() !== current), drop the stale callback to
            // avoid out-of-order RUNNING(new) -> CANCELLED(old).
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
                    // Precompute the file statuses for this review so comment anchoring can tell "file was deleted"
                    // from "line number could not be matched".
                    git.prepareReviewFileStatus(context)
                    comments.show(result.comments, context)
                }
                post(HostToWebview.ReviewDone(result))
            }
        })
    }

    /**
     * Determines the working directory the review runs in.
     *
     * The project root is preferred; when the project root is not inside a git repository, fall back to the repo
     * root -- otherwise the CLI cannot get a diff in workspace mode.
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

/** The mode/ref fields inside the `openFileDiff` message, shaped the same as [ReviewContext]. */
private fun WebviewToHost.OpenFileDiff.toReviewContext() = ReviewContext(mode, from, to, commit)
