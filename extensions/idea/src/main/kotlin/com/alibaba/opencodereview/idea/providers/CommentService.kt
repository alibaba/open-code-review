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
 * 评论挂载与操作。三级定位回退（行号可用 → existingCode 滑窗重定位 → 仅侧栏）
 * 见 [resolveCommentAnchor]。
 *
 * workspace 模式：RangeHighlighter 本身是 RangeMarker，文档编辑后 offset 自行调整，
 * apply 与 jumpTo 直接问它当前行号。
 *
 * branch/commit 模式：内容为 git 引用只读快照，mounts 仅记录跳转所需的侧与行号，
 * 编辑器内标记由 GitService.diffDecorator 在文档创建时回调挂载。
 */
class CommentService(
    private val project: Project,
    private val git: GitService,
    private val notify: (String, NotificationType) -> Unit,
    private val locale: () -> SupportedLocale,
) : Disposable {

    /**
     * 评论落点，两模式共用完整行范围（startLine/endLine，1-based 闭区间）。
     * 取范围统一走 lineRangeIn，不各自读取字段。
     *
     * workspace 以 RangeHighlighter（RangeMarker）为当前位置真相。
     * branch/commit 以存下的行号为真相，不需要活锚点。
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

    /** 评论状态变化时的回调，载荷即发给前端的 commentSync 内容。 */
    var onSync: ((List<CommentSyncState>) -> Unit)? = null

    /**
     * workspace 模式对齐 VS Code：文件打开时，该文件所有已挂载评论立即在各自代码行下方展开。
     * 文件未打开时不主动打开，靠 FileEditorManagerListener 补挂。
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

    /** diff 视图的标记，随每次 diff 打开临时文档重建。 */
    private val diffHighlighters = mutableMapOf<Int, RangeHighlighter>()

    /**
     * workspace 模式评论下方展开的内嵌面板。key 带 EditorEx 因 Inlay 挂在编辑器实例上
     * 而非 Document——同一文件分两窗格时各需独立面板。
     */
    private val panels = mutableMapOf<Pair<Int, EditorEx>, Inlay<*>>()

    /** 内嵌面板展开/折叠状态。setStatus 处理后折叠，jumpTo 重新展开。 */
    private val expanded = mutableMapOf<Int, Boolean>()

    /** 三级回退时「行号被重定位过」的提示，展示在评论正文前面。 */
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

    /** 挂载完成后自动跳到第一条挂上的评论。 */
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

    /** 清空所有评论与高亮。 */
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
     * 采纳建议。只有 workspace 模式允许 apply。无 suggestionCode 时删除被标记范围。
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

            // ensureFilesWritable 放在 isWritable 前：它会弹 IDEA「从版本控制签出」对话框。
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

    // ---------------------------------------------------------------- 内部

    private fun snapshot(index: Int): Pair<ReviewComment, ReviewContext>? = synchronized(lock) {
        val comment = comments.getOrNull(index) ?: return null
        val ctx = context ?: return null
        comment to ctx
    }

    /**
     * 第 index 条评论此刻在 document 里占哪几行（0-based 闭区间）。
     * 画行底色、挂内嵌面板、跳转、采纳均从此统一出口。
     *
     * workspace 优先问活锚点（RangeHighlighter 跟随用户编辑移动）；
     * 锚点失效退回解析阶段行号。返回 null 表示无落点（仅侧栏展示）。
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

    /** 状态变化时刷新 gutter 图标、tooltip、error stripe 和内嵌面板。 */
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

    /** 按最新状态/展开状态重建内嵌面板。找不到旧面板则跳过。 */
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
     * 给 diff 文档挂属于这一侧的评论标记（RangeHighlighter + 装订线图标）。
     * 由 GitService.openDiff 在 EDT 回调、左右两侧各一次。
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
     * 装订线图标，点击展开/收起内嵌面板。
     * 不跳转到该行：图标就在该行旁边，已可见无需再跳。
     * 跨位置导航走侧栏卡片「查看」按钮 → jumpTo。
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
     * 把评论的内嵌面板挂进 editor 的 offset 下方。
     * 已挂过则跳过（panels 按 (index, editor) 去重）。
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
     * 给 diff 视图挂内嵌评论面板。由 GitService.diffViewerReady 在 showDiff 之后回调。
     * 通过 EditorFactory 查询 document 对应的编辑器实例。
     */
    fun mountDiffPanels(relPath: String, side: Side, document: Document, clickedIndex: Int? = null) {
        val anchorSide = if (side == Side.LEFT) AnchorSide.LEFT else AnchorSide.RIGHT
        val targets = selectDiffMarks(relPath, anchorSide, diffMarks())
        if (targets.isEmpty() || document.lineCount == 0) return

        dropDisposedPanels()

        val editors = EditorFactory.getInstance().getEditors(document, project)
            .filterIsInstance<EditorEx>()
        if (editors.isEmpty()) return

        // 被点击的评论放到最后挂，使滚入目标落在点击行。
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

    /** 补挂 file 里所有已定位但未展开的评论面板。 */
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
     * 内嵌面板内容：定位提示 + 正文 + 分隔线 + suggestion + 按钮。
     * expanded 为 false 时只画标题行（折叠态）。
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

        // 已处理的评论不再给操作按钮。
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

    /** 把 CLI 给的相对路径解析成项目内文件；越出仓库根的路径一律拒绝。基准用 repoRoot：评论路径是仓库根相对（git diff 默认），子目录打开项目时 basePath≠repoRoot 会导致双重嵌套找不到文件。 */
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
