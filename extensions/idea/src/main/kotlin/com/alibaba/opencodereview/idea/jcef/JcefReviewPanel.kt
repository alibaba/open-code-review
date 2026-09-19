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
 * The sidebar's JCEF host: creates the webview, wires the message bridge, and delivers host messages.
 * Dispatch is left to the router layer. attach/detach go through the sidebar-specific entry points: the JCEF
 * callback carries only a string with no source identity, so the channel identity must be provided explicitly
 * at this layer, otherwise config-panel messages would bleed into the sidebar.
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
            // OcrWebview construction or attachSidebar can still throw errors even when isSupported is true:
            // the factory layer catches and shows the placeholder, but a half-constructed OcrWebview (timer already
            // started, message bus already connected) would leak if nobody disposes it -- so clean up here, then fall back to the placeholder.
            val (vw, comp, ch) = try {
                val view = OcrWebview(
                    html = { bridge -> WebviewHtml.sidebar(service.currentLocale(), bridge) },
                    onMessage = service::handleFromSidebar,
                )
                val outbound = WebviewChannel { json -> view.post(json) }
                try {
                    val viewComponent = view.component // fetched before attach to narrow the window in which attach can still throw
                    service.attachSidebar(outbound)
                    Triple(view, viewComponent, outbound)
                } catch (e: Exception) {
                    // Always attempt detach around attach (idempotent): the channel may already be partially
                    // registered; skipping cleanup would leak it and keep delivering to the disposed view.
                    runCatching { service.detachSidebar(outbound) }
                    runCatching { view.dispose() }
                    throw e
                }
            } catch (e: Exception) {
                // Catch Exceptions only: Errors such as OOM/LinkageError are not swallowed here -- the factory layer's
                // catch(Throwable) is the backstop for them, so fatal problems are not masked.
                // ProcessCanceledException is IntelliJ's cancellation signal and must never be swallowed (it would break cancellation).
                if (e is ProcessCanceledException) throw e
                thisLogger().warn("[ocr] Sidebar JCEF webview initialization failed, falling back to the placeholder", e)
                Triple(null, jcefUnsupportedPlaceholder(), null)
            }
            webview = vw
            component = comp
            channel = ch
            // OcrWebview's internal messageBus.connect(this) hooks it into the Disposer tree (under ROOT_DISPOSABLE);
            // it must be registered as a child node of this panel, otherwise the Disposer cannot find the parent on IDE
            // shutdown -> memory leak.
            vw?.let { Disposer.register(this, it) }
        }
    }

    override fun dispose() {
        // Capture the local before clearing: channel is @Volatile but read-then-clear is not atomic, so capturing
        // the local avoids racing a possible re-attach.
        val ch = channel
        channel = null
        // A detach throw should not block the webview release (otherwise the JCEF browser leaks its timer and
        // message-bus listeners), hence runCatching.
        if (ch != null) runCatching { service.detachSidebar(ch) }
        webview?.dispose()
    }
}

/**
 * Fallback shown when JCEF is unavailable (every piece of plugin UI is dead in that state). [locale] defaults to
 * the IDE UI language: reaching this path means the webview could not start, so the page-supplied locale is unavailable.
 */
internal fun jcefUnsupportedPlaceholder(locale: SupportedLocale = currentIdeLocale()): JComponent =
    JPanel(BorderLayout()).apply {
        add(
            JBLabel("<html>" + HostStrings.t(locale, "ext.jcef.unsupportedPlaceholder") + "</html>"),
            BorderLayout.NORTH,
        )
    }
