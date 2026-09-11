package com.alibaba.opencodereview.idea.model

/**
 * 宿主侧原生 UI 文案表（Swing 弹窗、NotificationGroup 通知、gutter popup 等）。
 * 与前端共享的 `ext.*` 词条，key 与值均应与前端保持一致。
 * 前端未覆盖的条目按相同风格新增（前缀仍为 `ext.`）。
 */
object HostStrings {

    /** `{param}` 占位符，单遍替换用（避免顺序依赖注入）。 */
    private val PARAM_REGEX = Regex("""\{(\w+)\}""")

    private val EN: Map<String, String> = mapOf(
        // ---------------------------------------------------------------- 评论
        "ext.comment.threadLabel" to "Code Review",
        "ext.comment.pending" to "⏳ [Pending]",
        "ext.comment.statusApplied" to "✅ [Applied]",
        "ext.comment.statusDiscarded" to "✅ [Discarded]",
        "ext.comment.statusFalsePositive" to "✅ [False Positive]",
        // popup 为纯文本 JTextArea，markdown 斜体的 `_` 已去除，加回会导致原样显示为下划线。
        "ext.comment.noSuggestion" to "💡 No code suggestion, please handle manually",
        "ext.comment.applyWorkspaceOnly" to "Apply is only available in Workspace review mode.",
        "ext.comment.applyFailedStale" to "Apply failed: code location is stale, please refresh and retry.",
        "ext.comment.applyFailedLocked" to "Apply failed: cannot modify file, check if it is read-only or locked.",
        "ext.comment.jumpFileMissing" to "Cannot jump to {path}: file not found in the review snapshot.",
        "ext.comment.jumpLineUnresolved" to "Cannot jump to {path}: line number could not be resolved.",
        "ext.comment.apply" to "Apply",
        "ext.comment.discard" to "Discard",
        "ext.comment.locateNoteRelocatedFrom" to "⚠ Line {original} could not be matched, showing line {resolved} instead.",
        "ext.comment.locateNoteRelocated" to "⚠ Line number was relocated based on code content.",

        // ---------------------------------------------------------------- 配置面板
        "ext.configPanelTitle" to "Model Configuration",
        "ext.deleteProviderConfirm" to "Delete custom provider \"{name}\"?",
        "ext.deleteProviderConfirmBtn" to "Delete",
        "ext.deleteProviderTitle" to "Delete Custom Provider",
        "ext.common.cancel" to "Cancel",

        // ---------------------------------------------------------------- CLI / 环境
        "ext.cli.installOk" to "✓ Install complete",
        "ext.cli.installFail" to "✗ Install failed (exit {code})",
        "ext.config.tempDirFailed" to "Failed to create temporary directory: {message}",

        // ---------------------------------------------------------------- 宿主生成、发给前端展示
        "ext.review.noProjectDir" to "No usable project directory in the current project.",
        "ext.message.parseFailed" to "Failed to parse frontend message: {message}",
        "ext.message.missingTypeField" to "Frontend message is missing the type field",
        "ext.message.missingField" to "{type} is missing required field: {field}",
        "ext.message.invalidReviewOptions" to "Invalid review options: {message}",
        "ext.message.invalidCommentAction" to "commentAction has an invalid action value",

        // ---------------------------------------------------------------- JCEF 兜底（IDEA 特有）
        "ext.jcef.unsupportedTitle" to "Cannot open configuration panel",
        "ext.jcef.unsupportedBody" to "The current runtime does not support JCEF. Please switch to the JetBrains Runtime bundled with IDEA, or configure via the `ocr config` command line first.",
        // 嵌入 JBLabel 的 <html>，故含 <br> 断行，不可删除。
        "ext.jcef.unsupportedPlaceholder" to "The current runtime does not support JCEF, so the Open Code Review UI cannot be shown.<br>" +
            "Please switch the IDE's boot runtime to the bundled JetBrains Runtime (Choose Boot Java Runtime for the IDE) and restart.",

        // ---------------------------------------------------------------- 前端 bundle 缺失（IDEA 特有）
        "ext.webview.missingBundleTitle" to "Frontend assets missing",
        "ext.webview.missingBundleBody" to "<code>{resource}</code> is not in the plugin resources.",
        "ext.webview.missingBundleHint" to "Build the frontend first:",
        "ext.webview.missingBundleAlt" to "Or just run <code>./gradlew build</code>, which includes this step.",

        // ---------------------------------------------------------------- git
        "ext.git.workspace" to "Workspace",
        "ext.git.emptyRef" to "(empty)",
        "ext.git.justNow" to "just now",
        "ext.git.hourAgo" to "1 hour ago",
        "ext.git.hoursAgo" to "{h} hours ago",
        "ext.git.yesterday" to "yesterday",
        "ext.git.daysAgo" to "{d} days ago",
        "ext.git.minutesAgo" to "{m} minutes ago",
        "ext.git.monthsAgo" to "{mo} months ago",
        "ext.git.yearsAgo" to "{y} years ago",
    )

