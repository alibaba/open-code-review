// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.providers

import com.alibaba.opencodereview.idea.model.CommentStatus
import com.alibaba.opencodereview.idea.model.CommentSyncState
import com.alibaba.opencodereview.idea.model.FileStatus
import com.alibaba.opencodereview.idea.model.HostStrings
import com.alibaba.opencodereview.idea.model.ReviewComment
import com.alibaba.opencodereview.idea.model.ReviewContext
import com.alibaba.opencodereview.idea.model.ReviewMode
import com.alibaba.opencodereview.idea.model.SupportedLocale
import com.alibaba.opencodereview.idea.services.GitService
import com.intellij.diff.util.Side
import com.intellij.icons.AllIcons
import com.intellij.notification.NotificationType
import com.intellij.openapi.Disposable
import com.intellij.openapi.actionSystem.AnAction
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.command.WriteCommandAction
import com.intellij.openapi.diagnostic.thisLogger
import com.intellij.openapi.editor.Document
import com.intellij.openapi.editor.EditorFactory
import com.intellij.openapi.editor.Inlay
import com.intellij.openapi.editor.ex.EditorEx
import com.intellij.openapi.editor.ex.RangeHighlighterEx
import com.intellij.openapi.editor.impl.DocumentMarkupModel
import com.intellij.openapi.editor.impl.EditorEmbeddedComponentManager
import com.intellij.openapi.editor.markup.EffectType
import com.intellij.openapi.editor.markup.GutterIconRenderer
import com.intellij.openapi.editor.markup.HighlighterLayer
import com.intellij.openapi.editor.markup.HighlighterTargetArea
import com.intellij.openapi.editor.markup.RangeHighlighter
import com.intellij.openapi.editor.markup.TextAttributes
import com.intellij.openapi.fileEditor.FileDocumentManager
import com.intellij.openapi.fileEditor.FileEditorManager
import com.intellij.openapi.fileEditor.FileEditorManagerListener
import com.intellij.openapi.fileEditor.OpenFileDescriptor
import com.intellij.openapi.fileEditor.TextEditor
import com.intellij.openapi.project.DumbAwareAction
import com.intellij.openapi.project.Project
import com.intellij.openapi.vfs.LocalFileSystem
import com.intellij.openapi.vfs.ReadonlyStatusHandler
import com.intellij.openapi.vfs.VirtualFile
import com.intellij.ui.ColorUtil
import com.intellij.ui.JBColor
import com.intellij.ui.components.JBLabel
import com.intellij.util.ui.JBUI
import java.awt.BorderLayout
import java.awt.Font

import java.nio.file.Path
import javax.swing.JButton
import javax.swing.JPanel
import javax.swing.JTextArea

/**
 * Comment mounting and actions. The three-stage fallback uses valid line numbers, then an existingCode sliding-window match,
 * then sidebar-only display; see [resolveCommentAnchor].
 *
 * Workspace mode: RangeHighlighter is itself a RangeMarker, so offsets adjust automatically after document edits.
 * [apply] reads the current line range from the anchor; [jumpTo] reads its current start line.
 *
 * Branch/commit mode: content is a read-only Git snapshot; [mounts] stores only the side and line numbers needed for navigation.
 * GitService.diffDecorator attaches editor markers through a callback when the document is created.
 */
