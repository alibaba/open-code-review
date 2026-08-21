package com.alibaba.opencodereview.idea.toolwindow

import com.alibaba.opencodereview.idea.jcef.JcefReviewPanel
import com.alibaba.opencodereview.idea.jcef.jcefUnsupportedPlaceholder
import com.intellij.openapi.Disposable
import com.intellij.openapi.diagnostic.thisLogger
import com.intellij.openapi.progress.ProcessCanceledException
import com.intellij.openapi.project.DumbAware
import com.intellij.openapi.project.Project
import com.intellij.openapi.util.Disposer
import com.intellij.openapi.wm.ToolWindow
import com.intellij.openapi.wm.ToolWindowFactory
import com.intellij.ui.content.ContentFactory
import javax.swing.JComponent

class OcrToolWindowFactory : ToolWindowFactory, DumbAware {
    override fun createToolWindowContent(project: Project, toolWindow: ToolWindow) {
        // JCEF may not be on the classpath in some IDEA versions/runtimes (NoClassDefFoundError).
        // The try/catch ensures the tool window shows a placeholder instead of crashing.
        val (panel, disposable) = try {
            val p = JcefReviewPanel(project)
            p.component to (p as Disposable)
        } catch (e: Throwable) {
            // ProcessCanceledException 是 IntelliJ 取消信号，绝不能吞（否则破坏取消机制）；其余（含 NoClassDefFoundError 等 JCEF 缺失）走占位。
            if (e is ProcessCanceledException) throw e
            thisLogger().warn("[ocr] JCEF 初始化失败，工具窗走占位", e)
            jcefUnsupportedPlaceholder() to null
        }
        val content = ContentFactory.getInstance().createContent(panel, "", false)
        disposable?.let { Disposer.register(content, it) }
        toolWindow.contentManager.addContent(content)
    }
}
