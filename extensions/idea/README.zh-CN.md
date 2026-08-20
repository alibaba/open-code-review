<p align="center">
  <a href="README.md">English</a> | 简体中文
</p>

# Open Code Review（IntelliJ IDEA 插件）

基于 [`open-code-review`](https://www.npmjs.com/package/@alibaba-group/open-code-review)（`ocr`）CLI 的 IntelliJ IDEA 插件。把 VS Code 扩展的 Preact WebView 承载于 JCEF 浏览器中，把 AI 代码审查能力集成进编辑器：在工具窗口发起审查、流式查看日志、在编辑器内逐条应用/忽略/标记误报评论，并与工具窗口双向同步。

---

## 功能

- **三种审查模式**：工作区变更、分支对比（`--from` / `--to`）、单次提交（`--commit`）。
- **待审查文件预览**：基于当前 Git 状态展示变更文件列表，点击文件可在原生 diff 视图中查看改动。
- **自定义审查提示词**：可选地为本次审查追加 `--background` 提示。
- **流式日志**：审查过程中实时滚动 CLI 输出，支持随时取消。
- **结果展示 + 双向同步**：完成后在工具窗口列出评论卡片，同时在编辑器内渲染行内评论面板；应用/忽略/误报操作在两侧同步。
- **空 / 取消 / 失败态**：无问题、用户取消、CLI 失败均有对应视图（失败可重试，并展示 CLI 返回的真实错误）。
- **配置管理**：在插件内查看/编辑 LLM 提供商配置（写入通过 `ocr config set`）。
- **模型切换 / 连通性测试**：在配置面板中切换模型、测试与 LLM 的连通性。

---

## 前置依赖

1. 全局安装 `ocr` CLI：

   ```bash
   npm i -g @alibaba-group/open-code-review
   ```

2. 配置可用的 LLM（接口地址、API Key、模型）。可用 CLI 直接配置，或在插件内的配置面板填写：

   ```bash
   ocr config set llm.url https://api.anthropic.com/v1/messages
   ocr config set llm.auth_token sk-...
   ocr config set llm.model claude-opus-4-6
   ocr config set llm.use_anthropic true
   ```

   配置写入 `~/.opencodereview/config.json`。

---

## 开发

### 环境准备

- JDK 21（Gradle 用它跑构建）。仓库自带 Gradle wrapper，无需单独安装 Gradle。
- Node.js + npm——仅构建期需要，用于打包 WebView，运行期不需要。
- 全局可用的 `ocr` CLI 与 `git`（见上文「前置依赖」），插件本质上是 `ocr` 的图形前端。

### 启动开发环境

```bash
cd extensions/idea
./gradlew runIde      # 起一个装好插件的沙箱 IDE
```

在沙箱 IDE 里打开一个有 Git 变更的项目，即可在工具窗口栏看到 Open Code Review 图标并发起审查。

> 改了代码后：WebView 改动需在沙箱 IDE 里 **重新打开工具窗口**；宿主侧（Kotlin）改动需 **重启 `runIde`**。

### 常用脚本

```bash
./gradlew build          # 编译 Kotlin + npm install + npm run build（打包 WebView）
./gradlew test           # 运行单元测试
./gradlew runIde         # 起一个装好插件的沙箱 IDE
./gradlew buildPlugin    # 生成可分发的 .zip 安装包（见下文「构建发布包」）
```

> PATH 里没有 Node.js / npm？用 `./gradlew <task> -PskipFrontend=true` 跳过前端构建
> （使用 `src/main/resources/webview/` 里已有的打包产物）。

### 调试要点

- **双端通信**：WebView 与宿主通过 JCEF 桥以 `postMessage` 通信，消息类型定义在
  `frontend/src/shared/messages.ts`。两端发收都走 `dispatch` / `handle`，定位问题先看这里。
  桥接传输层是唯一与 VS Code 扩展不同的文件（见 `frontend/UPSTREAM.md`）。
- **CLI 调用**：所有 `ocr` 子命令由
  `src/main/kotlin/com/alibaba/opencodereview/idea/services/CliService.kt` 通过 `ProcessBuilder` 执行。
  CLI 退出码非 0 时会 reject 并带上 stderr 中的 `Error:` 文本，便于排查「审查失败/连接失败」。
- **配置读写**：`ConfigService.kt` 读取 `~/.opencodereview/config.json`，写入则委托 `ocr config set`。
  WebView 端字段为 camelCase（如 `useAnthropic`），磁盘/CLI 端为 snake_case（如 `use_anthropic`），
  转换在 `ConfigParse.kt`。

---

## 构建

### 仅编译产物

```bash
./gradlew build        # 编译 Kotlin 并把 WebView 打包进 src/main/resources/webview/
```

产物：插件 jar（Kotlin 宿主）+ `src/main/resources/webview/{webview.js,configPanel.js}`（WebView SPA）。

### 构建发布包（.zip）

```bash
./gradlew buildPlugin
```

该命令编译宿主、打包 WebView，并在 `extensions/idea` 目录下生成
`build/distributions/open-code-review-idea-<version>.zip`。

### 本地安装 / 验证

在 IntelliJ IDEA 中：`Settings → Plugins → ⚙️ → Install Plugin from Disk…` → 选择生成的 `.zip` 文件。

> 若想用本机已安装的 IntelliJ IDEA 代替下载的平台包，把 `localIdePath` 写进
> `~/.gradle/gradle.properties`（不要写进仓库的 `gradle.properties`，它是机器无关的）。
> 路径不存在时构建会回退到下载平台包。

---

## 架构

采用与 VS Code 扩展相同的 **Monolithic WebView + Thin Host** 方案：

- **WebView** 是独立构建的 Preact SPA——`extensions/vscode/src/{webview,shared}` 的逐字节副本，
  靠 `frontend/UPSTREAM.md` 中的脚本保持同步。
- **宿主层**（Kotlin / IntelliJ）轻薄，只负责 CLI 调用、文件系统、Git 操作、编辑器评论，
  并通过 JCEF 承载 WebView。
- 两者通过 JCEF 桥以 `postMessage` 通信，用 `frontend/src/shared/` 中的 TypeScript 共享类型保证类型安全。

```
extensions/idea/
├── src/main/kotlin/         宿主（Kotlin / IntelliJ）：services / providers / jcef / messages
├── frontend/src/
│   ├── shared/              双端共享类型与 postMessage 协议（vscode 的逐字节副本）
│   └── webview/             WebView SPA（Preact）：views / components / store / bridge
└── src/main/resources/      plugin.xml / icons / 已打包的 webview 产物
```

---

## 兼容性

- `com.intellij.modules.jcef` 声明为**可选**依赖，因此插件在 2025.1（JCEF 在核心）和
  2026.2（JCEF 作为独立捆绑插件）上都能加载。
- WebView 的 `--vscode-*` CSS 变量取自当前 IntelliJ 主题注入，因此界面跟随 Darcula / Light 自动变化。

---

## License

Apache-2.0
