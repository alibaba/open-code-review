// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.messages

import com.alibaba.opencodereview.idea.model.CliRunOptions
import com.alibaba.opencodereview.idea.model.ConfigEntry
import com.alibaba.opencodereview.idea.model.FileStatus
import com.alibaba.opencodereview.idea.model.HostStrings
import com.alibaba.opencodereview.idea.model.OcrJson
import com.alibaba.opencodereview.idea.model.ReviewMode
import com.alibaba.opencodereview.idea.model.SupportedLocale
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.intOrNull
import kotlinx.serialization.json.jsonObject

/**
 * The inbound message collection: 10 sidebar types in the sidebar-channel `WebviewToHost` plus 11 config-panel
 * types, 21 in total, dispatched by source (see SidebarRouter / ConfigPanelRouter).
 *
 * kotlinx polymorphic deserialization is deliberately not used: it throws on an unrecognized `type`, while a newer
 * frontend sending new types is a normal situation and must not break the channel. Hand-written parsing also keeps
 * this layer free of IDE APIs, so all 21 types can be unit-tested directly.
 */
sealed class WebviewToHost {

    // ------------------------------------------------------------ Sidebar

    data object Ready : WebviewToHost()
    data class GetGitState(val mode: ReviewMode) : WebviewToHost()
    data class GetModeFiles(
        val mode: ReviewMode,
        val from: String? = null,
        val to: String? = null,
        val commit: String? = null,
    ) : WebviewToHost()

    data class OpenFileDiff(
        val path: String,
        val status: FileStatus,
        val mode: ReviewMode,
        val from: String? = null,
        val to: String? = null,
        val commit: String? = null,
    ) : WebviewToHost()

    data class StartReview(val options: CliRunOptions) : WebviewToHost()
    data object CancelReview : WebviewToHost()
    data object GetConfig : WebviewToHost()
    data class JumpToComment(val index: Int) : WebviewToHost()
    data class CommentAction(val index: Int, val action: CommentActionKind) : WebviewToHost()

    /** [focus] is a frontend-defined focus descriptor; the host passes it through without interpreting its contents. */
    data class OpenConfigPanel(val focus: JsonElement? = null) : WebviewToHost()

    // ------------------------------------------------------------ Config panel

    data object ReadyConfigPanel : WebviewToHost()
    data object CloseConfigPanel : WebviewToHost()
    data class SetConfig(val key: String, val value: String) : WebviewToHost()
    data class SetConfigBatch(val entries: List<ConfigEntry>) : WebviewToHost()
    data class TestConnection(val entries: List<ConfigEntry>) : WebviewToHost()
    data class DeleteCustomProvider(val name: String) : WebviewToHost()
    data class ActivateCustomProvider(val name: String) : WebviewToHost()
    data object CheckCli : WebviewToHost()
    data object CheckEnvironment : WebviewToHost()
    data object InstallCli : WebviewToHost()
    data class CopyToClipboard(val text: String) : WebviewToHost()

    // ------------------------------------------------------------ Fallback

    /** An unrecognized `type`. The original value is kept purely for logging -- routers ignore it and do not treat it as an error. */
    data class Unknown(val type: String) : WebviewToHost()

    /** The JSON itself failed to parse, or a required field is missing or of the wrong type. [reason] is shown to the user. */
    data class Malformed(val reason: String) : WebviewToHost()
}

/** The three `commentAction` actions, with values `'apply' | 'discard' | 'falsePositive'`, as agreed with the frontend. */
enum class CommentActionKind { APPLY, DISCARD, FALSE_POSITIVE }

private fun JsonObject.str(name: String): String? {
    val prim = this[name] as? JsonPrimitive ?: return null
    return if (prim.isString) prim.content else null
}

/** A blank string means "not filled in": when no branch is selected the frontend sends `""` rather than omitting the field. */
private fun JsonObject.optStr(name: String): String? = str(name)?.takeIf { it.isNotBlank() }

private fun JsonObject.int(name: String): Int? = (this[name] as? JsonPrimitive)?.intOrNull

private fun parseMode(raw: String?): ReviewMode = when (raw) {
    "branch" -> ReviewMode.BRANCH
    "commit" -> ReviewMode.COMMIT
    else -> ReviewMode.WORKSPACE
}

private fun parseStatus(raw: String?): FileStatus = when (raw) {
    "added" -> FileStatus.ADDED
    "deleted" -> FileStatus.DELETED
    "renamed" -> FileStatus.RENAMED
    "binary" -> FileStatus.BINARY
    else -> FileStatus.MODIFIED
}

private fun JsonObject.entries(name: String): List<ConfigEntry> {
    val array = this[name] as? JsonArray ?: return emptyList()
    return array.mapNotNull { item ->
        val obj = item as? JsonObject ?: return@mapNotNull null
        val key = obj.str("key") ?: return@mapNotNull null
        // value may be an empty string (clearing a field is expressed as an empty string), so optStr must not be used here.
        ConfigEntry(key, obj.str("value") ?: "")
    }
}

