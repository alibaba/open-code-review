// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.messages

import com.alibaba.opencodereview.idea.model.CliResult
import com.alibaba.opencodereview.idea.model.CommentSyncState
import com.alibaba.opencodereview.idea.model.EnvCheckResult
import com.alibaba.opencodereview.idea.model.FileChange
import com.alibaba.opencodereview.idea.model.GitState
import com.alibaba.opencodereview.idea.model.LogLine
import com.alibaba.opencodereview.idea.model.OcrConfig
import com.alibaba.opencodereview.idea.model.ReviewMode
import com.alibaba.opencodereview.idea.model.ReviewState
import com.alibaba.opencodereview.idea.model.SupportedLocale
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement

/**
 * Outbound messages: `HostToWebview` (8) for the sidebar channel and `ConfigPanelHostToWebview`
 * (10) for the config panel channel.
 *
 * Sealed classes with `classDiscriminator = "type"` rather than a hand-built `buildJsonObject`:
 * a hand-built object with a missing field or a snake_case name compiles fine and makes the
 * frontend silently render a blank card, while the sealed class turns a missing field into a
 * compile-time error.
 */

/**
 * Outbound JSON encoder. `explicitNulls = true` writes null fields explicitly as `"field": null`,
 * matching the contract of the frontend TypeScript declarations (`field: Type | null`, not
 * optional): the frontend expects the field to always be present.
 */
val HostJson: Json = Json {
    classDiscriminator = "type"
    encodeDefaults = true
    explicitNulls = true
}

@Serializable
sealed class HostToWebview {

    /** The first message after the frontend's `ready`. A missing `config` makes the frontend's
     *  `isConfigReady(null)` report the configuration as not ready, stranding the UI on the config
     *  view. */
    @Serializable
    @SerialName("init")
    data class Init(
        val config: OcrConfig?,
        val gitState: GitState,
        val locale: SupportedLocale,
    ) : HostToWebview()

    @Serializable
    @SerialName("gitState")
    data class GitStateChanged(val gitState: GitState) : HostToWebview()

    @Serializable
    @SerialName("modeFiles")
    data class ModeFiles(val mode: ReviewMode, val files: List<FileChange>) : HostToWebview()

    @Serializable
    @SerialName("logLine")
    data class Log(val line: LogLine) : HostToWebview()

    @Serializable
    @SerialName("stateChange")
    data class StateChange(val state: ReviewState, val error: String? = null) : HostToWebview()

    @Serializable
    @SerialName("reviewDone")
    data class ReviewDone(val result: CliResult) : HostToWebview()

    @Serializable
    @SerialName("config")
    data class Config(val config: OcrConfig?) : HostToWebview()

    @Serializable
    @SerialName("commentSync")
    data class CommentSync(val comments: List<CommentSyncState>) : HostToWebview()
}

/**
 * Outbound messages used by the config panel alone. Declared separately from [HostToWebview]
 * because the sidebar and the config panel are two independent webviews, each recognising only the
 * `type` values of its own channel; a shared channel would deliver a type one side cannot read.
 */
@Serializable
sealed class ConfigPanelHostToWebview {

    /**
     * [focus] is a frontend-defined focus description; the host does not interpret it and passes it
     * back unchanged, hence the [JsonElement] type rather than a Kotlin data class: the host only
     * relays it.
     */
    @Serializable
    @SerialName("configPanelInit")
    data class Init(
        val config: OcrConfig?,
        val focus: JsonElement? = null,
        val env: EnvCheckResult? = null,
        val skipEnvCheck: Boolean = false,
        val locale: SupportedLocale,
    ) : ConfigPanelHostToWebview()

    @Serializable
    @SerialName("configPanelFocus")
    data class Focus(val focus: JsonElement? = null) : ConfigPanelHostToWebview()

    @Serializable
    @SerialName("config")
    data class Config(val config: OcrConfig?) : ConfigPanelHostToWebview()

    @Serializable
    @SerialName("connectionResult")
    data class ConnectionResult(val ok: Boolean, val message: String? = null) : ConfigPanelHostToWebview()

    /** No longer sent (the CLI check result now travels as `environmentResult`); declared anyway to
     *  keep the message contract complete. */
    @Serializable
    @SerialName("cliStatus")
    data class CliStatus(val installed: Boolean) : ConfigPanelHostToWebview()

    @Serializable
    @SerialName("environmentResult")
    data class EnvironmentResult(val env: EnvCheckResult) : ConfigPanelHostToWebview()

    @Serializable
    @SerialName("copyDone")
    data object CopyDone : ConfigPanelHostToWebview()

    @Serializable
    @SerialName("panelError")
    data class PanelError(val message: String) : ConfigPanelHostToWebview()

    @Serializable
    @SerialName("installLog")
    data class InstallLog(val line: LogLine) : ConfigPanelHostToWebview()

    @Serializable
    @SerialName("installDone")
    data class InstallDone(val ok: Boolean) : ConfigPanelHostToWebview()
}

fun HostToWebview.toJson(): String = HostJson.encodeToString(HostToWebview.serializer(), this)

fun ConfigPanelHostToWebview.toJson(): String =
    HostJson.encodeToString(ConfigPanelHostToWebview.serializer(), this)

/** A single webview channel. The JCEF side implements this interface; the routing layer only hands
 *  it a JSON string to send. */
fun interface WebviewChannel {
    fun post(json: String)
}
