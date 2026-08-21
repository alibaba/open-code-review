package com.alibaba.opencodereview.idea.review

import com.alibaba.opencodereview.idea.messages.ConfigPanelHostToWebview
import com.alibaba.opencodereview.idea.messages.HostToWebview
import com.alibaba.opencodereview.idea.messages.WebviewChannel
import com.alibaba.opencodereview.idea.messages.parseWebviewMessage
import com.alibaba.opencodereview.idea.messages.toJson
import com.alibaba.opencodereview.idea.model.HostStrings
import com.alibaba.opencodereview.idea.model.OcrConfig
import com.alibaba.opencodereview.idea.model.SupportedLocale
import com.alibaba.opencodereview.idea.model.currentIdeLocale
import com.alibaba.opencodereview.idea.jcef.JcefConfigPanelHost
import com.alibaba.opencodereview.idea.providers.CommentService
import com.alibaba.opencodereview.idea.services.CliService
import com.alibaba.opencodereview.idea.services.ConfigService
import com.alibaba.opencodereview.idea.services.GitService
import com.intellij.notification.NotificationGroupManager
import com.intellij.notification.NotificationType
import com.intellij.openapi.Disposable
import com.intellij.openapi.components.Service
import com.intellij.openapi.diagnostic.thisLogger
import com.intellij.openapi.project.Project
import com.intellij.openapi.util.Disposer
import com.intellij.ui.jcef.JBCefApp
import kotlinx.serialization.json.JsonElement
import java.io.File
import java.util.concurrent.CopyOnWriteArraySet

/**
 * 项目级协调器：服务组装、webview 注册、跨组件连线均在此层。
 * 消息处理不在本层：侧栏在 [SidebarRouter]，配置面板在 [ConfigPanelRouter]。
 *
 * 为什么要区分消息来源：侧栏与配置面板是两个独立 webview，各自收到的消息天然带来源；
 * JCEF 这边只有一个 `handle(raw)` 回调、字符串无来源信息，故入口按通道分 [handleFromSidebar]/[handleFromConfigPanel]。
 * 若混成一个入口，侧栏和面板会收到对方消息，`config` 这种两侧同名出站类型会被投递到错误的 webview。
 */
@Service(Service.Level.PROJECT)
class ReviewProjectService(private val project: Project) : Disposable {

    private companion object {
        const val NOTIFICATION_GROUP = "Open Code Review"
    }

    private val cli = CliService()
    private val git = GitService(project)
    private val config = ConfigService(cli, projectDir())
    private val comments = CommentService(project, git, notify = ::notifyComment, locale = ::currentLocale)

    private val sidebarChannels = CopyOnWriteArraySet<WebviewChannel>()

    /**
     * 配置面板窗口。惰性创建：绝大多数会话不开它，提前创建 JCEF browser 会占用不必要的内存。
     * JCEF 不可用时为 null，此时 [openConfigPanel] 退化成一条通知。
     */
    private val panelHost: ConfigPanelHost? by lazy {
        if (!JBCefApp.isSupported()) return@lazy null
        JcefConfigPanelHost(project, ::currentLocale, ::handleFromConfigPanel)
            .also { Disposer.register(this, it) }
    }

    private val sidebar = SidebarRouter(
        project = project,
        cli = cli,
        config = config,
        git = git,
        comments = comments,
        locale = ::currentLocale,
        post = ::postSidebar,
        openConfigPanel = ::openConfigPanel,
    )

    private val panelRouter = ConfigPanelRouter(
        project = project,
        cli = cli,
        config = config,
        locale = ::currentLocale,
        post = ::postConfigPanel,
        closePanel = { panelHost?.close() },
        onConfigChanged = ::onConfigChanged,
    )

    init {
        Disposer.register(this, comments)
        // 评论状态变化（应用 / 忽略 / 误报）要回推给侧栏，前端依赖它更新卡片上的状态角标。
        comments.onSync = { states -> postSidebar(HostToWebview.CommentSync(states)) }
        // branch/commit 模式评论要挂 diff 两侧的临时文档上，只有 GitService 造文档那一刻能拿到句柄。
        // GitService 不认识评论，这里接起来。见 CommentService.decorateDiff。
        git.diffDecorator = { path, side, document -> comments.decorateDiff(path, side, document) }
        // DiffDecorator 在 showDiff 之前挂行高亮和装订线图标，diffViewerReady 在之后挂内嵌面板——两个时机相反，故两个钩子。见 GitService.diffViewerReady。
        git.diffViewerReady = { path, side, document, clickedIndex -> comments.mountDiffPanels(path, side, document, clickedIndex) }
    }