    private val ZH_CN: Map<String, String> = mapOf(
        // ---------------------------------------------------------------- 评论
        "ext.comment.threadLabel" to "Code Review", // "Code Review" 是产品名称，不翻译
        "ext.comment.pending" to "⏳ [未处理]",
        "ext.comment.statusApplied" to "✅ [已应用]",
        "ext.comment.statusDiscarded" to "✅ [已忽略]",
        "ext.comment.statusFalsePositive" to "✅ [已误报]",
        "ext.comment.noSuggestion" to "💡 无代码建议，请手动处理", // markdown 的 `_` 已去除，原因见 EN 侧同 key 注释
        "ext.comment.applyWorkspaceOnly" to "仅工作区审查模式支持应用建议。",
        "ext.comment.applyFailedStale" to "应用失败：代码位置已失效，请刷新后重试。",
        "ext.comment.applyFailedLocked" to "应用失败：无法修改文件，请检查文件是否被占用或处于只读状态。",
        "ext.comment.jumpFileMissing" to "无法跳转到 {path}：在审查快照中找不到该文件。",
        "ext.comment.jumpLineUnresolved" to "无法跳转到 {path}：未能解析行号。",
        "ext.comment.apply" to "应用",
        "ext.comment.discard" to "忽略",
        "ext.comment.locateNoteRelocatedFrom" to "⚠ 原本第 {original} 行没能匹配上，改为显示第 {resolved} 行。",
        "ext.comment.locateNoteRelocated" to "⚠ 行号是根据代码内容重新定位的。",

        // ---------------------------------------------------------------- 配置面板
        "ext.configPanelTitle" to "模型配置",
        "ext.deleteProviderConfirm" to "确定删除自定义 Provider「{name}」？",
        "ext.deleteProviderConfirmBtn" to "删除",
        "ext.deleteProviderTitle" to "删除自定义 Provider",
        "ext.common.cancel" to "取消",

        // ---------------------------------------------------------------- CLI / 环境
        "ext.cli.installOk" to "✓ 安装完成",
        "ext.cli.installFail" to "✗ 安装失败 (exit {code})",
        "ext.config.tempDirFailed" to "无法创建临时目录：{message}",

        // ---------------------------------------------------------------- 宿主生成、发给前端展示
        "ext.review.noProjectDir" to "当前项目没有可用的项目目录",
        "ext.message.parseFailed" to "无法解析前端消息：{message}",
        "ext.message.missingTypeField" to "前端消息缺少 type 字段",
        "ext.message.missingField" to "{type} 缺少 {field}",
        "ext.message.invalidReviewOptions" to "审查参数无效：{message}",
        "ext.message.invalidCommentAction" to "commentAction 的 action 无效",

        // ---------------------------------------------------------------- JCEF 兜底（IDEA 特有）
        "ext.jcef.unsupportedTitle" to "无法打开配置面板",
        "ext.jcef.unsupportedBody" to "当前运行时不支持 JCEF。请改用 IDEA 自带的 JetBrains Runtime，或先用 `ocr config` 命令行配置。",
        "ext.jcef.unsupportedPlaceholder" to "当前运行时不支持 JCEF，Open Code Review 的界面无法显示。<br>" +
            "请用 IDEA 自带的 JetBrains Runtime（Choose Boot Java Runtime for the IDE）后重启。",

        // ---------------------------------------------------------------- 前端 bundle 缺失（IDEA 特有）
        "ext.webview.missingBundleTitle" to "前端资源缺失",
        "ext.webview.missingBundleBody" to "<code>{resource}</code> 不在插件资源里。",
        "ext.webview.missingBundleHint" to "请先构建前端：",
        "ext.webview.missingBundleAlt" to "或直接 <code>./gradlew build</code>（会自动带上这一步）。",

        // ---------------------------------------------------------------- git
        "ext.git.workspace" to "工作区",
        "ext.git.emptyRef" to "（空）",
        "ext.git.justNow" to "刚刚",
        "ext.git.hourAgo" to "1 小时前",
        "ext.git.hoursAgo" to "{h} 小时前",
        "ext.git.yesterday" to "昨天",
        "ext.git.daysAgo" to "{d} 天前",
        "ext.git.minutesAgo" to "{m} 分钟前",
        "ext.git.monthsAgo" to "{mo} 个月前",
        "ext.git.yearsAgo" to "{y} 年前",
    )

    /** 取文案并执行 `{param}` 替换。未找到词条时回退至英文，两者皆无则原样返回 key——与前端的兜底策略一致。 */
    fun t(locale: SupportedLocale, key: String, vararg params: Pair<String, String>): String {
        val template = when (locale) {
            SupportedLocale.ZH_CN -> ZH_CN[key]
            SupportedLocale.EN -> null
        } ?: EN[key] ?: key
        // 单遍替换：避免某参数值含 `{otherParam}` 时被后续参数再次替换（顺序依赖注入）。
        val map = params.toMap()
        return PARAM_REGEX.replace(template) { m -> map[m.groupValues[1]] ?: m.value }
    }

    /** 返回两张表的原始内容，仅供单元测试——确保「两种语言 key 齐全且占位符一致」。 */
    internal fun tables(): Map<SupportedLocale, Map<String, String>> =
        mapOf(SupportedLocale.EN to EN, SupportedLocale.ZH_CN to ZH_CN)
}
