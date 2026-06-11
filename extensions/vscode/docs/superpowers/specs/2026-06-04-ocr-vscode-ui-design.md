# OCR VSCode 插件 UI 原型 · 设计文档

> 状态：vibe 阶段。本文只描述**产品形态**与**单文件 HTML 原型**的实现意图，不涉及 VSCode 插件工程化（webview API / extension activation / OCR CLI 调用桥接）—— 那些留给后续 spec。

---

## 1. 背景

[Open Code Review (OCR)](https://github.com/alibaba/open-code-review) 已经有 CLI 形态的 AI 代码审查工具：读取 Git diff，通过具备工具调用能力的 Agent 调 LLM 产出行级精度的结构化评论；支持工作区变更 / 分支差异 / 单 commit 三种范围；评论组织、规则匹配、并发打包等机制都已稳定。

本项目要为 OCR 补一个 **VSCode 插件**入口，让审查能在编辑器内触发、过程可见、结果可读。当前阶段先用单文件 HTML 把**产品形态**定下来，工程化之后再做。

视觉系统沿用本仓库 `silent-night-ui/`：dark-only · 单 mint accent · 大圆角暗层卡片 · Apple 式留白。**色系（CSS 变量）、字体栈、动效曲线、禁区清单**严格遵守 `silent-night-ui/style-reference.md`；但**组件层不强求复用 / 改造 silent-night-ui 现有组件**——code review 场景需要的不少组件（流式 timeline、文件勾选行、严重等级 pill、Provider 表单等）silent-night-ui 库里本来就没有，直接按需要写新组件即可，气质（圆角、暗层、留白节奏）保持一致就行。

## 2. 目标 / 非目标

**目标**

- 产出一个 `prototype.html`，单文件、无依赖、`file://` 双击可开
- 在 360px VSCode 侧栏宽度内忠实地讲清产品形态：从首次配置 → 选范围与文件 → 启动审查 → 流式过程 → 结果浏览 → 异常路径
- 提供 demo 状态切换器，便于 review 时点击就能跳到任意状态
- localStorage 持久化 Provider 配置 / active 模型 / 当前状态，以便刷新后保留

**非目标**

- 不实现真正的 VSCode webview / extension（不写 manifest、不接 OCR CLI、不读真实 git diff）
- 不画完整 VSCode 工作区交互（编辑器内 inline decoration / hover / quick-fix），仅画 chrome 装饰让面板有上下文感
- 不画"按文件分组浏览"或"按严重等级筛选"两种结果布局——已选 **平铺长列表**
- 不引入第二个 accent / 不偏离 silent-night-ui 禁区清单

## 3. 已确认的产品形态

| 维度 | 决策 |
|---|---|
| 原型形态 | 单页高保真交互 HTML，状态机驱动 |
| 默认使用场景 | workspace 未提交变更（`master` pill 可切到分支对比模式） |
| 顶部留存区块 | 仅 LLM 连接状态条 |
| 裁掉的区块 | Repository 名、Previous Reviews、Reviews/Plans Tab |
| 过程展示密度 | 详细流式日志（4 阶段进度条 + sub-agent 工具调用 timeline） |
| 结果组织方式 | 平铺评论长列表（不分组、不分严重等级筛选） |
| 评论卡片 | 文件 + 行号 + severity pill + 文字 + Open / Copy / Dismiss 三个 ghost btn |
| 叙事节奏 | **B · 上下分屏**：Setup 区固定可见，Action 区按状态切换 |
| 配置视图 | 升级为 **Provider 管理**（多 Provider · 每 Provider 多 model），覆盖式三视图 |

## 4. 布局骨架

固定 360px 宽度（VSCode 默认侧栏），外层包一层简化的 VSCode chrome 装饰让原型有"在编辑器里"的上下文感。

```
┌─ vscode-chrome (full screen) ────────────────────────────┐
│ [demo-switcher chips · floating]                         │
│ ┌──┬─────────────────────────────┬───────────────────┐   │
│ │  │ ● claude-opus-4-7 · OpenAI▾ │                   │   │
│ │A │                          [⚙]│                   │   │
│ │c │ ─────────────────────────── │  editor-stub      │   │
│ │t │ ▍Setup 区（始终可见）         │  灰色底色          │   │
│ │  │   New Review · workspace    │  无内容            │   │
│ │B │   Files to Review (3)       │                   │   │
│ │a │   • CHANGELOG.md  M         │                   │   │
│ │r │   • README.md     M         │                   │   │
│ │  │   • api.ts        A         │                   │   │
│ │48│   [── Review all changes ──]│                   │   │
│ │px│ ─────────────────────────── │                   │   │
│ │  │ ▍Action 区（按 data-state）   │                   │   │
│ │  │   Idle / Running / Done /   │                   │   │
│ │  │   Empty / Cancelled / Failed│                   │   │
│ └──┴─────────────────────────────┴───────────────────┘   │
└──────────────────────────────────────────────────────────┘
```

Config 视图通过 `data-config` 属性触发，覆盖整个 sidebar（z-index 高于 status-bar / setup / action）。

## 5. 状态序列

### 5.1 Action 区六态（Setup 区始终可见）

| # | 状态 | 触发 | Action 区呈现 |
|---|---|---|---|
| 1 | **Idle** | 默认 / Save 后 / 取消后 | 占位文字 "Ready to review · pick files above"，灰色 |
| 2 | **Running** | 点 Review all changes | 4 阶段进度条 + 流式 timeline + Cancel pill |
| 3 | **Done · with comments** | Running 完成且有评论 | 顶部 mint summary + 平铺评论卡片列表 |
| 4 | **Done · no comments** (Empty) | Running 完成且无评论 | mint 收束语 "No issues found · 8 files cleared" |
| 5 | **Cancelled** | Cancel 按钮 | 灰色摘要 + 已产生的部分评论卡 |
| 6 | **Failed** | LLM unreachable / 异常 | 错误说明 + Retry pill；顶部状态条同步变 dim |

### 5.2 Config 视图三视图（覆盖整个 sidebar）

进入路径：

- **首次启动**（`localStorage.providers` 空）→ 自动 `data-config="empty"`
- **任意时刻**点击 ⚙ → 已有 provider 时 `data-config="list"`，无 provider 时 `data-config="empty"`

| 视图 | 内容 |
|---|---|
| `config-empty` | 引导："Connect a model to begin" + 一句说明 + 大主按钮 `+ 添加提供商` |
| `config-list` | Providers 列表，每行 provider 名 + 下属 model 数 + active 标记；底部 `+ 添加提供商` |
| `config-form` | Provider 添加 / 编辑表单，参考用户给的截图字段 |

`config-form` 字段（窄空间适配）：

| 字段 | 说明 |
|---|---|
| Provider name | 文本，例 OpenAI / DeepSeek |
| Base URL | 文本，例 `https://api.openai.com/v1` |
| API Key | password，标 "(optional)" |
| Models | 多行可增删，每行 `Model ID` + `Display name`，行右 × 删除 |
| ▸ Advanced | 展开后含 `Use Anthropic protocol` toggle |
| 底部 | `Cancel` ghost · `Save` mint primary（**不是紫色**，遵守单 accent 原则） |

## 6. 组件清单

按需要直接写组件，不强求映射到 silent-night-ui 已有组件。统一约束只有三条：

- 用 silent-night-ui §1 的 CSS 变量取色（`--bg / --card / --card-soft / --ink / --mint / --rule` 等）
- 用 silent-night-ui §1 的字体栈（`--font / --font-display / --font-mono`）
- 不踩 §9 的视觉禁区清单（单 mint accent、无 emoji / SVG 装饰图标、无 blur、无渐变文字、无 `border-radius > 28px` 等）

### 6.1 组件列表

| 区域 | 类名 | 职责 / 关键样式 |
|---|---|---|
| 外层 | `.vscode-chrome` | 左 48px Activity Bar 占位 + 中 360px sidebar + 右编辑器灰底；让原型有 IDE 上下文感 |
| 外层 | `.activity-bar` | 48px 宽，几个 placeholder 圆角方块占位 |
| 外层 | `.editor-stub` | 右侧灰底；可加一行小字 "(editor area · prototype only)" |
| Demo | `.demo-switcher` | 顶部 floating chips 切 6 状态，仅原型用，发布前可以隐藏 |
| 顶部 | `.status-bar` | 一行：mint pulse + 模型显示名（`--ink`）+ ` · ` + provider 名（`--ink-quiet`）+ ▾ + ⚙ |
| 顶部 | `.model-dropdown` | ▾ 触发的弹层，按 provider 分组列出所有 models，active 项左侧 mint dot |
| Setup | `.range-pill` | 显当前范围（`workspace` / `branch:master..HEAD` / `branch:custom`），点击弹小 popover 切换 |
| Setup | `.file-row` | 紧凑单列：左 checkbox + 文件名（`--font-mono` 中等字号）+ 右侧 A/M/D 标记（mint=A、灰=M、更弱灰=D） |
| Setup | `.primary-btn` | 全宽 mint 主按钮 + 右侧 ▾ split（弹出 "Preview · dry-run" / "Review selected only"） |
| Action / Idle | `.idle-note` | 灰色占位文字 |
| Action / Running | `.stage-bar` | 横向 4 阶段：Parse → Pack → Review (n/m) → Reflect；当前阶段 mint pulse + label `--ink`；已完成阶段 mint 实心；未到达阶段灰 |
| Action / Running | `.timeline` | mono 字体流式日志；每行首一个状态点区分类型——`tool` 用 mint pulse 圆点，`file` 用 mint 实心圆点（不脉动），`comment` 用更小的灰色实心圆点。**不用 SVG 图标 / emoji** |
| Action / Running | `.cancel-pill` | 灰 ghost pill，靠右 |
| Action / Done | `.done-summary` | 一行总结："5 comments · 8 files · 24s"；mint 圆点起头 |
| Action / Done | `.comment-card` | 文件名 + 行号（`L42`）+ severity pill + 评论正文 + 底部三 ghost btn（Open / Copy / Dismiss） |
| Action / Done | `.severity-pill` | `critical` = `--mint-tint` 底 + `--mint` 字；`warn` = `--card-quiet` 底 + `--ink-soft` 字；`info` = 更深的灰底 + `--ink-quiet` 字 |
| Action / Empty | `.empty-note` | mint 圆点 + 收束语 "No issues found · 8 files cleared" |
| Action / Cancelled | `.cancelled-note` | 左 mint border + 灰字摘要，下方可挂已产生的部分评论卡 |
| Action / Failed | `.failed-card` | 暗灰底 + 错误信息 + Retry mint pill |
| Config | `.config-overlay` | 全屏覆盖 sidebar，z-index 高；顶部 status-bar 和 ⚙ 仍可见以便关闭 |
| Config | `.empty-onboard` | mint 圆点 + section-label "Configure" + 标题 "Connect a model to begin" + 一句说明 + 大主按钮 `+ 添加提供商` |
| Config | `.provider-list` | 每行：provider 名 + 下属 model 数 + 右侧 active 标记（mint dot），点击展开行内编辑/删除 |
| Config | `.provider-form` | 表单容器；标签字号 11.5px / `--ink-quiet` / uppercase；输入框 `--card-soft` 底 + `--rule` 描边 |
| Config | `.input` | 文本/password 输入。focus 时 mint border + 微 mint glow |
| Config | `.model-row` | 表单内多行子组件：左 `Model ID` 输入 + 右 `Display name` 输入 + 行右 × 删除（hover 显 mint） |
| Config | `.advanced-disclosure` | 原生 `<details>`，▸ 展开后显 `Use Anthropic protocol` toggle |
| Config | `.toggle` | 自定义 mint switch（开 = mint slider，关 = 灰 slider） |

### 6.2 窄空间密度策略

silent-night-ui 默认面向内容型 960px 页面，组件 padding/字号偏宽松。原型在 360px 侧栏里需要更紧凑：

| 项 | 原型默认 | 备注 |
|---|---|---|
| 卡片 / 区块 padding | 14-20px | 默认 18px；Action 区评论卡可降到 14px |
| 卡片 border-radius | 12-18px | 内层小卡片 12px，外层主区块 18px |
| 标题字号（区块顶部） | 16-17px | 不再用 32px hero 标题 |
| section-label 字号 | 10.5px | spacing 0.18em / uppercase 不变 |
| 正文字号 | 12.5-13px | 评论卡正文 13px，timeline mono 11.5px |
| 网格列数 | 1（默认） | 例外：model-row 内部用 2 列（Model ID + Display name） |
| 行高 | 1.5-1.6 | 比 silent-night-ui 默认 1.65 略紧 |

## 7. 关键交互逻辑

全部通过 `data-*` 属性 + CSS 选择器切换可见性，配最小 JS：

1. **状态机**：`body[data-state="running"] .action-running { display:block }`。Demo chip 仅修改 `body.dataset.state`。
2. **Config 覆盖**：`body[data-config="empty"|"list"|"form"]`。⚙ toggle，关闭恢复 Setup+Action。首次启动若 `localStorage.providers` 空 → 设 `data-config="empty"`。
3. **Range pill**：点击弹小 popover：`workspace` / `branch:master..HEAD` / `branch:custom`。选定回写 pill 文字。
4. **文件勾选**：默认全选。点击 toggle，主按钮文字随勾选数变 "Review 3 changes" / "Review all changes"。
5. **伪流式 Replay**：原型默认静态展示完整 timeline（约 10-15 行，覆盖一次审查的典型节奏）；额外提供 `▶ Replay` floating button，按 200ms 间隔依次给每行加 `.visible` 重现节奏。
6. **评论 Dismiss**：点击 → 加 `.dismissed`（fade-out + collapse），summary 计数同步减一。仅本地，不持久。
7. **Provider 表单的 model 行**：`+ 添加模型` 克隆 `<template>` 节点 append；行右 × 删除。
8. **localStorage 持久**：`localStorage.ocrPrototype = { providers, activeModelId, state, config }`。隐蔽 `Reset` 链接（藏在 status-bar 角落）清空回到首次安装。

## 8. 文件结构

```
prototype.html  ← 单文件交付
  <style>
    1. silent-night-ui CSS 变量（完整内联 §1）
    2. 全局基底 §2（body::before/after 光晕保留）
    3. .vscode-chrome 外层 layout
    4. 组件样式（按 §6.1）
    5. data-state / data-config 选择器
    6. responsive（仅保护极窄场景）
  </style>
  <body data-state="idle" data-config="">
    <div class="demo-switcher">…</div>
    <div class="vscode-chrome">
      <aside class="activity-bar">…icons…</aside>
      <aside class="sidebar">
        <div class="status-bar">…</div>
        <div class="setup">…</div>
        <div class="action-region">
          <div class="action-idle">…</div>
          <div class="action-running">…</div>
          <div class="action-done">…</div>
          <div class="action-empty">…</div>
          <div class="action-cancelled">…</div>
          <div class="action-failed">…</div>
        </div>
        <div class="config-overlay">
          <div class="config-empty">…</div>
          <div class="config-list">…</div>
          <div class="config-form">
            <template id="model-row-template">…</template>
          </div>
        </div>
      </aside>
      <main class="editor-stub"></main>
    </div>
    <script>state machine + interactions（< 200 行）</script>
  </body>
```

## 9. 视觉禁区自检

按 silent-night-ui §8 视觉禁区清单，原型必须满足：

- 单 mint accent，无第二种品牌色（截图里"提交"的紫色按钮 → mint）
- 无白色 / 米色卡片
- 无渐变文字、无 emoji 装饰、无音频波形
- 无 `backdrop-filter` / blur
- 无 `border-radius > 28px`（pill 999px 例外）
- 无网络资源（CDN / Google Fonts / fetch / 远程图片）
- 字体只用系统栈
- dark-only，不做日间模式切换
- 严重等级 `critical / warn / info` 三档**不分配三种颜色**，靠 mint vs 灰 vs 更弱灰对比表达

## 10. 交付与下一步

**交付物**：仓库根目录的 `prototype.html`（双击可开） + 顶部一段 README 段落讲清"点 chip 切状态、点 ⚙ 进配置、点 Reset 清空"。

**下一步**（在另一份 spec 里展开）：从 prototype.html 的产品形态出发，规划真正的 VSCode extension 工程：activate / webview 桥接 / OCR CLI 调用 / 评论流式协议 / extension settings 与 Provider 管理的持久化等。
