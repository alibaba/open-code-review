// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.services

import com.alibaba.opencodereview.idea.model.CommitInfo
import com.alibaba.opencodereview.idea.model.FileChange
import com.alibaba.opencodereview.idea.model.FileStatus
import com.alibaba.opencodereview.idea.model.GitState
import com.alibaba.opencodereview.idea.model.HostStrings
import com.alibaba.opencodereview.idea.model.ReviewContext
import com.alibaba.opencodereview.idea.model.ReviewMode
import com.alibaba.opencodereview.idea.model.SupportedLocale
import com.alibaba.opencodereview.idea.model.currentIdeLocale
import com.intellij.diff.DiffContentFactory

import com.intellij.diff.DiffManager
import com.intellij.diff.requests.SimpleDiffRequest
import com.intellij.diff.util.DiffUserDataKeys
import com.intellij.diff.util.Side
import com.intellij.openapi.Disposable
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.diagnostic.thisLogger
import com.intellij.openapi.editor.Document
import com.intellij.openapi.fileTypes.FileTypeManager
import com.intellij.openapi.project.Project
import com.intellij.openapi.util.Disposer
import com.intellij.openapi.vfs.VirtualFileManager
import com.intellij.openapi.vfs.newvfs.BulkFileListener
import com.intellij.openapi.vfs.newvfs.events.VFileEvent
import com.intellij.util.concurrency.AppExecutorUtil
import java.io.File
import java.util.concurrent.ScheduledFuture
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicReference

/**
 * Calls the command-line git directly. Every git invocation is forced to include `-c core.quotepath=false`.
 */
class GitService(private val project: Project) {

    companion object {
        private const val GIT_TIMEOUT_MS = 30_000L
        private const val RECENT_COMMITS = 20
        private const val BINARY_SAMPLE_BYTES = 8000
        private const val BINARY_SIZE_THRESHOLD = 512_000L

        /** Debounce window for VFS events. A single save or branch switch fires hundreds of events; one git run is enough. */
        private const val WATCH_DEBOUNCE_MS = 500L

        /** Field separator for `git log --format`. U+001F (Unit Separator) is used, which cannot appear in a commit message. */
        private const val UNIT = '\u001F'
    }

    @Volatile
    private var cache: GitState = GitState()

    @Volatile
    private var cachedRoot: File? = null

    /** Windows / macOS default filesystems are case-insensitive; VFS event paths may differ from root in case, so prefix matching must ignore case. The OS does not change at runtime, so this is computed once at construction. */
    private val caseInsensitiveFs: Boolean = System.getProperty("os.name").orEmpty().let {
        it.startsWith("Win", true) || it.startsWith("Mac", true)
    }

    /** Statuses of the files touched by the current review, used by comment mounting to judge "file deleted / file binary". */
    private val reviewFileStatus = mutableMapOf<String, FileStatus>()

    // ---------------------------------------------------------------- Repo root

    /** Repository root; returns null when not inside a git repository. The result is cached and re-resolved after [invalidate]. */
    fun repoRoot(): File? {
        cachedRoot?.let { return it }
        val base = project.basePath?.let(::File) ?: return null
        val out = runGitOrNull(base, "rev-parse", "--show-toplevel")?.trim()
        if (out.isNullOrEmpty()) return null
        val root = File(out)
        if (!root.isDirectory) return null
        cachedRoot = root
        return root
    }

    // ---------------------------------------------------------------- State refresh

    /**
     * Refreshes and returns the git state for the given review mode.
     * Only the data the current mode needs is queried: workspace mode does not need the branch list,
     * branch mode does not need the commit list.
     */
    fun getState(mode: ReviewMode): GitState = synchronized(this) {
        val root = repoRoot() ?: return@synchronized GitState()
        var state = cache
        when (mode) {
            ReviewMode.WORKSPACE -> state = state.copy(
                currentBranch = refreshCurrentBranch(root),
                workspaceFiles = refreshWorkspaceFiles(root),
            )

            ReviewMode.BRANCH -> state = state.copy(
                currentBranch = refreshCurrentBranch(root),
                branches = refreshBranches(root),
            )

            ReviewMode.COMMIT -> state = state.copy(
                currentBranch = refreshCurrentBranch(root),
                recentCommits = refreshRecentCommits(root),
            )
        }
        cache = state
        state
    }

