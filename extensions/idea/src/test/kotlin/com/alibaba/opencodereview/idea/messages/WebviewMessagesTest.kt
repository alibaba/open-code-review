// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.messages

import com.alibaba.opencodereview.idea.model.FileStatus
import com.alibaba.opencodereview.idea.model.ReviewMode
import com.alibaba.opencodereview.idea.model.SupportedLocale
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Inbound message contract tests. This layer has no IDE API dependencies, so all 21 types can be tested directly.
 *
 * The key requirement is that parsing failures never throw: a newer frontend version or a temporarily missing
 * field must not interrupt the entire message channel.
 */
class WebviewMessagesTest {

    /** Locale affects only Malformed messages; most tests do not care, so use Chinese consistently. */
    private fun parse(raw: String): WebviewToHost = parseWebviewMessage(raw, SupportedLocale.ZH_CN)

    // ------------------------------------------------------------ Parameterless types

    @Test
    fun `each parameterless message type is recognized`() {
        val cases = mapOf(
            "ready" to WebviewToHost.Ready,
            "readyConfigPanel" to WebviewToHost.ReadyConfigPanel,
            "closeConfigPanel" to WebviewToHost.CloseConfigPanel,
            "cancelReview" to WebviewToHost.CancelReview,
            "getConfig" to WebviewToHost.GetConfig,
            "checkCli" to WebviewToHost.CheckCli,
            "checkEnvironment" to WebviewToHost.CheckEnvironment,
            "installCli" to WebviewToHost.InstallCli,
        )
        cases.forEach { (type, expected) ->
            assertEquals(type, expected, parse("""{"type":"$type"}"""))
        }
    }

    // ------------------------------------------------------------ Sidebar

    @Test
    fun `getGitState parses the mode`() {
        assertEquals(
            WebviewToHost.GetGitState(ReviewMode.BRANCH),
            parse("""{"type":"getGitState","mode":"branch"}"""),
        )
        assertEquals(
            WebviewToHost.GetGitState(ReviewMode.COMMIT),
            parse("""{"type":"getGitState","mode":"commit"}"""),
        )
    }

    @Test
    fun `unknown modes fall back to workspace`() {
        assertEquals(
            WebviewToHost.GetGitState(ReviewMode.WORKSPACE),
            parse("""{"type":"getGitState","mode":"nonsense"}"""),
        )
        assertEquals(
            WebviewToHost.GetGitState(ReviewMode.WORKSPACE),
            parse("""{"type":"getGitState"}"""),
        )
    }

    @Test
    fun `getModeFiles includes both branch endpoints`() {
        val msg = parse(
            """{"type":"getModeFiles","mode":"branch","from":"main","to":"dev"}""",
        )
        assertEquals(WebviewToHost.GetModeFiles(ReviewMode.BRANCH, "main", "dev", null), msg)
    }

    @Test
    fun `empty getModeFiles strings are treated as missing`() {
        // The form sends an empty string for an unselected branch; passing it to git would produce an invalid ref.
        val msg = parse(
            """{"type":"getModeFiles","mode":"branch","from":"","to":"  "}""",
        ) as WebviewToHost.GetModeFiles
        assertNull(msg.from)
        assertNull(msg.to)
    }

    @Test
    fun `openFileDiff parses all fields`() {
        val msg = parse(
            """{"type":"openFileDiff","path":"src/a.kt","status":"deleted","mode":"commit","commit":"abc1234"}""",
        )
        assertEquals(
            WebviewToHost.OpenFileDiff("src/a.kt", FileStatus.DELETED, ReviewMode.COMMIT, null, null, "abc1234"),
            msg,
        )
    }

    @Test
    fun `unknown openFileDiff status falls back to modified`() {
        val msg = parse(
            """{"type":"openFileDiff","path":"a.kt","status":"copied","mode":"workspace"}""",
        ) as WebviewToHost.OpenFileDiff
        assertEquals(FileStatus.MODIFIED, msg.status)
    }

