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
 * 直接调用命令行 git。所有 git 调用强制加 `-c core.quotepath=false`。
 */
class GitService(private val project: Project) {

    companion object {
        private const val GIT_TIMEOUT_MS = 30_000L
        private const val RECENT_COMMITS = 20
        private const val BINARY_SAMPLE_BYTES = 8000
        private const val BINARY_SIZE_THRESHOLD = 512_000L

        /** VFS 事件防抖窗口。一次保存或切换分支会触发数百个事件，执行一次 git 即可。 */
        private const val WATCH_DEBOUNCE_MS = 500L

        /** `git log --format` 的字段分隔符。用 U+001F（Unit Separator），提交信息里不可能出现。 */
        private const val UNIT = '\u001F'
    }

    @Volatile
    private var cache: GitState = GitState()

    @Volatile
    private var cachedRoot: File? = null

    /** Windows / macOS 默认大小写不敏感 FS；VFS 事件路径大小写可能与 root 不同，匹配前缀时须忽略大小写。OS 运行期不变，构造时算一次。 */
    private val caseInsensitiveFs: Boolean = System.getProperty("os.name").orEmpty().let {
        it.startsWith("Win", true) || it.startsWith("Mac", true)
    }

    /** 本次审查涉及的文件状态，供评论挂载判断"文件是否已删除 / 是否二进制"。 */
    private val reviewFileStatus = mutableMapOf<String, FileStatus>()

    // ---------------------------------------------------------------- 仓库根

    /** 仓库根目录；不在 git 仓库内返回 null。结果缓存，[invalidate] 后重新解析。 */
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

    // ---------------------------------------------------------------- 状态刷新

    /**
     * 按审查模式刷新并返回 git 状态。
     * 只查询当前模式所需数据：workspace 模式不需要分支列表，branch 模式不需要提交列表。
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

    /** 最近一次 [getState] 的结果，不触发任何 git 调用。 */
    fun cachedState(): GitState = cache

    private fun refreshCurrentBranch(root: File): String {
        val branch = runGitOrNull(root, "rev-parse", "--abbrev-ref", "HEAD")?.trim().orEmpty()
        // 分离头指针时 --abbrev-ref 返回 "HEAD"，退化为短哈希更有意义。
        if (branch.isEmpty() || branch == "HEAD") {
            return runGitOrNull(root, "rev-parse", "--short", "HEAD")?.trim().orEmpty()
        }
        return branch
    }