    /** The result of the last [getState], triggering no git calls. */
    fun cachedState(): GitState = cache

    private fun refreshCurrentBranch(root: File): String {
        val branch = runGitOrNull(root, "rev-parse", "--abbrev-ref", "HEAD")?.trim().orEmpty()
        // On a detached HEAD, --abbrev-ref returns "HEAD"; falling back to the short hash is more meaningful.
        if (branch.isEmpty() || branch == "HEAD") {
            return runGitOrNull(root, "rev-parse", "--short", "HEAD")?.trim().orEmpty()
        }
        return branch
    }

    private fun refreshWorkspaceFiles(root: File): List<FileChange> {
        val diffHead = runGitOrNull(root, "diff", "--name-status", "HEAD").orEmpty()
        // No HEAD before the first commit; fall back to the staging area.
        val diffCached = if (diffHead.isBlank()) {
            runGitOrNull(root, "diff", "--name-status", "--cached").orEmpty()
        } else {
            ""
        }
        val untracked = runGitOrNull(root, "ls-files", "--others", "--exclude-standard").orEmpty()
        return buildWorkspaceFiles(diffHead, diffCached, untracked)
    }

    private fun refreshBranches(root: File): List<String> {
        val out = runGitOrNull(root, "branch", "-a", "--format=%(refname)") ?: return emptyList()
        val parsed = parseBranchList(out)
        // origin/HEAD points at the default remote branch (origin/master or origin/main); both are moved to the
        // front of the list, right after the local default branch.
        val defaultRemote = runGitOrNull(root, "symbolic-ref", "refs/remotes/origin/HEAD")
            ?.trim()?.removePrefix("refs/remotes/")?.takeIf { it.isNotBlank() }
        return pinDefaultBranches(parsed, defaultRemote)
    }

    private fun refreshRecentCommits(root: File): List<CommitInfo> {
        val format = "--format=%h$UNIT%s$UNIT%at"
        val out = runGitOrNull(root, "log", "-$RECENT_COMMITS", format) ?: return emptyList()
        val now = System.currentTimeMillis()
        val locale = currentIdeLocale()
        return out.lineSequence()
            .filter { it.isNotBlank() }
            .mapNotNull { line ->
                val parts = line.split(UNIT)
                if (parts.size < 3) return@mapNotNull null
                CommitInfo(
                    sha = parts[0].trim(),
                    message = parts[1].trim(),
                    relativeTime = formatRelative(parts[2].trim().toLongOrNull(), now, locale),
                )
            }
            .toList()
    }

    // ---------------------------------------------------------------- Diff queries

    /**
     * Changed files between branches. Uses the three-dot range (`from...to`), i.e. comparing from the merge-base,
     * so new commits on the from branch are not counted as "changes" -- consistent with the OCR CLI's branch mode.
     */
    fun getBranchDiff(from: String, to: String): List<FileChange> {
        val root = repoRoot() ?: return emptyList()
        val fromRef = resolveGitRef(root, from) ?: return emptyList()
        val toRef = if (to.isBlank()) "HEAD" else resolveGitRef(root, to) ?: return emptyList()
        val out = runGitOrNull(root, "diff", "--name-status", "$fromRef...$toRef") ?: return emptyList()
        return parseNameStatus(out)
    }

    /** Files changed by a single commit. */
    fun getCommitFiles(sha: String): List<FileChange> {
        val root = repoRoot() ?: return emptyList()
        val safeSha = safeRef(sha) ?: return emptyList() // safeRef has already rejected empty/blank input
        val out = runGitOrNull(root, "show", "--name-status", "--format=", safeSha) ?: return emptyList()
        return parseNameStatus(out)
    }

