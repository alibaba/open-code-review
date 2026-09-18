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
 * The outbound message collection: `HostToWebview` for the sidebar channel (8 types) and
 * `ConfigPanelHostToWebview` for the config-panel channel (10 types).
 *
 * Sealed classes with `classDiscriminator = "type"` are used instead of hand-building `buildJsonObject`: a
 * hand-built missing field or snake_case slip is invisible to the compiler and the frontend silently renders
 * blank cards; sealed classes turn "missing one field" into a compile-time error.
 */

/**
 * The outbound JSON encoder. `explicitNulls = true` emits null-valued fields explicitly as `"field": null`,
 * matching the frontend TypeScript declaration (`field: Type | null`, non-optional) -- the frontend expects
 * fields to always exist.
 */
val HostJson: Json = Json {
    classDiscriminator = "type"
    encodeDefaults = true
    explicitNulls = true
}

@Serializable
sealed class HostToWebview {

    /** The first message after the frontend's `ready`. A missing `config` makes the frontend's `isConfigReady(null)` judge the config not ready, leaving the UI stuck on the config view forever. */
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
 * Outbound messages specific to the config panel. Declared separately from [HostToWebview] because the sidebar
 * and the config panel are two independent webviews, each recognizing only its own channel's `type` values;
 * sharing a channel would deliver unrecognized messages to one of the sides.
 */
@Serializable
sealed class ConfigPanelHostToWebview {

    /**
     * [focus] is a frontend-defined focus descriptor; the host does not interpret its contents and returns it
     * as-is, hence the [JsonElement] type rather than a Kotlin data class -- the host side only passes it through.
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

    /** No longer sent (CLI check results now travel in `environmentResult`); kept declared for message-contract completeness. */
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

/** A single webview channel. The JCEF side implements this interface; routers only hand it JSON strings to send. */
fun interface WebviewChannel {
    fun post(json: String)
}
