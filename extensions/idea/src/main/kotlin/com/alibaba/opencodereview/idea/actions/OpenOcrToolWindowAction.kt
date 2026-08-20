package com.alibaba.opencodereview.idea.actions

import com.intellij.openapi.actionSystem.ActionUpdateThread
import com.intellij.openapi.actionSystem.AnAction
import com.intellij.openapi.actionSystem.AnActionEvent
import com.intellij.openapi.wm.ToolWindowManager

class OpenOcrToolWindowAction : AnAction() {
    override fun getActionUpdateThread() = ActionUpdateThread.BGT
    override fun update(event: AnActionEvent) {
        event.presentation.isEnabledAndVisible = event.project != null
    }
    override fun actionPerformed(event: AnActionEvent) {
        val project = event.project ?: return
        ToolWindowManager.getInstance(project).getToolWindow("Open Code Review")?.activate(null)
    }
}