    @Test
    fun `openFileDiff without path is Malformed`() {
        val msg = parse("""{"type":"openFileDiff","status":"added","mode":"workspace"}""")
        assertTrue(msg is WebviewToHost.Malformed)
    }

    @Test
    fun `startReview parses review options`() {
        val msg = parse(
            """{"type":"startReview","options":{"mode":"branch","from":"main","to":"dev","concurrency":4}}""",
        ) as WebviewToHost.StartReview
        assertEquals(ReviewMode.BRANCH, msg.options.mode)
        assertEquals("main", msg.options.from)
        assertEquals("dev", msg.options.to)
        assertEquals(4, msg.options.concurrency)
        assertNull(msg.options.customPrompt)
    }

    @Test
    fun `startReview without options is Malformed`() {
        assertTrue(parse("""{"type":"startReview"}""") is WebviewToHost.Malformed)
    }

    @Test
    fun `startReview with the wrong options type is Malformed instead of throwing`() {
        assertTrue(
            parse("""{"type":"startReview","options":"workspace"}""") is WebviewToHost.Malformed,
        )
        // Invalid field types that fail deserialization must also degrade gracefully without escaping the parser.
        assertTrue(
            parse("""{"type":"startReview","options":{"concurrency":"many"}}""")
                is WebviewToHost.Malformed,
        )
    }

    @Test
    fun `jumpToComment and commentAction parse correctly`() {
        assertEquals(
            WebviewToHost.JumpToComment(3),
            parse("""{"type":"jumpToComment","index":3}"""),
        )
        assertEquals(
            WebviewToHost.CommentAction(0, CommentActionKind.APPLY),
            parse("""{"type":"commentAction","index":0,"action":"apply"}"""),
        )
        assertEquals(
            WebviewToHost.CommentAction(1, CommentActionKind.DISCARD),
            parse("""{"type":"commentAction","index":1,"action":"discard"}"""),
        )
        assertEquals(
            WebviewToHost.CommentAction(2, CommentActionKind.FALSE_POSITIVE),
            parse("""{"type":"commentAction","index":2,"action":"falsePositive"}"""),
        )
    }

    @Test
    fun `missing or non-numeric index is Malformed`() {
        assertTrue(parse("""{"type":"jumpToComment"}""") is WebviewToHost.Malformed)
        assertTrue(parse("""{"type":"jumpToComment","index":"abc"}""") is WebviewToHost.Malformed)
        assertTrue(parse("""{"type":"jumpToComment","index":null}""") is WebviewToHost.Malformed)
        assertTrue(
            parse("""{"type":"commentAction","index":1,"action":"nope"}""")
                is WebviewToHost.Malformed,
        )
    }

    @Test
    fun `numeric string index is also accepted`() {
        // Match JavaScript, where `comments["3"]` also returns the fourth element.
        // Keep parsing permissive rather than generating Malformed for a shape the frontend does not send.
        assertEquals(
            WebviewToHost.JumpToComment(3),
            parse("""{"type":"jumpToComment","index":"3"}"""),
        )
    }

    @Test
    fun `openConfigPanel forwards focus unchanged`() {
        val msg = parse(
            """{"type":"openConfigPanel","focus":{"step":2,"tab":"custom"}}""",
        ) as WebviewToHost.OpenConfigPanel
        val focus = msg.focus as JsonObject
        assertEquals(JsonPrimitive(2), focus["step"])
        assertEquals(JsonPrimitive("custom"), focus["tab"])
    }

    @Test
    fun `openConfigPanel allows missing focus`() {
        val msg = parse("""{"type":"openConfigPanel"}""") as WebviewToHost.OpenConfigPanel
        assertNull(msg.focus)
    }

    // ------------------------------------------------------------ Configuration panel

    @Test
    fun `setConfig allows empty values`() {
        // Clearing a field sends an empty string, which must not be treated as a missing field.
        assertEquals(
            WebviewToHost.SetConfig("llm.url", ""),
            parse("""{"type":"setConfig","key":"llm.url","value":""}"""),
        )
    }

    @Test
    fun `setConfig without key is Malformed`() {
        assertTrue(parse("""{"type":"setConfig","value":"x"}""") is WebviewToHost.Malformed)
    }

