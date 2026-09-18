// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json

/**
 * Domain models agreed with the frontend, field for field.
 *
 * Field names must be camelCase: the frontend reads by property name, and snake_case would make the frontend
 * silently render blank cards or 0 line numbers.
 * The CLI's snake_case JSON is converted to this layer's types by
 * [com.alibaba.opencodereview.idea.services.parseCliResult]; the CLI's naming must not leak into this layer.
 */

/** Shared JSON config for outbound and inbound: defaults must be sent (the frontend checks property existence), nulls omitted (matching the frontend's `undefined` semantics). */
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

/** Note `falsePositive` is camelCase, not `false_positive` -- the frontend compares against that literal. */
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
     * `startLine` / `endLine` use 0 as the sentinel for "the CLI provided no usable line numbers".
     * The comment anchoring logic (CommentAnchor) relies on this convention to enter the existingCode relocation
     * branch; do not change it to 1.
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

/** [status] stays a String: the value domain (success / completed_with_errors / completed_with_warnings / skipped) is owned by the CLI, and new values must not break parsing. */
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

/** [sha] is a 7-character short hash, matching the frontend's truncation rule. */
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

/** The mount context for comments after a review completes; fields are taken from [CliRunOptions] (the subset matching the frontend). */
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