/** The "{type} missing {field}" style prompts differ only in two parameters; this one function generates them all, so the copy is not rebuilt at every call site. */
private fun missingField(locale: SupportedLocale, type: String, field: String): WebviewToHost.Malformed =
    WebviewToHost.Malformed(HostStrings.t(locale, "ext.message.missingField", "type" to type, "field" to field))

/**
 * Parses one frontend message without ever throwing: a parse failure returns [WebviewToHost.Malformed], an
 * unrecognized type returns [WebviewToHost.Unknown].
 * [locale] only affects the `Malformed` copy (those strings are sent back to the frontend for display as-is, so they
 * follow the IDE UI language).
 */
fun parseWebviewMessage(raw: String, locale: SupportedLocale): WebviewToHost {
    val msg = runCatching { OcrJson.parseToJsonElement(raw).jsonObject }.getOrElse {
        return WebviewToHost.Malformed(
            HostStrings.t(locale, "ext.message.parseFailed", "message" to it.message.orEmpty()),
        )
    }
    val type = msg.str("type")
        ?: return WebviewToHost.Malformed(HostStrings.t(locale, "ext.message.missingTypeField"))

    return when (type) {
        "ready" -> WebviewToHost.Ready
        "readyConfigPanel" -> WebviewToHost.ReadyConfigPanel
        "closeConfigPanel" -> WebviewToHost.CloseConfigPanel
        "cancelReview" -> WebviewToHost.CancelReview
        "getConfig" -> WebviewToHost.GetConfig
        "checkCli" -> WebviewToHost.CheckCli
        "checkEnvironment" -> WebviewToHost.CheckEnvironment
        "installCli" -> WebviewToHost.InstallCli

        "openConfigPanel" -> {
            // With "focus": null (a JSON null literal), msg["focus"] yields JsonNull, not a Kotlin null;
            // filter it out so "field absent" and "explicit null" mean the same thing downstream.
            val focus = msg["focus"]?.takeIf { it !is JsonNull }
            WebviewToHost.OpenConfigPanel(focus)
        }

        "getGitState" -> WebviewToHost.GetGitState(parseMode(msg.str("mode")))

        "getModeFiles" -> WebviewToHost.GetModeFiles(
            mode = parseMode(msg.str("mode")),
            from = msg.optStr("from"),
            to = msg.optStr("to"),
            commit = msg.optStr("commit"),
        )

        "openFileDiff" -> {
            val path = msg.optStr("path")
                ?: return missingField(locale, "openFileDiff", "path")
            WebviewToHost.OpenFileDiff(
                path = path,
                status = parseStatus(msg.str("status")),
                mode = parseMode(msg.str("mode")),
                from = msg.optStr("from"),
                to = msg.optStr("to"),
                commit = msg.optStr("commit"),
            )
        }

        "startReview" -> {
            val options = msg["options"] as? JsonObject
                ?: return missingField(locale, "startReview", "options")
            val parsed = runCatching {
                OcrJson.decodeFromJsonElement(CliRunOptions.serializer(), options)
            }.getOrElse {
                return WebviewToHost.Malformed(
                    HostStrings.t(locale, "ext.message.invalidReviewOptions", "message" to it.message.orEmpty()),
                )
            }
            WebviewToHost.StartReview(parsed)
        }

        "setConfig" -> {
            val key = msg.optStr("key")
                ?: return missingField(locale, "setConfig", "key")
            WebviewToHost.SetConfig(key, msg.str("value") ?: "")
        }

        "setConfigBatch" -> WebviewToHost.SetConfigBatch(msg.entries("entries"))
        "testConnection" -> WebviewToHost.TestConnection(msg.entries("entries"))

        "deleteCustomProvider" -> {
            val name = msg.optStr("name")
                ?: return missingField(locale, "deleteCustomProvider", "name")
            WebviewToHost.DeleteCustomProvider(name)
        }

        "activateCustomProvider" -> {
            val name = msg.optStr("name")
                ?: return missingField(locale, "activateCustomProvider", "name")
            WebviewToHost.ActivateCustomProvider(name)
        }

        // text may be an empty string (copying an empty field is a legal operation); an absent field is treated as empty too.
        "copyToClipboard" -> WebviewToHost.CopyToClipboard(msg.str("text") ?: "")

        "jumpToComment" -> {
            val index = msg.int("index")
                ?: return missingField(locale, "jumpToComment", "index")
            WebviewToHost.JumpToComment(index)
        }

        "commentAction" -> {
            val index = msg.int("index")
                ?: return missingField(locale, "commentAction", "index")
            val action = when (msg.str("action")) {
                "apply" -> CommentActionKind.APPLY
                "discard" -> CommentActionKind.DISCARD
                "falsePositive" -> CommentActionKind.FALSE_POSITIVE
                else -> return WebviewToHost.Malformed(HostStrings.t(locale, "ext.message.invalidCommentAction"))
            }
            WebviewToHost.CommentAction(index, action)
        }

        else -> WebviewToHost.Unknown(type)
    }
}
