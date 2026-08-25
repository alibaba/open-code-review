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
 * 侧栏的 JCEF 宿主：创建 webview、装配消息桥、投递宿主消息。消息分发交由路由层处理。
 * attach/detach 走侧栏专用入口：JCEF 回调仅提供字符串、不含来源标识，
 * 通道身份须由本层显式提供，否则配置面板消息会串入侧栏。
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
            // OcrWebview 构造或 attachSidebar 即便 isSupported 为真仍可能抛：
            // 工厂层会 catch 并显示占位，但半构造的 OcrWebview（已起 timer、连了消息总线）没人 dispose 会泄漏，故此处自清理后回退占位。
            val (vw, comp, ch) = try {
                val view = OcrWebview(
                    html = { bridge -> WebviewHtml.sidebar(service.currentLocale(), bridge) },
                    onMessage = service::handleFromSidebar,
                )
                val outbound = WebviewChannel { json -> view.post(json) }
                try {
                    val viewComponent = view.component // 取在 attach 之前，缩小 attach 之后仍可能抛错的窗口
                    service.attachSidebar(outbound)
                    Triple(view, viewComponent, outbound)
                } catch (e: Exception) {
                    // attach 前后都先尝试 detach（幂等）：channel 可能已被部分注册，不清理会泄漏且继续向已 dispose 的 view 投递。
                    runCatching { service.detachSidebar(outbound) }
                    runCatching { view.dispose() }
                    throw e
                }
            } catch (e: Exception) {
                // 只接 Exception：OOM/LinkageError 等 Error 不在此吞，交给工厂层 catch(Throwable) 统一兜底，避免掩盖致命问题。
                // ProcessCanceledException 是 IntelliJ 取消信号，绝不能吞（否则破坏取消机制）。
                if (e is ProcessCanceledException) throw e
                thisLogger().warn("[ocr] 侧栏 JCEF webview 初始化失败，回退占位", e)
                Triple(null, jcefUnsupportedPlaceholder(), null)
            }
            webview = vw
            component = comp
            channel = ch
            // OcrWebview 内部 messageBus.connect(this) 把自己挂到 Disposer 树（ROOT_DISPOSable 下）；
            // 必须注册为本面板的子节点，否则 IDE 关闭时 Disposer 找不到 parent → memory leak。
            vw?.let { Disposer.register(this, it) }
        }
    }

    override fun dispose() {
        // 先取局部再清空：channel 是 @Volatile 但读后置空非原子，捕获局部避免与可能的再 attach 竞态。
        val ch = channel
        channel = null
        // detach 抛也不应阻断 webview 释放（否则 JCEF browser 泄漏：timer、消息总线监听），故 runCatching。
        if (ch != null) runCatching { service.detachSidebar(ch) }
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