    /**
     * Resolves a user-entered branch name to a ref that actually exists.
     * Tries the candidates from [branchRefCandidates] in order (add the origin/ prefix, swap main with master)
     * and returns null when none of them exists.
     */
    fun resolveGitRef(root: File, ref: String): String? {
        val safe = safeRef(ref) ?: return null
        for (candidate in branchRefCandidates(safe)) {
            val ok = runGitOrNull(root, "rev-parse", "--verify", "--quiet", candidate)
            if (!ok.isNullOrBlank()) return candidate
        }
        return null
    }

    /** The merge-base of two refs; returns null on failure. */
    fun mergeBase(from: String, to: String): String? {
        val root = repoRoot() ?: return null
        val fromRef = safeRef(from) ?: return null
        val toRef = safeRef(to) ?: return null
        return runGitOrNull(root, "merge-base", fromRef, toRef)?.trim()?.takeIf { it.isNotEmpty() }
    }

    // ---------------------------------------------------------------- File content

    /** Reads a file's content at a given ref; returns null when the path does not exist at that ref. */
    fun readFileAtRef(ref: String, relPath: String): String? {
        val root = repoRoot() ?: return null
        val safe = safeRef(ref) ?: return null
        return runGitOrNull(root, "show", "$safe:$relPath")
    }

    /** Reads the file's current workspace content; returns null when the file does not exist. Paths are resolved against repoRoot: the comment paths the CLI emits are repo-root-relative (the git diff default), so they must be joined against the repo root even when the project is opened in a subdirectory. */
    fun readWorkspaceFile(relPath: String): String? {
        val root = repoRoot() ?: return null
        val file = File(root, relPath)
        // Path traversal guard: canonicalPath resolves ../ and symlinks to the real path, which is then checked
        // to still live under the repo root.
        // runCatching as the safety net: an IO error from canonicalPath must not crash; treat it as "not in the repo" (graceful degradation).
        val inRepo = runCatching { file.canonicalPath.startsWith(root.canonicalPath + File.separator) }.getOrDefault(false)
        if (!inRepo) return null
        if (!file.isFile) return null
        return runCatching { file.readText() }.getOrNull()
    }

    /** Whether a path exists at a given ref. Uses `cat-file -e`, reading no content. */
    fun pathExistsAtRef(ref: String, relPath: String): Boolean {
        val root = repoRoot() ?: return false
        val safe = safeRef(ref) ?: return false
        // cat-file -e prints nothing and exits 0 on success, so "null = failure" must be distinguished from "empty string = success".
        return runGitOrNull(root, "cat-file", "-e", "$safe:$relPath") != null
    }

    /**
     * Whether this is a binary file. The test is a NUL byte in the content -- the same heuristic git itself uses.
     * Called on demand instead of batch-checked in [prepareReviewFileStatus], so hundreds of files are not read
     * through for nothing.
     */
    fun isBinaryFile(relPath: String, ctx: ReviewContext): Boolean {
        val root = repoRoot() ?: return false
        if (ctx.mode == ReviewMode.WORKSPACE) {
            val file = File(root, relPath)
            // Path traversal guard: same as readWorkspaceFile, resolve via canonicalPath and confirm it stays under the repo root.
            val inRepo = runCatching { file.canonicalPath.startsWith(root.canonicalPath + File.separator) }.getOrDefault(false)
            if (!inRepo || !file.isFile) return false
            return runCatching {
                file.inputStream().use { it.readNBytes(BINARY_SAMPLE_BYTES).any { b -> b == 0.toByte() } }
            }.getOrDefault(false)
        }
        // Non-workspace: check the git object size first and declare oversized files binary without reading them
        val ref = when (ctx.mode) {
            ReviewMode.BRANCH -> ctx.to?.takeIf { it.isNotBlank() } ?: "HEAD"
            ReviewMode.COMMIT -> ctx.commit?.takeIf { it.isNotBlank() } ?: return false
            else -> return false
        }
        val sizeStr = runGitOrNull(root, "cat-file", "-s", "$ref:$relPath")?.trim()
        val size = sizeStr?.toLongOrNull() ?: return false
        if (size > BINARY_SIZE_THRESHOLD) return true
        val text = readFileAtRef(ref, relPath) ?: return false
        val limit = text.length.coerceAtMost(BINARY_SAMPLE_BYTES)
        return (0 until limit).any { text[it] == '\u0000' }
    }

