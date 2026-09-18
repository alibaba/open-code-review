// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.services

import com.alibaba.opencodereview.idea.model.FileChange
import com.alibaba.opencodereview.idea.model.FileStatus

/**
 * All pure functions (git text -> domain models) with no IDE API dependencies, so they can be unit-tested directly.
 */

fun mapStatusCode(code: Char): FileStatus = when (code) {
    'A' -> FileStatus.ADDED
    '?' -> FileStatus.ADDED
    'D' -> FileStatus.DELETED
    'R' -> FileStatus.RENAMED
    'M' -> FileStatus.MODIFIED
    else -> FileStatus.MODIFIED // C (copied) / T (typechange) / U (unmerged) and the like all degrade to MODIFIED, see GitMapTest
}

/**
 * Parses `git status --porcelain` output.
 * Each line is XY<space>path, where X is the staged status, Y the worktree status, and `??` means untracked.
 * A rename line looks like `R  old -> new`; take new.
 */
fun parsePorcelain(output: String): List<FileChange> {
    val files = mutableListOf<FileChange>()
    val seen = mutableSetOf<String>()
    for (rawLine in output.lineSequence()) {
        if (rawLine.isBlank() || rawLine.length <= 3) continue
        val x = rawLine[0]
        val y = rawLine[1]
        var path = rawLine.substring(3)
        val code: Char
        if (x == '?' && y == '?') {
            code = '?'
        } else if (x == 'R' || y == 'R' || x == 'C' || y == 'C') {
            // Both renames (R) and copies (C) use the `old -> new` format; take new.
            code = if (x == 'R' || y == 'R') 'R' else 'C'
            // Quoted form (the old path contains spaces or other special characters, so git quoted it): the
            // separator is `" -> ` (closing quote + arrow).
            // Do not search for `" -> "` (one quote more) -- the new path may be unquoted (no special characters),
            // so the new path cannot be required to carry quotes either.
            val quotedSep = path.indexOf("\" -> ")
            if (quotedSep >= 0) {
                path = path.substring(quotedSep + 5) // skip `" -> ` (5 characters), keeping the new path
            } else {
                // Unquoted form: the file name has no spaces, so ` -> ` cannot appear inside it and a plain search is safe.
                val arrow = path.indexOf(" -> ")
                if (arrow >= 0) path = path.substring(arrow + 4)
            }
        } else {
            // Staged status takes priority; fall back to the worktree status when there is none
            code = if (x != ' ' && x != '?') x else y
        }
        path = unquoteGitPath(path)
        if (!seen.add(path)) continue
        files += FileChange(path, mapStatusCode(code))
    }
    return files
}

/** Parses the untracked path list produced by `git ls-files --others --exclude-standard`. */
fun parseUntrackedList(output: String): List<String> =
    output.lineSequence().map { unquoteGitPath(it.trim()) }.filter(String::isNotEmpty).toList()

/** Merges tracked changes with untracked files, deduplicating by path with tracked entries taking priority. */
fun mergeWorkspaceFiles(tracked: List<FileChange>, untrackedPaths: List<String>): List<FileChange> {
    val files = mutableListOf<FileChange>()
    val seen = mutableSetOf<String>()
    for (file in tracked) {
        if (seen.add(file.path)) files += file
    }
    for (path in untrackedPaths) {
        if (seen.add(path)) files += FileChange(path, FileStatus.ADDED)
    }
    return files
}

/**
 * Builds the workspace file list, matching the OCR CLI's workspace mode:
 * `diff HEAD` first, falling back to `diff --cached` when it is empty (no HEAD before the first commit),
 * then merging in the untracked files.
 */
fun buildWorkspaceFiles(diffHeadOut: String, diffCachedOut: String, untrackedOut: String): List<FileChange> {
    var tracked = parseNameStatus(diffHeadOut)
    if (tracked.isEmpty()) tracked = parseNameStatus(diffCachedOut)
    return mergeWorkspaceFiles(tracked, parseUntrackedList(untrackedOut))
}

/**
 * Parses `git branch -a --format=%(refname)` output. The full refname is used deliberately instead of
 * `%(refname:short)`: git abbreviates `refs/remotes/origin/HEAD` to `origin`, which neither ends in HEAD nor
 * equals HEAD, so post-filtering cannot catch it and the dropdown would show a fake branch that errors with
 * "unknown revision" when selected. The full refname allows precisely excluding the `refs/remotes/<remote>/HEAD`
 * symbolic refs.
 */