    @Test
    fun `setConfigBatch and testConnection parse entries`() {
        val json = """{"type":"%s","entries":[{"key":"provider","value":"kimi"},{"key":"model","value":""}]}"""
        val batch = parse(json.format("setConfigBatch")) as WebviewToHost.SetConfigBatch
        assertEquals(2, batch.entries.size)
        assertEquals("provider", batch.entries[0].key)
        assertEquals("kimi", batch.entries[0].value)
        assertEquals("", batch.entries[1].value)

        val test = parse(json.format("testConnection")) as WebviewToHost.TestConnection
        assertEquals(batch.entries, test.entries)
    }

    @Test
    fun `invalid entries are discarded without failing the whole message`() {
        val msg = parse(
            """{"type":"setConfigBatch","entries":[{"value":"没有key"},"字符串",{"key":"model","value":"m"}]}""", // allow-non-english: fixture verifies malformed Unicode message handling
        ) as WebviewToHost.SetConfigBatch
        assertEquals(1, msg.entries.size)
        assertEquals("model", msg.entries[0].key)
    }

    @Test
    fun `missing entries become an empty list`() {
        val msg = parse("""{"type":"setConfigBatch"}""") as WebviewToHost.SetConfigBatch
        assertTrue(msg.entries.isEmpty())
    }

    @Test
    fun `custom provider deletion and activation parse correctly`() {
        assertEquals(
            WebviewToHost.DeleteCustomProvider("my-llm"),
            parse("""{"type":"deleteCustomProvider","name":"my-llm"}"""),
        )
        assertEquals(
            WebviewToHost.ActivateCustomProvider("my-llm"),
            parse("""{"type":"activateCustomProvider","name":"my-llm"}"""),
        )
        assertTrue(parse("""{"type":"deleteCustomProvider"}""") is WebviewToHost.Malformed)
        assertTrue(
            parse("""{"type":"activateCustomProvider","name":" "}""") is WebviewToHost.Malformed,
        )
    }

    @Test
    fun `copyToClipboard allows empty text`() {
        assertEquals(
            WebviewToHost.CopyToClipboard(""),
            parse("""{"type":"copyToClipboard"}"""),
        )
        assertEquals(
            WebviewToHost.CopyToClipboard("sk-xxx"),
            parse("""{"type":"copyToClipboard","text":"sk-xxx"}"""),
        )
    }

    // ------------------------------------------------------------ Fallbacks

    @Test
    fun `unrecognized types are Unknown rather than Malformed`() {
        // A newer frontend may send additional types; this is normal and should not show a user-facing error.
        assertEquals(
            WebviewToHost.Unknown("somethingNew"),
            parse("""{"type":"somethingNew","x":1}"""),
        )
    }

    @Test
    fun `invalid JSON and missing type are Malformed without throwing`() {
        assertTrue(parse("{不是json") is WebviewToHost.Malformed) // allow-non-english: fixture verifies malformed Unicode message handling
        assertTrue(parse("") is WebviewToHost.Malformed)
        assertTrue(parse("[1,2,3]") is WebviewToHost.Malformed)
        assertTrue(parse("""{"foo":"bar"}""") is WebviewToHost.Malformed)
        // A non-string type must also degrade gracefully.
        assertTrue(parse("""{"type":42}""") is WebviewToHost.Malformed)
    }

    @Test
    fun `Malformed messages follow the locale`() {
        // The reason is displayed unchanged by the frontend, so use the IDE language rather than hardcoding Chinese.
        val raw = """{"type":"setConfig","value":"x"}"""
        assertEquals(
            "setConfig 缺少 key", // allow-non-english: assertion verifies Chinese UI translations
            (parseWebviewMessage(raw, SupportedLocale.ZH_CN) as WebviewToHost.Malformed).reason,
        )
        assertEquals(
            "setConfig is missing required field: key",
            (parseWebviewMessage(raw, SupportedLocale.EN) as WebviewToHost.Malformed).reason,
        )
    }
}