    // ------------------------------------------------------------ 侧栏通道

    /** 工作区变更订阅，只在侧栏存活时存在。 */
    private var gitWatch: Disposable? = null
    private val watchLock = Any()

    fun attachSidebar(channel: WebviewChannel) {
        sidebarChannels.add(channel)
        synchronized(watchLock) {
            // 用户在 IDE 里改了文件或切了分支，侧栏的工作区文件列表要随之更新。
            if (gitWatch == null) {
                gitWatch = git.watchWorkspaceChanges(this) { state -> postSidebar(HostToWebview.GitStateChanged(state)) }
            }
        }
    }

    fun detachSidebar(channel: WebviewChannel) {
        sidebarChannels.remove(channel)
        if (sidebarChannels.isNotEmpty()) return
        synchronized(watchLock) {
            if (sidebarChannels.isNotEmpty()) return // attach 与 detach 交错时不要误断开
            gitWatch?.let(Disposer::dispose)
            gitWatch = null
        }
    }

    fun handleFromSidebar(raw: String) {
        sidebar.handle(parseWebviewMessage(raw, currentLocale()))
    }

    private fun postSidebar(msg: HostToWebview) {
        if (sidebarChannels.isEmpty()) return
        val json = msg.toJson()
        sidebarChannels.forEach { channel ->
            runCatching { channel.post(json) }
                .onFailure { thisLogger().warn("[ocr] Sidebar post failed", it) }
        }
    }

    // ------------------------------------------------------------ 配置面板通道

    fun handleFromConfigPanel(raw: String) {
        panelRouter.handle(parseWebviewMessage(raw, currentLocale()))
    }

    private fun postConfigPanel(msg: ConfigPanelHostToWebview) {
        val host = panelHost ?: return
        runCatching { host.channel.post(msg.toJson()) }
            .onFailure { thisLogger().warn("[ocr] Config panel post failed", it) }
    }

    /**
     * 打开配置面板。已开时不重建，只补发 `configPanelFocus` 跳到目标步骤——
     * 重建会丢用户填了一半的表单。
     */
    fun openConfigPanel(focus: JsonElement?) {
        val host = panelHost
        if (host == null) {
            notifyJcefUnsupported()
            return
        }
        if (host.isOpen) {
            postConfigPanel(ConfigPanelHostToWebview.Focus(focus))
            host.open() // 已打开时等价于显露，把面板带到前台
            panelRouter.setPendingFocus(null)
        } else {
            // 面板要等 webview 就绪才会发 readyConfigPanel，focus 先暂存。
            panelRouter.setPendingFocus(focus)
            host.open()
        }
    }

    /** 配置面板改了配置之后，把新配置推给侧栏——侧栏依赖它判断能否开始审查。 */
    private fun onConfigChanged(updated: OcrConfig?) {
        postSidebar(HostToWebview.Config(updated))
    }

    private fun notifyJcefUnsupported() {
        val loc = currentLocale()
        val group = NotificationGroupManager.getInstance().getNotificationGroup(NOTIFICATION_GROUP)
        group.createNotification(
            HostStrings.t(loc, "ext.jcef.unsupportedTitle"),
            HostStrings.t(loc, "ext.jcef.unsupportedBody"),
            NotificationType.WARNING,
        ).notify(project)
    }

    /** [CommentService] 的跳转失败 / 采纳失败通知都走这里——原生 IDE 通知。 */
    private fun notifyComment(message: String, type: NotificationType) {
        val group = NotificationGroupManager.getInstance().getNotificationGroup(NOTIFICATION_GROUP)
        group.createNotification(message, type).notify(project)
    }

    // ------------------------------------------------------------ 杂项

    /** 当前 IDE 界面语言。JCEF 面板装配 HTML 时也要用，所以是 internal 而不是 private。 */
    internal fun currentLocale(): SupportedLocale = currentIdeLocale()

    private fun projectDir(): File =
        project.basePath?.let(::File)?.takeIf { it.isDirectory }
            ?: File(System.getProperty("user.dir"))

    override fun dispose() {
        sidebar.cancelActiveSession()
        sidebarChannels.clear()
        comments.onSync = null
        // 清空 git 回调：openDiff 里可能有未执行的 invokeLater，服务已释放后再触发会调到已 dispose 的 comments。
        git.diffDecorator = null
        git.diffViewerReady = null
        // panelHost 是 by lazy，这里不应访问——未打开过配置面板的会话不应因 dispose 去初始化 JCEF browser。
    }
}
