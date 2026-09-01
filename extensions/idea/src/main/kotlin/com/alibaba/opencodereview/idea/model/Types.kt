package com.alibaba.opencodereview.idea.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json

/**
 * 与前端约定的领域模型，字段逐一对应。
 *
 * 字段名须为 camelCase：前端按属性名取值，snake_case 会导致前端静默渲染为空白卡片或 0 行号。
 * CLI 的 snake_case JSON 在 [com.alibaba.opencodereview.idea.services.parseCliResult] 转换为本层类型，不应将 CLI 的命名带入本层。
 */

/** 出站与入站 JSON 的统一配置：默认值须发送（前端依属性存在性判断），null 不发送（对应前端 `undefined` 语义）。 */
val OcrJson: Json = Json {
    ignoreUnknownKeys = true
    encodeDefaults = true
    explicitNulls = false
}

@Serializable
enum class ReviewMode {
    @SerialName("workspace") WORKSPACE,
    @SerialName("branch") BRANCH,
    @SerialName("commit") COMMIT,
}

@Serializable
enum class ReviewState {
    @SerialName("idle") IDLE,
    @SerialName("running") RUNNING,
    @SerialName("done") DONE,
    @SerialName("empty") EMPTY,
    @SerialName("cancelled") CANCELLED,
    @SerialName("failed") FAILED,
}

/** 注意 `falsePositive` 为 camelCase，而非 `false_positive`——前端按此字面量进行比较。 */
@Serializable
enum class CommentStatus {
    @SerialName("pending") PENDING,
    @SerialName("applied") APPLIED,
    @SerialName("discarded") DISCARDED,
    @SerialName("falsePositive") FALSE_POSITIVE,
}

@Serializable
enum class LogLevel {
    @SerialName("info") INFO,
    @SerialName("warn") WARN,
    @SerialName("error") ERROR,
}

@Serializable
enum class FileStatus {
    @SerialName("added") ADDED,
    @SerialName("modified") MODIFIED,
    @SerialName("deleted") DELETED,
    @SerialName("renamed") RENAMED,
    @SerialName("binary") BINARY,
}

/**
 * `startLine` / `endLine` 取 0 作为哨兵值，表示 CLI 未提供可用行号。
 * 评论定位逻辑（CommentAnchor）依据此约定进入 existingCode 重定位分支，不可改为 1。
 */
@Serializable
data class ReviewComment(
    val path: String = "",
    val content: String = "",
    val suggestionCode: String? = null,
    val existingCode: String? = null,
    val startLine: Int = 0,
    val endLine: Int = 0,
    val thinking: String? = null,
)

@Serializable
data class ReviewSummary(
    val filesReviewed: Int = 0,
    val comments: Int = 0,
    val totalTokens: Int = 0,
    val inputTokens: Int = 0,
    val outputTokens: Int = 0,
    val elapsed: String = "",
)

@Serializable
data class AgentWarning(
    val type: String = "",
    val file: String = "",
    val message: String = "",
)

/** [status] 保持 String 类型：CLI 的取值域（success / completed_with_errors / completed_with_warnings / skipped）由 CLI 决定，出现新增取值不应导致解析失败。 */
@Serializable
data class CliResult(
    val status: String = "",
    val comments: List<ReviewComment> = emptyList(),
    val warnings: List<AgentWarning> = emptyList(),
    val summary: ReviewSummary? = null,
    val message: String? = null,
)

@Serializable
data class ProviderEntry(
    val apiKey: String = "",
    val url: String = "",
    val protocol: String = "",
    val model: String = "",
    val models: List<String>? = null,
    val authHeader: String = "",
)

@Serializable
data class LlmConfig(
    val url: String = "",
    val authToken: String = "",
    val model: String = "",
    val useAnthropic: Boolean = true,
    val authHeader: String = "",
)

@Serializable
data class OcrConfig(
    val provider: String = "",
    val model: String = "",
    val providers: Map<String, ProviderEntry> = emptyMap(),
    val customProviders: Map<String, ProviderEntry> = emptyMap(),
    val llm: LlmConfig = LlmConfig(),
    val language: String = "Chinese",
)

/** [sha] 采用 7 位短哈希，与前端截取规则一致。 */
@Serializable
data class CommitInfo(
    val sha: String = "",
    val message: String = "",
    val relativeTime: String = "",
)

@Serializable
data class FileChange(
    val path: String = "",
    val status: FileStatus = FileStatus.MODIFIED,
)

@Serializable
data class GitState(
    val branches: List<String> = emptyList(),
    val currentBranch: String = "",
    val recentCommits: List<CommitInfo> = emptyList(),
    val workspaceFiles: List<FileChange> = emptyList(),
)

@Serializable
data class LogLine(
    val text: String,
    val level: LogLevel = LogLevel.INFO,
)

@Serializable
data class EnvToolStatus(
    val ok: Boolean = false,
    val version: String? = null,
)

@Serializable
data class EnvCheckResult(
    val node: EnvToolStatus = EnvToolStatus(),
    val npm: EnvToolStatus = EnvToolStatus(),
    val ocr: EnvToolStatus = EnvToolStatus(),
)

@Serializable
data class CliRunOptions(
    val mode: ReviewMode = ReviewMode.WORKSPACE,
    val from: String? = null,
    val to: String? = null,
    val commit: String? = null,
    val customPrompt: String? = null,
    val concurrency: Int? = null,
)

/** 审查完成后评论的挂载上下文，字段取自 [CliRunOptions]（对应前端的字段子集）。 */
@Serializable
data class ReviewContext(
    val mode: ReviewMode = ReviewMode.WORKSPACE,
    val from: String? = null,
    val to: String? = null,
    val commit: String? = null,
)

fun CliRunOptions.toReviewContext(): ReviewContext = ReviewContext(mode, from, to, commit)

@Serializable
data class CommentSyncState(
    val index: Int,
    val status: CommentStatus = CommentStatus.PENDING,
    val jumpable: Boolean? = null,
)

@Serializable
data class ConfigEntry(
    val key: String = "",
    val value: String = "",
)
