package com.alibaba.opencodereview.idea.providers

import com.alibaba.opencodereview.idea.model.FileStatus
import com.alibaba.opencodereview.idea.model.HostStrings
import com.alibaba.opencodereview.idea.model.ReviewComment
import com.alibaba.opencodereview.idea.model.ReviewContext
import com.alibaba.opencodereview.idea.model.ReviewMode
import com.alibaba.opencodereview.idea.model.SupportedLocale
import com.alibaba.opencodereview.idea.services.GitService

/**
 * 评论定位：将 CLI 行号落到文件内容，行号失效时用 existingCode 滑窗匹配兜底。
 * 挂载后行号漂移由 CommentService 的 RangeHighlighter（RangeMarker）处理。
 */

/** 评论挂载的落点：workspace 模式挂当前文件；branch/commit 模式挂 diff 的左侧或右侧。 */
enum class AnchorSide { WORKSPACE, LEFT, RIGHT }

/** 定位失败时，只能在侧栏展示、不能跳转的原因。 */
enum class SidebarOnlyReason {
    /** 二进制文件——通过 GitService.isBinaryFile 内容嗅探判定。 */
    BINARY,

    /** 文件存在，但行号（含 existingCode 兜底）均无法对齐。 */
    UNRESOLVED,

    /** 文件在本次审查范围之外，或指定的引用根本读不到内容。 */
    MISSING_FILE,

    /** 定位算出来了，但挂载到编辑器这一步本身失败（如 Document 获取失败）。仅 [CommentService] 会用到这个原因。 */
    MOUNT_FAILED,
}

sealed class CommentAnchorResult {
    /** [startLine] / [endLine] 是 1-based、已解析好的行号；[relocated] 表示是否靠 existingCode 兜底重定位。 */
    data class Mountable(
        val startLine: Int,
        val endLine: Int,
        val side: AnchorSide,
        val relocated: Boolean,
        val locateNote: String?,
    ) : CommentAnchorResult()

    data class SidebarOnly(val reason: SidebarOnlyReason) : CommentAnchorResult()
}

/** 去掉行首的 diff 标记（`+`/`-`）与首尾空白，用于 existingCode 与文件内容的宽松比较。 */
internal fun normalizeLine(line: String): String {
    var s = line.trim()
    if (s.startsWith("+") || s.startsWith("-")) s = s.substring(1).trim()
    return s
}

/** 按行拆分并逐行 [normalizeLine]，丢弃空行——空行在不同版本间易增减，纳入匹配只会拖累滑窗。 */
internal fun splitAndNormalize(code: String): List<String> {
    val result = mutableListOf<String>()
    for (raw in code.split("\n")) {
        val n = normalizeLine(raw)
        if (n.isNotEmpty()) result += n
    }
    return result
}

internal data class LineSpan(val start: Int, val end: Int)

/** 在文件内容里滑窗匹配 [existingCode]，返回 1-based 行号；找不到时返回 null。 */
internal fun findLinesByExistingCode(content: String, existingCode: String): LineSpan? {
    val target = splitAndNormalize(existingCode)
    if (target.isEmpty()) return null

    val normalized = mutableListOf<String>()
    val lineNums = mutableListOf<Int>()
    content.split("\n").forEachIndexed { index, raw ->
        val n = normalizeLine(raw.removeSuffix("\r"))
        if (n.isNotEmpty()) {
            normalized += n
            lineNums += index + 1
        }
    }
    if (normalized.size < target.size) return null

    for (i in 0..(normalized.size - target.size)) {
        var matched = true
        for (j in target.indices) {
            if (normalized[i + j] != target[j]) {
                matched = false
                break
            }
        }
        if (matched) return LineSpan(lineNums[i], lineNums[i + target.size - 1])
    }
    return null
}

internal data class ResolvedLines(val start: Int, val end: Int, val relocated: Boolean)

/**
 * 把 CLI 给的行号落到 [content] 里；行号在范围内直接使用，否则退到 [existingCode] 滑窗重定位。
 * 两者均失败时返回 null——调用方应转为侧栏兜底展示。
 */
internal fun resolveLinesInContent(
    content: String,
    startLine: Int,
    endLine: Int,
    existingCode: String?,
): ResolvedLines? {
    val lineCount = content.split("\n").size // 与本文件 splitAndNormalize 的 split("\n") 分行方式保持一致
    val start = if (startLine > 0) startLine else 0
    val end = if (endLine > 0) endLine else start

    if (start > 0 && end > 0 && start <= lineCount && end <= lineCount && start <= end) {
        return ResolvedLines(start, end, relocated = false)
    }

    if (!existingCode.isNullOrBlank()) {
        val found = findLinesByExistingCode(content, existingCode)
        if (found != null) return ResolvedLines(found.start, found.end, relocated = true)
    }
    return null
}

/** 重定位说明，附在评论正文前面，提示用户行号是推算得到的。 */
internal fun formatLocateNote(originalLine: Int, resolvedLine: Int, locale: SupportedLocale): String =
    if (originalLine > 0 && originalLine != resolvedLine) {
        HostStrings.t(
            locale,
            "ext.comment.locateNoteRelocatedFrom",
            "original" to originalLine.toString(),
            "resolved" to resolvedLine.toString(),
        )
    } else {
        HostStrings.t(locale, "ext.comment.locateNoteRelocated")
    }