    // ---------------------------------------------------------------- Review file status

    /**
     * Records the files involved in this review and their statuses at review start, for [getReviewFileStatus] to query.
     * Comment mounting relies on this data to tell "file was deleted, cannot jump" from "file exists but the line does not match".
     */
    fun prepareReviewFileStatus(ctx: ReviewContext) {
        val files = when (ctx.mode) {
            ReviewMode.WORKSPACE -> repoRoot()?.let(::refreshWorkspaceFiles).orEmpty()
            ReviewMode.BRANCH -> getBranchDiff(ctx.from.orEmpty(), ctx.to.orEmpty())
            ReviewMode.COMMIT -> getCommitFiles(ctx.commit.orEmpty())
        }
        synchronized(reviewFileStatus) {
            reviewFileStatus.clear()
            files.forEach { reviewFileStatus[it.path] = it.status }
        }
    }

    /** The change status of this path within the current review; returns null when it is out of scope. */
    fun getReviewFileStatus(relPath: String): FileStatus? =
        synchronized(reviewFileStatus) { reviewFileStatus[relPath] }

    // ---------------------------------------------------------------- Diff view

    /**
     * Hook that mounts content onto the two side documents when a diff opens. In branch/commit mode the two sides
     * are anonymous documents freshly created by DiffContentFactory with no handle available externally,
     * so the callback can only fire at creation time.
     */
    @Volatile
    var diffDecorator: ((relPath: String, side: Side, document: Document) -> Unit)? = null

    /**
     * Hook that runs after the diff viewer is created, used to mount inline panels.
     * Kept separate from diffDecorator: line highlights attach to the document and must run before showDiff,
     * while panels attach to the editor instance and must run after it.
     */
    @Volatile
    var diffViewerReady: ((relPath: String, side: Side, document: Document, clickedIndex: Int?) -> Unit)? = null

    /**
     * Opens a file's changes in the IDEA diff viewer. An added file gets empty left-side content and a deleted
     * file empty right-side content instead of an error, so the diff always opens and the user sees all-additions
     * or all-deletions.
     *
     * [scrollToSide]/[scrollToLine] make the opened diff scroll to and select the given line -- this is how
     * comment jumping lands on the comment's line inside the already-open diff.
     * [scrollToLine] is a 0-based document line number. Both must be provided to take effect; if either is null
     * the diff opens normally.
     */
    fun openDiff(
        relPath: String,
        status: FileStatus,
        ctx: ReviewContext,
        scrollToSide: Side? = null,
        scrollToLine: Int? = null,
        clickedIndex: Int? = null,
    ) {
        val root = repoRoot() ?: return
        val leftRef = leftRefFor(root, ctx)

        val left = if (status == FileStatus.ADDED) "" else readAtRefOrEmpty(leftRef, relPath)
        val right = if (status == FileStatus.DELETED) "" else readRightSide(relPath, ctx) ?: ""

        val fileType = FileTypeManager.getInstance().getFileTypeByFileName(File(relPath).name)
        val factory = DiffContentFactory.getInstance()
        val leftContent = factory.create(project, left, fileType)
        val rightContent = factory.create(project, right, fileType)

        val locale = currentIdeLocale()
        val rightTitle = if (ctx.mode == ReviewMode.WORKSPACE) {
            HostStrings.t(locale, "ext.git.workspace")
        } else {
            ctx.describeRight(locale)
        }
        val request = SimpleDiffRequest(
            relPath,
            leftContent,
            rightContent,
            leftRef ?: HostStrings.t(locale, "ext.git.emptyRef"),
            rightTitle,
        )
        if (scrollToSide != null && scrollToLine != null) {
            request.putUserData(DiffUserDataKeys.SCROLL_TO_LINE, com.intellij.openapi.util.Pair.create(scrollToSide, scrollToLine))
        }

        // DiffManager must be called on the EDT, while the review flow runs on a background thread.
        ApplicationManager.getApplication().invokeLater {
            if (project.isDisposed) return@invokeLater
            // Markers must attach before showDiff: the viewer builds its editors from these two documents, so with
            // markers already in place the gutter icons are there the moment the diff appears, with no "blank first,
            // redraw later" flash.
            diffDecorator?.let { decorate ->
                runCatching {
                    decorate(relPath, Side.LEFT, leftContent.document)
                    decorate(relPath, Side.RIGHT, rightContent.document)
                }.onFailure { thisLogger().warn("[ocr] Failed to decorate diff with comment markers", it) }
            }
            DiffManager.getInstance().showDiff(project, request)
            // Mount the inline panels only after the viewer has finished building -- the editor instances for the
            // documents are looked up through EditorFactory.
            diffViewerReady?.let { ready ->
                ApplicationManager.getApplication().invokeLater {
                    if (project.isDisposed) return@invokeLater
                    runCatching {
                        ready(relPath, Side.LEFT, leftContent.document, clickedIndex)
                        ready(relPath, Side.RIGHT, rightContent.document, clickedIndex)
                    }.onFailure { thisLogger().warn("[ocr] Failed to mount inline panels on diff", it) }
                }
            }
        }
    }

