// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

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

class OcrToolWindowFactory : ToolWindowFactory, DumbAware {
    override fun createToolWindowContent(project: Project, toolWindow: ToolWindow) {
        // JCEF may not be on the classpath in some IDEA versions/runtimes (NoClassDefFoundError).
        // The try/catch ensures the tool window shows a placeholder instead of crashing.
        val (panel, disposable) = try {
            val p = JcefReviewPanel(project)
            p.component to (p as Disposable)
        } catch (e: Throwable) {
            // ProcessCanceledException is IntelliJ's cancellation signal and must never be swallowed
            // (that would break cancellation); anything else — including a missing JCEF such as
            // NoClassDefFoundError — falls back to the placeholder.
            if (e is ProcessCanceledException) throw e
            thisLogger().warn("[ocr] JCEF initialization failed, the tool window falls back to the placeholder", e)
            jcefUnsupportedPlaceholder() to null
        }
        val content = ContentFactory.getInstance().createContent(panel, "", false)
        disposable?.let { Disposer.register(content, it) }
        toolWindow.contentManager.addContent(content)
    }
}
