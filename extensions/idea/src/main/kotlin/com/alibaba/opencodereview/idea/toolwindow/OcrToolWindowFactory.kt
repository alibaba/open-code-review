package com.alibaba.opencodereview.idea.toolwindow

import com.alibaba.opencodereview.idea.jcef.JcefReviewPanel
import com.alibaba.opencodereview.idea.jcef.jcefUnsupportedPlaceholder
import com.intellij.openapi.Disposable
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
            jcefUnsupportedPlaceholder() to null
        }
        val content = ContentFactory.getInstance().createContent(panel, "", false)
        disposable?.let { Disposer.register(content, it) }
        toolWindow.contentManager.addContent(content)
    }
}
