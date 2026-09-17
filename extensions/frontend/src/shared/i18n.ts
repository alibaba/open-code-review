// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

/**
 * Supported display locales for the extension UI.
 * Add new entries here and in the `messages` dictionary below to extend.
 *
 * - `en`    — English (default, fallback for all unrecognized locales)
 * - `zh-cn` — Simplified Chinese (matches VS Code `zh-cn` / `zh-CN`)
 * - `ja-jp` — Japanese (matches VS Code `ja` / `ja-JP`)
 * - `ko-kr` — Korean (matches VS Code `ko` / `ko-KR`)
 * - `ru-ru` — Russian (matches VS Code `ru` / `ru-RU`)
 */
export type SupportedLocale = 'en' | 'zh-cn' | 'ja-jp' | 'ko-kr' | 'ru-ru';

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

  'ja-jp': {
    // ── IdleView ──
    'view.idle.configFirst': '先にモデルを設定してください',
    'view.idle.reviewing': 'レビュー中…',
    'view.idle.selectBranch': '比較するブランチを選択',
    'view.idle.selectCommit': 'コミットを選択',
    'view.idle.noFiles': 'レビュー対象のファイルがありません',
    'view.idle.reviewAll': 'すべての変更をレビュー',
    'view.idle.workspace': 'ワークスペース',
    'view.idle.branch': 'ブランチ比較',
    'view.idle.commit': '単一コミット',
    'view.idle.baseRef': 'ベース参照',
    'view.idle.targetRef': 'ターゲット参照',
    'view.idle.chooseBranch': 'ブランチを選択',
    'view.idle.commitHistory': 'コミット履歴',
    'view.idle.customPrompt': 'カスタムレビュープロンプト（任意）',
    'view.idle.manageCustom': 'カスタムプロバイダーを管理',
    'view.idle.modelConfig': 'モデル設定',

    // ── RunningView ──
    'view.running.reviewLog': 'レビューログ',
    'view.running.cancel': 'キャンセル',

    // ── DoneView ──
    'view.done.comments': '件のコメント',
    'view.done.files': 'ファイル',
    'view.done.processLog': '処理ログ',

    // ── EmptyView ──
    'view.empty.noIssues': '問題は見つかりませんでした · 合格',
    'view.empty.processLog': '処理ログ',

    // ── CancelledView ──
    'view.cancelled.title': 'レビューをキャンセルしました',

    // ── FailedView ──
    'view.failed.title': 'レビューに失敗しました。',
    'view.failed.checkConfig': 'モデル設定を確認して再試行してください。',
    'view.failed.checkApiKey': 'API キーとネットワーク接続を確認してください。',
    'view.failed.retry': '再試行',

    // ── ConfigView ──
    'view.config.title': 'モデル設定',
    'view.config.desc': 'コードレビューを始めるには LLM プロバイダーを接続してください',
    'view.config.close': '閉じる',
    'view.config.step1': '環境のセットアップ',
    'view.config.step2': 'プロバイダー設定',
    'view.config.checking': 'ocr を確認中…',
    'view.config.notInstalled': 'ocr がインストールされていません',
    'view.config.official': '公式プロバイダー',
    'view.config.custom': 'カスタムプロバイダー',
    'view.config.currentUse': '現在使用中',
    'view.config.notConfigured': 'プロバイダーが未設定です',
    'view.config.officialLabel': '公式',
    'view.config.customLabel': 'カスタム',
    'view.config.legacyLabel': 'レガシー',
    'view.config.model': 'モデル',
    'view.config.customModel': 'カスタムモデルを入力…',
    'view.config.apiKey': 'API キー',
    'view.config.apiKeyEnvHint': '環境変数でも設定できます',
    'view.config.apiKeySaved': '保存済み（空欄なら変更しません）',
    'view.config.testing': '接続をテスト中…',
    'view.config.testOk': '✓ 接続できました',
    'view.config.testFail': '✗ 接続に失敗しました',
    'view.config.previous': '前へ',
    'view.config.testFailDetail': '✗ 接続に失敗しました: {message}',
    'view.config.test': '接続をテスト',
    'view.config.save': '保存',
    'view.config.continueProvider': 'プロバイダー設定へ進む',
    'view.config.providerName': 'プロバイダー名',
    'view.config.protocol': 'プロトコル',
    'view.config.baseUrl': 'Base URL',
    'view.config.modelList': 'モデル一覧',
    'view.config.modelListPlaceholder': 'カンマ区切り、例: model-a, model-b',
    'view.config.authHeader': 'Auth ヘッダー',
    'view.config.authHeaderHint': 'Anthropic プロトコルでは x-api-key または authorization を任意で指定',
    'view.config.authHeaderDefault': 'デフォルト (Authorization)',
    'view.config.backToList': '← 一覧に戻る',
    'view.config.optional': '（任意）',
    'view.config.ocrVersionTooltip': 'Open Code Review CLI のバージョン',

    // ── EnvSetupGuide ──
    'view.env.installing': 'ocr CLI をインストール中…',
    'view.env.checking': '確認中です。しばらくお待ちください…',
    'view.env.ready': '環境の準備ができました。プロバイダー設定へ進んでください。',
    'view.env.stepLead': '各ステップを順番に完了し、合格したら次へ進んでください。',
    'view.env.nodeHint': 'Node.js が見つかりません。nodejs.org から LTS 版をインストールし、VS Code を再起動してください。',
    'view.env.npmHint': 'npm が見つかりません。npm は通常 Node.js に同梱されています。Node のインストールを確認してください。',
    'view.env.ocrHint': 'ターミナルで open-code-review をグローバルにインストールするか、下の「ワンクリックインストール」をクリックしてください。',
    'view.env.oneClickInstall': 'ワンクリックインストール',
    'view.env.redetect': '再検出',
    'view.env.checkingStatus': '確認中',
    'view.env.readyStatus': '準備完了',
    'view.env.notReady': '未準備',
    'view.env.pass': '合格',
    'view.env.fail': '不合格',
    'view.env.waitPrev': '前のステップを待機中',
    'view.env.copy': 'コピー',
    'view.env.copiedToast': 'コピーしました ✓',

    // ── CustomProviderManager ──
    'cmp.custom.title': 'カスタムプロバイダー',
    'cmp.custom.desc': 'セルフホスト型の LLM ゲートウェイや互換エンドポイントを管理し、使用するレビューモデルを切り替えます。',
    'cmp.custom.add': '追加',
    'cmp.custom.empty': 'カスタムプロバイダーはありません',
    'cmp.custom.addFirst': 'カスタムプロバイダーを追加',
    'cmp.custom.currentUse': '現在使用中',
    'cmp.custom.model': 'モデル',
    'cmp.custom.edit': '編集',
    'cmp.custom.setCurrent': '現在の設定にする',
    'cmp.custom.delete': '削除',

    // ── FileList ──
    'cmp.fileList.pending': 'レビュー待ちファイル',
    'cmp.fileList.noChanges': '変更されたファイルはありません',
    'cmp.fileList.viewDiff': 'クリックで差分を表示',

    // ── LogViewer ──
    'cmp.log.waiting': '出力を待機中',

    // ── CommentCard ──
    'cmp.comment.view': '表示',
    'cmp.comment.discard': '無視',

    // ── PasswordInput ──
    'cmp.password.hideSecret': 'シークレットを隠す',
    'cmp.password.showSecret': 'シークレットを表示',

    // ── Select ──
    'cmp.select.placeholder': '選択してください',

    // ── Extension ──
    'ext.commentController': 'Open Code Review',
    'ext.configPanelTitle': 'モデル設定',
    'ext.config.legacyDisplayName': 'レガシー LLM エンドポイント',
    'ext.comment.threadLabel': 'Code Review',
    'ext.comment.pending': '⏳ [未対応]',
    'ext.comment.noSuggestion': '_💡 コード提案はありません。手動で対応してください_',
    'ext.comment.applyFailedStale': '適用に失敗しました: コードの位置が古くなっています。更新して再試行してください。',
    'ext.comment.applyFailedLocked': '適用に失敗しました: ファイルを変更できません。読み取り専用またはロックされていないか確認してください。',
    'ext.comment.statusApplied': '✅ [適用済み]',
    'ext.comment.statusDiscarded': '✅ [無視済み]',
    'ext.comment.statusFalsePositive': '✅ [誤検出]',
    'ext.comment.jumpFailed': '次の場所に移動できません: ',
    'ext.comment.jumpNotAFile': ': 開けるファイルではありません。',
    'ext.comment.jumpLineUnresolved': '{path} に移動できません: 行番号を特定できませんでした。',
    'ext.comment.jumpFileMissing': '{path} に移動できません: レビューのスナップショットにファイルが見つかりません。',
    'ext.comment.applyWorkspaceOnly': '適用はワークスペースレビューモードでのみ使用できます。',
    'ext.deleteProviderConfirm': 'カスタムプロバイダー「{name}」を削除しますか？',
    'ext.deleteProviderConfirmBtn': '削除',
    'ext.git.justNow': 'たった今',
    'ext.git.hoursAgo': '{h} 時間前',
    'ext.git.hourAgo': '1 時間前',
    'ext.git.yesterday': '昨日',
    'ext.git.daysAgo': '{d} 日前',
    'ext.git.workspaceVsHead': 'ワークスペース ↔ HEAD',
    'ext.cli.installOk': '✓ インストールが完了しました',
    'ext.cli.installFail': '✗ インストールに失敗しました (exit ',
  },

  'ko-kr': {
    // ── IdleView ──
    'view.idle.configFirst': '먼저 모델을 설정하세요',
    'view.idle.reviewing': '검토 중…',
    'view.idle.selectBranch': '비교할 브랜치 선택',
    'view.idle.selectCommit': '커밋 선택',
    'view.idle.noFiles': '검토할 파일이 없습니다',
    'view.idle.reviewAll': '모든 변경 사항 검토',
    'view.idle.workspace': '워크스페이스',
    'view.idle.branch': '브랜치 비교',
    'view.idle.commit': '단일 커밋',
    'view.idle.baseRef': '기준 참조',
    'view.idle.targetRef': '대상 참조',
    'view.idle.chooseBranch': '브랜치 선택',
    'view.idle.commitHistory': '커밋 기록',
    'view.idle.customPrompt': '사용자 지정 검토 프롬프트(선택)',
    'view.idle.manageCustom': '사용자 지정 프로바이더 관리',
    'view.idle.modelConfig': '모델 설정',

    // ── RunningView ──
    'view.running.reviewLog': '검토 로그',
    'view.running.cancel': '취소',

    // ── DoneView ──
    'view.done.comments': '개 댓글',
    'view.done.files': '개 파일',
    'view.done.processLog': '처리 로그',

    // ── EmptyView ──
    'view.empty.noIssues': '문제를 찾지 못했습니다 · 통과',
    'view.empty.processLog': '처리 로그',

    // ── CancelledView ──
    'view.cancelled.title': '검토를 취소했습니다',

    // ── FailedView ──
    'view.failed.title': '검토에 실패했습니다.',
    'view.failed.checkConfig': '모델 설정을 확인한 뒤 다시 시도하세요.',
    'view.failed.checkApiKey': 'API 키와 네트워크 연결을 확인하세요.',
    'view.failed.retry': '다시 시도',

    // ── ConfigView ──
    'view.config.title': '모델 설정',
    'view.config.desc': '코드 검토를 시작하려면 LLM 프로바이더를 연결하세요',
    'view.config.close': '닫기',
    'view.config.step1': '환경 설정',
    'view.config.step2': '프로바이더 설정',
    'view.config.checking': 'ocr 확인 중…',
    'view.config.notInstalled': 'ocr가 설치되지 않았습니다',
    'view.config.official': '공식 프로바이더',
    'view.config.custom': '사용자 지정 프로바이더',
    'view.config.currentUse': '현재 사용 중',
    'view.config.notConfigured': '설정된 프로바이더가 없습니다',
    'view.config.officialLabel': '공식',
    'view.config.customLabel': '사용자 지정',
    'view.config.legacyLabel': '레거시',
    'view.config.model': '모델',
    'view.config.customModel': '사용자 지정 모델 입력…',
    'view.config.apiKey': 'API 키',
    'view.config.apiKeyEnvHint': '환경 변수로도 설정할 수 있습니다',
    'view.config.apiKeySaved': '저장됨(비워 두면 유지)',
    'view.config.testing': '연결을 테스트하는 중…',
    'view.config.testOk': '✓ 연결되었습니다',
    'view.config.testFail': '✗ 연결에 실패했습니다',
    'view.config.previous': '이전',
    'view.config.testFailDetail': '✗ 연결에 실패했습니다: {message}',
    'view.config.test': '연결 테스트',
    'view.config.save': '저장',
    'view.config.continueProvider': '프로바이더 설정으로 계속',
    'view.config.providerName': '프로바이더 이름',
    'view.config.protocol': '프로토콜',
    'view.config.baseUrl': 'Base URL',
    'view.config.modelList': '모델 목록',
    'view.config.modelListPlaceholder': '쉼표로 구분, 예: model-a, model-b',
    'view.config.authHeader': 'Auth 헤더',
    'view.config.authHeaderHint': 'Anthropic 프로토콜에서는 x-api-key 또는 authorization을 선택적으로 지정',
    'view.config.authHeaderDefault': '기본값 (Authorization)',
    'view.config.backToList': '← 목록으로 돌아가기',
    'view.config.optional': '(선택)',
    'view.config.ocrVersionTooltip': 'Open Code Review CLI 버전',

    // ── EnvSetupGuide ──
    'view.env.installing': 'ocr CLI 설치 중…',
    'view.env.checking': '확인 중입니다. 잠시만 기다려 주세요…',
    'view.env.ready': '환경이 준비되었습니다. 프로바이더 설정으로 계속하세요.',
    'view.env.stepLead': '각 단계를 순서대로 완료하고, 통과한 뒤 다음 단계로 이동하세요.',
    'view.env.nodeHint': 'Node.js를 찾을 수 없습니다. nodejs.org에서 LTS 버전을 설치한 뒤 VS Code를 다시 시작하세요.',
    'view.env.npmHint': 'npm을 찾을 수 없습니다. npm은 보통 Node.js에 포함되어 있으니 Node 설치를 확인하세요.',
    'view.env.ocrHint': '터미널에서 open-code-review를 전역 설치하거나 아래 "원클릭 설치"를 클릭하세요.',
    'view.env.oneClickInstall': '원클릭 설치',
    'view.env.redetect': '다시 감지',
    'view.env.checkingStatus': '확인 중',
    'view.env.readyStatus': '준비됨',
    'view.env.notReady': '준비 안 됨',
    'view.env.pass': '통과',
    'view.env.fail': '실패',
    'view.env.waitPrev': '이전 단계 대기 중',
    'view.env.copy': '복사',
    'view.env.copiedToast': '복사했습니다 ✓',

    // ── CustomProviderManager ──
    'cmp.custom.title': '사용자 지정 프로바이더',
    'cmp.custom.desc': '자체 호스팅 LLM 게이트웨이와 호환 엔드포인트를 관리하고 사용할 검토 모델을 전환합니다.',
    'cmp.custom.add': '추가',
    'cmp.custom.empty': '사용자 지정 프로바이더가 없습니다',
    'cmp.custom.addFirst': '사용자 지정 프로바이더 추가',
    'cmp.custom.currentUse': '현재 사용 중',
    'cmp.custom.model': '모델',
    'cmp.custom.edit': '편집',
    'cmp.custom.setCurrent': '현재로 설정',
    'cmp.custom.delete': '삭제',

    // ── FileList ──
    'cmp.fileList.pending': '검토 대기 파일',
    'cmp.fileList.noChanges': '변경된 파일이 없습니다',
    'cmp.fileList.viewDiff': '클릭하여 diff 보기',

    // ── LogViewer ──
    'cmp.log.waiting': '출력을 기다리는 중',

    // ── CommentCard ──
    'cmp.comment.view': '보기',
    'cmp.comment.discard': '무시',

    // ── PasswordInput ──
    'cmp.password.hideSecret': '시크릿 숨기기',
    'cmp.password.showSecret': '시크릿 표시',

    // ── Select ──
    'cmp.select.placeholder': '선택하세요',

    // ── Extension ──
    'ext.commentController': 'Open Code Review',
    'ext.configPanelTitle': '모델 설정',
    'ext.config.legacyDisplayName': '레거시 LLM 엔드포인트',
    'ext.comment.threadLabel': 'Code Review',
    'ext.comment.pending': '⏳ [대기 중]',
    'ext.comment.noSuggestion': '_💡 코드 제안이 없습니다. 직접 처리해 주세요_',
    'ext.comment.applyFailedStale': '적용 실패: 코드 위치가 오래되었습니다. 새로 고친 뒤 다시 시도하세요.',
    'ext.comment.applyFailedLocked': '적용 실패: 파일을 수정할 수 없습니다. 읽기 전용이거나 잠겨 있는지 확인하세요.',
    'ext.comment.statusApplied': '✅ [적용됨]',
    'ext.comment.statusDiscarded': '✅ [무시됨]',
    'ext.comment.statusFalsePositive': '✅ [오탐]',
    'ext.comment.jumpFailed': '위치를 찾을 수 없습니다: ',
    'ext.comment.jumpNotAFile': ': 열 수 있는 파일이 아닙니다.',
    'ext.comment.jumpLineUnresolved': '{path}(으)로 이동할 수 없습니다: 줄 번호를 확인할 수 없습니다.',
    'ext.comment.jumpFileMissing': '{path}(으)로 이동할 수 없습니다: 검토 스냅샷에서 파일을 찾을 수 없습니다.',
    'ext.comment.applyWorkspaceOnly': '적용은 워크스페이스 검토 모드에서만 사용할 수 있습니다.',
    'ext.deleteProviderConfirm': '사용자 지정 프로바이더 "{name}"을(를) 삭제할까요?',
    'ext.deleteProviderConfirmBtn': '삭제',
    'ext.git.justNow': '방금',
    'ext.git.hoursAgo': '{h}시간 전',
    'ext.git.hourAgo': '1시간 전',
    'ext.git.yesterday': '어제',
    'ext.git.daysAgo': '{d}일 전',
    'ext.git.workspaceVsHead': '워크스페이스 ↔ HEAD',
    'ext.cli.installOk': '✓ 설치 완료',
    'ext.cli.installFail': '✗ 설치 실패 (exit ',
  },

  'ru-ru': {
    // ── IdleView ──
    'view.idle.configFirst': 'Сначала настройте модель',
    'view.idle.reviewing': 'Проверка…',
    'view.idle.selectBranch': 'Выберите ветку для сравнения',
    'view.idle.selectCommit': 'Выберите коммит',
    'view.idle.noFiles': 'Нет файлов для проверки',
    'view.idle.reviewAll': 'Проверить все изменения',
    'view.idle.workspace': 'Рабочая копия',
    'view.idle.branch': 'Сравнение веток',
    'view.idle.commit': 'Один коммит',
    'view.idle.baseRef': 'Базовая ссылка',
    'view.idle.targetRef': 'Целевая ссылка',
    'view.idle.chooseBranch': 'Выбрать ветку',
    'view.idle.commitHistory': 'История коммитов',
    'view.idle.customPrompt': 'Свой промпт для проверки (необязательно)',
    'view.idle.manageCustom': 'Управление своими провайдерами',
    'view.idle.modelConfig': 'Настройка модели',

    // ── RunningView ──
    'view.running.reviewLog': 'Журнал проверки',
    'view.running.cancel': 'Отмена',

    // ── DoneView ──
    'view.done.comments': 'комм.',
    'view.done.files': 'файл(ов)',
    'view.done.processLog': 'Журнал выполнения',

    // ── EmptyView ──
    'view.empty.noIssues': 'Проблем не найдено · Проверка пройдена',
    'view.empty.processLog': 'Журнал выполнения',

    // ── CancelledView ──
    'view.cancelled.title': 'Проверка отменена',

    // ── FailedView ──
    'view.failed.title': 'Проверка не удалась.',
    'view.failed.checkConfig': 'Проверьте настройки модели и повторите попытку.',
    'view.failed.checkApiKey': 'Проверьте API-ключ и сетевое подключение.',
    'view.failed.retry': 'Повторить',

    // ── ConfigView ──
    'view.config.title': 'Настройка модели',
    'view.config.desc': 'Подключите провайдера LLM, чтобы начать проверку кода',
    'view.config.close': 'Закрыть',
    'view.config.step1': 'Подготовка окружения',
    'view.config.step2': 'Настройка провайдера',
    'view.config.checking': 'Проверка ocr…',
    'view.config.notInstalled': 'ocr не установлен',
    'view.config.official': 'Официальный провайдер',
    'view.config.custom': 'Свой провайдер',
    'view.config.currentUse': 'Используется сейчас',
    'view.config.notConfigured': 'Провайдер не настроен',
    'view.config.officialLabel': 'Официальный',
    'view.config.customLabel': 'Свой',
    'view.config.legacyLabel': 'Устаревший',
    'view.config.model': 'Модель',
    'view.config.customModel': 'Введите свою модель…',
    'view.config.apiKey': 'API-ключ',
    'view.config.apiKeyEnvHint': 'Также задаётся переменной окружения',
    'view.config.apiKeySaved': 'Сохранено (оставьте пустым, чтобы не менять)',
    'view.config.testing': 'Проверка подключения…',
    'view.config.testOk': '✓ Подключено',
    'view.config.testFail': '✗ Не удалось подключиться',
    'view.config.previous': 'Назад',
    'view.config.testFailDetail': '✗ Не удалось подключиться: {message}',
    'view.config.test': 'Проверить подключение',
    'view.config.save': 'Сохранить',
    'view.config.continueProvider': 'Перейти к настройке провайдера',
    'view.config.providerName': 'Название провайдера',
    'view.config.protocol': 'Протокол',
    'view.config.baseUrl': 'Base URL',
    'view.config.modelList': 'Список моделей',
    'view.config.modelListPlaceholder': 'Через запятую, например: model-a, model-b',
    'view.config.authHeader': 'Заголовок Auth',
    'view.config.authHeaderHint': 'Для протокола Anthropic можно указать x-api-key или authorization',
    'view.config.authHeaderDefault': 'По умолчанию (Authorization)',
    'view.config.backToList': '← Назад к списку',
    'view.config.optional': '(необязательно)',
    'view.config.ocrVersionTooltip': 'Версия Open Code Review CLI',

    // ── EnvSetupGuide ──
    'view.env.installing': 'Установка ocr CLI…',
    'view.env.checking': 'Идёт проверка, подождите…',
    'view.env.ready': 'Окружение готово. Переходите к настройке провайдера.',
    'view.env.stepLead': 'Выполняйте шаги по порядку и переходите к следующему после успешного прохождения.',
    'view.env.nodeHint': 'Node.js не найден. Установите LTS-версию с сайта nodejs.org и перезапустите VS Code.',
    'view.env.npmHint': 'npm не найден. Обычно npm устанавливается вместе с Node.js — проверьте установку Node.',
    'view.env.ocrHint': 'Установите open-code-review глобально в терминале или нажмите «Установить в один клик» ниже.',
    'view.env.oneClickInstall': 'Установить в один клик',
    'view.env.redetect': 'Проверить снова',
    'view.env.checkingStatus': 'Проверка',
    'view.env.readyStatus': 'Готово',
    'view.env.notReady': 'Не готово',
    'view.env.pass': 'Пройдено',
    'view.env.fail': 'Не пройдено',
    'view.env.waitPrev': 'Ожидание предыдущего шага',
    'view.env.copy': 'Копировать',
    'view.env.copiedToast': 'Скопировано ✓',

    // ── CustomProviderManager ──
    'cmp.custom.title': 'Свои провайдеры',
    'cmp.custom.desc': 'Управляйте собственными шлюзами LLM и совместимыми эндпоинтами. Переключайте модель, которой проверяется код.',
    'cmp.custom.add': 'Добавить',
    'cmp.custom.empty': 'Своих провайдеров нет',
    'cmp.custom.addFirst': 'Добавить своего провайдера',
    'cmp.custom.currentUse': 'Используется сейчас',
    'cmp.custom.model': 'Модель',
    'cmp.custom.edit': 'Изменить',
    'cmp.custom.setCurrent': 'Сделать текущим',
    'cmp.custom.delete': 'Удалить',

    // ── FileList ──
    'cmp.fileList.pending': 'Файлы для проверки',
    'cmp.fileList.noChanges': 'Изменённых файлов нет',
    'cmp.fileList.viewDiff': 'Нажмите, чтобы посмотреть diff',

    // ── LogViewer ──
    'cmp.log.waiting': 'Ожидание вывода',

    // ── CommentCard ──
    'cmp.comment.view': 'Открыть',
    'cmp.comment.discard': 'Отклонить',

    // ── PasswordInput ──
    'cmp.password.hideSecret': 'Скрыть секрет',
    'cmp.password.showSecret': 'Показать секрет',

    // ── Select ──
    'cmp.select.placeholder': 'Выберите',

    // ── Extension ──
    'ext.commentController': 'Open Code Review',
    'ext.configPanelTitle': 'Настройка модели',
    'ext.config.legacyDisplayName': 'Устаревший эндпоинт LLM',
    'ext.comment.threadLabel': 'Code Review',
    'ext.comment.pending': '⏳ [Ожидает]',
    'ext.comment.noSuggestion': '_💡 Предложений по коду нет, обработайте вручную_',
    'ext.comment.applyFailedStale': 'Не удалось применить: позиция кода устарела. Обновите и повторите попытку.',
    'ext.comment.applyFailedLocked': 'Не удалось применить: файл нельзя изменить. Проверьте, не доступен ли он только для чтения и не заблокирован ли.',
    'ext.comment.statusApplied': '✅ [Применено]',
    'ext.comment.statusDiscarded': '✅ [Отклонено]',
    'ext.comment.statusFalsePositive': '✅ [Ложное срабатывание]',
    'ext.comment.jumpFailed': 'Не удалось найти ',
    'ext.comment.jumpNotAFile': ': не является открываемым файлом.',
    'ext.comment.jumpLineUnresolved': 'Не удалось перейти к {path}: не удалось определить номер строки.',
    'ext.comment.jumpFileMissing': 'Не удалось перейти к {path}: файл не найден в снимке проверки.',
    'ext.comment.applyWorkspaceOnly': 'Применение доступно только в режиме проверки рабочей копии.',
    'ext.deleteProviderConfirm': 'Удалить своего провайдера "{name}"?',
    'ext.deleteProviderConfirmBtn': 'Удалить',
    'ext.git.justNow': 'только что',
    'ext.git.hoursAgo': '{h} ч. назад',
    'ext.git.hourAgo': '1 час назад',
    'ext.git.yesterday': 'вчера',
    'ext.git.daysAgo': '{d} дн. назад',
    'ext.git.workspaceVsHead': 'Рабочая копия ↔ HEAD',
    'ext.cli.installOk': '✓ Установка завершена',
    'ext.cli.installFail': '✗ Не удалось установить (код ',
  },
};

