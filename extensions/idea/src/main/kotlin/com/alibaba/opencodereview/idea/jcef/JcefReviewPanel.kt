// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.jcef

import com.alibaba.opencodereview.idea.messages.WebviewChannel
import com.alibaba.opencodereview.idea.model.HostStrings
import com.alibaba.opencodereview.idea.model.SupportedLocale
import com.alibaba.opencodereview.idea.model.currentIdeLocale
import com.alibaba.opencodereview.idea.review.ReviewProjectService
import com.intellij.openapi.Disposable
import com.intellij.openapi.diagnostic.thisLogger
import com.intellij.openapi.project.Project
import com.intellij.openapi.progress.ProcessCanceledException
import com.intellij.openapi.util.Disposer
import com.intellij.ui.components.JBLabel
import com.intellij.ui.jcef.JBCefApp
import javax.swing.JComponent
import javax.swing.JPanel
import java.awt.BorderLayout

/**
 * The sidebar's JCEF host: creates the webview, wires the message bridge and posts host messages.
 * Dispatching is left to the routing layer.
 * attach/detach go through the sidebar-only entry points: a JCEF callback supplies a bare string
 * with no origin, so the channel identity has to come from this layer — otherwise config panel
 * messages would leak into the sidebar.
 */
class JcefReviewPanel(project: Project) : Disposable {

    private val service = project.getService(ReviewProjectService::class.java)
    private val webview: OcrWebview?
    @Volatile
    private var channel: WebviewChannel? = null

    val component: JComponent

    init {
        if (!JBCefApp.isSupported()) {
            webview = null
            component = jcefUnsupportedPlaceholder()
        } else {
            // OcrWebview's constructor and attachSidebar can throw even when isSupported is true:
            // the factory layer catches and shows the placeholder, but a half-constructed
            // OcrWebview (timers already running, message bus connected) leaks unless someone
            // disposes it, so clean up here and fall back to the placeholder.
            val (vw, comp, ch) = try {
                val view = OcrWebview(
                    html = { bridge -> WebviewHtml.sidebar(service.currentLocale(), bridge) },
                    onMessage = service::handleFromSidebar,
                )
                val outbound = WebviewChannel { json -> view.post(json) }
                try {
                    val viewComponent = view.component // read before attach, narrowing the window in which attach can still throw
                    service.attachSidebar(outbound)
                    Triple(view, viewComponent, outbound)
                } catch (e: Exception) {
                    // Try detach first in both cases (it is idempotent): the channel may be
                    // partially registered, and leaving it leaks and keeps posting to a disposed view.
                    runCatching { service.detachSidebar(outbound) }
                    runCatching { view.dispose() }
                    throw e
                }
            } catch (e: Exception) {
                // Exception only: Errors such as OOM/LinkageError are not swallowed here but left to
                // the factory's catch(Throwable), so a fatal problem is not masked.
                // ProcessCanceledException is IntelliJ's cancellation signal and must never be
                // swallowed (that would break cancellation).
                if (e is ProcessCanceledException) throw e
                thisLogger().warn("[ocr] Sidebar JCEF webview failed to initialize, falling back to placeholder", e)
                Triple(null, jcefUnsupportedPlaceholder(), null)
            }
            webview = vw
            component = comp
            channel = ch
            // Inside OcrWebview, messageBus.connect(this) attaches it to the Disposer tree (under
            // ROOT_DISPOSABLE); it must be registered as a child of this panel, or the Disposer
            // finds no parent when the IDE closes → memory leak.
            vw?.let { Disposer.register(this, it) }
        }
    }

    override fun dispose() {
        // Read into a local before clearing: channel is @Volatile but reading then clearing is not
        // atomic, so capture the local to avoid racing a possible re-attach.
        val ch = channel
        channel = null
        // A throwing detach must not block the webview release (that would leak the JCEF browser:
        // timers, message bus listeners), hence runCatching.
        if (ch != null) runCatching { service.detachSidebar(ch) }
        webview?.dispose()
    }
}

/**
 * Fallback notice for when JCEF is unavailable (all of the plugin's UI is then unusable).
 * [locale] defaults to the IDE UI language: reaching this path means the webview cannot start, so
 * no page language is available.
 */
internal fun jcefUnsupportedPlaceholder(locale: SupportedLocale = currentIdeLocale()): JComponent =
    JPanel(BorderLayout()).apply {
        add(
            JBLabel("<html>" + HostStrings.t(locale, "ext.jcef.unsupportedPlaceholder") + "</html>"),
            BorderLayout.NORTH,
        )
    }
