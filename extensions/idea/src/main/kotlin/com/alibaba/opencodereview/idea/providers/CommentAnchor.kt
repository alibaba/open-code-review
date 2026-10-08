// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.providers

import com.alibaba.opencodereview.idea.model.FileStatus
import com.alibaba.opencodereview.idea.model.HostStrings
import com.alibaba.opencodereview.idea.model.ReviewComment
import com.alibaba.opencodereview.idea.model.ReviewContext
import com.alibaba.opencodereview.idea.model.ReviewMode
import com.alibaba.opencodereview.idea.model.SupportedLocale
import com.alibaba.opencodereview.idea.services.GitService

/**
 * Resolve CLI line numbers in file content, falling back to an existingCode sliding-window match for invalid lines.
 * After mounting, CommentService handles line drift through its RangeHighlighter (RangeMarker).
 */

/** Where to mount a comment: the current file in workspace mode, or the left or right diff side in branch/commit mode. */
enum class AnchorSide { WORKSPACE, LEFT, RIGHT }

/** Reasons a comment cannot be located and can only be shown in the sidebar without navigation. */
enum class SidebarOnlyReason {
    /** A binary file, detected by GitService.isBinaryFile through content inspection. */
    BINARY,

    /** The file exists, but neither the supplied line numbers nor existingCode identifies a usable location. */
    UNRESOLVED,

    /** The file is outside the review scope, or its content cannot be read at the specified ref. */
    MISSING_FILE,

    /** The anchor was resolved, but attaching it to the editor failed (for example, Document lookup failed). Used only by [CommentService]. */
    MOUNT_FAILED,
}

sealed class CommentAnchorResult {
    /** [startLine] / [endLine] are resolved, 1-based line numbers; [relocated] indicates use of the existingCode fallback. */
    data class Mountable(
        val startLine: Int,
        val endLine: Int,
        val side: AnchorSide,
        val relocated: Boolean,
        val locateNote: String?,
    ) : CommentAnchorResult()

    data class SidebarOnly(val reason: SidebarOnlyReason) : CommentAnchorResult()
}

/** Remove leading diff markers (`+`/`-`) and surrounding whitespace for lenient comparison of existingCode with file content. */
internal fun normalizeLine(line: String): String {
    val s = line.trim()
    return if (s.startsWith("+") || s.startsWith("-")) s.substring(1).trim() else s
}

/** Split into lines and apply [normalizeLine]. Discard blank lines so changes in blank lines between revisions do not prevent a match. */
internal fun splitAndNormalize(code: String): List<String> {
    val result = mutableListOf<String>()
    for (raw in code.split("\n")) {
        val n = normalizeLine(raw)
        if (n.isNotEmpty()) result += n
    }
    return result
}

internal data class LineSpan(val start: Int, val end: Int)

/** Find [existingCode] in file content with a sliding window; return 1-based line numbers, or null when no match exists. */
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
 * Resolve CLI line numbers in [content]: use valid lines directly, otherwise relocate with an [existingCode] sliding-window match.
 * Return null if both fail; the caller should fall back to sidebar-only display.
 */
internal fun resolveLinesInContent(
    content: String,
    startLine: Int,
    endLine: Int,
    existingCode: String?,
): ResolvedLines? {
    val lineCount = content.split("\n").size // Use the same split("\n") line boundaries as splitAndNormalize in this file.
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

/** Prepend a relocation note to the comment body to indicate that its line number was inferred. */
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

/** Choose candidate (ref, side) pairs by file status, in the order they should be tried. */
private fun candidateRefs(git: GitService, ctx: ReviewContext, status: FileStatus): List<Pair<String, AnchorSide>> {
    val leftRef = if (status == FileStatus.ADDED) null else git.leftRefFor(ctx)
    val rightRef = if (status == FileStatus.DELETED) null else git.rightRefFor(ctx)
    val mountLeft = status == FileStatus.DELETED

    val primary = if (mountLeft) leftRef?.let { it to AnchorSide.LEFT } else rightRef?.let { it to AnchorSide.RIGHT }
    // Added files have no content at the old ref; skip the git show call for that side.
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
 * Resolve a comment anchor: check for binary content first, then read the workspace file or try refs chosen by status.
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

    // Outside workspace mode, treat paths not recorded by [GitService.prepareReviewFileStatus] as missing files.
    // Do not try any refs, matching the short-circuit rule that a null status means missing-file.
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
 * The location of a comment in a diff, passed to CommentService.decorateDiff.
 * startLine/endLine form a 1-based inclusive range.
 */
internal data class DiffMark(val index: Int, val path: String, val side: AnchorSide, val startLine: Int, val endLine: Int)

/**
 * Select comments for this diff document by both path and side. A diff exposes both documents, so checking the side
 * prevents duplicate icons: each comment belongs only on the corresponding side of the `git:` document.
 */
internal fun selectDiffMarks(relPath: String, side: AnchorSide, all: List<DiffMark>): List<DiffMark> =
    all.filter { it.path == relPath && it.side == side }

/**
 * Clamp a 1-based inclusive range to the document bounds and return a 0-based inclusive range.
 * Edits after review or differences between diff sides may leave lines out of bounds; clamping avoids IndexOutOfBounds.
 * Also ensure start<=end. Empty documents return 0..0; callers guard lineCount==0 before accessing any lines.
 */
internal fun clampLineRange(startLine: Int, endLine: Int, lineCount: Int): IntRange {
    if (lineCount <= 0) return 0..0
    val last = lineCount - 1
    val start = (startLine - 1).coerceIn(0, last)
    val end = (endLine - 1).coerceIn(start, last)
    return start..end
}
