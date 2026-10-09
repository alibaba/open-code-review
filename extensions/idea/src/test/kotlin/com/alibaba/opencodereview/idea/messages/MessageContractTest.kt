// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.messages

import com.alibaba.opencodereview.idea.FrontendSources
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Check consistency of frontend and host message discriminators (`type` literals).
 *
 * The frontend `frontend/src/shared/messages.ts` is the source of truth. Kotlin mirrors the contract through
 * inbound `WebviewToHost` `when` branch keys and the `@SerialName` annotations on outbound `HostToWebview`
 * and `ConfigPanelHostToWebview` variants. Both sides must have identical discriminator sets:
 *
 * a new frontend type missing from the host `when` becomes `WebviewToHost.Unknown` and is silently ignored;
 * an unknown host `@SerialName` has no frontend `onMessage` branch, so the UI silently fails to update.
 * Both are hard to detect manually because they produce no error; build-time checks expose this drift.
 *
 * Like [com.alibaba.opencodereview.idea.services.ProvidersTest], read frontend source with a regex and compare
 * the names with the host implementation. `ProvidersTest` uses the existing `presetProviderNames()` helper.
 * Keep the message discriminator sets here to avoid adding helpers to production code that has already
 * undergone manual regression testing. Update these sets when the corresponding implementation changes,
 * or the test will report the set difference.
 */
class MessageContractTest {

    /** Match union member discriminators of the form `type: 'xxx'` or `type: "xxx"`. */
    private val typeLiteralRegex = Regex("""\btype:\s*['"]([^'"]+)['"]""")

    // -------------------------------------------------------------- Hardcoded host sets
    //
    // The three sets mirror:
    //     inbound: `when` branch keys in WebviewMessages.parseWebviewMessage
    //                                         (Unknown / Malformed are fallbacks, not actual discriminators).
    //     outboundSidebar: @SerialName on each HostToWebview variant.
    //     outboundConfigPanel: @SerialName on each ConfigPanelHostToWebview variant.

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
    fun `inbound discriminators exactly match frontend WebviewToHost`() {
        val fromFrontend = typeLiteralsOf("WebviewToHost")
        assertEquals(
            "Inbound contract mismatch. Frontend-only: ${fromFrontend - inboundTypes}; host-only: ${inboundTypes - fromFrontend}",
            fromFrontend, inboundTypes,
        )
    }

    @Test
    fun `sidebar outbound discriminators exactly match frontend HostToWebview`() {
        val fromFrontend = typeLiteralsOf("HostToWebview")
        assertEquals(
            "Sidebar outbound contract mismatch. Frontend-only: ${fromFrontend - outboundSidebarTypes}; host-only: ${outboundSidebarTypes - fromFrontend}",
            fromFrontend, outboundSidebarTypes,
        )
    }

    @Test
    fun `configuration panel outbound discriminators exactly match frontend ConfigPanelHostToWebview`() {
        val fromFrontend = typeLiteralsOf("ConfigPanelHostToWebview")
        assertEquals(
            "Configuration panel outbound contract mismatch. Frontend-only: ${fromFrontend - outboundConfigPanelTypes}; host-only: ${outboundConfigPanelTypes - fromFrontend}",
            fromFrontend, outboundConfigPanelTypes,
        )
    }

    /**
     * Read the member discriminator set for [unionName] in `messages.ts`.
     *
     * Split at `export type <Name> =` and search only that section to avoid mixing the three unions.
     * Require `isNotEmpty` so upstream syntax changes cannot leave the regex matching nothing and silently passing.
     */
    private fun typeLiteralsOf(unionName: String): Set<String> {
        val ts = FrontendSources.file("src/shared/messages.ts").readText()
        val header = "export type $unionName ="
        val start = ts.indexOf(header)
            .also { check(it >= 0) { "Cannot find $header in messages.ts (has the frontend layout changed?)" } }
        val end = ts.indexOf("export type ", start + header.length).let { if (it < 0) ts.length else it }
        val body = ts.substring(start, end)
        val found = typeLiteralRegex.findAll(body).map { it.groupValues[1] }.toSortedSet()
        assertTrue(
            "No type literals matched in $unionName; upstream syntax changed and this test needs updating",
            found.isNotEmpty(),
        )
        return found
    }
}
