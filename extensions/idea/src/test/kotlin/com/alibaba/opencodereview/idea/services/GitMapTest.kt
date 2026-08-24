package com.alibaba.opencodereview.idea.services

import com.alibaba.opencodereview.idea.model.FileChange
import com.alibaba.opencodereview.idea.model.FileStatus
import com.alibaba.opencodereview.idea.model.SupportedLocale
import kotlin.test.Test
import kotlin.test.assertEquals

class GitMapTest {

    // ------------------------------------------------------------ mapStatusCode

    @Test
    fun `状态码映射覆盖 git 的五种码`() {
        assertEquals(FileStatus.ADDED, mapStatusCode('A'))
        assertEquals(FileStatus.ADDED, mapStatusCode('?'))
        assertEquals(FileStatus.DELETED, mapStatusCode('D'))
        assertEquals(FileStatus.RENAMED, mapStatusCode('R'))
        assertEquals(FileStatus.MODIFIED, mapStatusCode('M'))
    }

    @Test
    fun `未知状态码退化成 modified`() {
        // git 另有 C（copied）/ T（type change）/ U（unmerged），一律按修改处理。
        assertEquals(FileStatus.MODIFIED, mapStatusCode('C'))
        assertEquals(FileStatus.MODIFIED, mapStatusCode('T'))
        assertEquals(FileStatus.MODIFIED, mapStatusCode('X'))
    }

    // ------------------------------------------------------------ parseNameStatus

    @Test
    fun `解析 name-status 的制表符格式`() {
        val out = "M\tsrc/a.kt\nA\tsrc/b.kt\nD\tsrc/c.kt\n"
        assertEquals(
            listOf(
                FileChange("src/a.kt", FileStatus.MODIFIED),
                FileChange("src/b.kt", FileStatus.ADDED),
                FileChange("src/c.kt", FileStatus.DELETED),
            ),
            parseNameStatus(out),
        )
    }

    @Test
    fun `重命名行取新路径`() {
        // R<score> 后跟旧路径和新路径，需要的是改动后的位置。
        val out = "R100\tsrc/old.kt\tsrc/new.kt\n"
        assertEquals(listOf(FileChange("src/new.kt", FileStatus.RENAMED)), parseNameStatus(out))
    }

    @Test
    fun `name-status 忽略空行与缺列的行`() {
        val out = "\nM\tsrc/a.kt\n\nonlyonecolumn\n\n"
        assertEquals(listOf(FileChange("src/a.kt", FileStatus.MODIFIED)), parseNameStatus(out))
    }

    @Test
    fun `name-status 按路径去重且首次出现优先`() {
        val out = "M\tsrc/a.kt\nD\tsrc/a.kt\n"
        assertEquals(listOf(FileChange("src/a.kt", FileStatus.MODIFIED)), parseNameStatus(out))
    }

    // ------------------------------------------------------------ parsePorcelain

    @Test
    fun `porcelain 暂存区状态优先于工作区状态`() {
        // "AM" = 已暂存的新增 + 之后又改过，应算 added。
        assertEquals(
            listOf(FileChange("src/a.kt", FileStatus.ADDED)),
            parsePorcelain("AM src/a.kt\n"),
        )
    }

    @Test
    fun `porcelain 暂存区为空时取工作区状态`() {
        assertEquals(
            listOf(FileChange("src/a.kt", FileStatus.MODIFIED)),
            parsePorcelain(" M src/a.kt\n"),
        )
    }

    @Test
    fun `porcelain 双问号是未跟踪文件`() {
        assertEquals(
            listOf(FileChange("src/new.kt", FileStatus.ADDED)),
            parsePorcelain("?? src/new.kt\n"),
        )
    }

    @Test
    fun `porcelain 重命名行取箭头右侧`() {
        assertEquals(
            listOf(FileChange("b.kt", FileStatus.RENAMED)),
            parsePorcelain("R  a.kt -> b.kt\n"),
        )
    }

    @Test
    fun `porcelain 重命名-含空格引号格式取新路径`() {
        assertEquals(
            listOf(FileChange("your file.kt", FileStatus.RENAMED)),
            parsePorcelain("R  \"my file.kt\" -> \"your file.kt\"\n"),
        )
    }

    @Test
    fun `porcelain 重命名-文件名含箭头不误匹配`() {
        // 文件名 "a -> b.kt" 含空格+箭头，git 加引号；新路径 result.kt 无引号。
        // 旧代码 indexOf(" -> ") 会匹配到文件名内部的箭头，取错路径。
        // 修复后优先找 `" -> `（闭引号+箭头），正确定位分隔符。
        assertEquals(
            listOf(FileChange("result.kt", FileStatus.RENAMED)),
            parsePorcelain("R  \"a -> b.kt\" -> result.kt\n"),
        )
    }