/** 按状态选出要尝试挂载的 (引用, 落点) 候选，顺序即尝试顺序。 */
private fun candidateRefs(git: GitService, ctx: ReviewContext, status: FileStatus): List<Pair<String, AnchorSide>> {
    val leftRef = if (status == FileStatus.ADDED) null else git.leftRefFor(ctx)
    val rightRef = if (status == FileStatus.DELETED) null else git.rightRefFor(ctx)
    val mountLeft = status == FileStatus.DELETED

    val primary = if (mountLeft) leftRef?.let { it to AnchorSide.LEFT } else rightRef?.let { it to AnchorSide.RIGHT }
    // 新增文件没有「改动前」一侧，不去尝试另一侧——尝试了也读不到内容，徒增一次 git show。
    val alt = if (status == FileStatus.ADDED) {
        null
    } else if (mountLeft) {
        rightRef?.let { it to AnchorSide.RIGHT }
    } else {
        leftRef?.let { it to AnchorSide.LEFT }
    }
    return listOfNotNull(primary, alt)
}

/**
 * 解析评论定位。二进制判定置最前，workspace 读工作区文件，非 workspace 按状态选引用尝试。
 */
fun resolveCommentAnchor(comment: ReviewComment, ctx: ReviewContext, git: GitService, locale: SupportedLocale): CommentAnchorResult {
    if (git.isBinaryFile(comment.path, ctx)) {
        return CommentAnchorResult.SidebarOnly(SidebarOnlyReason.BINARY)
    }

    if (ctx.mode == ReviewMode.WORKSPACE) {
        val content = git.readWorkspaceFile(comment.path)
            ?: return CommentAnchorResult.SidebarOnly(SidebarOnlyReason.MISSING_FILE)
        return mountableOrUnresolved(comment, content, AnchorSide.WORKSPACE, locale)
    }

    // 非 workspace 模式：路径不在本次审查范围内（[GitService.prepareReviewFileStatus] 未记录到）视为文件缺失，
    // 不再往下尝试任何引用——与「status 为 null 即归为 missing-file」的短路逻辑一致。
    val status = git.getReviewFileStatus(comment.path)
        ?: return CommentAnchorResult.SidebarOnly(SidebarOnlyReason.MISSING_FILE)

    val candidates = candidateRefs(git, ctx, status)
    if (candidates.isEmpty()) return CommentAnchorResult.SidebarOnly(SidebarOnlyReason.MISSING_FILE)

    for ((ref, side) in candidates) {
        val content = git.readFileAtRef(ref, comment.path) ?: continue
        val result = mountableOrUnresolved(comment, content, side, locale)
        if (result is CommentAnchorResult.Mountable) return result
    }
    return CommentAnchorResult.SidebarOnly(SidebarOnlyReason.UNRESOLVED)
}

private fun mountableOrUnresolved(comment: ReviewComment, content: String, side: AnchorSide, locale: SupportedLocale): CommentAnchorResult {
    val lines = resolveLinesInContent(content, comment.startLine, comment.endLine, comment.existingCode)
        ?: return CommentAnchorResult.SidebarOnly(SidebarOnlyReason.UNRESOLVED)
    return CommentAnchorResult.Mountable(
        startLine = lines.start,
        endLine = lines.end,
        side = side,
        relocated = lines.relocated,
        locateNote = if (lines.relocated) formatLocateNote(comment.startLine, lines.start, locale) else null,
    )
}

/**
 * 一条评论在 diff 里的落点，CommentService.decorateDiff 的输入。
 * startLine/endLine 均为 1-based 闭区间。
 */
internal data class DiffMark(val index: Int, val path: String, val side: AnchorSide, val startLine: Int, val endLine: Int)

/**
 * 挑出该挂到这一侧文档上的评论：路径与侧别均须匹配。侧别必须判——一次 diff 把左右两个文档都交出来，
 * 不判的话每条评论会在两侧各挂一个图标，而本插件一条评论只挂到对应那一侧的 `git:` 文档。
 */
internal fun selectDiffMarks(relPath: String, side: AnchorSide, all: List<DiffMark>): List<DiffMark> =
    all.filter { it.path == relPath && it.side == side }

/**
 * 把 1-based 闭区间夹进文档实际范围，返回 0-based 闭区间。
 * 行号在审查后可能因文件被修改、diff 两侧内容不同等原因超出范围，不夹会导致 IndexOutOfBounds。
 * startLine > endLine 也在此收敛为 start<=end。空文档返回 0..0（调用方在进入前已对 lineCount==0 做守卫，不会据此访问不存在的行）。
 */
internal fun clampLineRange(startLine: Int, endLine: Int, lineCount: Int): IntRange {
    if (lineCount <= 0) return 0..0
    val last = lineCount - 1
    val start = (startLine - 1).coerceIn(0, last)
    val end = (endLine - 1).coerceIn(start, last)
    return start..end
}
