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
import com.intellij.ui.jcef.JBCefJSQuery
import kotlinx.serialization.builtins.serializer
import kotlinx.serialization.json.Json
import org.cef.handler.CefLoadHandlerAdapter
import java.awt.event.HierarchyEvent
import javax.swing.JComponent

/**
 * 装配双向消息桥的 JCEF 页面。侧栏与配置面板共用同一实现。
 *
 * 消息桥两个方向：页面→宿主 `window.__ocrPost(json)`；宿主→页面 `window.__ocrReceive(msg)`。
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
    private val query = JBCefJSQuery.create(browser)

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

    /** resize 后强制整屏重绘，修复离屏渲染模式下 resize 后面板大面积变白。 */
    private fun scheduleFullRepaint() {
        if (disposed) return
        repaintTimers.forEach { it.restart() }
    }

    /**
     * 从页面端触发整屏重绘。给根元素 opacity 施加近乎无感的扰动再撤销，
     * 使合成器整块重绘但不触发重排。两层 rAF 确保修改与撤销落入不同帧。
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
     * 同步 OSR 渲染器的像素密度到组件当前屏幕的 DPI。
     * 切屏后若不一致，画面会按错误比例缩放。先 setScreenInfo 更新缓存再 notifyScreenInfoChanged。
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
            // 必须设置 isAccessible = true：JBCefOsrHandler 类为包私有，反射调用需绕过访问控制。
            handler.javaClass
                .getMethod("setScreenInfo", Double::class.javaPrimitiveType, Double::class.javaPrimitiveType)
                .also { it.isAccessible = true }
                .invoke(handler, gcScale, scaleFactor)
            browser.cefBrowser.notifyScreenInfoChanged()
            thisLogger().info("[ocr] Corrected JCEF pixelDensity: $cached -> $gcScale")
        }.onFailure { thisLogger().warn("[ocr] Failed to correct JCEF pixelDensity, skipping", it) }
    }

    /** 重算 `--vscode-*` 并替换主题 style 内容。必须 EDT 取色，统一 invokeLater 调度。 */
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

    /** 把一条已序列化好的宿主消息推进页面。 */
    fun post(json: String) {
        // post 由消息处理线程调用、dispose 在 EDT 触发，二者并发：browser 可能已释放。
        // 与 forceFullRepaint/applyTheme 一致，先看 disposed 守卫；executeJavaScript 失败（browser 已销毁）也不让通道崩溃。
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
        // 幂等：JcefConfigPanelHost 在项目关闭路径与对话框关闭回调都可能各调一次。
        if (disposed) return
        disposed = true
        repaintTimers.forEach { it.stop() }
        runCatching { query.dispose() }
        runCatching { Disposer.dispose(browser) }
    }
}