    @Test
    fun `porcelain 重命名-含箭头无空格不引号`() {
        // x->y.kt 含 -> 但无空格，git 不加引号；` -> ` 不会出现在文件名里。
        assertEquals(
            listOf(FileChange("z.kt", FileStatus.RENAMED)),
            parsePorcelain("R  x->y.kt -> z.kt\n"),
        )
    }

    @Test
    fun `porcelain 重命名-旧不引号新引号`() {
        // 旧路径无特殊字符（不引号），新路径含空格（引号）。
        // 走 ` -> ` 回退分支（旧路径无 ` -> `，不会误匹配），unquoteGitPath 去掉新路径引号。
        assertEquals(
            listOf(FileChange("new name.kt", FileStatus.RENAMED)),
            parsePorcelain("R  normal.kt -> \"new name.kt\"\n"),
        )
    }

    @Test
    fun `porcelain 忽略空行和过短的行`() {
        assertEquals(emptyList(), parsePorcelain("\nM\n  \n"))
    }

    // ------------------------------------------------------------ 未跟踪 / 合并

    @Test
    fun `解析未跟踪列表并去掉空行`() {
        assertEquals(
            listOf("a.kt", "dir/b.kt"),
            parseUntrackedList("a.kt\n\ndir/b.kt\n\n"),
        )
    }

    @Test
    fun `合并时已跟踪状态优先于未跟踪的 added`() {
        val merged = mergeWorkspaceFiles(
            tracked = listOf(FileChange("a.kt", FileStatus.DELETED)),
            untrackedPaths = listOf("a.kt", "b.kt"),
        )
        assertEquals(
            listOf(FileChange("a.kt", FileStatus.DELETED), FileChange("b.kt", FileStatus.ADDED)),
            merged,
        )
    }

    // ------------------------------------------------------------ buildWorkspaceFiles

    @Test
    fun `工作区文件优先用 diff HEAD 的结果`() {
        val files = buildWorkspaceFiles(
            diffHeadOut = "M\ta.kt\n",
            diffCachedOut = "A\tshould-be-ignored.kt\n",
            untrackedOut = "c.kt\n",
        )
        assertEquals(
            listOf(FileChange("a.kt", FileStatus.MODIFIED), FileChange("c.kt", FileStatus.ADDED)),
            files,
        )
    }

    @Test
    fun `diff HEAD 为空时回退到暂存区`() {
        // 首次提交前不存在 HEAD，只能查看 --cached。
        val files = buildWorkspaceFiles(
            diffHeadOut = "",
            diffCachedOut = "A\ta.kt\n",
            untrackedOut = "b.kt\n",
        )
        assertEquals(
            listOf(FileChange("a.kt", FileStatus.ADDED), FileChange("b.kt", FileStatus.ADDED)),
            files,
        )
    }

    // ------------------------------------------------------------ parseBranchList

    @Test
    fun `分支列表裁掉 refs 前缀并保留远端 HEAD`() {
        // origin/HEAD 是指向远端默认分支的符号引用——用户选择它作为比较目标表示
        // "对比远端默认分支"，是一个有效的分支引用，不应过滤。
        val out = """
            refs/heads/main
            refs/remotes/origin/HEAD
            refs/remotes/origin/dependabot/npm_and_yarn/x
            refs/remotes/origin/main
        """.trimIndent()
        assertEquals(
            listOf("main", "origin/HEAD", "origin/dependabot/npm_and_yarn/x", "origin/main"),
            parseBranchList(out),
        )
    }

    @Test
    fun `分支列表去重并忽略空行`() {
        val out = "refs/heads/main\n\nrefs/heads/main\n  \n"
        assertEquals(listOf("main"), parseBranchList(out))
    }

    @Test
    fun `分支列表跳过 tag 之类的非分支引用`() {
        val out = "refs/heads/main\nrefs/tags/v1.0\nrefs/stash\n"
        assertEquals(listOf("main"), parseBranchList(out))
    }

    @Test
    fun `裸 HEAD 不算分支`() {
        assertEquals(emptyList(), parseBranchList("HEAD\n"))
    }

    // ------------------------------------------------------------ branchRefCandidates

    @Test
    fun `裸分支名补上 origin 前缀`() {
        assertEquals(listOf("feature/x"), branchRefCandidates("feature/x"))
        assertEquals(listOf("dev", "origin/dev"), branchRefCandidates("dev"))
    }

    @Test
    fun `main 与 master 互相作为候选`() {
        assertEquals(
            listOf("main", "origin/main", "master", "origin/master"),
            branchRefCandidates("main"),
        )
        assertEquals(
            listOf("master", "origin/master", "main", "origin/main"),
            branchRefCandidates("master"),
        )
    }

