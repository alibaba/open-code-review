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
 * Window implementation for the config panel, using a non-modal dialog: it can sit next to the
 * editor, be dragged and resized, and is released as soon as it closes.
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
        // Silently dropped while the panel is closed: log lines from long tasks such as an install
        // can arrive after it closes.
        webview?.post(json)
    }

    override fun open() {
        onEdt {
            dialog?.let { existing ->
                if (existing.isShowing) {
                    // When already open, just bring it to the front without rebuilding the window
                    // (rebuilding would lose the user's filled-in form).
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
        // Inside OcrWebview, messageBus.connect(this) attaches it to the Disposer tree (under
        // ROOT_DISPOSABLE); register it as a child of this host, or the Disposer finds no parent
        // when the IDE closes → memory leak.
        Disposer.register(this, view)
        // Dispose the webview when the dialog closes (close button, Esc, or a closeConfigPanel sent
        // by the page), or the CEF browser lingers and has to be created again next time.
        Disposer.register(created.disposable) {
            view.dispose()  // idempotent; the Disposer calls it too
            if (webview === view) webview = null
            if (dialog === created) dialog = null
        }
        return created
    }

    override fun dispose() {
        // Read into locals and clear, to avoid racing the dialog-close callback's second dispose.
        val w = webview
        val d = dialog
        webview = null
        dialog = null
        // Both closing a still-visible dialog (so that closing a project with the panel open leaves
        // no orphan native window) and releasing the JCEF browser must happen on the EDT (the
        // Swing/CEF contract). onEdt is not used because its isDisposed guard skips the project-close
        // path and the browser would never be released; this invokeLater has no guard, so the EDT
        // still dispatches during shutdown, and if it does not, the JVM exit reclaims the resources.
        // OcrWebview and dialog.close are idempotent.
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
            // Re-fetch the strings each time the window opens: the user may switch the IDE language
            // in the meantime, and the next open should use the new one.
            title = HostStrings.t(locale(), "ext.configPanelTitle")
            isModal = false // the user must be able to go back and read code while configuring
            init()
        }

        override fun createCenterPanel(): JComponent = content.apply {
            preferredSize = Dimension(WIDTH, HEIGHT)
        }

        /** The page already has save/close buttons; another row of OK/Cancel at the bottom would
         *  only confuse. */
        override fun createActions(): Array<Action> = emptyArray()

        /** Remembers the window size the user set. */
        override fun getDimensionServiceKey(): String = "ocr.configPanel"
    }
}
