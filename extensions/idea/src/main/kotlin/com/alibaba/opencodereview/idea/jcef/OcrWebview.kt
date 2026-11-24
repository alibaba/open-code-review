// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.jcef

import com.intellij.ide.ui.LafManagerListener
import com.intellij.openapi.Disposable
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.diagnostic.thisLogger
import com.intellij.openapi.editor.colors.EditorColorsListener
import com.intellij.openapi.editor.colors.EditorColorsManager
import com.intellij.openapi.util.Disposer
import com.intellij.ui.JreHiDpiUtil
import com.intellij.ui.jcef.JBCefBrowser
import com.intellij.ui.jcef.JBCefBrowserBase
import com.intellij.ui.jcef.JBCefJSQuery
import kotlinx.serialization.builtins.serializer
import kotlinx.serialization.json.Json
import org.cef.handler.CefLoadHandlerAdapter
import java.awt.event.HierarchyEvent
import javax.swing.JComponent

/**
 * A JCEF page with a two-way message bridge. The sidebar and the config panel share this one
 * implementation.
 *
 * The bridge runs both ways: page → host through `window.__ocrPost(json)`, host → page through
 * `window.__ocrReceive(msg)`.
 */
internal class OcrWebview(
    html: (bridgeScript: String) -> String,
    private val onMessage: (String) -> Unit,
) : Disposable {

    private companion object {
        const val DEVTOOLS_PROPERTY = "ocr.devtools"
        val REPAINT_NUDGE_DELAYS_MS = listOf(150, 500, 1200)
    }

    private val browser = JBCefBrowser()
    // The create(JBCefBrowser) overload is a scheduled-for-removal API that the Plugin Verifier and
    // the marketplace review call out, so use the JBCefBrowserBase overload.
    private val query = JBCefJSQuery.create(browser as JBCefBrowserBase)

    @Volatile
    private var disposed = false

    private val repaintTimers = REPAINT_NUDGE_DELAYS_MS.map { delay ->
        javax.swing.Timer(delay) { forceFullRepaint() }.apply { isRepeats = false }
    }

    val component: JComponent get() = browser.component

    init {
        runCatching {
            thisLogger().info(
                "[ocr] browser=${browser.cefBrowser.javaClass.name} " +
                    "windowless=${browser.cefBrowser.isWindowless}",
            )
        }
        query.addHandler { raw ->
            runCatching { onMessage(raw) }
                .onFailure { thisLogger().warn("[ocr] Failed to handle page message", it) }
            null
        }

        val bus = ApplicationManager.getApplication().messageBus.connect(this)
        bus.subscribe(LafManagerListener.TOPIC, LafManagerListener { applyTheme() })
        bus.subscribe(EditorColorsManager.TOPIC, EditorColorsListener { applyTheme() })

        browser.jbCefClient.addLoadHandler(
            object : CefLoadHandlerAdapter() {
                override fun onLoadEnd(b: org.cef.browser.CefBrowser?, f: org.cef.browser.CefFrame?, code: Int) {
                    if (f?.isMain != false) {
                        applyTheme()
                        scheduleFullRepaint()
                    }
                }
            },
            browser.cefBrowser,
        )

        component.addHierarchyListener { e ->
            if (e.changeFlags and HierarchyEvent.SHOWING_CHANGED.toLong() == 0L) return@addHierarchyListener
            if (!component.isShowing) return@addHierarchyListener
            resyncPixelDensity()
            scheduleFullRepaint()
        }
        component.addPropertyChangeListener("graphicsConfiguration") { resyncPixelDensity() }
        component.addComponentListener(object : java.awt.event.ComponentAdapter() {
            override fun componentResized(e: java.awt.event.ComponentEvent?) = scheduleFullRepaint()
        })

        browser.loadHTML(html(query.inject("json")))
        if (System.getProperty(DEVTOOLS_PROPERTY)?.toBoolean() == true) {
            runCatching { browser.openDevtools() }
                .onFailure { thisLogger().warn("[ocr] Failed to open devtools", it) }
        }
    }

    private fun findOsrComponent(root: java.awt.Component): java.awt.Component? {
        if (root.javaClass.simpleName == "JBCefOsrComponent") return root
        if (root !is java.awt.Container) return null
        return root.components.asSequence().mapNotNull { findOsrComponent(it) }.firstOrNull()
    }

    private fun readField(target: Any, name: String): Any? = runCatching {
        generateSequence<Class<*>>(target.javaClass) { it.superclass }
            .mapNotNull { k -> k.declaredFields.firstOrNull { it.name == name } }
            .firstOrNull()
            ?.also { it.isAccessible = true }
            ?.get(target)
    }.getOrNull()

    /** Forces a full repaint after a resize, fixing the large white areas that appear after a
     *  resize in off-screen rendering mode. */
    private fun scheduleFullRepaint() {
        if (disposed) return
        repaintTimers.forEach { it.restart() }
    }

    /**
     * Triggers a full repaint from the page side. The root element's opacity gets a nudge that is
     * barely perceptible and is then reverted, which makes the compositor repaint the whole layer
     * without a reflow. Two nested rAFs keep the change and the revert in different frames.
     */
    private fun forceFullRepaint() {
        if (disposed) return
        val js = "(function(){var d=document.documentElement;if(!d)return;" +
            "d.style.opacity='0.999';" +
            "requestAnimationFrame(function(){requestAnimationFrame(function(){d.style.opacity='';});});" +
            "})();"
        runCatching { browser.cefBrowser.executeJavaScript(js, browser.cefBrowser.url, 0) }
            .onFailure { thisLogger().warn("[ocr] Failed to trigger full page repaint", it) }
    }

    /**
     * Syncs the OSR renderer's pixel density to the DPI of the screen the component is now on.
     * After a screen change a mismatch scales the picture by the wrong ratio. setScreenInfo updates
     * the cache first, then notifyScreenInfoChanged applies it.
     */
    private fun resyncPixelDensity() {
        if (!JreHiDpiUtil.isJreHiDPIEnabled()) return
        runCatching {
            val gcScale = component.graphicsConfiguration?.defaultTransform?.scaleX ?: return
            val osr = findOsrComponent(component) ?: return
            val handler = readField(osr, "myRenderHandler") ?: return
            val cached = readField(handler, "myPixelDensity") as? Double ?: return
            if (cached == gcScale) return
            val scaleFactor = readField(handler, "myScaleFactor") as? Double ?: 1.0
            // isAccessible = true is required: the JBCefOsrHandler class is package-private, so the
            // reflective call has to bypass access control.
            handler.javaClass
                .getMethod("setScreenInfo", Double::class.javaPrimitiveType, Double::class.javaPrimitiveType)
                .also { it.isAccessible = true }
                .invoke(handler, gcScale, scaleFactor)
            browser.cefBrowser.notifyScreenInfoChanged()
            thisLogger().info("[ocr] Corrected JCEF pixelDensity: $cached -> $gcScale")
        }.onFailure { thisLogger().warn("[ocr] Failed to correct JCEF pixelDensity, skipping", it) }
    }

    /** Recomputes the `--vscode-*` variables and replaces the theme style content. Colors must be
     *  read on the EDT, so everything is dispatched through invokeLater. */
    private fun applyTheme() {
        ApplicationManager.getApplication().invokeLater {
            if (disposed) return@invokeLater
            val literal = Json.encodeToString(String.serializer(), IdeaTheme.cssVariables())
            val js = "(function(){var s=document.getElementById('${WebviewHtml.THEME_STYLE_ID}');" +
                "if(s){s.textContent=$literal;}})();"
            runCatching { browser.cefBrowser.executeJavaScript(js, browser.cefBrowser.url, 0) }
                .onFailure { thisLogger().warn("[ocr] Failed to refresh theme variables", it) }
        }
    }

    /** Pushes one already-serialized host message into the page. */
    fun post(json: String) {
        // post is called from message-handling threads and dispose fires on the EDT, so the two run
        // concurrently and the browser may already be released. Like forceFullRepaint/applyTheme,
        // check the disposed guard first, and a failed executeJavaScript (browser destroyed) must
        // not take the channel down.
        if (disposed) return
        val literal = Json.encodeToString(String.serializer(), json)
        runCatching {
            browser.cefBrowser.executeJavaScript(
                "window.__ocrReceive && window.__ocrReceive(JSON.parse($literal));",
                browser.cefBrowser.url,
                0,
            )
        }.onFailure { thisLogger().warn("[ocr] Failed to post message to page", it) }
    }

    override fun dispose() {
        // Idempotent: JcefConfigPanelHost may call this once on the project-close path and once from
        // the dialog-close callback.
        if (disposed) return
        disposed = true
        repaintTimers.forEach { it.stop() }
        runCatching { query.dispose() }
        runCatching { Disposer.dispose(browser) }
    }
}
