package com.alibaba.opencodereview.idea.messages

import com.alibaba.opencodereview.idea.FrontendSources
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * 前端 ↔ 宿主 消息判别符（`type` 字面量）集合的一致性检查。
 *
 * 前端 `frontend/src/shared/messages.ts` 是消息契约的 single source of truth；宿主侧用 Kotlin 重新
 * 实现了同一份契约——入站 `WebviewToHost` 的 `when` 分支键、出站 `HostToWebview` /
 * `ConfigPanelHostToWebview` 各 variant 的 `@SerialName`。两端判别符集合必须一致：
 *
 * 前端新增一个消息类型而宿主 `when` 未跟进时，该消息落到 `WebviewToHost.Unknown` 被路由层静默忽略，
 * 不报错；宿主侧 `@SerialName` 发了前端不认识的类型时，前端 `onMessage` 无对应分支，UI 静默不更新。
 * 两种都是不报错的"静默降级"，靠人工观察几乎无法发现，只能靠构建期校验暴露。
 *
 * 与 [com.alibaba.opencodereview.idea.services.ProvidersTest] 同构：读前端源码做正则，与宿主侧硬编码
 * 集合做差集断言。为**不改动经过人工回归的生产代码**，宿主侧集合在此处硬编码，不在生产类上加任何
 * 函数；这一点与 `ProvidersTest` 的 `presetProviderNames()` 一致。改任一对应实现时必须同步改这里，
 * 否则测试以差集形式报错。
 */
class MessageContractTest {

    /** 匹配 union 成员里的 `type: 'xxx'` / `type: "xxx"` 判别符。 */
    private val typeLiteralRegex = Regex("""\btype:\s*['"]([^'"]+)['"]""")

    // -------------------------------------------------------------- 宿主侧硬编码集合
    //
    // 三组分别镜像：
    //   · inbound               —— WebviewMessages.parseWebviewMessage 的 `when` 分支键
    //                              （Unknown / Malformed 是兜底，不算真实判别符）。
    //   · outboundSidebar       —— HostToWebview 各 variant 的 @SerialName。
    //   · outboundConfigPanel   —— ConfigPanelHostToWebview 各 variant 的 @SerialName。

    private val inboundTypes: Set<String> = sortedSetOf(
        "ready", "readyConfigPanel", "closeConfigPanel", "cancelReview", "getConfig",
        "checkCli", "checkEnvironment", "installCli", "openConfigPanel", "getGitState",
        "getModeFiles", "openFileDiff", "startReview", "setConfig", "setConfigBatch",
        "testConnection", "deleteCustomProvider", "activateCustomProvider",
        "copyToClipboard", "jumpToComment", "commentAction",
    )

    private val outboundSidebarTypes: Set<String> = sortedSetOf(
        "init", "gitState", "modeFiles", "logLine", "stateChange", "reviewDone",
        "config", "commentSync",
    )

    private val outboundConfigPanelTypes: Set<String> = sortedSetOf(
        "configPanelInit", "configPanelFocus", "config", "connectionResult",
        "cliStatus", "environmentResult", "copyDone", "panelError", "installLog",
        "installDone",
    )

    @Test
    fun `入站消息判别符与前端 WebviewToHost 完全一致`() {
        val fromFrontend = typeLiteralsOf("WebviewToHost")
        assertEquals(
            "入站消息契约不一致。前端多出 ${fromFrontend - inboundTypes}，宿主多出 ${inboundTypes - fromFrontend}",
            fromFrontend, inboundTypes,
        )
    }

    @Test
    fun `侧栏出站消息判别符与前端 HostToWebview 完全一致`() {
        val fromFrontend = typeLiteralsOf("HostToWebview")
        assertEquals(
            "侧栏出站消息契约不一致。前端多出 ${fromFrontend - outboundSidebarTypes}，宿主多出 ${outboundSidebarTypes - fromFrontend}",
            fromFrontend, outboundSidebarTypes,
        )
    }

    @Test
    fun `配置面板出站消息判别符与前端 ConfigPanelHostToWebview 完全一致`() {
        val fromFrontend = typeLiteralsOf("ConfigPanelHostToWebview")
        assertEquals(
            "配置面板出站消息契约不一致。前端多出 ${fromFrontend - outboundConfigPanelTypes}，宿主多出 ${outboundConfigPanelTypes - fromFrontend}",
            fromFrontend, outboundConfigPanelTypes,
        )
    }

    /**
     * 取 [unionName] 在 `messages.ts` 里的成员判别符集合。
     *
     * 按 `export type <Name> =` 切段后仅在该段内正则，避免三个 union 的判别符互相串入。
     * 自检 `isNotEmpty` 防止上游改写法（例如不再用 `type: 'xxx'` 字面量）导致正则空转却默默通过。
     */
    private fun typeLiteralsOf(unionName: String): Set<String> {
        val ts = FrontendSources.file("src/shared/messages.ts").readText()
        val header = "export type $unionName ="
        val start = ts.indexOf(header)
            .also { check(it >= 0) { "messages.ts 里找不到 $header（前端目录结构变了？）" } }
        val end = ts.indexOf("export type ", start + header.length).let { if (it < 0) ts.length else it }
        val body = ts.substring(start, end)
        val found = typeLiteralRegex.findAll(body).map { it.groupValues[1] }.toSortedSet()
        assertTrue(
            "没能从 $unionName 里 regex 出任何 type 字面量，说明上游改了写法，本测试需要更新",
            found.isNotEmpty(),
        )
        return found
    }
}