    /** The ref corresponding to the diff's left side (before the change). */
    private fun leftRefFor(root: File, ctx: ReviewContext): String? = when (ctx.mode) {
        ReviewMode.WORKSPACE -> "HEAD"
        ReviewMode.BRANCH -> {
            val from = resolveGitRef(root, ctx.from.orEmpty())
            val to = ctx.to?.takeIf { it.isNotBlank() }?.let { resolveGitRef(root, it) } ?: "HEAD"
            // Consistent with getBranchDiff's three-dot range: the left side takes the merge-base, not the current
            // position of from.
            // When mergeBase fails, return null (letting openDiff degrade gracefully to an empty left side) rather
            // than falling back to from, which would disagree with the file list's baseline.
            if (from != null) mergeBase(from, to) else null
        }

        ReviewMode.COMMIT -> ctx.commit?.takeIf { it.isNotBlank() }?.let { "$it^" }
    }

    /**
     * Root-less variant reused by [com.alibaba.opencodereview.idea.providers.resolveCommentAnchor] --
     * comment mounting and [openDiff] must use the same ref resolution, otherwise the line numbers computed at
     * mount time would not match the content version actually shown in the diff.
     */
    internal fun leftRefFor(ctx: ReviewContext): String? {
        val root = repoRoot() ?: return null
        return leftRefFor(root, ctx)
    }

    /**
     * The diff's right-side (after the change) ref; returns null for workspace mode, which has no notion of a ref.
     * Unlike [readRightSide], there is no "fall back to the workspace file when to is empty" safety net here --
     * comment mounting needs the plain "can this ref be resolved" judgment, and falling back to the workspace
     * would mask an incomplete review run that should report missing-file.
     */
    internal fun rightRefFor(ctx: ReviewContext): String? {
        val root = repoRoot() ?: return null
        return when (ctx.mode) {
            ReviewMode.WORKSPACE -> null
            ReviewMode.BRANCH -> ctx.to?.takeIf { it.isNotBlank() }?.let { resolveGitRef(root, it) }
            ReviewMode.COMMIT -> ctx.commit?.takeIf { it.isNotBlank() }
        }
    }

    /** The diff's right-side (after the change) content. Workspace mode reads the workspace; the other modes read the corresponding ref. */
    private fun readRightSide(relPath: String, ctx: ReviewContext): String? = when (ctx.mode) {
        ReviewMode.WORKSPACE -> readWorkspaceFile(relPath)
        ReviewMode.BRANCH -> {
            val to = ctx.to?.takeIf { it.isNotBlank() }
            if (to == null) readWorkspaceFile(relPath) else readFileAtRef(to, relPath)
        }

        ReviewMode.COMMIT -> ctx.commit?.takeIf { it.isNotBlank() }?.let { readFileAtRef(it, relPath) }
    }

