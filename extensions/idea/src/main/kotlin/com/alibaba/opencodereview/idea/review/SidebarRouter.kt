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
 * Sidebar message routing: receives and dispatches every message the frontend webview sends during
 * the workspace review flow.
 *
 * The message types handled here are listed in [WebviewToHost]; config panel messages go to
 * [ConfigPanelRouter], each handling only the types belonging to its own channel.
 *
 * Threading: anything touching git or the CLI is submitted to the shared thread pool and never
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
    /** Opens the config panel. The host-side panel implementation is injected by
     *  [ReviewProjectService]. */
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
                // openDiff switches to the EDT itself to open the diff view; this only reads the
                // content.
                git.openDiff(msg.path, msg.status, msg.toReviewContext())
            }

            is WebviewToHost.StartReview -> background { startReview(msg) }

            WebviewToHost.CancelReview -> {
                // Capture the target session on the calling thread: background
                // (executeOnPooledThread) and StartReview have no ordering guarantee, so if
                // background runs first and StartReview races ahead, session.get() returns the new
                // session and cancels the wrong one; capturing target pins the old target.
                val target = session.get() ?: return
                background {
                    target.cancel { state ->
                        // If a new run has taken over, do not deliver the old run's state, avoiding a
                        // RUNNING(new) → CANCELLED(old) reordering.
                        if (session.get() === target) post(HostToWebview.StateChange(state))
                    }
                }
            }

            WebviewToHost.GetConfig -> background { post(HostToWebview.Config(config.read())) }

            // In diff mode jumpTo reads file contents from a git ref, a blocking process call; it
            // must not run on the JCEF message callback thread, where the block would queue the
            // frontend's later messages and freeze the UI.
            is WebviewToHost.JumpToComment -> background { comments.jumpTo(msg.index) }

            // Same for apply: the write itself happens on the EDT, but the path must first be
            // resolved and the read-only state checked.
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
                // Only send FAILED while a review is running, so IDLE/DONE and other normal states
                // are not overwritten
                if (session.get() != null) {
                    session.getAndSet(null)
                    post(HostToWebview.StateChange(ReviewState.FAILED, msg.reason))
                }
            }

            // Config panel messages should not appear on the sidebar channel; an unrecognised type
            // may come from a newer frontend, so ignore it.
            else -> thisLogger().debug("[ocr] Sidebar ignoring message: $msg")
        }
    }

    /**
     * Answers the frontend's `ready` message with the three pieces of data initialization needs.
     *
     * All three fields must be present: without `config` the frontend treats the configuration as
     * unset and stays on the config view, and without `gitState` the mode selector has no branches
     * or files to choose from.
     */
    private fun sendInit() {
        post(HostToWebview.Init(config.read(), git.getState(ReviewMode.WORKSPACE), locale()))
    }

    private fun sendModeFiles(msg: WebviewToHost.GetModeFiles) {
        // workspace mode is not handled here: the workspace file list arrives in the gitState sent
        // at initialization.
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
        // Then clear the previous run's line marks, so the two runs' highlights do not stack up on
        // the same file.
        comments.clear()

        current.run(options, object : SessionCallbacks {
            // Identity check: cancel() kills the process without detaching callbacks, so the old run
            // may still call onState and friends from another thread; if a new run has replaced this
            // session (session.get() !== current), drop the stale callback to avoid a
            // RUNNING(new) → CANCELLED(old) reordering.
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
                    // Precompute the file states for this review, so comment anchoring can tell a
                    // deleted file from a line number that cannot be matched.
                    git.prepareReviewFileStatus(context)
                    comments.show(result.comments, context)
                }
                post(HostToWebview.ReviewDone(result))
            }
        })
    }

    /**
     * Determines the working directory for the review.
     *
     * Prefers the project root; when the project root is not inside a git repository it falls back
     * to the repository root, without which the CLI could not obtain a diff in workspace mode.
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

/** The mode and ref fields of an `openFileDiff` message, shaped like [ReviewContext]. */
private fun WebviewToHost.OpenFileDiff.toReviewContext() = ReviewContext(mode, from, to, commit)
