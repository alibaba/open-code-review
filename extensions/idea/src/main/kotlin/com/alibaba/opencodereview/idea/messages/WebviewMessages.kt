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
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.intOrNull
import kotlinx.serialization.json.jsonObject

/**
 * 入站消息集合，分为侧栏通道使用的 `WebviewToHost` 侧栏 10 条与配置面板 11 条，共 21 条，
 * 按来源分派处理（参见 SidebarRouter / ConfigPanelRouter）。
 *
 * 刻意不采用 kotlinx 多态反序列化：该方式遇到未识别的 `type` 会抛异常，而前端版本较新时发送新增类型属正常情形，
 * 不应导致通道中断。手写解析还使本层不依赖 IDE API，21 个类型均可直接进行单元测试。
 */
sealed class WebviewToHost {

    // ------------------------------------------------------------ 侧栏

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

    /** [focus] 表示前端自定义的焦点描述，宿主仅透传，不解释其内容。 */
    data class OpenConfigPanel(val focus: JsonElement? = null) : WebviewToHost()

    // ------------------------------------------------------------ 配置面板

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

    // ------------------------------------------------------------ 兜底

    /** 无法识别的 `type`。保留原值仅供日志记录——路由层将其忽略，不视为错误。 */
    data class Unknown(val type: String) : WebviewToHost()

    /** JSON 本身解析失败，或必填字段缺失、类型不符。[reason] 将展示给用户。 */
    data class Malformed(val reason: String) : WebviewToHost()
}

/** `commentAction` 的三种动作，取值为 `'apply' | 'discard' | 'falsePositive'`，与前端约定一致。 */
enum class CommentActionKind { APPLY, DISCARD, FALSE_POSITIVE }

private fun JsonObject.str(name: String): String? {
    val prim = this[name] as? JsonPrimitive ?: return null
    return if (prim.isString) prim.content else null
}

/** 空串视为"未填写"：前端表单中未选择分支时发送的是 `""` 而非省略字段。 */
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
        // value 允许为空串（清空某字段即以空串表达），因此此处不能使用 optStr。
        ConfigEntry(key, obj.str("value") ?: "")
    }
}

/** 「{type} 缺少 {field}」这类提示仅差两个参数，统一由本函数生成，避免各处重复构造文案。 */
private fun missingField(locale: SupportedLocale, type: String, field: String): WebviewToHost.Malformed =
    WebviewToHost.Malformed(HostStrings.t(locale, "ext.message.missingField", "type" to type, "field" to field))

/**
 * 解析一条前端消息，过程中不抛出异常：解析失败返回 [WebviewToHost.Malformed]，未识别类型返回 [WebviewToHost.Unknown]。
 * [locale] 仅影响 `Malformed` 的提示文案（这些字符串原样回传前端展示，故随 IDE 界面语言而定）。
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

        "openConfigPanel" -> WebviewToHost.OpenConfigPanel(msg["focus"])

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

        // text 可为空串（复制空字段属合法操作），故仅校验字段是否存在。
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
