# Open Code Review (VSCode 插件)

基于 [`open-code-review`](https://www.npmjs.com/package/@alibaba-group/open-code-review) (`ocr`) CLI 的 VSCode 代码审查插件。以 Preact WebView 还原原型交互体验，把 AI 代码审查能力集成进编辑器：在侧边栏发起审查、流式查看日志、在编辑器内逐条应用/忽略/标记误报评论，并与侧边栏双向同步。

---

## 功能

- **三种审查模式**：工作区变更、分支对比（`--from` / `--to`）、单次提交（`--commit`）。
- **待审查文件预览**：基于当前 Git 状态展示变更文件列表。
- **自定义审查提示词**：可选地为本次审查追加 `--background` 提示。
- **流式日志**：审查过程中实时滚动 CLI 输出，支持随时取消。
- **结果展示 + 双向同步**：完成后在侧边栏列出评论卡片，同时在编辑器内渲染 CommentThread；应用/忽略/误报操作在两侧同步。
- **空 / 取消 / 失败态**：无问题、用户取消、CLI 失败均有对应视图（失败可重试）。
- **配置管理**：在插件内查看/编辑 LLM 提供商配置（写入通过 `ocr config set`）。
- **模型切换 / 连通性测试**：状态栏切换模型、测试与 LLM 的连通性。

---

## 前置依赖

1. 全局安装 `ocr` CLI：

   ```bash
   npm i -g @alibaba-group/open-code-review
   ```

2. 配置可用的 LLM（接口地址、API Key、模型）。可用 CLI 直接配置，或在插件内的配置视图填写：

   ```bash
   ocr config set llm.url https://api.anthropic.com/v1/messages
   ocr config set llm.auth_token sk-...
   ocr config set llm.model claude-opus-4-6
   ocr config set llm.use_anthropic true
   ```

   配置写入 `~/.opencodereview/config.json`。

---

## 开发

```bash
yarn install      # 安装依赖
yarn compile      # 开发构建（webpack development，产出 out/extension.js + out/webview.js）
```

然后在 VSCode 中按 **F5** 启动 Extension Development Host（已提供 `.vscode/launch.json`），打开一个有 Git 变更的项目即可体验。

其他脚本：

```bash
yarn watch        # 监听式开发构建
yarn test         # 运行 Jest 单测
yarn lint         # ESLint
```

---

## 构建

```bash
yarn build        # 生产构建（webpack production）
```

产物：`out/extension.js`（Extension Host）+ `out/webview.js`（WebView SPA）。

---

## 架构

采用 **Monolithic WebView + Thin Extension Host** 方案：

- **WebView** 是独立构建的 Preact SPA，还原原型的全部视觉与交互。
- **Extension Host** 层轻薄，只负责 CLI 调用、文件系统、Git 操作、编辑器评论。
- 两者通过 `postMessage` 通信，用 `src/shared/` 中的 TypeScript 共享类型保证类型安全。

详细的架构设计、技术决策与数据结构参考见
[`docs/superpowers/specs/2026-06-08-ocr-vscode-extension-design.md`](docs/superpowers/specs/2026-06-08-ocr-vscode-extension-design.md)。

```
src/
├── extension/      Extension Host（Node.js）：services / providers / commands
├── webview/        WebView SPA（Preact）：views / components / store / bridge
└── shared/         双端共享类型与 postMessage 协议（不依赖 vscode）
```

---

## License

Apache-2.0
