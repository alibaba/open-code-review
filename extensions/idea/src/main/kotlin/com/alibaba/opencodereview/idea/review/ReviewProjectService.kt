// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

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
 * Project-level coordinator: service assembly, webview registration, and cross-component wiring all live here.
 * Message handling does not: the sidebar is handled by [SidebarRouter], the config panel by [ConfigPanelRouter].
 *
 * Why message sources are distinguished: the sidebar and the config panel are two independent webviews whose
 * messages naturally carry their source; the JCEF side has only a `handle(raw)` callback with no origin
 * information in the string, so the entry points are split per channel into [handleFromSidebar]/[handleFromConfigPanel].
 * Merging them into one entry would deliver each side the other's messages -- `config`, an outbound type name
 * used by both sides, would reach the wrong webview.
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
     * The config panel window. Created lazily: most sessions never open it, and creating a JCEF browser eagerly
     * would tie up memory for nothing. Null when JCEF is unavailable, in which case [openConfigPanel]
     * degrades to a notification.
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
        // Comment status changes (apply / discard / false positive) must be pushed back to the sidebar; the frontend
        // relies on them to update the status badge on the card.
        comments.onSync = { states -> postSidebar(HostToWebview.CommentSync(states)) }
        // In branch/commit mode comments attach to the temporary documents on both diff sides, and only GitService
        // can grab the handle at the moment it creates the document.
        // GitService knows nothing about comments; wire them together here. See CommentService.decorateDiff.
        git.diffDecorator = { path, side, document -> comments.decorateDiff(path, side, document) }
        // DiffDecorator attaches line highlights and gutter icons before showDiff, while diffViewerReady mounts the
        // inline panels after it -- opposite timings, hence two hooks. See GitService.diffViewerReady.
        git.diffViewerReady = { path, side, document, clickedIndex -> comments.mountDiffPanels(path, side, document, clickedIndex) }
    }

    // ------------------------------------------------------------ Sidebar channel

    /** The workspace-change subscription; exists only while a sidebar is alive. */
    private var gitWatch: Disposable? = null
    private val watchLock = Any()

    fun attachSidebar(channel: WebviewChannel) {
        sidebarChannels.add(channel)
        synchronized(watchLock) {
            // When the user edits files or switches branches in the IDE, the sidebar's workspace file list must follow.
            if (gitWatch == null) {
                gitWatch = git.watchWorkspaceChanges(this) { state -> postSidebar(HostToWebview.GitStateChanged(state)) }
            }
        }
    }

    fun detachSidebar(channel: WebviewChannel) {
        sidebarChannels.remove(channel)
        if (sidebarChannels.isNotEmpty()) return
        synchronized(watchLock) {
            if (sidebarChannels.isNotEmpty()) return // do not disconnect on an attach/detach interleave
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

    // ------------------------------------------------------------ Config-panel channel

    fun handleFromConfigPanel(raw: String) {
        panelRouter.handle(parseWebviewMessage(raw, currentLocale()))
    }

    private fun postConfigPanel(msg: ConfigPanelHostToWebview) {
        val host = panelHost ?: return
        runCatching { host.channel.post(msg.toJson()) }
            .onFailure { thisLogger().warn("[ocr] Config panel post failed", it) }
    }

    /**
     * Opens the config panel. When already open, do not rebuild -- just re-send `configPanelFocus` to jump to the
     * target step, since rebuilding would lose the user's half-filled form.
     */
    fun openConfigPanel(focus: JsonElement?) {
        val host = panelHost
        if (host == null) {
            notifyJcefUnsupported()
            return
        }
        if (host.isOpen) {
            postConfigPanel(ConfigPanelHostToWebview.Focus(focus))
            host.open() // equivalent to showing when already open; brings the panel to the front
            panelRouter.setPendingFocus(null)
        } else {
            // The panel only sends readyConfigPanel once the webview is ready, so stash the focus for now.
            panelRouter.setPendingFocus(focus)
            host.open()
        }
    }

    /** After the config panel changes the configuration, push the new config to the sidebar -- the sidebar relies on it to decide whether a review can start. */
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

    /** Jump-failure / apply-failure notifications from [CommentService] all go through here -- native IDE notifications. */
    private fun notifyComment(message: String, type: NotificationType) {
        val group = NotificationGroupManager.getInstance().getNotificationGroup(NOTIFICATION_GROUP)
        group.createNotification(message, type).notify(project)
    }

    // ------------------------------------------------------------ Misc

    /** The current IDE UI language. The JCEF panel needs it too when assembling HTML, hence internal rather than private. */
    internal fun currentLocale(): SupportedLocale = currentIdeLocale()

    private fun projectDir(): File =
        project.basePath?.let(::File)?.takeIf { it.isDirectory }
            ?: File(System.getProperty("user.dir"))

    override fun dispose() {
        sidebar.cancelActiveSession()
        sidebarChannels.clear()
        comments.onSync = null
        // Clear the git callbacks: openDiff may have pending invokeLater runs that, after this service is disposed,
        // would call into the disposed comments.
        git.diffDecorator = null
        git.diffViewerReady = null
        // panelHost is by lazy and must not be touched here -- a session that never opened the config panel should
        // not initialize a JCEF browser just because of dispose.
    }
}
