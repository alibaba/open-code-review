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

/** Trim a line, and nothing else: on the content side a leading '+' or '-' is code, not a diff marker. */
internal fun normalizeLine(line: String): String = line.trim()

/** Trim each line and drop the blank ones, so a snippet's blank lines never become match targets of their own. */
private fun normalizeLines(lines: List<String>): List<String> = lines.map(::normalizeLine).filter { it.isNotEmpty() }

/**
 * Remove one leading diff marker from each line: the reading for a snippet quoted out of diff output
 * ("+  - name: app" becomes "- name: app").
 *
 * Exactly one marker per line, and only from the first character, so a deleted YAML list item keeps its own dash
 * ("-- name: app" becomes "- name: app"). A line that is nothing but a marker loses the marker, is left blank,
 * and is dropped.
 */
private fun stripDiffMarkers(lines: List<String>): List<String> = normalizeLines(
    lines.map { if (it.startsWith("+") || it.startsWith("-")) it.substring(1) else it },
)

/**
 * The readings an [existingCode] snippet may be matched against, in priority order.
 *
 * Verbatim first: the model copied the code out of the file, where a leading '-' is code, a YAML list item being
 * the everyday case. Diff-quoted second: the model copied it out of the diff instead, where that first character
 * is a marker. A snippet carrying no marker reads the same both ways, so the two collapse into one form: callers
 * would otherwise scan the file twice for an answer the first scan already settled.
 */
internal fun snippetForms(existingCode: String): List<List<String>> {
    val lines = existingCode.split("\n")
    val verbatim = normalizeLines(lines)
    if (verbatim.isEmpty()) return emptyList()
    val stripped = stripDiffMarkers(lines)
    if (stripped.isNotEmpty() && stripped != verbatim) return listOf(verbatim, stripped)
    return listOf(verbatim)
}

internal data class LineSpan(val start: Int, val end: Int)

/** Find [existingCode] in file content with a sliding window; return 1-based line numbers, or null when no match exists. */
internal fun findLinesByExistingCode(content: String, existingCode: String): LineSpan? {
    val forms = snippetForms(existingCode)
    if (forms.isEmpty()) return null

    val normalized = mutableListOf<String>()
    val lineNums = mutableListOf<Int>()
    content.split("\n").forEachIndexed { index, raw ->
        val n = normalizeLine(raw.removeSuffix("\r"))
        if (n.isNotEmpty()) {
            normalized += n
            lineNums += index + 1
        }
    }

    for (target in forms) {
        if (normalized.size < target.size) continue
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
    // Count lines the same way findLinesByExistingCode splits them, so an in-range line number means the same
    // thing to both.
    val lineCount = content.split("\n").size
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
