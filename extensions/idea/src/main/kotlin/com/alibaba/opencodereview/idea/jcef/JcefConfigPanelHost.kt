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
 * 配置面板的窗口实现，使用非模态对话框：可与编辑器并排查看、可拖动缩放、关闭即释放。
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
        // 面板已关闭时静默丢弃：安装等长耗时任务的日志可能在关闭后才到达。
        webview?.post(json)
    }

    override fun open() {
        onEdt {
            dialog?.let { existing ->
                if (existing.isShowing) {
                    // 已打开时仅置顶，不重建窗口（重建会丢失用户已填写的表单）。
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
        // 对话框关闭（点击关闭按钮、按 Esc 或由页面发送 closeConfigPanel）时销毁 webview，否则 CEF browser 会持续驻留，下次又重新创建。
        Disposer.register(created.disposable) {
            view.dispose()
            if (webview === view) webview = null
            if (dialog === created) dialog = null
        }
        return created
    }

    override fun dispose() {
        // 取局部后清空，避免与对话框关闭回调的二次 dispose 竞态。
        val w = webview
        val d = dialog
        webview = null
        dialog = null
        // 关闭仍可见的对话框（项目关闭时若面板开着，避免留孤儿原生窗口）+ 释放 JCEF browser 都须在 EDT
        // （Swing/CEF 契约）。不用 onEdt——其 isDisposed 守卫会在项目关闭路径跳过、browser 永不释放；
        // 此处 invokeLater 不带守卫，关闭时 EDT 仍会派发；派发不到也无妨（JVM 退出由 OS 回收）。OcrWebview/dialog.close 幂等。
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
            // 每次开窗时重新取词：用户可能中途更换 IDE 语言，下次打开即采用新语言。
            title = HostStrings.t(locale(), "ext.configPanelTitle")
            isModal = false // 配置时要能回去看代码
            init()
        }

        override fun createCenterPanel(): JComponent = content.apply {
            preferredSize = Dimension(WIDTH, HEIGHT)
        }

        /** 页面自身已有保存/关闭按钮，底部再放置一排 OK/Cancel 仅会造成困惑。 */
        override fun createActions(): Array<Action> = emptyArray()

        /** 记住用户调过的窗口大小。 */
        override fun getDimensionServiceKey(): String = "ocr.configPanel"
    }
}
