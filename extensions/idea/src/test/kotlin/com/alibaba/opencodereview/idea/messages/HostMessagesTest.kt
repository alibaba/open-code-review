// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.messages

import com.alibaba.opencodereview.idea.model.CliResult
import com.alibaba.opencodereview.idea.model.CommentStatus
import com.alibaba.opencodereview.idea.model.CommentSyncState
import com.alibaba.opencodereview.idea.model.EnvCheckResult
import com.alibaba.opencodereview.idea.model.EnvToolStatus
import com.alibaba.opencodereview.idea.model.FileChange
import com.alibaba.opencodereview.idea.model.FileStatus
import com.alibaba.opencodereview.idea.model.GitState
import com.alibaba.opencodereview.idea.model.LogLevel
import com.alibaba.opencodereview.idea.model.LogLine
import com.alibaba.opencodereview.idea.model.OcrConfig
import com.alibaba.opencodereview.idea.model.ReviewComment
import com.alibaba.opencodereview.idea.model.ReviewMode
import com.alibaba.opencodereview.idea.model.ReviewState
import com.alibaba.opencodereview.idea.model.SupportedLocale
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.put
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * JSON shapes of outbound messages.
 *
 * These assertions catch silent blank-frontend failures:
 * misspelled fields, mismatched `type` literals, and enums serialized as uppercase Kotlin constants
 * neither fail compilation nor throw at runtime; the frontend simply cannot render the content.
 */
class HostMessagesTest {

    private fun parse(json: String) = Json.parseToJsonElement(json).jsonObject

    @Test
    fun `init includes config gitState and locale`() {
        val json = parse(
            HostToWebview.Init(
                config = OcrConfig(provider = "kimi", model = "k2"),
                gitState = GitState(currentBranch = "main"),
                locale = SupportedLocale.ZH_CN,
            ).toJson(),
        )
        assertEquals("init", json["type"].toString().trim('"'))
        assertTrue(json.containsKey("config"))
        assertTrue(json.containsKey("gitState"))
        // Without config, the frontend readiness check stays false and the UI remains on the configuration view.
        assertEquals("kimi", json["config"]!!.jsonObject["provider"].toString().trim('"'))
        assertEquals("main", json["gitState"]!!.jsonObject["currentBranch"].toString().trim('"'))
        assertEquals("zh-cn", json["locale"].toString().trim('"'))
    }

    @Test
    fun `locale serializes to literals recognized by the frontend`() {
        val en = parse(
            HostToWebview.Init(null, GitState(), SupportedLocale.EN).toJson(),
        )
        assertEquals("en", en["locale"].toString().trim('"'))
    }

    @Test
    fun `null config is sent explicitly rather than omitted`() {
        val json = parse(HostToWebview.Config(null).toJson())
        assertEquals("config", json["type"].toString().trim('"'))
        // With explicitNulls = true, send "config": null to match the frontend OcrConfig | null contract.
        assertTrue(json.containsKey("config"))
        assertTrue(json["config"] is JsonNull)
    }

    @Test
    fun `stateChange has a null error field when no error occurs`() {
        val ok = parse(HostToWebview.StateChange(ReviewState.RUNNING).toJson())
        assertEquals("stateChange", ok["type"].toString().trim('"'))
        assertEquals("running", ok["state"].toString().trim('"'))
        // With explicitNulls = true, send "error": null to match the frontend String | null contract.
        assertTrue(ok.containsKey("error"))
        assertTrue(ok["error"] is JsonNull)

        val failed = parse(HostToWebview.StateChange(ReviewState.FAILED, "炸了").toJson()) // allow-non-english: fixture verifies Unicode message serialization
        assertEquals("failed", failed["state"].toString().trim('"'))
        assertEquals("炸了", failed["error"].toString().trim('"')) // allow-non-english: fixture verifies Unicode message serialization
    }

    @Test
    fun `modeFiles uses lowercase mode and status literals`() {
        val json = parse(
            HostToWebview.ModeFiles(
                ReviewMode.BRANCH,
                listOf(FileChange("src/a.kt", FileStatus.RENAMED)),
            ).toJson(),
        )
        assertEquals("modeFiles", json["type"].toString().trim('"'))
        assertEquals("branch", json["mode"].toString().trim('"'))
        val first = json["files"].toString()
        assertTrue(first.contains("\"renamed\""))
        assertTrue(first.contains("\"src/a.kt\""))
    }

    @Test
    fun `logLine wraps the payload in a line object`() {
        val json = parse(HostToWebview.Log(LogLine("hello", LogLevel.ERROR)).toJson())
        assertEquals("logLine", json["type"].toString().trim('"'))
        assertEquals("hello", json["line"]!!.jsonObject["text"].toString().trim('"'))
        assertEquals("error", json["line"]!!.jsonObject["level"].toString().trim('"'))
    }

