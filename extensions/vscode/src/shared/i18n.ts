/**
 * Supported display locales for the extension UI.
 * Add new entries here and in the `messages` dictionary below to extend.
 *
 * - `en`    — English (default, fallback for all unrecognized locales)
 * - `zh-cn` — Simplified Chinese (matches VS Code `zh-cn` / `zh-CN`)
 */
export type SupportedLocale = 'en' | 'zh-cn';

const messages: Record<SupportedLocale, Record<string, string>> = {
  en: {
    // ── IdleView ──
    'view.idle.configFirst': 'Configure model first',
    'view.idle.reviewing': 'Reviewing…',
    'view.idle.selectBranch': 'Select comparison branch',
    'view.idle.selectCommit': 'Select a commit',
    'view.idle.noFiles': 'No files to review',
    'view.idle.reviewAll': 'Review all changes',
    'view.idle.workspace': 'Workspace',
    'view.idle.branch': 'Branch Compare',
    'view.idle.commit': 'Single Commit',
    'view.idle.baseRef': 'Base ref',
    'view.idle.targetRef': 'Target ref',
    'view.idle.chooseBranch': 'Choose branch',
    'view.idle.commitHistory': 'Commit history',
    'view.idle.customPrompt': 'Custom review prompt (optional)',
    'view.idle.manageCustom': 'Manage custom providers',
    'view.idle.modelConfig': 'Model config',

    // ── RunningView ──
    'view.running.reviewLog': 'Review log',
    'view.running.cancel': 'Cancel',

    // ── DoneView ──
    'view.done.comments': 'comments',
    'view.done.files': 'files',
    'view.done.processLog': 'Process log',

    // ── EmptyView ──
    'view.empty.noIssues': 'No issues found · Passed',
    'view.empty.processLog': 'Process log',

    // ── CancelledView ──
    'view.cancelled.title': 'Review cancelled',

    // ── FailedView ──
    'view.failed.title': 'Review failed.',
    'view.failed.checkConfig': 'Please check model configuration and retry.',
    'view.failed.checkApiKey': 'Please check API key and network connection.',
    'view.failed.retry': 'Retry',

    // ── ConfigView ──
    'view.config.title': 'Model Configuration',
    'view.config.desc': 'Connect an LLM provider to start code review',
    'view.config.close': 'Close',
    'view.config.step1': 'Environment Setup',
    'view.config.step2': 'Provider Config',
    'view.config.checking': 'ocr checking…',
    'view.config.notInstalled': 'ocr not installed',
    'view.config.official': 'Official Provider',
    'view.config.custom': 'Custom Provider',
    'view.config.currentUse': 'Currently using',
    'view.config.notConfigured': 'No provider configured',
    'view.config.officialLabel': 'Official',
    'view.config.customLabel': 'Custom',
    'view.config.legacyLabel': 'Legacy',
    'view.config.model': 'Model',
    'view.config.customModel': 'Enter custom model…',
    'view.config.apiKey': 'API Key',
    'view.config.apiKeyEnvHint': 'Also available via env var',
    'view.config.apiKeySaved': 'Saved (leave blank to keep)',
    'view.config.testing': 'Testing connection…',
    'view.config.testOk': '✓ Connected',
    'view.config.testFail': '✗ Connection failed',
    'view.config.previous': 'Previous',
    'view.config.testFailDetail': '✗ Connection failed: {message}',
    'view.config.test': 'Test Connection',
    'view.config.save': 'Save',
    'view.config.continueProvider': 'Continue to Provider Config',
    'view.config.providerName': 'Provider Name',
    'view.config.protocol': 'Protocol',
    'view.config.baseUrl': 'Base URL',
    'view.config.modelList': 'Model list',
    'view.config.modelListPlaceholder': 'Comma-separated, e.g. model-a, model-b',
    'view.config.authHeader': 'Auth Header',
    'view.config.authHeaderHint': 'Optional x-api-key or authorization for Anthropic protocol',
    'view.config.authHeaderDefault': 'Default (Authorization)',
    'view.config.backToList': '← Back to list',
    'view.config.optional': '(optional)',
    'view.config.ocrVersionTooltip': 'Open Code Review CLI Version',

    // ── EnvSetupGuide ──
    'view.env.installing': 'Installing ocr CLI…',
    'view.env.checking': 'Checking, please wait…',
    'view.env.ready': 'Environment is ready. Continue to Provider Config.',
    'view.env.stepLead': 'Complete each step in order. Move to the next after each passes.',
    'view.env.nodeHint': 'Node.js not detected. Visit nodejs.org to install the LTS version, then restart VS Code.',
    'view.env.npmHint': 'npm not detected. npm is usually bundled with Node.js — verify your Node installation.',
    'view.env.ocrHint': 'Install open-code-review globally in your terminal, or click "One-Click Install" below.',
    'view.env.oneClickInstall': 'One-Click Install',
    'view.env.redetect': 'Re-detect',
    'view.env.checkingStatus': 'Checking',
    'view.env.readyStatus': 'Ready',
    'view.env.notReady': 'Not ready',
    'view.env.pass': 'Pass',
    'view.env.fail': 'Fail',
    'view.env.waitPrev': 'Waiting for previous',
    'view.env.copy': 'Copy',
    'view.env.copiedToast': 'Copied ✓',

    // ── CustomProviderManager ──
    'cmp.custom.title': 'Custom Providers',
    'cmp.custom.desc': 'Manage self-hosted LLM gateways and compatible endpoints. Switch the active review model.',
    'cmp.custom.add': 'Add',
    'cmp.custom.empty': 'No custom providers',
    'cmp.custom.addFirst': 'Add custom provider',
    'cmp.custom.currentUse': 'Currently using',
    'cmp.custom.model': 'Model',
    'cmp.custom.edit': 'Edit',
    'cmp.custom.setCurrent': 'Set as current',
    'cmp.custom.delete': 'Delete',

    // ── FileList ──
    'cmp.fileList.pending': 'Pending files',
    'cmp.fileList.noChanges': 'No changed files',
    'cmp.fileList.viewDiff': 'Click to view diff',

    // ── LogViewer ──
    'cmp.log.waiting': 'Waiting for output',

    // ── CommentCard ──
    'cmp.comment.view': 'View',
    'cmp.comment.discard': 'Discard',

    // ── PasswordInput ──
    'cmp.password.hideSecret': 'Hide secret',
    'cmp.password.showSecret': 'Show secret',

    // ── Select ──
    'cmp.select.placeholder': 'Select',

    // ── Extension ──
    'ext.commentController': 'Open Code Review',
    'ext.configPanelTitle': 'Model Configuration',
    'ext.config.legacyDisplayName': 'Legacy LLM Endpoint',
    'ext.comment.threadLabel': 'Code Review',
    'ext.comment.pending': '⏳ [Pending]',
    'ext.comment.noSuggestion': '_💡 No code suggestion, please handle manually_',
    'ext.comment.applyFailedStale': 'Apply failed: code location is stale, please refresh and retry.',
    'ext.comment.applyFailedLocked': 'Apply failed: cannot modify file, check if it is read-only or locked.',
    'ext.comment.statusApplied': '✅ [Applied]',
    'ext.comment.statusDiscarded': '✅ [Discarded]',
    'ext.comment.statusFalsePositive': '✅ [False Positive]',
    'ext.comment.jumpFailed': 'Cannot locate ',
    'ext.comment.jumpNotAFile': ': is not an openable file.',
    'ext.comment.jumpLineUnresolved': 'Cannot jump to {path}: line number could not be resolved.',
    'ext.comment.jumpFileMissing': 'Cannot jump to {path}: file not found in the review snapshot.',
    'ext.comment.applyWorkspaceOnly': 'Apply is only available in Workspace review mode.',
    'ext.deleteProviderConfirm': 'Delete custom provider "{name}"?',
    'ext.deleteProviderConfirmBtn': 'Delete',
    'ext.git.justNow': 'just now',
    'ext.git.hoursAgo': '{h} hours ago',
    'ext.git.hourAgo': '1 hour ago',
    'ext.git.yesterday': 'yesterday',
    'ext.git.daysAgo': '{d} days ago',
    'ext.git.workspaceVsHead': 'Workspace ↔ HEAD',
    'ext.cli.installOk': '✓ Install complete',
    'ext.cli.installFail': '✗ Install failed (exit ',
  },

  'zh-cn': {
    'view.idle.configFirst': '请先配置模型',
    'view.idle.reviewing': '审查中…',
    'view.idle.selectBranch': '请选择对比分支',
    'view.idle.selectCommit': '请选择提交',
    'view.idle.noFiles': '无可审查文件',
    'view.idle.reviewAll': '审查所有变更',
    'view.idle.workspace': '工作区',
    'view.idle.branch': '分支对比',
    'view.idle.commit': '单次提交',
    'view.idle.baseRef': '基础引用',
    'view.idle.targetRef': '目标引用',
    'view.idle.chooseBranch': '选择分支',
    'view.idle.commitHistory': '提交历史',
    'view.idle.customPrompt': '自定义审查提示词（可选）',
    'view.idle.manageCustom': '管理自定义 Provider',
    'view.idle.modelConfig': '模型配置',

    'view.running.reviewLog': '审查日志',
    'view.running.cancel': '取消',

    'view.done.comments': '条评论',
    'view.done.files': '个文件',
    'view.done.processLog': '过程日志',

    'view.empty.noIssues': '未发现问题 · 已通过',
    'view.empty.processLog': '过程日志',

    'view.cancelled.title': '审查已取消',

    'view.failed.title': '审查失败。',
    'view.failed.checkConfig': '请检查模型配置后重试。',
    'view.failed.checkApiKey': '请检查 API Key 和网络连接。',
    'view.failed.retry': '重试',

    'view.config.title': '模型配置',
    'view.config.desc': '连接 LLM Provider 以开始代码审查',
    'view.config.close': '关闭',
    'view.config.step1': '环境检测',
    'view.config.step2': 'Provider 配置',
    'view.config.checking': 'ocr 检测中…',
    'view.config.notInstalled': 'ocr 未安装',
    'view.config.official': '官方 Provider',
    'view.config.custom': '自定义 Provider',
    'view.config.currentUse': '当前使用',
    'view.config.notConfigured': '尚未配置 Provider',
    'view.config.officialLabel': '官方',
    'view.config.customLabel': '自定义',
    'view.config.legacyLabel': 'Legacy',
    'view.config.model': '模型',
    'view.config.customModel': '输入自定义模型…',
    'view.config.apiKey': 'API 密钥',
    'view.config.apiKeyEnvHint': '也可通过环境变量',
    'view.config.apiKeySaved': '已保存（留空保持不变）',
    'view.config.testing': '正在测试连接…',
    'view.config.testOk': '✓ 连接成功',
    'view.config.testFail': '✗ 连接失败',
    'view.config.testFailDetail': '✗ 连接失败：{message}',
    'view.config.previous': '上一步',
    'view.config.test': '测试连接',
    'view.config.save': '保存',
    'view.config.continueProvider': '继续配置 Provider',
    'view.config.providerName': 'Provider 名称',
    'view.config.protocol': '协议',
    'view.config.baseUrl': 'Base URL',
    'view.config.modelList': '模型列表',
    'view.config.modelListPlaceholder': '逗号分隔，如 model-a, model-b',
    'view.config.authHeader': 'Auth Header',
    'view.config.authHeaderHint': 'Anthropic 协议下可选 x-api-key 或 authorization',
    'view.config.authHeaderDefault': '默认 (Authorization)',
    'view.config.backToList': '← 返回列表',
    'view.config.optional': '（可选）',
    'view.config.ocrVersionTooltip': 'Open Code Review CLI 版本',

    'view.env.installing': '正在安装 ocr CLI…',
    'view.env.checking': '正在检测，请稍候…',
    'view.env.ready': '环境已就绪，可继续配置 Provider。',
    'view.env.stepLead': '按顺序完成环境准备，通过一项后再进行下一项。',
    'view.env.nodeHint': '未检测到 Node.js。请前往 nodejs.org 安装 LTS 版本，完成后重启 VS Code。',
    'view.env.npmHint': '未检测到 npm。npm 通常随 Node 一起安装，请确认 Node 安装完整。',
    'view.env.ocrHint': '在终端全局安装 open-code-review，或点击下方「一键安装」。',
    'view.env.oneClickInstall': '一键安装',
    'view.env.redetect': '重新检测',
    'view.env.checkingStatus': '检测中',
    'view.env.readyStatus': '就绪',
    'view.env.notReady': '未就绪',
    'view.env.pass': '通过',
    'view.env.fail': '未通过',
    'view.env.waitPrev': '等待上一步',
    'view.env.copy': '复制',
    'view.env.copiedToast': '已复制到剪贴板 ✓',

    'cmp.custom.title': '自定义 Provider',
    'cmp.custom.desc': '管理自建 LLM 网关与兼容端点，可切换为当前审查模型。',
    'cmp.custom.add': '添加',
    'cmp.custom.empty': '暂无自定义 Provider',
    'cmp.custom.addFirst': '添加自定义 Provider',
    'cmp.custom.currentUse': '当前使用',
    'cmp.custom.model': '模型',
    'cmp.custom.edit': '编辑',
    'cmp.custom.setCurrent': '设为当前',
    'cmp.custom.delete': '删除',

    'cmp.fileList.pending': '待审查文件',
    'cmp.fileList.noChanges': '无变更文件',
    'cmp.fileList.viewDiff': '点击查看 diff',

    'cmp.log.waiting': '等待输出',

    'cmp.comment.view': '查看',
    'cmp.comment.discard': '忽略',

    'cmp.password.hideSecret': '隐藏密钥',
    'cmp.password.showSecret': '显示密钥',

    'cmp.select.placeholder': '请选择',

    'ext.commentController': 'Open Code Review',
    'ext.configPanelTitle': '模型配置',
    'ext.config.legacyDisplayName': 'Legacy LLM 端点',
    'ext.comment.threadLabel': 'Code Review',
    'ext.comment.pending': '⏳ [未处理]',
    'ext.comment.noSuggestion': '_💡 无代码建议，请手动处理_',
    'ext.comment.applyFailedStale': '应用失败：代码位置已失效，请刷新后重试。',
    'ext.comment.applyFailedLocked': '应用失败：无法修改文件，请检查文件是否被占用或处于只读状态。',
    'ext.comment.statusApplied': '✅ [已应用]',
    'ext.comment.statusDiscarded': '✅ [已忽略]',
    'ext.comment.statusFalsePositive': '✅ [已误报]',
    'ext.comment.jumpFailed': '无法定位到 ',
    'ext.comment.jumpNotAFile': '：该路径不是可打开的文件。',
    'ext.comment.jumpLineUnresolved': '无法跳转到 {path}：未能解析行号。',
    'ext.comment.jumpFileMissing': '无法跳转到 {path}：在审查快照中找不到该文件。',
    'ext.comment.applyWorkspaceOnly': '仅工作区审查模式支持应用建议。',
    'ext.deleteProviderConfirm': '确定删除自定义 Provider「{name}」？',
    'ext.deleteProviderConfirmBtn': '删除',
    'ext.git.justNow': '刚刚',
    'ext.git.hoursAgo': '{h} 小时前',
    'ext.git.hourAgo': '1 小时前',
    'ext.git.yesterday': '昨天',
    'ext.git.daysAgo': '{d} 天前',
    'ext.git.workspaceVsHead': '工作区 ↔ HEAD',
    'ext.cli.installOk': '✓ 安装完成',
    'ext.cli.installFail': '✗ 安装失败 (exit ',
  },
};

export function t(locale: SupportedLocale, key: string): string {
  return messages[locale]?.[key] ?? messages.en[key] ?? key;
}

/**
 * Resolve a VS Code locale string to a {@link SupportedLocale}.
 * Only `zh-cn` (case-insensitive) maps to Simplified Chinese;
 * other Chinese variants like `zh-tw` / `zh-hk` fall back to English
 * until their translations are added.
 */
export function resolveLocale(raw: string): SupportedLocale {
  if (raw.toLowerCase() === 'zh-cn') return 'zh-cn';
  return 'en';
}

/**
 * Convert a {@link SupportedLocale} to the BCP 47 HTML `lang` attribute value.
 * `zh-cn` → `zh-CN`, others stay as-is.
 */
export function toHtmlLang(locale: SupportedLocale): string {
  return locale === 'zh-cn' ? 'zh-CN' : locale;
}
