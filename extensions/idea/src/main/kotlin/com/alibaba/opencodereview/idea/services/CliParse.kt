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

/**
 * CLI snake_case JSON 与插件 camelCase 契约之间的唯一转换点。
 * 下列 Dto 仅用于反序列化，不暴露到 model 包之外。
 */

@Serializable
private data class CliCommentDto(
    val path: String = "",
    val content: String = "",
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
    val status: String = "",
    val message: String? = null,
    val comments: List<CliCommentDto> = emptyList(),
    val warnings: List<AgentWarning> = emptyList(),
    val summary: CliSummaryDto? = null,
)

fun buildReviewArgs(opts: CliRunOptions): List<String> = buildList {
    add("review")
    when (opts.mode) {
        ReviewMode.WORKSPACE -> Unit
        ReviewMode.BRANCH -> {
            opts.from?.takeIf(String::isNotBlank)?.let { addAll(listOf("--from", it)) }
            opts.to?.takeIf(String::isNotBlank)?.let { addAll(listOf("--to", it)) }
        }
        ReviewMode.COMMIT -> opts.commit?.takeIf(String::isNotBlank)?.let { addAll(listOf("--commit", it)) }
    }
    addAll(listOf("--format", "json"))
    // JSON 结果走 stdout，进度日志走 stderr，供插件实时回显。
    opts.customPrompt?.takeIf(String::isNotBlank)?.let { addAll(listOf("--background", it.trim())) }
    opts.concurrency?.let { addAll(listOf("--concurrency", it.toString())) }
}

private fun CliCommentDto.toComment(): ReviewComment = ReviewComment(
    path = path,
    content = content,
    // 将空串归一为缺失：若直接透传空串，调用方会把空串当作"存在建议"。
    suggestionCode = suggestionCode?.takeIf(String::isNotEmpty),
    existingCode = existingCode?.takeIf(String::isNotEmpty),
    startLine = startLine,
    endLine = endLine,
    thinking = thinking?.takeIf(String::isNotEmpty),
)

/**
 * 从 stdout 中查找并反序列化 CLI JSON 结果。
 * 不假设第一个 `{` 即起点，避免前置日志混入 `{` 导致解析失败。
 */
private fun findCliResultDto(stdout: String): CliResultDto {
    val end = stdout.lastIndexOf('}')
    require(end >= 0) { "no JSON in CLI output" }
    var start = stdout.indexOf('{')
    while (start in 0..end) {
        runCatching { OcrJson.decodeFromString(CliResultDto.serializer(), stdout.substring(start, end + 1)) }
            .getOrNull()
            ?.let { return it }
        start = stdout.indexOf('{', start + 1)
    }
    throw IllegalArgumentException("no JSON in CLI output")
}

fun parseCliResult(stdout: String): CliResult {
    val dto = findCliResultDto(stdout)
    return CliResult(
        status = dto.status,
        message = dto.message,
        comments = dto.comments.map { it.toComment() },
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

/** 从 CLI stderr 中提取最有用的报错文本：优先最后一条 `error:` 行，否则取最后一行非空内容。 */
fun extractCliError(stderr: String): String {
    val lines = stderr.lineSequence().map(String::trim).filter(String::isNotEmpty).toList()
    val errLine = lines.lastOrNull { it.startsWith("error:", ignoreCase = true) }
    if (errLine != null) return errLine.replaceFirst(Regex("^error:\\s*", RegexOption.IGNORE_CASE), "")
    return lines.lastOrNull().orEmpty()
}

private val WARN_PATTERN = Regex("retrying|warning|warn", RegexOption.IGNORE_CASE)

fun parseLogLine(raw: String): LogLine? {
    val text = raw.trimEnd()
    if (text.isBlank()) return null
    return LogLine(text, if (WARN_PATTERN.containsMatchIn(text)) LogLevel.WARN else LogLevel.INFO)
}
