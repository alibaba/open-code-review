// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

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
 * Message routing for the config panel; dispatches by message type.
 *
 * Two key behaviours to confirm before changing:
 * 1. Every handler wraps its work in try/catch and turns the exception into a `panelError` sent back
 *    to the frontend (which renders a dedicated error bar; logging alone leaves the user seeing
 *    "I clicked save and nothing happened").
 * 2. Every config write is followed by `notifyConfig` — returning `config` to the panel and pushing
 *    it to the sidebar, which would otherwise keep judging readiness from the old config.
 */
class ConfigPanelRouter(
    private val project: Project,
    private val cli: CliService,
    private val config: ConfigService,
    private val locale: () -> SupportedLocale,
    private val post: (ConfigPanelHostToWebview) -> Unit,
    /** Closes the panel window. */
    private val closePanel: () -> Unit,
    /** Notifies the sidebar after a config change. */
    private val onConfigChanged: (OcrConfig?) -> Unit,
) {

    /** The focus held between `open(focus)` and `readyConfigPanel`. */
    private val pendingFocus = AtomicReference<JsonElement?>(null)

    fun setPendingFocus(focus: JsonElement?) {
        pendingFocus.set(focus)
    }

    fun takePendingFocus(): JsonElement? = pendingFocus.getAndSet(null)

    fun handle(msg: WebviewToHost) {
        // closeConfigPanel only closes the window; moving it to a background thread would race with
        // dispose, so handle it synchronously.
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
                // Deleting a config cannot be undone, so a modal confirmation intercepts it first.
                if (!confirmDelete(msg.name)) return
                notifyConfig(config.deleteCustomProvider(msg.name))
            }

            is WebviewToHost.ActivateCustomProvider -> {
                config.set("provider", msg.name)
                notifyConfig(config.read())
            }

            // checkCli and checkEnvironment share a branch: both force a fresh probe.
            WebviewToHost.CheckCli, WebviewToHost.CheckEnvironment ->
                post(ConfigPanelHostToWebview.EnvironmentResult(cli.checkEnvironment(force = true)))

            is WebviewToHost.CopyToClipboard -> {
                // The clipboard write and the acknowledgement both happen on the EDT: otherwise
                // post(CopyDone) reaches the frontend before the write, and a user who sees "copied",
                // switches away and pastes may find nothing on the clipboard.
                invokeOnEdt {
                    CopyPasteManager.getInstance().setContents(StringSelection(msg.text))
                    post(ConfigPanelHostToWebview.CopyDone)
                }
            }

            WebviewToHost.InstallCli -> {
                val ok = cli.install { line -> post(ConfigPanelHostToWebview.InstallLog(line)) }
                post(ConfigPanelHostToWebview.InstallDone(ok))
                // A successful install clears the environment cache, so this re-probes even without
                // force.
                post(ConfigPanelHostToWebview.EnvironmentResult(cli.checkEnvironment()))
            }

            is WebviewToHost.Malformed -> post(ConfigPanelHostToWebview.PanelError(msg.reason))

            // Sidebar messages should not be delivered through this channel; an unrecognised type
            // may come from a newer frontend, so ignoring it is enough.
            else -> thisLogger().debug("[ocr] Config panel ignoring message: $msg")
        }
    }

    /**
     * Response to `readyConfigPanel`. `env` deliberately comes from the cache rather than a fresh
     * probe: the panel sends `checkEnvironment` itself when it needs one, and probing here would
     * block the first paint for seconds.
     */
    private fun sendInit() {
        val focus = takePendingFocus()
        val current = config.read()
        post(
            ConfigPanelHostToWebview.Init(
                config = current,
                focus = focus,
                env = cli.getCachedEnvironment(),
                // Jumping straight to step 2, or an already complete config, means the environment
                // check walkthrough is not needed.
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
        // The project may already be closed by the time a pooled thread reaches here: invokeAndWait
        // for a modal dialog on a disposed project can deadlock.
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

/** Reads `ConfigPanelFocus.step`. The host does not interpret the rest of the focus, only this
 *  number. */
private fun JsonElement?.step(): Int? =
    ((this as? JsonObject)?.get("step") as? JsonPrimitive)?.intOrNull
