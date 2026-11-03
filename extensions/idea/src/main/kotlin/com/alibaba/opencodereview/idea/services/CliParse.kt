// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.services

import com.alibaba.opencodereview.idea.model.AgentWarning
import com.alibaba.opencodereview.idea.model.CliResult
import com.alibaba.opencodereview.idea.model.CliRunOptions
import com.alibaba.opencodereview.idea.model.LogLevel
import com.alibaba.opencodereview.idea.model.LogLine
import com.alibaba.opencodereview.idea.model.OcrJson
import com.alibaba.opencodereview.idea.model.ReviewComment
import com.alibaba.opencodereview.idea.model.ReviewMode
import com.alibaba.opencodereview.idea.model.ReviewSummary
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.SerializationException
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive

/**
 * The single conversion point between the CLI's snake_case JSON and the plugin's camelCase contract.
 * The DTOs below are deserialization-only and never leave the model package.
 */

@Serializable
private data class CliCommentDto(
    val path: String,
    val content: String,
    @SerialName("suggestion_code") val suggestionCode: String? = null,
    @SerialName("existing_code") val existingCode: String? = null,
    @SerialName("start_line") val startLine: Int = 0,
    @SerialName("end_line") val endLine: Int = 0,
    val thinking: String? = null,
)

@Serializable
private data class CliSummaryDto(
    @SerialName("files_reviewed") val filesReviewed: Int = 0,
    val comments: Int = 0,
    @SerialName("total_tokens") val totalTokens: Int = 0,
    @SerialName("input_tokens") val inputTokens: Int = 0,
    @SerialName("output_tokens") val outputTokens: Int = 0,
    val elapsed: String = "",
)

@Serializable
private data class CliResultDto(
    val status: String,
    val message: String? = null,
    val comments: List<CliCommentDto>?,
    val warnings: List<AgentWarning> = emptyList(),
    val summary: CliSummaryDto? = null,
)

fun buildReviewArgs(opts: CliRunOptions): List<String> = buildList {
    add("review")
    when (opts.mode) {
        ReviewMode.WORKSPACE -> Unit
        ReviewMode.BRANCH -> {
            opts.from?.takeIf(String::isNotBlank)?.let { addAll(listOf("--from", it.trim())) }
            opts.to?.takeIf(String::isNotBlank)?.let { addAll(listOf("--to", it.trim())) }
        }
        ReviewMode.COMMIT -> opts.commit?.takeIf(String::isNotBlank)?.let { addAll(listOf("--commit", it.trim())) }
    }
    addAll(listOf("--format", "json"))
    // The CLI writes the JSON result to stdout and progress logs to stderr, so the plugin can stream them live.
    opts.customPrompt?.takeIf(String::isNotBlank)?.let { addAll(listOf("--background", it.trim())) }
    opts.concurrency?.let { addAll(listOf("--concurrency", it.toString())) }
}

private fun CliCommentDto.toComment(): ReviewComment = ReviewComment(
    path = path,
    content = content,
    // Normalize blank strings to missing: passed through as-is, an empty/blank string would make callers
    // treat it as "a suggestion/existing code is present".
    suggestionCode = suggestionCode?.takeIf(String::isNotBlank),
    existingCode = existingCode?.takeIf(String::isNotBlank),
    startLine = startLine,
    endLine = endLine,
    thinking = thinking?.takeIf(String::isNotEmpty),
)

/** Find complete top-level values without interpreting braces inside JSON strings. */
private fun jsonCandidates(stdout: String): Sequence<String> = sequence {
    var start = -1
    var depth = 0
    var quoted = false
    var escaped = false
    for ((index, char) in stdout.withIndex()) {
        if (start < 0) {
            if (char != '{' && char != '[') continue
            start = index
            depth = 1
            continue
        }
        if (quoted) {
            if (escaped) escaped = false
            else when (char) {
                '\\' -> escaped = true
                '"' -> quoted = false
            }
        } else {
            when (char) {
                '"' -> quoted = true
                '{', '[' -> depth++
                '}', ']' -> depth--
            }
            if (depth == 0) {
                yield(stdout.substring(start, index + 1))
                start = -1
            }
        }
    }
    // An incomplete result must not be hidden by an earlier successful-looking value.
    if (start >= 0) yield(stdout.substring(start))
}

private val RESULT_FIELD = Regex("\"(?:status|comments)\"\\s*:")

/** Accept one validated review result; unrelated log objects cannot become empty results. */
private fun findCliResultDto(stdout: String): CliResultDto {
    var result: CliResultDto? = null
    for (candidate in jsonCandidates(stdout)) {
        val element = try {
            OcrJson.parseToJsonElement(candidate)
        } catch (error: SerializationException) {
            require(!RESULT_FIELD.containsMatchIn(candidate)) { "Invalid review JSON in CLI output" }
            continue
        }
        val obj = element as? JsonObject ?: continue
        if ("status" !in obj && "comments" !in obj) continue
        val status = obj["status"] as? JsonPrimitive
        require(status?.isString == true && status.content in supportedReviewStatuses) {
            "Missing or unsupported review status in CLI output"
        }
        require("comments" in obj && (obj["comments"] is JsonArray || obj["comments"] == JsonNull)) {
            "Missing or invalid review comments in CLI output"
        }
        val dto = try {
            OcrJson.decodeFromJsonElement(CliResultDto.serializer(), obj)
        } catch (error: SerializationException) {
            throw IllegalArgumentException("Invalid review result in CLI output", error)
        }
        require(dto.comments.orEmpty().all { it.path.isNotBlank() && it.content.isNotBlank() }) {
            "Invalid review comment in CLI output"
        }
        require(result == null) { "Multiple review results in CLI output" }
        result = dto
    }
    return result ?: throw IllegalArgumentException("No valid review result in CLI output")
}

fun parseCliResult(stdout: String): CliResult {
    val dto = findCliResultDto(stdout)
    return CliResult(
        status = dto.status,
        message = dto.message,
        comments = dto.comments.orEmpty().map { it.toComment() },
        warnings = dto.warnings,
        summary = dto.summary?.let {
            ReviewSummary(
                filesReviewed = it.filesReviewed,
                comments = it.comments,
                totalTokens = it.totalTokens,
                inputTokens = it.inputTokens,
                outputTokens = it.outputTokens,
                elapsed = it.elapsed,
            )
        },
    )
}

/** Regex for the `error:` prefix, stripped when extracting the error text. */
private val ERROR_PREFIX_REGEX = Regex("^error:\\s*", RegexOption.IGNORE_CASE)

/** Extracts the most useful error text from the CLI's stderr: prefer the last `error:` line, otherwise the last non-empty line. */
fun extractCliError(stderr: String): String {
    val lines = stderr.lineSequence().map(String::trim).filter(String::isNotEmpty)
    val errLine = lines.lastOrNull { it.startsWith("error:", ignoreCase = true) }
    if (errLine != null) return errLine.replaceFirst(ERROR_PREFIX_REGEX, "")
    return lines.lastOrNull().orEmpty()
}

private val WARN_PATTERN = Regex("retrying|warning|warn", RegexOption.IGNORE_CASE)

fun parseLogLine(raw: String): LogLine? {
    val text = raw.trimEnd()
    if (text.isBlank()) return null
    return LogLine(text, if (WARN_PATTERN.containsMatchIn(text)) LogLevel.WARN else LogLevel.INFO)
}
