# OCR VSCode 插件 — 架构设计文档

**日期**：2026-06-08
**状态**：已通过设计评审，待编写实现计划
**关联文档**：[UI 设计](./2026-06-04-ocr-vscode-ui-design.md)、原型 `prototype.html`

---

## 1. 背景与目标

### 1.1 是什么

基于 [open-code-review](https://github.com/alibaba/open-code-review)（下称 OCR）CLI 构建一个 VSCode 插件。插件以 `prototype.html` 为产品体验蓝本，底层全部通过调用 `ocr` CLI 完成代码审查的完整流程。

### 1.2 双重目标

1. **独立开源项目**：`open-code-review-vscode` 可独立打包成 `.vsix` 发布运行。
2. **可被复用的 CR 模块**：通过一键同步脚本把核心代码复制进 `aone-copilot-vscode`，使两个项目共享同一份 CR 实现。**新方案稳定后，aone-copilot 现有的 `src/codeReview/` 老方案将下线。**

核心诉求：**我只迭代 OCR-vscode 一个项目，代码可以在两边被消费。**

---

## 2. 总体架构

采用 **Monolithic WebView + Thin Extension Host** 方案：
- WebView 是一个独立构建的 SPA（Preact），还原原型的全部视觉与交互。
- Extension Host 层轻薄，只负责 CLI 调用、文件系统、Git 操作、编辑器评论。
- 两者通过 `postMessage` 通信，用 TypeScript 共享类型保证类型安全。

### 2.1 项目结构

```
open-code-review-vscode/
├── src/
│   ├── extension/                  ← Extension Host（Node.js 环境）
│   │   ├── extension.ts            ← 独立运行入口（薄包装，调用 activateOcr）
│   │   ├── index.ts                ← 公共导出入口（activateOcr + OcrAdapter）
│   │   ├── services/
│   │   │   ├── CliService.ts       ← spawn ocr 进程，流式解析输出
│   │   │   ├── ConfigService.ts    ← 读写 ~/.opencodereview/config.json
│   │   │   ├── GitService.ts       ← git 状态感知（分支/commit/diff files）
│   │   │   └── ReviewSession.ts    ← 审查会话状态机 + 生命周期管理
│   │   ├── providers/
│   │   │   ├── SidebarProvider.ts  ← WebviewViewProvider 注册
│   │   │   └── CommentProvider.ts  ← Comment API（结果展示 + 应用/忽略/误报）
│   │   └── commands.ts             ← 命令注册入口
│   │
│   ├── webview/                    ← WebView SPA（浏览器环境，独立构建）
│   │   ├── index.tsx               ← SPA 入口
│   │   ├── App.tsx                 ← 根组件，路由状态
│   │   ├── views/
│   │   │   ├── IdleView.tsx        ← 空闲态（模式选择 + 文件列表 + 开始按钮）
│   │   │   ├── RunningView.tsx     ← 审查中（流式日志 + 取消）
│   │   │   ├── DoneView.tsx        ← 完成（评论列表 + 操作）
│   │   │   ├── EmptyView.tsx       ← 无问题
│   │   │   ├── FailedView.tsx      ← 失败 + 重试
│   │   │   └── ConfigView.tsx      ← 配置管理（引导/列表/表单）
│   │   ├── components/
│   │   │   ├── StatusBar.tsx
│   │   │   ├── ModelDropdown.tsx
│   │   │   ├── FileList.tsx
│   │   │   ├── LogViewer.tsx
│   │   │   └── CommentCard.tsx
│   │   ├── bridge.ts               ← postMessage 发送/接收封装
│   │   └── styles/                 ← CSS（沿用原型 silent-night 风格）
│   │
│   └── shared/                     ← 双端共享（类型 + 协议定义，不依赖 vscode）
│       ├── types.ts                ← ReviewComment, ReviewMode, OcrConfig 等
│       ├── messages.ts             ← postMessage 协议（请求/响应类型）
│       └── constants.ts            ← 状态枚举、命令 ID
│
├── scripts/
│   └── sync-to-aone.js             ← 一键同步脚本
├── package.json                    ← VSCode extension manifest
├── tsconfig.extension.json         ← Extension Host 编译配置（commonjs + Node）
├── tsconfig.webview.json           ← WebView 编译配置（esnext + DOM）
├── webpack.config.js               ← 双入口构建（extension + webview）
└── reference/open-code-review/     ← CLI 源码（只读参考，不打包）
```

### 2.2 关键技术决策

| 项 | 选择 | 理由 |
|----|------|------|
| WebView 框架 | **Preact** | 体积小（~4KB），API 兼容 React |
| 样式 | 原型 CSS 直接迁移 | silent-night 风格已成型 |
| 构建 | webpack 双入口 | `extension.ts` + `webview/index.tsx` |
| 图标 | VSCode Codicons | 原生一致性 |
| CLI 分发 | **依赖用户全局安装** | 插件只检测和引导，体积小 |
| 状态管理 | Preact signals / useReducer + Context | 轻量，不引入 Redux |

---

## 3. Extension Host 层

### 3.1 CliService — CLI 进程管理

职责：spawn `ocr` 进程，流式解析输出，支持取消。

```typescript
interface CliRunOptions {
  mode: 'workspace' | 'branch' | 'commit';
  from?: string;
  to?: string;
  commit?: string;
  customPrompt?: string;   // 自定义审查提示词 → 通过 --background 传入
  format: 'json';
  concurrency?: number;
}

interface CliResult {
  status: 'success' | 'completed_with_errors' | 'completed_with_warnings' | 'skipped';
  comments: ReviewComment[];
  warnings: AgentWarning[];
  summary: { filesReviewed: number; totalTokens: number; elapsed: string; };
}
```

进程管理策略：
- `child_process.spawn`，流式读取 stderr（日志行）和 stdout（最终 JSON）。
- stderr 中的 `[ocr]`/`[llm]` 行实时推送给 WebView。
- stdout 在进程退出后整体解析 JSON 作为结果（对应 CLI `--format json`）。
- 取消：`process.kill(pid, 'SIGTERM')`；超时兜底强制 SIGKILL。

**CLI 命令映射**：
- workspace 模式 → `ocr review --format json`
- branch 模式 → `ocr review --from <from> --to <to> --format json`
- commit 模式 → `ocr review --commit <hash> --format json`
- 自定义 prompt → 追加 `--background <text>`
- 连通性测试 → `ocr llm test`

### 3.2 文件筛选策略

**决策：文件列表只做"预览/确认"用途，不真正过滤 CLI 行为。**

- 理由：CLI 内部有完善的文件过滤逻辑（规则链 + include/exclude + 二进制/扩展名过滤）。
- UI 侧用 `ocr review --preview` 获取将被审查的文件列表展示给用户确认。
- 勾选状态仅为视觉确认，不阻断审查。
- 若未来需要精细控制，再给 CLI 贡献 `--include` 参数。

### 3.3 ConfigService — 配置读写

**决策：插件配置直接操作 CLI config，单一数据源。**

```typescript
interface OcrConfig {
  llm: { url: string; auth_token: string; model: string; use_anthropic: boolean; };
  language: string;
}
```

- 读取：直接解析 `~/.opencodereview/config.json`。
- 写入：调用 `ocr config set <key> <value>` 子进程，成功后重新读取文件刷新 UI。
- 启动时读取 config 判断是否需要进入配置引导流程。

### 3.4 GitService — Git 状态

```typescript
interface GitState {
  branches: string[];
  currentBranch: string;
  recentCommits: CommitInfo[];
  workspaceFiles: FileChange[];   // staged + unstaged + untracked
}
```

- 使用 VSCode 内置 Git extension API（`vscode.extensions.getExtension('vscode.git')`）。
- 模式切换时动态加载对应数据。

### 3.5 ReviewSession — 状态机

```
States: idle → running → done | empty | cancelled | failed
         ↑                         |
         └─────── (重试/新审查) ────┘
```

- 每次审查创建一个 session 实例，持有 CLI 子进程引用。
- 状态变更与日志行通过 postMessage 实时通知 WebView。

### 3.6 CommentProvider — 编辑器评论

复用 aone-copilot 现有 Comment API 模式（`src/codeReview/codeReviewProvider.ts` 可作为实现参考）：
- 审查完成后将 `ReviewComment[]` 转为 CommentThread。
- 支持操作：应用建议（替换代码）、忽略、标记误报。
- 点击侧边栏 CommentCard 触发 `jumpToComment(index)`。
- 行号偏移追踪（成熟实现可搬运）。

---

## 4. WebView 层与通信协议

### 4.1 postMessage 协议（shared/messages.ts）

```typescript
// WebView → Extension（请求）
type WebviewToHost =
  | { type: 'ready' }
  | { type: 'getGitState'; mode: ReviewMode }
  | { type: 'startReview'; options: CliRunOptions }
  | { type: 'cancelReview' }
  | { type: 'getConfig' }
  | { type: 'setConfig'; key: string; value: string }
  | { type: 'testConnection' }
  | { type: 'jumpToComment'; index: number }
  | { type: 'commentAction'; index: number; action: 'apply'|'discard'|'falsePositive' };

// Extension → WebView（响应/推送）
type HostToWebview =
  | { type: 'init'; config: OcrConfig | null; gitState: GitState }
  | { type: 'gitState'; gitState: GitState }
  | { type: 'logLine'; line: LogLine }
  | { type: 'stateChange'; state: ReviewState }
  | { type: 'reviewDone'; result: CliResult }
  | { type: 'config'; config: OcrConfig | null }
  | { type: 'connectionResult'; ok: boolean; message?: string }
  | { type: 'commentSync'; comments: CommentSyncState[] };
```

### 4.2 bridge.ts — 通信封装

```typescript
const bridge = {
  post(msg: WebviewToHost): void,
  on<T>(type: string, handler: (msg: T) => void): Dispose,
  request<Req, Res>(msg: Req): Promise<Res>,   // 带 requestId 的请求-响应
};
```

### 4.3 状态管理

- 单一 `AppState`：`{ view, config, gitState, session: { state, logs, result } }`。
- 所有 HostToWebview 消息归约为 state 更新，视图纯函数渲染。

### 4.4 编辑器评论与 WebView 双向同步

- 侧边栏 CommentCard 点操作 → postMessage → CommentProvider 执行 → 回传 `commentSync` 更新卡片。
- 编辑器 CommentThread 点操作 → CommentProvider 执行 → 回传 `commentSync` 同步侧边栏。
- 两侧共享同一份 comment 数据（以 index 为 key），保证状态一致。

---

## 5. 代码复用方案（一键复制 + 外层适配）

### 5.1 核心原则

- **OCR-vscode 自包含**：所有 CR 代码集中在 `src/` 内，不依赖 aone 特有代码，可独立打包运行。
- **单向同步**：`open-code-review-vscode` 是唯一真源。同步脚本把核心代码复制进 `aone-copilot-vscode/src/ocr/`。
- **外层适配**：aone 不修改被复制的代码，只在 `src/ocr/` 之外写适配代码。
- 复制进来的代码视为**只读 vendor 目录**，不在 aone 仓库手改。
- **不用 submodule**：copy 产物是普通文件，aone 仓库自包含，CI/构建无额外依赖，心智负担低。

### 5.2 自包含模块入口

```typescript
// src/extension/index.ts （会被一起 copy 进 aone）
export interface OcrAdapter {
  extensionUri: vscode.Uri;
  telemetry?: (event: string, data: Record<string, unknown>) => void;
  cliPath?: string;            // aone 可指定自带 ocr 路径
}

export function activateOcr(ctx: vscode.ExtensionContext, adapter?: OcrAdapter): void;
export function deactivateOcr(): void;
```

- adapter 是**可选**的，保持简单，不强制注入。
- **命名空间不做运行时参数化**，固定用 `ocr.*`。理由：老方案将下线，用户不会同时装两个插件，无冲突风险。

### 5.3 独立运行入口

```typescript
// src/extension/extension.ts
export function activate(ctx: vscode.ExtensionContext) {
  activateOcr(ctx);   // adapter 用默认值
}
```

### 5.4 aone 侧接入（一次性，量很小）

```typescript
// aone-copilot-vscode/src/extension.ts
import { activateOcr } from './ocr/extension/index';

export function activate(ctx: vscode.ExtensionContext) {
  // ... aone 自己的初始化 ...
  activateOcr(ctx, { extensionUri: ctx.extensionUri, telemetry: report });
}
```

适配工作清单：
1. `src/extension.ts` 加一行 `activateOcr(ctx, {...})`。
2. `package.json` 由同步脚本自动 merge OCR 的 contributes。
3. `webpack.config.js` 加 OCR webview 构建入口。
4. 老的 `src/codeReview/` 保留到新方案稳定后删除。

### 5.5 同步脚本（scripts/sync-to-aone.js）

```
node scripts/sync-to-aone.js --target ../aone-copilot-vscode/src/ocr

做的事：
1. 清空目标目录
2. copy src/ 下源码（排除独立入口 extension.ts、test 文件等）
3. copy webview 相关源码（由 aone webpack 一起构建）
4. 自动 merge contributes（命令/视图/菜单）进 aone 的 package.json（幂等处理）
5. 写 .synced-version 标记文件（记录来源 commit，便于追溯）
```

**contributes 自动 merge 要点**：
- 幂等：重复运行结果一致，不产生重复条目。
- 用明确的标记区分 OCR 贡献的条目（如 ID 前缀 `ocr.`），merge 时先移除旧的 OCR 条目再插入新的。
- 保留 aone 自己的贡献点不受影响。

### 5.6 需要参数化/隔离的资源

| 资源 | 处理方式 |
|------|---------|
| 命令 ID | 固定 `ocr.*` 前缀 |
| 视图 ID | 固定 `ocr.sidebar` 等 |
| Comment Controller ID | 固定 `ocr-review` |
| WebView assets | 通过 `adapter.extensionUri` 定位（默认用 ctx.extensionUri） |
| 配置存储 | 共享 CLI config `~/.opencodereview/`，不变 |
| 埋点 | `adapter.telemetry` 可选注入；独立运行时无埋点 |

---

## 6. 第一版范围

**全量实现**原型中的所有功能：

- 三种审查模式（workspace / branch / commit）
- 文件列表预览与确认
- 自定义审查提示词
- 流式日志展示（running 态）
- 结果展示（Comment API + 侧边栏列表，双向同步）
- 空/取消/失败状态处理 + 重试
- 配置管理（引导 / 列表 / 表单，对接 CLI config）
- 模型切换（下拉，对接 CLI config）
- 连通性测试

---

## 7. 数据结构参考（来自 CLI）

CLI 的 `LlmComment`（`internal/model/review.go`）：

```go
type LlmComment struct {
  Path           string `json:"path"`
  Content        string `json:"content"`
  SuggestionCode string `json:"suggestion_code,omitempty"`
  ExistingCode   string `json:"existing_code,omitempty"`
  StartLine      int    `json:"start_line"`
  EndLine        int    `json:"end_line"`
  Thinking       string `json:"thinking,omitempty"`
}
```

CLI `--format json` 输出结构（`cmd/opencodereview/output.go`）：

```json
{
  "status": "success",
  "comments": [ /* LlmComment[] */ ],
  "summary": {
    "files_reviewed": 3,
    "comments": 4,
    "total_tokens": 8120,
    "input_tokens": 7200,
    "output_tokens": 920,
    "elapsed": "12s"
  },
  "warnings": [ /* AgentWarning[] */ ]
}
```

插件侧 `ReviewComment` 类型应与此对齐（字段名转 camelCase）。

---

## 8. 未决问题 / 后续迭代

- CLI 缺少 `--include` 文件级筛选参数；若产品需要精细控制再向 CLI 贡献。
- 老 `src/codeReview/` 下线时机：新方案在 aone 中稳定运行后。
- WebView 独立开发调试（dev server）配置在实现阶段确定。