    private fun readAtRefOrEmpty(ref: String?, relPath: String): String {
        if (ref == null) return ""
        return readFileAtRef(ref, relPath) ?: ""
    }

    private fun ReviewContext.describeRight(locale: SupportedLocale): String = when (mode) {
        ReviewMode.WORKSPACE -> HostStrings.t(locale, "ext.git.workspace")
        ReviewMode.BRANCH -> to?.takeIf { it.isNotBlank() } ?: HostStrings.t(locale, "ext.git.workspace")
        ReviewMode.COMMIT -> commit.orEmpty()
    }

    // ---------------------------------------------------------------- Change listener

    /**
     * Subscribes to workspace file changes and, debounced, calls back with the new workspace state.
     * Without this listener, the sidebar's to-review list would remain the snapshot taken when it was opened,
     * even after the user edits files.
     *
     * The return value is the subscription itself; disposing it disconnects (listening only while the sidebar lives).
     * [parent] is just a safety net (cleaned up together when the project closes); callers should dispose the
     * return value themselves when the sidebar closes, otherwise VFS events keep triggering `git status` for the
     * whole project lifetime with nobody receiving the results.
     */
    fun watchWorkspaceChanges(parent: Disposable, onChange: (GitState) -> Unit): Disposable {
        val subscription = Disposer.newDisposable("ocr.gitWatch")
        Disposer.register(parent, subscription)
        val pending = AtomicReference<ScheduledFuture<*>?>(null)
        val connection = project.messageBus.connect(subscription)
        connection.subscribe(VirtualFileManager.VFS_CHANGES, object : BulkFileListener {
            override fun after(events: List<VFileEvent>) {
                val root = cachedRoot ?: repoRoot() ?: return
                if (!events.any { it.isRelevantTo(root) }) return
                // A single save or branch switch fires tens to hundreds of VFS events; debouncing is mandatory,
                // otherwise git would run once per event.
                pending.getAndSet(
                    AppExecutorUtil.getAppScheduledExecutorService().schedule(
                        {
                            if (!project.isDisposed) {
                                runCatching { onChange(getState(ReviewMode.WORKSPACE)) }
                                    .onFailure { thisLogger().warn("[ocr] Failed to refresh workspace state", it) }
                            }
                        },
                        WATCH_DEBOUNCE_MS,
                        TimeUnit.MILLISECONDS,
                    ),
                )?.cancel(false)
            }
        })
        // Cancel the pending debounce task when the subscription disconnects, otherwise it would run git once more with nobody listening.
        Disposer.register(subscription) { pending.getAndSet(null)?.cancel(false) }
        return subscription
    }

    /**
     * Filters out events unrelated to the repository and noise from inside `.git`. Under `.git/`, only `index`
     * (staging changes, so `git add` is reflected in the list) and `HEAD` (branch switches) count;
     * the rest (objects/, logs/, lock files) change far too often and do not affect the file list -- letting them
     * through would keep resetting the debounce window so the state never finished refreshing.
     */
    private fun VFileEvent.isRelevantTo(root: File): Boolean {
        val path = path.replace(File.separatorChar, '/')
        val rootPath = root.absolutePath.replace(File.separatorChar, '/')
        if (!path.startsWith("$rootPath/", ignoreCase = caseInsensitiveFs)) return false
        // On a case-insensitive filesystem the path's case may differ from rootPath, and removePrefix is an exact
        // match that would fail; truncate by index instead so the two stay consistent.
        val relative = path.substring(rootPath.length + 1)
        if (!relative.startsWith(".git/") && relative != ".git") return true
        val inner = relative.removePrefix(".git/")
        return inner == "index" || inner == "HEAD"
    }

    // ---------------------------------------------------------------- Cache

    /** Drops the repo-root and state caches. Call after a repository switch or external changes. */
    fun invalidate() {
        cachedRoot = null
        cache = GitState()
        synchronized(reviewFileStatus) { reviewFileStatus.clear() }
    }