    private fun refreshWorkspaceFiles(root: File): List<FileChange> {
        val diffHead = runGitOrNull(root, "diff", "--name-status", "HEAD").orEmpty()
        // 首次提交前没有 HEAD，回退到暂存区。
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
        // origin/HEAD 指向默认远程分支（origin/master 或 origin/main），把它俩提到列表前部、紧跟本地默认分支之后。
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

    // ---------------------------------------------------------------- 差异查询

    /**
     * 分支间变更文件。使用三点差异（`from...to`），即从 merge-base 开始比较，
     * 使 from 分支上的新提交不会被计入"本次改动"——与 OCR CLI 的 branch 模式一致。
     */
    fun getBranchDiff(from: String, to: String): List<FileChange> {
        val root = repoRoot() ?: return emptyList()
        val fromRef = resolveGitRef(root, from) ?: return emptyList()
        val toRef = if (to.isBlank()) "HEAD" else resolveGitRef(root, to) ?: return emptyList()
        val out = runGitOrNull(root, "diff", "--name-status", "$fromRef...$toRef") ?: return emptyList()
        return parseNameStatus(out)
    }

    /** 单个提交引入的变更文件。 */
    fun getCommitFiles(sha: String): List<FileChange> {
        val root = repoRoot() ?: return emptyList()
        val safeSha = safeRef(sha) ?: return emptyList() // safeRef 已拒空/空白
        val out = runGitOrNull(root, "show", "--name-status", "--format=", safeSha) ?: return emptyList()
        return parseNameStatus(out)
    }

    /**
     * 将用户填写的分支名解析为真实存在的引用。
     * 依次尝试 [branchRefCandidates] 给出的候选（补 origin/、main 与 master 互换），
     * 全部不存在则返回 null。
     */
    fun resolveGitRef(root: File, ref: String): String? {
        val safe = safeRef(ref) ?: return null
        for (candidate in branchRefCandidates(safe)) {
            val ok = runGitOrNull(root, "rev-parse", "--verify", "--quiet", candidate)
            if (!ok.isNullOrBlank()) return candidate
        }
        return null
    }

    /** 两个引用的 merge-base；失败返回 null。 */
    fun mergeBase(from: String, to: String): String? {
        val root = repoRoot() ?: return null
        val fromRef = safeRef(from) ?: return null
        val toRef = safeRef(to) ?: return null
        return runGitOrNull(root, "merge-base", fromRef, toRef)?.trim()?.takeIf { it.isNotEmpty() }
    }

    // ---------------------------------------------------------------- 文件内容

    /** 读取某个引用下的文件内容；路径不存在于该引用返回 null。 */
    fun readFileAtRef(ref: String, relPath: String): String? {
        val root = repoRoot() ?: return null
        val safe = safeRef(ref) ?: return null
        return runGitOrNull(root, "show", "$safe:$relPath")
    }

    /** 读取工作区当前文件内容；文件不存在返回 null。基准用 repoRoot：CLI 输出的评论路径是仓库根相对（git diff 默认），子目录打开项目时也须按仓库根拼。 */
    fun readWorkspaceFile(relPath: String): String? {
        val root = repoRoot() ?: return null
        val file = File(root, relPath)
        // 防路径遍历：canonicalPath 解析 ../ 和符号链接后的真实路径，确认仍在仓库根内。
        // runCatching 兜底：canonicalPath 遇 IO 异常不崩，按"不在仓库内"处理（优雅降级）。
        val inRepo = runCatching { file.canonicalPath.startsWith(root.canonicalPath + File.separator) }.getOrDefault(false)
        if (!inRepo) return null
        if (!file.isFile) return null
        return runCatching { file.readText() }.getOrNull()
    }

    /** 某路径是否存在于给定引用。用 `cat-file -e`，不读内容。 */
    fun pathExistsAtRef(ref: String, relPath: String): Boolean {
        val root = repoRoot() ?: return false
        val safe = safeRef(ref) ?: return false
        // cat-file -e 存在时无输出且退出码 0，须区分"null=失败"和"空串=成功"。
        return runGitOrNull(root, "cat-file", "-e", "$safe:$relPath") != null
    }

    /**
     * 是否是二进制文件。判据是内容中出现 NUL 字节——与 git 自身的启发式一致。
     * 按需调用而非在 [prepareReviewFileStatus] 中批量预判，避免为数百个文件空读一遍内容。
     */
    fun isBinaryFile(relPath: String, ctx: ReviewContext): Boolean {
        val root = repoRoot() ?: return false
        if (ctx.mode == ReviewMode.WORKSPACE) {
            val file = File(root, relPath)
            // 防路径遍历：同 readWorkspaceFile，canonicalPath 解析后确认在仓库根内。
            val inRepo = runCatching { file.canonicalPath.startsWith(root.canonicalPath + File.separator) }.getOrDefault(false)
            if (!inRepo || !file.isFile) return false
            return runCatching {
                file.inputStream().use { it.readNBytes(BINARY_SAMPLE_BYTES).any { b -> b == 0.toByte() } }
            }.getOrDefault(false)
        }
        // 非 workspace：先查 git 对象大小，超大文件直接判定 binary，不读内容
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

    // ---------------------------------------------------------------- 审查文件状态

    /**
     * 审查开始时记录本次涉及的文件及其状态，供 [getReviewFileStatus] 查询。
     * 评论挂载依靠此数据区分"文件已删除故无法跳转"和"文件存在但行号不匹配"。
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

    /** 本次审查中该路径的变更状态；不在本次范围内返回 null。 */
    fun getReviewFileStatus(relPath: String): FileStatus? =
        synchronized(reviewFileStatus) { reviewFileStatus[relPath] }

    // ---------------------------------------------------------------- 差异视图

    /**
     * diff 打开时为两侧文档挂载内容的钩子。branch/commit 模式两侧内容是
     * DiffContentFactory 现创建的匿名文档，外部无法获取句柄，只能在创建当口回调。
     */
    @Volatile
    var diffDecorator: ((relPath: String, side: Side, document: Document) -> Unit)? = null

    /**
     * diff 视图创建之后的钩子，用于挂载内嵌面板。
     * 与 diffDecorator 分开：行高亮挂在文档上须在 showDiff 之前，面板挂在编辑器实例上须在之后。
     */
    @Volatile
    var diffViewerReady: ((relPath: String, side: Side, document: Document, clickedIndex: Int?) -> Unit)? = null

    /**
     * 在 IDEA 差异视图中打开某文件的本次改动。新增文件左侧、删除文件右侧使用空内容而非报错，
     * 使 diff 始终能打开、用户看到全增或全删。
     *
     * [scrollToSide]/[scrollToLine] 使 diff 打开后自动滚动到并选中指定行——实现评论跳转中"定位到已打开 diff 的评论行"的效果。
     * [scrollToLine] 为 0-based 文档行号。两者均提供才生效，任一为 null 则为普通打开。
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

        // DiffManager 须在 EDT 上调用，审查流程在后台线程执行。
        ApplicationManager.getApplication().invokeLater {
            if (project.isDisposed) return@invokeLater
            // 挂标记须在 showDiff 之前：viewer 用这两份 document 构建 editor，先挂好、diff 一出现装订线图标即就位，不出现"先空一下再刷出来"。
            diffDecorator?.let { decorate ->
                runCatching {
                    decorate(relPath, Side.LEFT, leftContent.document)
                    decorate(relPath, Side.RIGHT, rightContent.document)
                }.onFailure { thisLogger().warn("[ocr] Failed to decorate diff with comment markers", it) }
            }
            DiffManager.getInstance().showDiff(project, request)
            // 待 viewer 构建完毕再挂内嵌面板——通过 EditorFactory 查询 document 对应的编辑器实例。
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

    /** 差异左侧（改动前）对应的引用。 */
    private fun leftRefFor(root: File, ctx: ReviewContext): String? = when (ctx.mode) {
        ReviewMode.WORKSPACE -> "HEAD"
        ReviewMode.BRANCH -> {
            val from = resolveGitRef(root, ctx.from.orEmpty())
            val to = ctx.to?.takeIf { it.isNotBlank() }?.let { resolveGitRef(root, it) } ?: "HEAD"
            // 与 getBranchDiff 的三点差异保持一致：左侧取 merge-base，而非 from 的当前位置。
            // mergeBase 失败时返回 null（让 openDiff 优雅降级为空左侧），而非回退到 from（会与文件列表基准不一致）。
            if (from != null) mergeBase(from, to) else null
        }

        ReviewMode.COMMIT -> ctx.commit?.takeIf { it.isNotBlank() }?.let { "$it^" }
    }

    /**
     * 供 [com.alibaba.opencodereview.idea.providers.resolveCommentAnchor] 复用的无 root 版本——
     * 评论挂载和 [openDiff] 须使用同一套引用解析，否则挂载算出的行号与 diff 中实际展示的内容版本不一致。
     */
    internal fun leftRefFor(ctx: ReviewContext): String? {
        val root = repoRoot() ?: return null
        return leftRefFor(root, ctx)
    }

    /**
     * 差异右侧（改动后）引用；workspace 无"引用"概念返回 null。与 [readRightSide] 的区别：
     * 此处不做"to 为空退回工作区文件"的兜底——评论挂载需要的是"此引用能否解析"的判断本身，退回工作区会掩盖"审查未完整运行"本应报 missing-file 的情况。
     */
    internal fun rightRefFor(ctx: ReviewContext): String? {
        val root = repoRoot() ?: return null
        return when (ctx.mode) {
            ReviewMode.WORKSPACE -> null
            ReviewMode.BRANCH -> ctx.to?.takeIf { it.isNotBlank() }?.let { resolveGitRef(root, it) }
            ReviewMode.COMMIT -> ctx.commit?.takeIf { it.isNotBlank() }
        }
    }

    /** 差异右侧（改动后）内容。workspace 模式读取工作区，其余读取对应引用。 */
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

    // ---------------------------------------------------------------- 变更监听

    /**
     * 订阅工作区文件变更，防抖后回调新的 workspace 状态。
     * 不加此监听，用户修改文件后侧栏待审查列表仍是打开时的快照。
     *
     * 返回值是此次订阅本身，dispose 即断开（仅侧栏存活时才监听）。
     * [parent] 仅作兜底（项目关闭时统一清理），调用方应在侧栏关闭时主动 dispose 返回值，
     * 否则 VFS 事件会在整个项目生命周期内持续触发 `git status` 而无人接收。
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
                // 一次保存或切换分支会触发数十乃至上百个 VFS 事件，必须防抖，否则每个事件都会执行一遍 git。
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
        // 断开订阅时将尚未执行的那次防抖任务一并取消，否则会在无人接收后再执行一次 git。
        Disposer.register(subscription) { pending.getAndSet(null)?.cancel(false) }
        return subscription
    }

    /**
     * 过滤掉与仓库无关以及 `.git` 内部的噪声事件。`.git/` 下仅认 `index`（暂存区变化，`git add` 要反映到列表）和 `HEAD`（切分支）；
     * 其余（objects/、logs/、锁文件）改动极频繁且不影响文件列表，放进来会使防抖窗口持续重置、状态始终刷新不出来。
     */
    private fun VFileEvent.isRelevantTo(root: File): Boolean {
        val path = path.replace(File.separatorChar, '/')
        val rootPath = root.absolutePath.replace(File.separatorChar, '/')
        if (!path.startsWith("$rootPath/", ignoreCase = caseInsensitiveFs)) return false
        // 大小写不敏感 FS 上 path 大小写可能与 rootPath 不同，removePrefix 是精确匹配会失败；用索引截断保证一致。
        val relative = path.substring(rootPath.length + 1)
        if (!relative.startsWith(".git/") && relative != ".git") return true
        val inner = relative.removePrefix(".git/")
        return inner == "index" || inner == "HEAD"
    }

    // ---------------------------------------------------------------- 缓存

    /** 丢弃仓库根与状态缓存。仓库切换、外部改动后调用。 */
    fun invalidate() {
        cachedRoot = null
        cache = GitState()
        synchronized(reviewFileStatus) { reviewFileStatus.clear() }
    }

    // ---------------------------------------------------------------- 进程

    /** 校验用户填写的 ref/sha：trim 后非空、不以 `-` 开头、不含空白/控制符，否则返回 null（防被 git 当选项或断行注入）。 */
    private fun safeRef(ref: String): String? {
        val t = ref.trim()
        if (t.isEmpty() || t.startsWith("-") || t.any { it.isWhitespace() || it.code < 0x20 }) return null
        return t
    }

    /**
     * 执行一条 git 命令，返回 stdout。任何失败（非零退出、超时、git 不存在）均返回 null，
     * 调用方按"取不到则降级"处理——git 查询失败不应导致整个审查流程崩溃。
     *
     * stderr 丢弃：git 的进度和提示信息会混入输出，而所有解析仅认 stdout 的格式。
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
            // stdout 必须在独立线程读取：当前线程 readText() 读到 EOF 才会 waitFor，进程挂住不关闭 stdout 时会永久阻塞，后续超时判定形同虚设。
            val stdout = StringBuilder()
            val reader = Thread({
                runCatching { process.inputStream.bufferedReader(Charsets.UTF_8).use { stdout.append(it.readText()) } }
                    .onFailure { thisLogger().warn("[ocr] Failed to read git stdout: ${args.joinToString(" ")}", it) }
            }, "ocr-git-stdout").apply { isDaemon = true; start() }
            if (!process.waitFor(GIT_TIMEOUT_MS, TimeUnit.MILLISECONDS)) {
                process.destroyForcibly()
                reader.join(1_000) // 强杀后等 reader 收尾，避免 daemon 在反复超时中堆积
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
 * 将 unix 秒时间戳格式化为相对时间。[nowMillis] 由调用方传入而非内部取 `System.currentTimeMillis()`，便于测试；
 * 同理 [locale] 由调用方提供，保持纯函数。
 * 仅 justNow/hourAgo/hoursAgo/yesterday/daysAgo 五档为基础，此处新增分钟、月、年三档。
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
