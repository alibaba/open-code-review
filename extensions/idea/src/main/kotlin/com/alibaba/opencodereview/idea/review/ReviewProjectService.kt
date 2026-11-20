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
 * Project-level coordinator: service assembly, webview registration and cross-component wiring all
 * happen in this layer. Message handling does not: the sidebar is in [SidebarRouter] and the config
 * panel in [ConfigPanelRouter].
 *
 * Why the message origin is tracked: the sidebar and the config panel are two independent webviews,
 * and the messages each one receives inherently carry their origin, while JCEF gives us a single
 * `handle(raw)` callback whose string says nothing about the source — hence the split into
 * [handleFromSidebar]/[handleFromConfigPanel]. A single entry point would deliver each side the
 * other's messages, and an outbound type shared by name such as `config` would reach the wrong
 * webview.
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
     * The config panel window. Created lazily: most sessions never open it, and creating a JCEF
     * browser early would hold memory needlessly.
     * Null when JCEF is unavailable, in which case [openConfigPanel] degrades to a notification.
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
        // Comment status changes (applied / discarded / false positive) have to be pushed back to the
        // sidebar; the frontend uses them to update the status badge on each card.
        comments.onSync = { states -> postSidebar(HostToWebview.CommentSync(states)) }
        // In branch/commit mode comments are mounted on the temporary documents on either side of
        // the diff, and only GitService, at the moment it creates them, holds a handle. GitService
        // knows nothing about comments; this wires the two together. See CommentService.decorateDiff.
        git.diffDecorator = { path, side, document -> comments.decorateDiff(path, side, document) }
        // DiffDecorator attaches line highlights and gutter icons before showDiff, diffViewerReady
        // attaches inline panels after — opposite timings, hence two hooks. See
        // GitService.diffViewerReady.
        git.diffViewerReady = { path, side, document, clickedIndex -> comments.mountDiffPanels(path, side, document, clickedIndex) }
    }

    // ------------------------------------------------------------ Sidebar channel

    /** Workspace change subscription, alive only while the sidebar is. */
    private var gitWatch: Disposable? = null
    private val watchLock = Any()

    fun attachSidebar(channel: WebviewChannel) {
        sidebarChannels.add(channel)
        synchronized(watchLock) {
            // When the user edits a file or switches branches in the IDE, the sidebar's workspace
            // file list has to follow.
            if (gitWatch == null) {
                gitWatch = git.watchWorkspaceChanges(this) { state -> postSidebar(HostToWebview.GitStateChanged(state)) }
            }
        }
    }

    fun detachSidebar(channel: WebviewChannel) {
        sidebarChannels.remove(channel)
        if (sidebarChannels.isNotEmpty()) return
        synchronized(watchLock) {
            if (sidebarChannels.isNotEmpty()) return // do not disconnect while attach and detach interleave
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

    // ------------------------------------------------------------ Config panel channel

    fun handleFromConfigPanel(raw: String) {
        panelRouter.handle(parseWebviewMessage(raw, currentLocale()))
    }

    private fun postConfigPanel(msg: ConfigPanelHostToWebview) {
        val host = panelHost ?: return
        runCatching { host.channel.post(msg.toJson()) }
            .onFailure { thisLogger().warn("[ocr] Config panel post failed", it) }
    }

    /**
     * Opens the config panel. When it is already open, recreate nothing and just resend
     * `configPanelFocus` to jump to the target step — rebuilding would lose a half-filled form.
     */
    fun openConfigPanel(focus: JsonElement?) {
        val host = panelHost
        if (host == null) {
            notifyJcefUnsupported()
            return
        }
        if (host.isOpen) {
            postConfigPanel(ConfigPanelHostToWebview.Focus(focus))
            host.open() // when already open this just surfaces it, bringing the panel to the front
            panelRouter.setPendingFocus(null)
        } else {
            // The panel only sends readyConfigPanel once its webview is ready, so stash the focus first.
            panelRouter.setPendingFocus(focus)
            host.open()
        }
    }

    /** After the config panel changes the config, push the new config to the sidebar — which relies
     *  on it to decide whether a review can start. */
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

    /** Jump failures and apply failures from [CommentService] all come through here — native IDE
     *  notifications. */
    private fun notifyComment(message: String, type: NotificationType) {
        val group = NotificationGroupManager.getInstance().getNotificationGroup(NOTIFICATION_GROUP)
        group.createNotification(message, type).notify(project)
    }

    // ------------------------------------------------------------ Misc

    /** The current IDE UI language. Used when the JCEF panels assemble their HTML too, hence
     *  internal rather than private. */
    internal fun currentLocale(): SupportedLocale = currentIdeLocale()

    private fun projectDir(): File =
        project.basePath?.let(::File)?.takeIf { it.isDirectory }
            ?: File(System.getProperty("user.dir"))

    override fun dispose() {
        sidebar.cancelActiveSession()
        sidebarChannels.clear()
        comments.onSync = null
        // Clear the git callbacks: openDiff may have an unpicked invokeLater, and firing it after the
        // service is disposed would reach a disposed comments object.
        git.diffDecorator = null
        git.diffViewerReady = null
        // panelHost is by lazy and must not be touched here — a session that never opened the config
        // panel should not initialize a JCEF browser just because it is being disposed.
    }
}