export function t(locale: SupportedLocale, key: string): string {
  return messages[locale]?.[key] ?? messages.en[key] ?? key;
}

/**
 * Resolve a VS Code locale string to a {@link SupportedLocale}.
 * VS Code reports the display language either as a plain language code
 * (`ja`, `ko`, `ru`) or as a language/region pair (`zh-cn`), so both spellings
 * map to the same translation. Matching is case-insensitive; variants without
 * a translation, such as `zh-tw` / `zh-hk`, fall back to English until their
 * translations are added.
 */
export function resolveLocale(raw: string): SupportedLocale {
  switch (raw.toLowerCase()) {
    case 'zh-cn':
      return 'zh-cn';
    case 'ja':
    case 'ja-jp':
      return 'ja-jp';
    case 'ko':
    case 'ko-kr':
      return 'ko-kr';
    case 'ru':
    case 'ru-ru':
      return 'ru-ru';
    default:
      return 'en';
  }
}

/**
 * Convert a {@link SupportedLocale} to the BCP 47 HTML `lang` attribute value.
 * The region subtag is upper-cased: `zh-cn` → `zh-CN`, `ja-jp` → `ja-JP`;
 * a locale without a region (`en`) stays as-is.
 */
export function toHtmlLang(locale: SupportedLocale): string {
  const [language, region] = locale.split('-');
  return region ? `${language}-${region.toUpperCase()}` : language;
}