    // ---------------------------------------------------------------- Process

    /** Validates a user-entered ref/sha: non-empty after trim, not starting with `-`, and free of whitespace/control characters; otherwise returns null (so it cannot be parsed as a git option or used for line-break injection). */
    private fun safeRef(ref: String): String? {
        val t = ref.trim()
        if (t.isEmpty() || t.startsWith("-") || t.any { it.isWhitespace() || it.code < 0x20 }) return null
        return t
    }

    /**
     * Runs one git command and returns its stdout. Any failure (non-zero exit, timeout, missing git) returns null,
     * and callers treat "nothing returned" as "degrade" -- a failed git query must not crash the whole review flow.
     *
     * stderr is discarded: git's progress and hint output would pollute the stream, and all parsers only trust
     * the stdout format.
     */
    private fun runGitOrNull(cwd: File, vararg args: String): String? {
        val git = ShellEnv.resolveBin("git")
        val command = listOf(git, "-c", "core.quotepath=false") + args
        return try {
            val builder = ProcessBuilder(command)
                .directory(cwd)
                .redirectErrorStream(false)
                .redirectError(ProcessBuilder.Redirect.DISCARD)
            builder.environment().putAll(ShellEnv.env())
            val process = builder.start()
            process.outputStream.close()
            // stdout must be read on a separate thread: readText() on the current thread only returns at EOF, before
            // waitFor runs -- a hung process that never closes stdout would block forever, rendering the timeout moot.
            val stdout = StringBuilder()
            val reader = Thread({
                runCatching { process.inputStream.bufferedReader(Charsets.UTF_8).use { stdout.append(it.readText()) } }
                    .onFailure { thisLogger().warn("[ocr] Failed to read git stdout: ${args.joinToString(" ")}", it) }
            }, "ocr-git-stdout").apply { isDaemon = true; start() }
            if (!process.waitFor(GIT_TIMEOUT_MS, TimeUnit.MILLISECONDS)) {
                process.destroyForcibly()
                reader.join(1_000) // after the force-kill, let the reader finish so daemons do not pile up across repeated timeouts
                thisLogger().warn("[ocr] git timed out: ${args.joinToString(" ")}")
                return null
            }
            reader.join()
            if (process.exitValue() != 0) null else stdout.toString()
        } catch (e: Exception) {
            thisLogger().warn("[ocr] git execution failed: ${args.joinToString(" ")}", e)
            null
        }
    }
}

/**
 * Formats a unix-seconds timestamp as relative time. [nowMillis] is passed in by the caller instead of reading
 * `System.currentTimeMillis()` internally, for testability; likewise [locale] is provided by the caller to keep
 * the function pure.
 * The frontend's five buckets (justNow/hourAgo/hoursAgo/yesterday/daysAgo) are the base; this adds minutes,
 * months, and years.
 */
internal fun formatRelative(epochSeconds: Long?, nowMillis: Long, locale: SupportedLocale): String {
    if (epochSeconds == null || epochSeconds <= 0) return ""
    val diffMs = nowMillis - epochSeconds * 1000
    if (diffMs < 0) return HostStrings.t(locale, "ext.git.justNow")
    val minutes = diffMs / 60_000
    val hours = minutes / 60
    val days = hours / 24
    return when {
        minutes < 1 -> HostStrings.t(locale, "ext.git.justNow")
        minutes < 60 -> HostStrings.t(locale, "ext.git.minutesAgo", "m" to minutes.toString())
        hours == 1L -> HostStrings.t(locale, "ext.git.hourAgo")
        hours < 24 -> HostStrings.t(locale, "ext.git.hoursAgo", "h" to hours.toString())
        days == 1L -> HostStrings.t(locale, "ext.git.yesterday")
        days < 30 -> HostStrings.t(locale, "ext.git.daysAgo", "d" to days.toString())
        days < 365 -> HostStrings.t(locale, "ext.git.monthsAgo", "mo" to (days / 30).toString())
        else -> HostStrings.t(locale, "ext.git.yearsAgo", "y" to (days / 365).toString())
    }
}
