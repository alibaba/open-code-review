// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.jcef

import com.alibaba.opencodereview.idea.messages.WebviewChannel
import com.alibaba.opencodereview.idea.model.HostStrings
import com.alibaba.opencodereview.idea.model.SupportedLocale
import com.alibaba.opencodereview.idea.review.ConfigPanelHost
import com.intellij.openapi.Disposable
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.project.Project
import com.intellij.openapi.ui.DialogWrapper
import com.intellij.openapi.util.Disposer
import java.awt.Dimension
import javax.swing.Action
import javax.swing.JComponent

/**
 * Window implementation of the config panel, as a non-modal dialog: viewable side-by-side with the editor,
 * resizable by dragging, and released on close.
 */
class JcefConfigPanelHost(
    private val project: Project,
    private val locale: () -> SupportedLocale,
    private val onMessage: (String) -> Unit,
) : ConfigPanelHost, Disposable {

    private companion object {
        const val WIDTH = 900
        const val HEIGHT = 720
    }

    @Volatile
    private var dialog: PanelDialog? = null

    @Volatile
    private var webview: OcrWebview? = null

    override val isOpen: Boolean
        get() = dialog?.isShowing == true

    override val channel: WebviewChannel = WebviewChannel { json ->
        // Silently dropped when the panel is closed: logs from long-running tasks such as installs may arrive after closing.
        webview?.post(json)
    }

    override fun open() {
        onEdt {
            dialog?.let { existing ->
                if (existing.isShowing) {
                    // Already open: just bring to front instead of rebuilding (rebuilding would lose the user's half-filled form).
                    existing.window?.toFront()
                    return@onEdt
                }
            }
            createDialog().show()
        }
    }

    override fun close() {
        onEdt { dialog?.close(DialogWrapper.OK_EXIT_CODE) }
    }

    private fun createDialog(): PanelDialog {
        val view = OcrWebview(
            html = { bridge -> WebviewHtml.configPanel(locale(), bridge) },
            onMessage = onMessage,
        )
        webview = view
        val created = PanelDialog(view.component)
        dialog = created
        // OcrWebview's internal messageBus.connect(this) hooks it into the Disposer tree (under ROOT_DISPOSABLE);
        // register it as a child of this host, otherwise the Disposer cannot find the parent on IDE shutdown -> memory leak.
        Disposer.register(this, view)
        // Destroy the webview when the dialog closes (close button, Esc, or a closeConfigPanel sent by the page),
        // otherwise the CEF browser lingers and gets created anew next time.
        Disposer.register(created.disposable) {
            view.dispose()  // idempotent; the Disposer calls it once more too
            if (webview === view) webview = null
            if (dialog === created) dialog = null
        }
        return created
    }

    override fun dispose() {
        // Capture the locals then clear, avoiding a race with the dialog-close callback's second dispose.
        val w = webview
        val d = dialog
        webview = null
        dialog = null
        // Closing a still-visible dialog (if the panel is open during project shutdown, avoiding an orphaned native
        // window) and releasing the JCEF browser must both happen on the EDT (the Swing/CEF contract).
        // Deliberately not onEdt -- its isDisposed guard would skip on the project-close path and the browser would
        // never be released; invokeLater here is unguarded because the EDT still dispatches during shutdown, and if
        // it never dispatches, JVM exit reclaims everything anyway. OcrWebview/dialog.close are idempotent.
        val app = ApplicationManager.getApplication()
        val cleanup: () -> Unit = {
            d?.let { runCatching { it.close(DialogWrapper.OK_EXIT_CODE) } }
            w?.let { runCatching { it.dispose() } }
        }
        if (app.isDispatchThread) cleanup() else app.invokeLater(cleanup, com.intellij.openapi.application.ModalityState.any())
    }

    private fun onEdt(block: () -> Unit) {
        ApplicationManager.getApplication().invokeLater {
            if (!project.isDisposed) block()
        }
    }

    private inner class PanelDialog(private val content: JComponent) :
        DialogWrapper(project, /* canBeParent = */ false) {

        init {
            // Re-resolve the wording on every open: the user may switch the IDE language mid-session, and the next
            // open should use the new language.
            title = HostStrings.t(locale(), "ext.configPanelTitle")
            isModal = false // so the user can go back to the code while configuring
            init()
        }

        override fun createCenterPanel(): JComponent = content.apply {
            preferredSize = Dimension(WIDTH, HEIGHT)
        }

        /** The page already has its own save/close buttons; another row of OK/Cancel at the bottom would lead to confusion. */
        override fun createActions(): Array<Action> = emptyArray()

        /** Remembers the user-adjusted window size. */
        override fun getDimensionServiceKey(): String = "ocr.configPanel"
    }
}
