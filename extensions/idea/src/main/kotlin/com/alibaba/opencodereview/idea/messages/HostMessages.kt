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
 * 出站消息集合，分为侧栏通道使用的 `HostToWebview`（8 条）与配置面板通道使用的 `ConfigPanelHostToWebview`（10 条）。
 *
 * 采用密封类加 `classDiscriminator = "type"`，而非手工拼接 `buildJsonObject`：手工拼接漏字段或误用 snake_case
 * 时编译器无法发现，前端将静默渲染空白卡片；密封类把"缺一个字段"提升为编译期错误。
 */

/**
 * 出站 JSON 编码器。`explicitNulls = true` 使值为 null 的字段显式输出为 `"field": null`，
 * 与前端 TypeScript 类型声明（`field: Type | null` 非可选）的契约一致——前端期望字段始终存在。
 */
val HostJson: Json = Json {
    classDiscriminator = "type"
    encodeDefaults = true
    explicitNulls = true
}

@Serializable
sealed class HostToWebview {

    /** 前端 `ready` 之后的第一条消息。缺少 `config` 会使前端 `isConfigReady(null)` 判定配置未就绪，导致 UI 永久停留在配置视图。 */
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
 * 配置面板专用出站消息。与 [HostToWebview] 分开声明，原因是侧栏与配置面板为两个独立 webview，
 * 各自只识别本通道的 `type` 取值；共用通道会使其中一侧收到无法识别的消息。
 */
@Serializable
sealed class ConfigPanelHostToWebview {

    /**
     * [focus] 表示前端自定义的焦点描述，宿主不解释其内容、原样回传，
     * 故类型为 [JsonElement] 而非 Kotlin data class——宿主侧仅作透传。
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

    /** 该类型已不再发送（CLI 检查结果改由 `environmentResult` 承载）；为保持消息契约完整仍予声明。 */
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

/** 单条 webview 通道。由 JCEF 侧实现此接口，路由层仅负责将 JSON 字符串交付其发送。 */
fun interface WebviewChannel {
    fun post(json: String)
}