    @Test
    fun `reviewDone wraps the result and uses camelCase comment fields`() {
        val json = parse(
            HostToWebview.ReviewDone(
                CliResult(
                    status = "success",
                    comments = listOf(
                        ReviewComment(
                            path = "a.kt",
                            content = "改这里", // allow-non-english: fixture verifies Unicode message serialization
                            suggestionCode = "val x = 1",
                            existingCode = "var x = 1",
                            startLine = 10,
                            endLine = 12,
                        ),
                    ),
                ),
            ).toJson(),
        )
        assertEquals("reviewDone", json["type"].toString().trim('"'))
        val result = json["result"]!!.jsonObject.toString()
        // CLI output uses suggestion_code / existing_code / start_line; do not pass these names through to the frontend.
        assertTrue(result.contains("\"suggestionCode\""))
        assertTrue(result.contains("\"existingCode\""))
        assertTrue(result.contains("\"startLine\""))
        assertFalse(result.contains("suggestion_code"))
        assertFalse(result.contains("start_line"))
    }

    @Test
    fun `commentSync uses the falsePositive status literal`() {
        val json = parse(
            HostToWebview.CommentSync(
                listOf(
                    CommentSyncState(0, CommentStatus.FALSE_POSITIVE, jumpable = false),
                    CommentSyncState(1),
                ),
            ).toJson(),
        )
        assertEquals("commentSync", json["type"].toString().trim('"'))
        val body = json["comments"].toString()
        assertTrue(body.contains("\"falsePositive\""))
        assertFalse(body.contains("false_positive"))
        assertTrue(body.contains("\"pending\""))
        assertTrue(body.contains("\"jumpable\":false"))
    }

    @Test
    fun `the gitState message type is gitState rather than gitStateChanged`() {
        // The Kotlin class is GitStateChanged, but the message contract uses the literal gitState.
        val json = parse(HostToWebview.GitStateChanged(GitState()).toJson())
        assertEquals("gitState", json["type"].toString().trim('"'))
    }

    // ------------------------------------------------------------ Configuration panel

    @Test
    fun `configPanelInit includes focus env and skipEnvCheck`() {
        val focus = buildJsonObject { put("step", 2) }
        val json = parse(
            ConfigPanelHostToWebview.Init(
                config = OcrConfig(),
                focus = focus,
                env = EnvCheckResult(node = EnvToolStatus(ok = true, version = "v20.0.0")),
                skipEnvCheck = true,
                locale = SupportedLocale.EN,
            ).toJson(),
        )
        assertEquals("configPanelInit", json["type"].toString().trim('"'))
        assertEquals(2, json["focus"]!!.jsonObject["step"].toString().toInt())
        assertTrue(json["env"].toString().contains("\"v20.0.0\""))
        assertEquals("true", json["skipEnvCheck"].toString())
    }

    @Test
    fun `the remaining configPanel outbound message shapes match`() {
        assertEquals(
            "configPanelFocus",
            parse(ConfigPanelHostToWebview.Focus(null).toJson())["type"].toString().trim('"'),
        )
        val conn = parse(ConfigPanelHostToWebview.ConnectionResult(false, "401").toJson())
        assertEquals("connectionResult", conn["type"].toString().trim('"'))
        assertEquals("false", conn["ok"].toString())
        assertEquals("401", conn["message"].toString().trim('"'))

        // On success, send "message": null explicitly to match the frontend String | null contract.
        val okConn = parse(ConfigPanelHostToWebview.ConnectionResult(true).toJson())
        assertTrue(okConn.containsKey("message"))
        assertTrue(okConn["message"] is JsonNull)

        assertEquals(
            "environmentResult",
            parse(ConfigPanelHostToWebview.EnvironmentResult(EnvCheckResult()).toJson())["type"]
                .toString().trim('"'),
        )
        assertEquals(
            "cliStatus",
            parse(ConfigPanelHostToWebview.CliStatus(true).toJson())["type"].toString().trim('"'),
        )
        assertEquals(
            "panelError",
            parse(ConfigPanelHostToWebview.PanelError("x").toJson())["type"].toString().trim('"'),
        )
        assertEquals(
            "installLog",
            parse(ConfigPanelHostToWebview.InstallLog(LogLine("npm...")).toJson())["type"]
                .toString().trim('"'),
        )
        assertEquals(
            "installDone",
            parse(ConfigPanelHostToWebview.InstallDone(true).toJson())["type"].toString().trim('"'),
        )
    }

    @Test
    fun `copyDone is an empty message with only type`() {
        val json = parse(ConfigPanelHostToWebview.CopyDone.toJson())
        assertEquals("copyDone", json["type"].toString().trim('"'))
        assertEquals(1, json.size)
    }
}