    // ------------------------------------------------------------ unquoteGitPath

    @Test
    fun `没有引号的路径原样返回`() {
        assertEquals("src/a.kt", unquoteGitPath("src/a.kt"))
    }

    @Test
    fun `八进制转义还原成中文路径`() {
        // "中" 的 UTF-8 是 E4 B8 AD => \344\270\255
        assertEquals("中", unquoteGitPath("\"\\344\\270\\255\""))
    }

    @Test
    fun `常见反斜杠转义还原`() {
        assertEquals("a\"b", unquoteGitPath("\"a\\\"b\""))
        assertEquals("a\\b", unquoteGitPath("\"a\\\\b\""))
        assertEquals("a\tb", unquoteGitPath("\"a\\tb\""))
    }

    @Test
    fun `非法八进制不会抛异常`() {
        // JS 的 parseInt("899", 8) 会截断，Kotlin 的 toInt(8) 会抛出异常；
        // 因此 GitMap 将判据收紧到 0-7，此处守住"不抛出"这一底线。
        unquoteGitPath("\"\\899\"")
        unquoteGitPath("\"\\9\"")
        unquoteGitPath("\"\\\"")
    }

    // ------------------------------------------------------------ formatRelative

    @Test
    fun `相对时间覆盖各档位`() {
        val now = 1_700_000_000_000L
        fun ago(ms: Long) = formatRelative((now - ms) / 1000, now, SupportedLocale.ZH_CN)

        assertEquals("刚刚", ago(30_000))
        assertEquals("5 分钟前", ago(5 * 60_000))
        assertEquals("1 小时前", ago(3_600_000))
        assertEquals("3 小时前", ago(3 * 3_600_000))
        assertEquals("昨天", ago(25 * 3_600_000))
        assertEquals("5 天前", ago(5 * 86_400_000L))
        assertEquals("2 个月前", ago(70 * 86_400_000L))
        assertEquals("2 年前", ago(800 * 86_400_000L))
    }

    @Test
    fun `相对时间跟着 locale 走`() {
        // 同一档位切换为英文——守住宿主侧 i18n 字典两种语言均已齐备这一不变量。
        val now = 1_700_000_000_000L
        fun ago(ms: Long) = formatRelative((now - ms) / 1000, now, SupportedLocale.EN)

        assertEquals("just now", ago(30_000))
        assertEquals("5 minutes ago", ago(5 * 60_000))
        assertEquals("1 hour ago", ago(3_600_000))
        assertEquals("3 hours ago", ago(3 * 3_600_000))
        assertEquals("yesterday", ago(25 * 3_600_000))
        assertEquals("5 days ago", ago(5 * 86_400_000L))
        assertEquals("2 months ago", ago(70 * 86_400_000L))
        assertEquals("2 years ago", ago(800 * 86_400_000L))
    }

    @Test
    fun `时间戳缺失或非法返回空串`() {
        assertEquals("", formatRelative(null, 1_700_000_000_000L, SupportedLocale.ZH_CN))
        assertEquals("", formatRelative(0, 1_700_000_000_000L, SupportedLocale.ZH_CN))
        assertEquals("", formatRelative(-1, 1_700_000_000_000L, SupportedLocale.ZH_CN))
    }

    @Test
    fun `未来时间戳按刚刚处理`() {
        // 机器时钟漂移或提交时间被修改时不应显示负数。
        assertEquals("刚刚", formatRelative(1_700_000_060L, 1_700_000_000_000L, SupportedLocale.ZH_CN))
    }

    // ------------------------------------------------------------ pinDefaultBranches

    @Test
    fun `origin HEAD 与默认分支提到第二三位其余保序`() {
        val branches = listOf("master", "origin/368-x", "origin/Avasam", "origin/HEAD", "origin/add-once", "origin/master")
        assertEquals(
            listOf("master", "origin/HEAD", "origin/master", "origin/368-x", "origin/Avasam", "origin/add-once"),
            pinDefaultBranches(branches, "origin/master"),
        )
    }

    @Test
    fun `默认分支是 main 时也生效`() {
        val branches = listOf("main", "origin/HEAD", "origin/chore", "origin/main", "origin/feat")
        assertEquals(
            listOf("main", "origin/HEAD", "origin/main", "origin/chore", "origin/feat"),
            pinDefaultBranches(branches, "origin/main"),
        )
    }

    @Test
    fun `没有 origin HEAD 时不重排原样返回`() {
        val branches = listOf("master", "origin/develop", "origin/feature")
        assertEquals(branches, pinDefaultBranches(branches, null))
    }

    @Test
    fun `默认远程分支不在列表里时不重排`() {
        val branches = listOf("master", "origin/develop")
        assertEquals(branches, pinDefaultBranches(branches, "origin/master"))
    }
}
