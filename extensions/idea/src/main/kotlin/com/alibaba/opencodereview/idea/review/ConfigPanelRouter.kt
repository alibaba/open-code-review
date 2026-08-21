package com.alibaba.opencodereview.idea.review

import com.alibaba.opencodereview.idea.messages.ConfigPanelHostToWebview
import com.alibaba.opencodereview.idea.messages.WebviewToHost
import com.alibaba.opencodereview.idea.model.HostStrings
import com.alibaba.opencodereview.idea.model.OcrConfig
import com.alibaba.opencodereview.idea.model.SupportedLocale
import com.alibaba.opencodereview.idea.services.CliService
import com.alibaba.opencodereview.idea.services.ConfigService
import com.alibaba.opencodereview.idea.services.isConfigReady
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.diagnostic.thisLogger
import com.intellij.openapi.ide.CopyPasteManager
import com.intellij.openapi.project.Project
import com.intellij.openapi.ui.MessageDialogBuilder
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.intOrNull
import java.awt.datatransfer.StringSelection
import java.util.concurrent.atomic.AtomicReference

/**
 * 配置面板的消息路由，按消息类型分派处理。
 *
 * 两个关键行为，变更前需确认：
 * 1. 处理逻辑都包在 try/catch 里，异常统一转 `panelError` 发回前端（前端有专门错误条渲染；只写日志用户看到的是"点了保存无响应"）。
 * 2. 每次写配置后 `notifyConfig`——既回 `config` 给面板，又推给侧栏，否则侧栏仍以旧配置判断能否开始审查。
 */
class ConfigPanelRouter(
    private val project: Project,
    private val cli: CliService,
    private val config: ConfigService,
    private val locale: () -> SupportedLocale,
    private val post: (ConfigPanelHostToWebview) -> Unit,
    /** 关闭面板窗口。 */
    private val closePanel: () -> Unit,
    /** 配置变化后通知侧栏。 */
    private val onConfigChanged: (OcrConfig?) -> Unit,
) {

    /** `open(focus)` 与 `readyConfigPanel` 之间暂存的 focus。 */
    private val pendingFocus = AtomicReference<JsonElement?>(null)

    fun setPendingFocus(focus: JsonElement?) {
        pendingFocus.set(focus)
    }

    fun takePendingFocus(): JsonElement? = pendingFocus.getAndSet(null)

    fun handle(msg: WebviewToHost) {
        // closeConfigPanel 只是关窗口，切到后台线程反而会与 dispose 竞争，直接同步处理。
        if (msg is WebviewToHost.CloseConfigPanel) {
            invokeOnEdt { closePanel() }
            return
        }
        background {
            try {
                handleMessage(msg)
            } catch (e: Exception) {
                thisLogger().warn("[ocr] Config panel message handling failed", e)
                post(ConfigPanelHostToWebview.PanelError(e.message ?: e.javaClass.simpleName))
            }
        }
    }

    private fun handleMessage(msg: WebviewToHost) {
        when (msg) {
            WebviewToHost.ReadyConfigPanel -> sendInit()

            is WebviewToHost.SetConfig -> {
                config.set(msg.key, msg.value)
                notifyConfig(config.read())
            }

            is WebviewToHost.SetConfigBatch -> {
                config.setMany(msg.entries)
                notifyConfig(config.read())
            }

            is WebviewToHost.TestConnection -> {
                val (ok, message) = config.testWithEntries(msg.entries)
                post(ConfigPanelHostToWebview.ConnectionResult(ok, message))
            }

            is WebviewToHost.DeleteCustomProvider -> {
                // 删除配置不可撤销，需以模态确认框拦截一次。
                if (!confirmDelete(msg.name)) return
                notifyConfig(config.deleteCustomProvider(msg.name))
            }

            is WebviewToHost.ActivateCustomProvider -> {
                config.set("provider", msg.name)
                notifyConfig(config.read())
            }

            // checkCli 和 checkEnvironment 合到同一分支，均为强制重新探测。
            WebviewToHost.CheckCli, WebviewToHost.CheckEnvironment ->
                post(ConfigPanelHostToWebview.EnvironmentResult(cli.checkEnvironment(force = true)))

            is WebviewToHost.CopyToClipboard -> {
                // 写剪贴板与回执都在 EDT 内完成：否则 post(CopyDone) 在写盘前就到前端，
                // 用户看到"已复制"立即切走应用去粘贴时，剪贴板里可能还没有内容。
                invokeOnEdt {
                    CopyPasteManager.getInstance().setContents(StringSelection(msg.text))
                    post(ConfigPanelHostToWebview.CopyDone)
                }
            }

            WebviewToHost.InstallCli -> {
                val ok = cli.install { line -> post(ConfigPanelHostToWebview.InstallLog(line)) }
                post(ConfigPanelHostToWebview.InstallDone(ok))
                // 安装成功时已清环境缓存，此处不带 force 也会重新探测一次。
                post(ConfigPanelHostToWebview.EnvironmentResult(cli.checkEnvironment()))
            }

            is WebviewToHost.Malformed -> post(ConfigPanelHostToWebview.PanelError(msg.reason))

            // 侧栏的消息不应经此通道投递；无法识别的类型可能来自更新的前端，忽略即可。
            else -> thisLogger().debug("[ocr] Config panel ignoring message: $msg")
        }
    }

    /**
     * `readyConfigPanel` 的响应。`env` 刻意只取缓存而不主动探测：
     * 面板需要时会自行发 `checkEnvironment`，此处同步探测会把首屏阻塞数秒。
     */
    private fun sendInit() {
        val focus = takePendingFocus()
        val current = config.read()
        post(
            ConfigPanelHostToWebview.Init(
                config = current,
                focus = focus,
                env = cli.getCachedEnvironment(),
                // 直接跳到第 2 步、或者配置本就齐备，则无需再走环境检查引导。
                skipEnvCheck = focus.step() == 2 || isConfigReady(current),
                locale = locale(),
            ),
        )
    }

    private fun notifyConfig(updated: OcrConfig?) {
        post(ConfigPanelHostToWebview.Config(updated))
        onConfigChanged(updated)
    }

    private fun confirmDelete(name: String): Boolean {
        // 在 pooled 线程上走到这里时项目可能已被关闭：对已释放 project 调 invokeAndWait 弹模态框可能死锁。
        if (project.isDisposed) return false
        var confirmed = false
        val loc = locale()
        ApplicationManager.getApplication().invokeAndWait {
            if (project.isDisposed) return@invokeAndWait
            confirmed = MessageDialogBuilder
                .yesNo(
                    HostStrings.t(loc, "ext.deleteProviderTitle"),
                    HostStrings.t(loc, "ext.deleteProviderConfirm", "name" to name),
                )
                .yesText(HostStrings.t(loc, "ext.deleteProviderConfirmBtn"))
                .noText(HostStrings.t(loc, "ext.common.cancel"))
                .ask(project)
        }
        return confirmed
    }

    private fun invokeOnEdt(block: () -> Unit) {
        ApplicationManager.getApplication().invokeLater {
            if (!project.isDisposed) runCatching(block).onFailure {
                thisLogger().warn("[ocr] Config panel UI operation failed", it)
            }
        }
    }

    private fun background(block: () -> Unit) {
        ApplicationManager.getApplication().executeOnPooledThread {
            if (!project.isDisposed) block()
        }
    }
}

/** 读取 `ConfigPanelFocus.step`。宿主不解释 focus 的其余字段，只需要这一个数字。 */
private fun JsonElement?.step(): Int? =
    ((this as? JsonObject)?.get("step") as? JsonPrimitive)?.intOrNull