class CommentService(
    private val project: Project,
    private val git: GitService,
    private val notify: (String, NotificationType) -> Unit,
    private val locale: () -> SupportedLocale,
) : Disposable {

    /**
     * Comment anchor. Both modes use the full line range (startLine/endLine, 1-based and inclusive).
     * Always obtain the range through lineRangeIn instead of reading the fields independently.
     *
     * Workspace mode uses the live RangeHighlighter (RangeMarker) as the source of the current position.
     * Branch/commit mode uses the stored line numbers and does not need a live anchor.
     */
    private sealed class MountTarget {
        abstract val startLine: Int
        abstract val endLine: Int

        data class Workspace(
            val highlighter: RangeHighlighter,
            override val startLine: Int,
            override val endLine: Int,
        ) : MountTarget()

        data class Diff(
            val side: AnchorSide,
            override val startLine: Int,
            override val endLine: Int,
        ) : MountTarget()
    }

    /** Callback for comment state changes, carrying the commentSync payload sent to the frontend. */
    var onSync: ((List<CommentSyncState>) -> Unit)? = null

    /**
     * Match VS Code in workspace mode: opening a file expands all mounted comments below their respective code lines.
     * Do not open closed files proactively; FileEditorManagerListener mounts their comments when opened.
     */
    init {
        project.messageBus.connect(this).subscribe(
            FileEditorManagerListener.FILE_EDITOR_MANAGER,
            object : FileEditorManagerListener {
                override fun fileOpened(source: FileEditorManager, file: VirtualFile) {
                    mountInlineCommentsForFile(file)
                }
            },
        )
    }

    private val lock = Any()
    private var comments: List<ReviewComment> = emptyList()
    private var context: ReviewContext? = null
    private val statuses = mutableMapOf<Int, CommentStatus>()
    private val mounts = mutableMapOf<Int, MountTarget>()
    private val jumpBlockReasons = mutableMapOf<Int, SidebarOnlyReason>()

    /** Diff markers, rebuilt for the temporary documents each time a diff is opened. */
    private val diffHighlighters = mutableMapOf<Int, RangeHighlighter>()

    /**
     * Inline panels expanded below workspace comments. The key includes EditorEx because an Inlay belongs to an editor
     * rather than a Document; two panes showing the same file need independent panels.
     */
    private val panels = mutableMapOf<Pair<Int, EditorEx>, Inlay<*>>()

    /** Inline panel expansion state. setStatus collapses handled comments; jumpTo expands them again. */
    private val expanded = mutableMapOf<Int, Boolean>()

    /** The relocation note from the three-stage fallback, displayed before the comment body. */
    private val locateNotes = mutableMapOf<Int, String>()

    fun show(comments: List<ReviewComment>, context: ReviewContext) {
        synchronized(lock) {
            this.comments = comments
            this.context = context
            statuses.clear()
            expanded.clear()
            jumpBlockReasons.clear()
            locateNotes.clear()
        }
        clearHighlighters()

        val currentLocale = locale()
        val resolved = comments.mapIndexed { index, comment ->
            index to resolveCommentAnchor(comment, context, git, currentLocale)
        }
        synchronized(lock) {
            resolved.forEach { (index, result) ->
                when (result) {
                    is CommentAnchorResult.Mountable -> {
                        result.locateNote?.let { locateNotes[index] = it }
                        if (result.side != AnchorSide.WORKSPACE) {
                            mounts[index] = MountTarget.Diff(result.side, result.startLine, result.endLine)
                        }
                    }

                    is CommentAnchorResult.SidebarOnly -> jumpBlockReasons[index] = result.reason
                }
            }
        }

        val workspaceItems = resolved.filter { (_, r) ->
            r is CommentAnchorResult.Mountable && r.side == AnchorSide.WORKSPACE
        }
        if (workspaceItems.isEmpty()) {
            publishSync()
            jumpToFirstMounted()
            return
        }
        ApplicationManager.getApplication().invokeLater {
            if (!project.isDisposed) {
                workspaceItems.forEach { (index, result) ->
                    mountWorkspaceHighlighter(index, comments[index], result as CommentAnchorResult.Mountable)
                }
            }
            publishSync()
            jumpToFirstMounted()
        }
    }

    /** Automatically navigate to the first mounted comment after mounting finishes. */
    private fun jumpToFirstMounted() {
        val first = synchronized(lock) { mounts.keys.minOrNull() } ?: return
        if (ApplicationManager.getApplication().isDispatchThread) {
            ApplicationManager.getApplication().executeOnPooledThread {
                if (!project.isDisposed) jumpTo(first)
            }
        } else {
            jumpTo(first)
        }
    }

    /** Clear all comments and highlights. */
    fun clear() {
        synchronized(lock) {
            comments = emptyList()
            context = null
            statuses.clear()
            expanded.clear()
        }
        clearHighlighters()
        publishSync()
    }

    fun jumpTo(index: Int) {
        val (comment, ctx) = snapshot(index) ?: return
        when (val target = synchronized(lock) { mounts[index] }) {
            is MountTarget.Workspace -> ApplicationManager.getApplication().invokeLater {
                if (project.isDisposed) return@invokeLater
                val highlighter = target.highlighter
                val file = resolveProjectFile(comment.path)
                if (!highlighter.isValid || file == null) {
                    notifyJumpFailed(index, comment)
                    return@invokeLater
                }
                val line = highlighter.document.getLineNumber(highlighter.startOffset)
                FileEditorManager.getInstance(project).openTextEditor(
                    OpenFileDescriptor(project, file, line, 0), true
                )
                synchronized(lock) { expanded[index] = true }
                rebuildInlinePanels(index, comment)
                mountInlineCommentsForFile(file)
            }

            is MountTarget.Diff -> {
                val status = git.getReviewFileStatus(comment.path) ?: FileStatus.MODIFIED
                val side = if (target.side == AnchorSide.LEFT) Side.LEFT else Side.RIGHT
                git.openDiff(comment.path, status, ctx, side, (target.startLine - 1).coerceAtLeast(0), index)
            }

            null -> notifyJumpFailed(index, comment)
        }
    }

    /**
     * Apply a suggestion, allowed only in workspace mode. Delete the marked range when suggestionCode is absent.
     */
    fun apply(index: Int) {
        val (comment, ctx) = snapshot(index) ?: return
        if (ctx.mode != ReviewMode.WORKSPACE) {
            notify(HostStrings.t(locale(), "ext.comment.applyWorkspaceOnly"), NotificationType.WARNING)
            return
        }
        val file = resolveProjectFile(comment.path) ?: return

        ApplicationManager.getApplication().invokeLater {
            if (project.isDisposed) return@invokeLater
            val document = FileDocumentManager.getInstance().getDocument(file) ?: return@invokeLater
            if (document.lineCount == 0) return@invokeLater

            val range = lineRangeIn(index, document) ?: run {
                notify(HostStrings.t(locale(), "ext.comment.applyFailedStale"), NotificationType.ERROR)
                return@invokeLater
            }
            val startLine = range.first
            val endLine = range.last

            // Call ensureFilesWritable before isWritable so IDEA can show its version-control checkout dialog.
            val writable = ReadonlyStatusHandler.getInstance(project).ensureFilesWritable(listOf(file))
            if (writable.hasReadonlyFiles() || !document.isWritable) {
                notify(HostStrings.t(locale(), "ext.comment.applyFailedLocked"), NotificationType.ERROR)
                return@invokeLater
            }

            val suggestion = comment.suggestionCode?.takeIf { it.isNotBlank() }
            val edited = runCatching {
                WriteCommandAction.runWriteCommandAction(project) {
                    val startOffset = document.getLineStartOffset(startLine)
                    if (suggestion != null) {
                        val endOffset = document.getLineEndOffset(endLine)
                        document.replaceString(startOffset, endOffset, suggestion)
                    } else {
                        val endOffset = if (endLine + 1 < document.lineCount)
                            document.getLineStartOffset(endLine + 1)
                        else
                            document.getLineEndOffset(endLine)
                        document.deleteString(startOffset, endOffset)
                    }
                    FileDocumentManager.getInstance().saveDocument(document)
                }
            }
            if (edited.isFailure) {
                thisLogger().warn("apply comment #$index failed", edited.exceptionOrNull())
                notify(HostStrings.t(locale(), "ext.comment.applyFailedLocked"), NotificationType.ERROR)
                return@invokeLater
            }
            FileEditorManager.getInstance(project).openTextEditor(
                OpenFileDescriptor(project, file, startLine, 0), true
            )
            setStatus(index, CommentStatus.APPLIED)
        }
    }

    fun discard(index: Int) = setStatus(index, CommentStatus.DISCARDED)

    fun falsePositive(index: Int) = setStatus(index, CommentStatus.FALSE_POSITIVE)

    // ---------------------------------------------------------------- Internal helpers

    private fun snapshot(index: Int): Pair<ReviewComment, ReviewContext>? = synchronized(lock) {
        val comment = comments.getOrNull(index) ?: return null
        val ctx = context ?: return null
        comment to ctx
    }

    /**
     * The lines currently occupied by the comment at [index] in the document (0-based inclusive range).
     * Line backgrounds, inline panels, navigation, and applying suggestions all use this single entry point.
     *
     * In workspace mode, prefer the live anchor (RangeHighlighter follows user edits).
     * If it is invalid, use the originally resolved lines. Null means no anchor, so display only in the sidebar.
     */
    private fun lineRangeIn(index: Int, document: Document): IntRange? {
        if (document.lineCount == 0) return null
        val target = synchronized(lock) { mounts[index] } ?: return null
        if (target is MountTarget.Workspace) {
            val highlighter = target.highlighter
            if (highlighter.isValid && highlighter.document == document) {
                return document.getLineNumber(highlighter.startOffset)..
                    document.getLineNumber(highlighter.endOffset)
            }
        }
        return clampLineRange(target.startLine, target.endLine, document.lineCount)
    }

    private fun setStatus(index: Int, status: CommentStatus) {
        val comment = synchronized(lock) {
            if (index !in comments.indices) return
            statuses[index] = status
            expanded[index] = false
            comments[index]
        }
        refreshGutter(index, comment, status)
        publishSync()
    }

    /** Refresh gutter icons, tooltips, error stripes, and inline panels when status changes. */
    private fun refreshGutter(index: Int, comment: ReviewComment, status: CommentStatus) {
        val loc = locale()
        val targets = synchronized(lock) {
            listOfNotNull(
                (mounts[index] as? MountTarget.Workspace)?.highlighter,
                diffHighlighters[index],
            )
        }
        if (targets.isNotEmpty()) {
            ApplicationManager.getApplication().invokeLater {
                if (project.isDisposed) return@invokeLater
                val diffHl = synchronized(lock) { diffHighlighters[index] }
                targets.forEach { highlighter ->
                    if (!highlighter.isValid) return@forEach
                    highlighter.gutterIconRenderer = CommentGutterIconRenderer(index, comment, status)
                    highlighter.setErrorStripeTooltip(tooltipFor(index, comment, status, loc))
                    highlighter.setErrorStripeMarkColor(
                        if (status == CommentStatus.PENDING) PENDING_STRIPE else null
                    )
                    val attrs = if (highlighter === diffHl) diffLineAttributes(status) else lineAttributes(status)
                    (highlighter as? RangeHighlighterEx)?.setTextAttributes(attrs)
                }
            }
        }
        rebuildInlinePanels(index, comment)
    }

    /** Rebuild the inline panel with the latest status and expansion state; skip if no existing panel is found. */
    private fun rebuildInlinePanels(index: Int, comment: ReviewComment) {
        val stale = synchronized(lock) { panels.keys.filter { it.first == index } }
        if (stale.isEmpty()) return
        ApplicationManager.getApplication().invokeLater {
            if (project.isDisposed) return@invokeLater
            stale.forEach { key ->
                val editor = key.second
                val old = synchronized(lock) { panels.remove(key) }
                val offset = old?.offset ?: return@forEach
                old.let { runCatching { it.dispose() } }
                if (editor.isDisposed) return@forEach
                mountInlineComment(index, comment, editor, offset)
            }
        }
    }

    private fun toggleExpanded(index: Int, comment: ReviewComment) {
        synchronized(lock) { expanded[index] = !(expanded[index] ?: true) }
        rebuildInlinePanels(index, comment)
    }

    private fun tooltipFor(index: Int, comment: ReviewComment, status: CommentStatus, loc: SupportedLocale): String =
        buildString {
            append(statusLabel(status, loc)).append('\n')
            synchronized(lock) { locateNotes[index] }?.let { append(it).append('\n') }
            append(comment.content)
        }

    private fun statusLabel(status: CommentStatus, loc: SupportedLocale): String = HostStrings.t(
        loc,
        when (status) {
            CommentStatus.PENDING -> "ext.comment.pending"
            CommentStatus.APPLIED -> "ext.comment.statusApplied"
            CommentStatus.DISCARDED -> "ext.comment.statusDiscarded"
            CommentStatus.FALSE_POSITIVE -> "ext.comment.statusFalsePositive"
        },
    )

    private fun publishSync() {
        val states = synchronized(lock) {
            comments.mapIndexed { index, _ ->
                CommentSyncState(
                    index = index,
                    status = statuses[index] ?: CommentStatus.PENDING,
                    jumpable = mounts.containsKey(index),
                )
            }
        }
        onSync?.invoke(states)
    }

    private fun notifyJumpFailed(index: Int, comment: ReviewComment) {
        val reason = synchronized(lock) { jumpBlockReasons[index] } ?: inferJumpBlockReason(comment)
        val message = when (reason) {
            SidebarOnlyReason.MISSING_FILE, SidebarOnlyReason.MOUNT_FAILED ->
                HostStrings.t(locale(), "ext.comment.jumpFileMissing", "path" to comment.path)

            SidebarOnlyReason.BINARY, SidebarOnlyReason.UNRESOLVED ->
                HostStrings.t(locale(), "ext.comment.jumpLineUnresolved", "path" to comment.path)
        }
        notify(message, NotificationType.WARNING)
    }

    private fun inferJumpBlockReason(comment: ReviewComment): SidebarOnlyReason =
        if (comment.startLine <= 0 && comment.endLine <= 0) SidebarOnlyReason.UNRESOLVED
        else SidebarOnlyReason.MISSING_FILE

    private fun mountWorkspaceHighlighter(
        index: Int, comment: ReviewComment, mountable: CommentAnchorResult.Mountable,
    ) {
        val file = resolveProjectFile(comment.path)
        val document = file?.let { FileDocumentManager.getInstance().getDocument(it) }
        if (document == null || document.lineCount == 0) {
            synchronized(lock) { jumpBlockReasons[index] = SidebarOnlyReason.MOUNT_FAILED }
            return
        }
        val range = clampLineRange(mountable.startLine, mountable.endLine, document.lineCount)
        val status = synchronized(lock) { statuses[index] ?: CommentStatus.PENDING }
        val highlighter = DocumentMarkupModel.forDocument(document, project, true)
            .addRangeHighlighter(
                document.getLineStartOffset(range.first),
                document.getLineEndOffset(range.last),
                HighlighterLayer.WARNING,
                lineAttributes(status),
                HighlighterTargetArea.EXACT_RANGE,
            )
        highlighter.setErrorStripeMarkColor(PENDING_STRIPE)
        synchronized(lock) {
            mounts[index] = MountTarget.Workspace(highlighter, mountable.startLine, mountable.endLine)
        }
        highlighter.setErrorStripeTooltip(tooltipFor(index, comment, status, locale()))
        highlighter.gutterIconRenderer = CommentGutterIconRenderer(index, comment, status)

        editorsShowing(file).forEach { editor ->
            mountInlineComment(index, comment, editor, document.getLineEndOffset(range.last))
        }
    }

    /**
     * Attach comment markers (RangeHighlighter and gutter icons) belonging to this side of a diff document.
     * Called by GitService.openDiff on the EDT, once for each side.
     */
    fun decorateDiff(relPath: String, side: Side, document: Document) {
        val anchorSide = if (side == Side.LEFT) AnchorSide.LEFT else AnchorSide.RIGHT
        val all = diffMarks()
        val targets = selectDiffMarks(relPath, anchorSide, all)
        if (targets.isEmpty() || document.lineCount == 0) return

        val loc = locale()
        val markup = DocumentMarkupModel.forDocument(document, project, true)
        targets.forEach { mark ->
            val index = mark.index
            val comment = synchronized(lock) { comments.getOrNull(index) } ?: return@forEach
            val range = lineRangeIn(index, document) ?: return@forEach
            val status = synchronized(lock) { statuses[index] ?: CommentStatus.PENDING }
            val highlighter = markup.addRangeHighlighter(
                document.getLineStartOffset(range.first),
                document.getLineEndOffset(range.last),
                HighlighterLayer.WARNING,
                diffLineAttributes(status),
                HighlighterTargetArea.EXACT_RANGE,
            )
            highlighter.setErrorStripeMarkColor(
                if (status == CommentStatus.PENDING) PENDING_STRIPE else null
            )
            highlighter.setErrorStripeTooltip(tooltipFor(index, comment, status, loc))
            highlighter.gutterIconRenderer = CommentGutterIconRenderer(index, comment, status)
            val stale = synchronized(lock) { diffHighlighters.put(index, highlighter) }
            stale?.let { runCatching { it.dispose() } }
        }
    }

    /**
     * A gutter icon that toggles the inline panel when clicked.
     * Do not navigate to the line: the icon is already beside it, so the line is visible.
     * Navigation to other locations uses the sidebar card View button, which calls jumpTo.
     */
    private inner class CommentGutterIconRenderer(
        private val index: Int,
        private val comment: ReviewComment,
        private val status: CommentStatus,
    ) : GutterIconRenderer() {
        override fun getIcon() =
            if (status == CommentStatus.PENDING) AllIcons.General.TodoDefault
            else AllIcons.General.InspectionsOK

        override fun getTooltipText(): String = tooltipFor(index, comment, status, locale())
        override fun isNavigateAction(): Boolean = true
        override fun getClickAction(): AnAction = DumbAwareAction.create { toggleExpanded(index, comment) }

        override fun equals(other: Any?): Boolean =
            other is CommentGutterIconRenderer && other.index == index && other.status == status

        override fun hashCode(): Int = 31 * index + status.ordinal
    }

    private fun lineAttributes(status: CommentStatus): TextAttributes? =
        if (status != CommentStatus.PENDING) null
        else TextAttributes(null, ColorUtil.withAlpha(PENDING_STRIPE, 0.15), null, null, Font.PLAIN)

    private fun diffLineAttributes(status: CommentStatus): TextAttributes? =
        if (status != CommentStatus.PENDING) null
        else TextAttributes(null, null, PENDING_STRIPE, EffectType.ROUNDED_BOX, Font.PLAIN)

    private fun editorsShowing(file: VirtualFile?): List<EditorEx> {
        file ?: return emptyList()
        return FileEditorManager.getInstance(project).getEditors(file)
            .filterIsInstance<TextEditor>()
            .mapNotNull { it.editor as? EditorEx }
    }

    /**
     * Attach the inline comment panel below the given [offset] in the editor.
     * Skip existing panels; panels are deduplicated by (index, editor).
     */
    private fun mountInlineComment(index: Int, comment: ReviewComment, editor: EditorEx, offset: Int) {
        val key = index to editor
        synchronized(lock) { if (panels.containsKey(key)) return }
        val (status, isExpanded) = synchronized(lock) {
            (statuses[index] ?: CommentStatus.PENDING) to (expanded[index] ?: true)
        }
        val panel = buildCommentPanel(index, comment, status, isExpanded)
        runCatching {
            EditorEmbeddedComponentManager.getInstance().addComponent(
                editor,
                panel,
                EditorEmbeddedComponentManager.Properties(
                    EditorEmbeddedComponentManager.ResizePolicy.none(),
                    null,
                    true,   // relatesToPrecedingText
                    false,  // showAbove
                    false,  // showWhenFolded
                    true,   // fullWidth
                    0,
                    offset,
                ),
            )
        }.onSuccess { inlay ->
            if (inlay == null) return@onSuccess
            synchronized(lock) { panels.put(key, inlay) }?.let { runCatching { it.dispose() } }
        }.onFailure { thisLogger().warn("[ocr] Failed to mount inline comment panel #$index, falling back to gutter icon", it) }
    }

    /**
     * Attach inline comment panels to a diff view. Called by GitService.diffViewerReady after showDiff.
     * Find editor instances for the document through EditorFactory.
     */
    fun mountDiffPanels(relPath: String, side: Side, document: Document, clickedIndex: Int? = null) {
        val anchorSide = if (side == Side.LEFT) AnchorSide.LEFT else AnchorSide.RIGHT
        val targets = selectDiffMarks(relPath, anchorSide, diffMarks())
        if (targets.isEmpty() || document.lineCount == 0) return

        dropDisposedPanels()

        val editors = EditorFactory.getInstance().getEditors(document, project)
            .filterIsInstance<EditorEx>()
        if (editors.isEmpty()) return

        // Mount the clicked comment last so scrolling targets the clicked line.
        val ordered = if (clickedIndex == null) targets
            else targets.sortedBy { it.index == clickedIndex }

        ordered.forEach { mark ->
            val comment = synchronized(lock) { comments.getOrNull(mark.index) } ?: return@forEach
            val range = lineRangeIn(mark.index, document) ?: return@forEach
            editors.forEach { editor ->
                mountInlineComment(mark.index, comment, editor, document.getLineEndOffset(range.last))
            }
        }
    }

    private fun diffMarks(): List<DiffMark> = synchronized(lock) {
        comments.indices.mapNotNull { index ->
            val mount = mounts[index] as? MountTarget.Diff ?: return@mapNotNull null
            DiffMark(index, comments[index].path, mount.side, mount.startLine, mount.endLine)
        }
    }

    private fun dropDisposedPanels() {
        val dead = synchronized(lock) {
            val keys = panels.keys.filter { it.second.isDisposed }
            keys.mapNotNull { panels.remove(it) }
        }
        dead.forEach { runCatching { it.dispose() } }
    }

    /** Attach panels for resolved comments in [file], skipping any that already have a panel in the same editor. */
    private fun mountInlineCommentsForFile(file: VirtualFile) {
        val toMount = synchronized(lock) {
            mounts.entries.mapNotNull { (index, target) ->
                if (target !is MountTarget.Workspace) return@mapNotNull null
                val comment = comments.getOrNull(index) ?: return@mapNotNull null
                if (resolveProjectFile(comment.path) != file) return@mapNotNull null
                index to comment
            }
        }
        if (toMount.isEmpty()) return
        ApplicationManager.getApplication().invokeLater {
            if (project.isDisposed) return@invokeLater
            val editors = editorsShowing(file)
            if (editors.isEmpty()) return@invokeLater
            val document = FileDocumentManager.getInstance().getDocument(file) ?: return@invokeLater
            toMount.forEach { (index, comment) ->
                val range = lineRangeIn(index, document) ?: return@forEach
                val offset = document.getLineEndOffset(range.last)
                editors.forEach { editor -> mountInlineComment(index, comment, editor, offset) }
            }
        }
    }

    /**
     * Inline panel content: relocation note, body, separator, suggestion, and buttons.
     * When expanded is false, render only the title row (collapsed state).
     */
    private fun buildCommentPanel(
        index: Int, comment: ReviewComment, status: CommentStatus, expanded: Boolean,
    ): JPanel {
        val loc = locale()
        val total = synchronized(lock) { comments.size }
        val panel = JPanel(BorderLayout(0, 6)).apply { border = JBUI.Borders.empty(8, 12) }

        val arrow = if (expanded) "▾" else "▸"
        val header = JBLabel(
            "$arrow ${HostStrings.t(loc, "ext.comment.threadLabel")} (${index + 1} / $total) · ${statusLabel(status, loc)}"
        ).apply {
            font = font.deriveFont(Font.BOLD, font.size2D - 1f)
            cursor = java.awt.Cursor.getPredefinedCursor(java.awt.Cursor.HAND_CURSOR)
            addMouseListener(object : java.awt.event.MouseAdapter() {
                override fun mouseClicked(e: java.awt.event.MouseEvent) = toggleExpanded(index, comment)
            })
        }
        panel.add(header, BorderLayout.NORTH)
        if (!expanded) return panel

        val centerPanel = JPanel().apply {
            layout = javax.swing.BoxLayout(this, javax.swing.BoxLayout.Y_AXIS)
            isOpaque = false
        }
        val contentText = buildString {
            synchronized(lock) { locateNotes[index] }?.let { append(it).append("\n\n") }
            append(comment.content)
        }
        centerPanel.add(JTextArea(contentText).apply {
            isEditable = false; isOpaque = false; lineWrap = true; wrapStyleWord = true
        })
        centerPanel.add(javax.swing.Box.createVerticalStrut(8))
        centerPanel.add(JPanel().apply {
            background = javax.swing.UIManager.getColor("Label.foreground") ?: JBColor.foreground()
            minimumSize = java.awt.Dimension(0, 1)
            preferredSize = java.awt.Dimension(0, 1)
            maximumSize = java.awt.Dimension(Int.MAX_VALUE, 1)
        })
        centerPanel.add(javax.swing.Box.createVerticalStrut(8))
        val suggestionText = comment.suggestionCode?.takeIf { it.isNotBlank() }
            ?: HostStrings.t(loc, "ext.comment.noSuggestion")
        centerPanel.add(JTextArea(suggestionText).apply {
            isEditable = false; isOpaque = false; lineWrap = true; wrapStyleWord = true
        })
        panel.add(centerPanel, BorderLayout.CENTER)

        // Do not show action buttons for comments that have already been handled.
        if (status == CommentStatus.PENDING) {
            val buttons = JPanel()
            val workspace = snapshot(index)?.second?.mode == ReviewMode.WORKSPACE
            if (workspace && !comment.suggestionCode.isNullOrBlank()) {
                buttons.add(JButton(HostStrings.t(loc, "ext.comment.apply")).apply {
                    addActionListener { apply(index) }
                })
            }
            buttons.add(JButton(HostStrings.t(loc, "ext.comment.discard")).apply {
                addActionListener { discard(index) }
            })
            panel.add(buttons, BorderLayout.SOUTH)
        }
        return panel
    }

    private fun clearHighlighters() {
        val stale = synchronized(lock) {
            val copy = mounts.values.filterIsInstance<MountTarget.Workspace>().map { it.highlighter } +
                diffHighlighters.values
            val stalePanels = panels.values.toList()
            mounts.clear()
            diffHighlighters.clear()
            panels.clear()
            jumpBlockReasons.clear()
            locateNotes.clear()
            copy to stalePanels
        }
        if (stale.first.isEmpty() && stale.second.isEmpty()) return
        ApplicationManager.getApplication().invokeLater {
            stale.first.forEach { runCatching { it.dispose() } }
            stale.second.forEach { runCatching { it.dispose() } }
        }
    }

    /**
     * Resolve a CLI path within the repository, rejecting paths outside repoRoot.
     * Git diff comment paths are repository-relative. If the project is opened in a subdirectory,
     * resolving them against basePath would repeat the subdirectory and fail to find the file.
     */
    private fun resolveProjectFile(relative: String): VirtualFile? {
        val base = git.repoRoot()?.toPath()?.toRealPath() ?: return null
        val target = runCatching { base.resolve(relative).toRealPath() }.getOrNull() ?: return null
        if (!target.startsWith(base)) return null
        return LocalFileSystem.getInstance().refreshAndFindFileByNioFile(target)
    }

    override fun dispose() {
        onSync = null
        clearHighlighters()
    }

    private companion object {
        private val PENDING_STRIPE = JBColor(0xD97706, 0xF59E0B)
    }
}
