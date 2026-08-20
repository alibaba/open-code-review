package com.alibaba.opencodereview.idea.jcef

import com.alibaba.opencodereview.idea.messages.WebviewChannel
import com.alibaba.opencodereview.idea.model.HostStrings
import com.alibaba.opencodereview.idea.model.SupportedLocale
import com.alibaba.opencodereview.idea.model.currentIdeLocale
import com.alibaba.opencodereview.idea.review.ReviewProjectService
import com.intellij.openapi.Disposable
import com.intellij.openapi.project.Project
import com.intellij.ui.components.JBLabel
import com.intellij.ui.jcef.JBCefApp
import javax.swing.JComponent
import javax.swing.JPanel
import java.awt.BorderLayout

/**
 * 侧栏的 JCEF 宿主：创建 webview、装配消息桥、投递宿主消息。消息分发交由路由层处理。
 * attach/detach 走侧栏专用入口：JCEF 回调仅提供字符串、不含来源标识，
 * 通道身份须由本层显式提供，否则配置面板消息会串入侧栏。
 */
class JcefReviewPanel(project: Project) : Disposable {

    private val service = project.getService(ReviewProjectService::class.java)
    private val webview: OcrWebview?
    private var channel: WebviewChannel? = null

    val component: JComponent

    init {
        if (!JBCefApp.isSupported()) {
            webview = null
            component = jcefUnsupportedPlaceholder()
        } else {
            val view = OcrWebview(
                html = { bridge -> WebviewHtml.sidebar(service.currentLocale(), bridge) },
                onMessage = service::handleFromSidebar,
            )
            webview = view
            component = view.component
            val outbound = WebviewChannel { json -> view.post(json) }
            channel = outbound
            service.attachSidebar(outbound)
        }
    }

    override fun dispose() {
        channel?.let(service::detachSidebar)
        channel = null
        webview?.dispose()
    }
}

/**
 * JCEF 不可用时的兜底提示（此时插件所有 UI 均不可用）。[locale] 默认取 IDE 界面语言：
 * 进入此路径意味着 webview 无法启动，页面语言信息不可用。
 */
internal fun jcefUnsupportedPlaceholder(locale: SupportedLocale = currentIdeLocale()): JComponent =
    JPanel(BorderLayout()).apply {
        add(
            JBLabel("<html>" + HostStrings.t(locale, "ext.jcef.unsupportedPlaceholder") + "</html>"),
            BorderLayout.NORTH,
        )
    }
