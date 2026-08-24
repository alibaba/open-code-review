package com.alibaba.opencodereview.idea.services

import com.alibaba.opencodereview.idea.model.FileChange
import com.alibaba.opencodereview.idea.model.FileStatus

/**
 * 全为纯函数（git 文本 → 领域模型），不依赖 IDE API，可直接单测。
 */

fun mapStatusCode(code: Char): FileStatus = when (code) {
    'A' -> FileStatus.ADDED
    '?' -> FileStatus.ADDED
    'D' -> FileStatus.DELETED
    'R' -> FileStatus.RENAMED
    'M' -> FileStatus.MODIFIED
    else -> FileStatus.MODIFIED // C（copied）/T（typechange）/U（unmerged）等一律降级为修改，见 GitMapTest
}

/**
 * 解析 `git status --porcelain` 输出。
 * 每行格式：XY<space>path，X 为暂存区状态，Y 为工作区状态，`??` 表示未跟踪。
 * 重命名行格式 `R  old -> new`，取 new。
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
            // 重命名(R)与复制(C)都是 `old -> new` 格式，取 new。
            code = if (x == 'R' || y == 'R') 'R' else 'C'
            // 引号格式（旧路径含空格等特殊字符，git 加了引号）：分隔符是 `" -> `（闭引号 + 箭头）。
            // 不用 `" -> "`（多一个引号）——新路径可能没引号（无特殊字符），不要求新路径也带引号。
            val quotedSep = path.indexOf("\" -> ")
            if (quotedSep >= 0) {
                path = path.substring(quotedSep + 5) // 跳过 `" -> `（5 字符），保留新路径
            } else {
                // 无引号格式：文件名不含空格，` -> ` 不会出现在文件名里，直接找即可。
                val arrow = path.indexOf(" -> ")
                if (arrow >= 0) path = path.substring(arrow + 4)
            }
        } else {
            // 暂存区状态优先，没有再取工作区状态
            code = if (x != ' ' && x != '?') x else y
        }
        path = unquoteGitPath(path)
        if (!seen.add(path)) continue
        files += FileChange(path, mapStatusCode(code))
    }
    return files
}

/** 解析 `git ls-files --others --exclude-standard` 输出的未跟踪路径列表。 */
fun parseUntrackedList(output: String): List<String> =
    output.lineSequence().map { unquoteGitPath(it.trim()) }.filter(String::isNotEmpty).toList()

/** 合并已跟踪变更与未跟踪文件，按路径去重，已跟踪的优先。 */
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
 * 构建工作区文件列表。与 OCR CLI 的 workspace 模式一致：
 * 先 `diff HEAD`，为空则回退 `diff --cached`（首次提交前没有 HEAD），最后合并未跟踪文件。
 */
fun buildWorkspaceFiles(diffHeadOut: String, diffCachedOut: String, untrackedOut: String): List<FileChange> {
    var tracked = parseNameStatus(diffHeadOut)
    if (tracked.isEmpty()) tracked = parseNameStatus(diffCachedOut)
    return mergeWorkspaceFiles(tracked, parseUntrackedList(untrackedOut))
}

/**
 * 解析 `git branch -a --format=%(refname)` 输出。刻意取完整 refname 而非 `%(refname:short)`：
 * git 会把 `refs/remotes/origin/HEAD` 缩写为 `origin`——既不以 HEAD 结尾、本身也不等于 HEAD，后置过滤无法拦截，
 * 下拉列表中会出现选中后报 "unknown revision" 的假分支。完整 refname 可精确排除 `refs/remotes/<remote>/HEAD` 符号引用。
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
            // 非 heads/remotes 的引用（tags、stash 等）不应出现在 branch -a 输出中，保险起见跳过。
            ref.startsWith("refs/") -> continue
            else -> ref
        }
        if (name.isEmpty() || name == "HEAD") continue
        if (seen.add(name)) branches += name
    }
    return branches
}

/**
 * 把 `origin/HEAD` 和它指向的默认远程分支（[defaultRemote]）提到列表最前、紧跟第一个本地分支之后，
 * 其余条目保持原序（对齐 GitHub 网页端的分支顺序）。[defaultRemote] 为 null（仓库没有 origin/HEAD）
 * 时不重排，原样返回。位次对齐 VS Code 扩展：本地默认分支、`origin/HEAD`、默认远程分支、其余。
 */
fun pinDefaultBranches(branches: List<String>, defaultRemote: String?): List<String> {
    if (defaultRemote.isNullOrBlank()) return branches
    val pin = listOf("origin/HEAD", defaultRemote).filter { it in branches }.distinct()
    if (pin.isEmpty()) return branches
    val rest = branches.filter { it !in pin }
    if (rest.isEmpty()) return pin
    return listOf(rest.first()) + pin + rest.drop(1)
}

/** 生成用于 `rev-parse --verify` 的分支引用候选：补 origin/、并把 main 与 master 互换。 */
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
 * 解码 git 的 quotepath 转义（`core.quotepath=true` 时中文会变成 `"\344\275\240"`）。
 * 所有 git 调用均带 `-c core.quotepath=false`，但仓库/全局配置仍可能引入引号形式，
 * 故保留此层处理。
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
        // 八进制转义：字符范围收紧为 0-7，避免 toInt(8) 遇 '8'/'9' 抛异常。
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
 * 解析 `git diff --name-status` / `git show --name-status` 输出。
 * 制表符分隔：`status<TAB>path`；重命名是 `R<score><TAB>old<TAB>new`，取 new。
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
