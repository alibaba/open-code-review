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
 * The config panel's message router, dispatching by message type.
 *
 * Two key behaviors to confirm before changing:
 * 1. All handling is wrapped in try/catch and exceptions are uniformly converted to `panelError` sent back to the
 *    frontend (the frontend has a dedicated error bar; logging alone leaves the user seeing "clicked save, nothing happened").
 * 2. Every config write calls `notifyConfig` -- replying `config` to the panel AND pushing to the sidebar;
 *    otherwise the sidebar keeps judging review-readiness from stale config.
 */
class ConfigPanelRouter(
    private val project: Project,
    private val cli: CliService,
    private val config: ConfigService,
    private val locale: () -> SupportedLocale,
    private val post: (ConfigPanelHostToWebview) -> Unit,
    /** Closes the panel window. */
    private val closePanel: () -> Unit,
    /** Notifies the sidebar after the configuration changes. */
    private val onConfigChanged: (OcrConfig?) -> Unit,
) {

    /** The focus stashed between `open(focus)` and `readyConfigPanel`. */
    private val pendingFocus = AtomicReference<JsonElement?>(null)

    fun setPendingFocus(focus: JsonElement?) {
        pendingFocus.set(focus)
    }

    fun takePendingFocus(): JsonElement? = pendingFocus.getAndSet(null)

    fun handle(msg: WebviewToHost) {
        // closeConfigPanel only closes the window; switching to a background thread would race dispose, so handle it synchronously.
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
                // Deleting a provider is irreversible; gate it behind a modal confirmation once.
                if (!confirmDelete(msg.name)) return
                notifyConfig(config.deleteCustomProvider(msg.name))
            }

            is WebviewToHost.ActivateCustomProvider -> {
                config.set("provider", msg.name)
                notifyConfig(config.read())
            }

            // checkCli and checkEnvironment share one branch; both force a re-probe.
            WebviewToHost.CheckCli, WebviewToHost.CheckEnvironment ->
                post(ConfigPanelHostToWebview.EnvironmentResult(cli.checkEnvironment(force = true)))

            is WebviewToHost.CopyToClipboard -> {
                // Both the clipboard write and the receipt happen inside the EDT: otherwise post(CopyDone) reaches
                // the frontend before the clipboard is written, and a user who switches apps to paste on seeing
                // "copied" may find the clipboard still empty.
                invokeOnEdt {
                    CopyPasteManager.getInstance().setContents(StringSelection(msg.text))
                    post(ConfigPanelHostToWebview.CopyDone)
                }
            }

            WebviewToHost.InstallCli -> {
                val ok = cli.install { line -> post(ConfigPanelHostToWebview.InstallLog(line)) }
                post(ConfigPanelHostToWebview.InstallDone(ok))
                // A successful install already cleared the environment cache, so this re-probes even without force.
                post(ConfigPanelHostToWebview.EnvironmentResult(cli.checkEnvironment()))
            }

            is WebviewToHost.Malformed -> post(ConfigPanelHostToWebview.PanelError(msg.reason))

            // Sidebar messages must not be delivered through this channel; unrecognized types may come from a newer frontend -- ignore them.
            else -> thisLogger().debug("[ocr] Config panel ignoring message: $msg")
        }
    }

    /**
     * The response to `readyConfigPanel`. `env` deliberately reads the cache only, never probing:
     * the panel sends `checkEnvironment` itself when it needs fresh data, and a synchronous probe here would
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
                // Jumping straight to step 2, or an already-complete config, means no env-check onboarding is needed.
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
        // By the time a pooled thread gets here the project may already be closed: calling invokeAndWait for a modal
        // dialog on a disposed project can deadlock.
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

/** Reads `ConfigPanelFocus.step`. The host does not interpret the rest of focus; this one number is all it needs. */
private fun JsonElement?.step(): Int? =
    ((this as? JsonObject)?.get("step") as? JsonPrimitive)?.intOrNull