fun parseBranchList(output: String): List<String> {
    val branches = mutableListOf<String>()
    val seen = mutableSetOf<String>()
    for (rawLine in output.lineSequence()) {
        val ref = rawLine.trim()
        if (ref.isEmpty()) continue
        val name = when {
            ref.startsWith("refs/heads/") -> ref.removePrefix("refs/heads/")
            ref.startsWith("refs/remotes/") -> ref.removePrefix("refs/remotes/")
            // Refs other than heads/remotes (tags, stash, ...) should not appear in branch -a output; skip them defensively.
            ref.startsWith("refs/") -> continue
            else -> ref
        }
        if (name.isEmpty() || name == "HEAD") continue
        if (seen.add(name)) branches += name
    }
    return branches
}

/**
 * Moves `origin/HEAD` and the default remote branch it points at ([defaultRemote]) to the front of the list,
 * right after the first local branch, keeping the remaining entries in their original order (matching the
 * branch order on GitHub's web UI). When [defaultRemote] is null (the repository has no origin/HEAD), the list
 * is returned as-is with no reordering. The positions align with the VS Code extension: local default branch,
 * `origin/HEAD`, default remote branch, then the rest.
 */
fun pinDefaultBranches(branches: List<String>, defaultRemote: String?): List<String> {
    if (defaultRemote.isNullOrBlank()) return branches
    val pin = listOf("origin/HEAD", defaultRemote).filter { it in branches }.distinct()
    if (pin.isEmpty()) return branches
    val rest = branches.filter { it !in pin }
    if (rest.isEmpty()) return pin
    return listOf(rest.first()) + pin + rest.drop(1)
}

/** Builds branch ref candidates for `rev-parse --verify`: add the origin/ prefix and swap main with master. */
fun branchRefCandidates(ref: String): List<String> {
    val candidates = mutableListOf(ref)
    if (!ref.contains('/')) candidates += "origin/$ref"
    when (ref) {
        "master" -> candidates += listOf("main", "origin/main")
        "main" -> candidates += listOf("master", "origin/master")
    }
    return candidates.distinct()
}

/**
 * Decodes git's quotepath escapes (with `core.quotepath=true`, Chinese paths become `"\344\275\240"`).
 * Every git invocation passes `-c core.quotepath=false`, but repository/global config can still introduce
 * the quoted form, so this decoding layer is kept.
 */
fun unquoteGitPath(path: String): String {
    if (path.length < 2 || !path.startsWith('"') || !path.endsWith('"')) return path

    val bytes = mutableListOf<Byte>()
    val inner = path.substring(1, path.length - 1)
    var i = 0
    while (i < inner.length) {
        val ch = inner[i]
        if (ch != '\\' || i + 1 >= inner.length) {
            bytes += ch.code.toByte()
            i++
            continue
        }
        // Octal escape: the digit range is tightened to 0-7 so toInt(8) does not throw on '8'/'9'.
        if (i + 3 < inner.length && inner.substring(i + 1, i + 4).all { it in '0'..'7' }) {
            bytes += inner.substring(i + 1, i + 4).toInt(8).toByte()
            i += 4
            continue
        }
        i++
        when (val esc = inner[i]) {
            'n' -> bytes += 0x0a.toByte()
            't' -> bytes += 0x09.toByte()
            'r' -> bytes += 0x0d.toByte()
            '\\' -> bytes += 0x5c.toByte()
            '"' -> bytes += 0x22.toByte()
            else -> bytes += esc.code.toByte()
        }
        i++
    }
    return String(bytes.toByteArray(), Charsets.UTF_8)
}

/**
 * Parses `git diff --name-status` / `git show --name-status` output.
 * Tab-separated: `status<TAB>path`; a rename is `R<score><TAB>old<TAB>new`, take new.
 */
fun parseNameStatus(output: String): List<FileChange> {
    val files = mutableListOf<FileChange>()
    val seen = mutableSetOf<String>()
    for (rawLine in output.lineSequence()) {
        if (rawLine.isBlank()) continue
        val parts = rawLine.split('\t')
        if (parts.size < 2) continue
        val codeChar = parts[0].firstOrNull() ?: continue
        val path = unquoteGitPath(if (parts.size >= 3) parts.last() else parts[1])
        if (!seen.add(path)) continue
        files += FileChange(path, mapStatusCode(codeChar))
    }
    return files
}
