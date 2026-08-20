# frontend/ 的来历与维护约定

这个目录是 **VS Code 扩展前端的副本**，不是这个项目自己写的 UI。

| | |
|---|---|
| 上游仓库 | https://github.com/alibaba/open-code-review |
| 上游路径 | `extensions/vscode/src/webview` + `extensions/vscode/src/shared` |
| 复制自 commit | `1b193db3587e4d3f429bd8c8213479d63e3b4f21`（`1b193db`，2026-08-03） |
| 上游 commit 主题 | `feat(cli): add 'ocr session comments' to display saved review comments (#505) (#646)` |
| 许可证 | Apache-2.0，见同目录 `LICENSE` |

## 为什么是复制而不是引用

目标是**只维护一份前端**：webview 的唯一源头是 `extensions/vscode/src/{webview,shared}`，
由 VS Code 扩展维护；IDEA 插件这里只是**消费者**，放一份逐字节副本。

目前没有直接 import 共享，原因有二：

1. IntelliJ 侧的 Gradle 构建需要在本地 `frontend/` 跑 `npm run build` 产出 JCEF 用的 bundle，
   不能跨项目引用 vscode 扩展目录下的源。
2. 上游 webview 目前**强耦合 `acquireVsCodeApi()`**，宿主无关性还没抽出来。
   本目录里唯一改写的 `bridge.ts` 就是这层缝；等上游接受把传输层抽成 `Transport` 契约后
   （见后续 PR），这份 `bridge.ts` 也能回到逐字节上游，届时"副本"可降级为"引用"。

在那之前，靠下面的同步脚本保证副本与上游逐字节一致，靠两个漂移测试兜底。

## 唯一被改写的文件

只有一个：`src/webview/bridge.ts`。

上游用 `acquireVsCodeApi()` 发消息、`window.addEventListener('message')` 收消息；
IDEA 侧没有这套 API，改成了宿主注入的两个全局 `window.__ocrPost` / `window.__ocrReceive`。
对外暴露的 `bridge.post` / `bridge.onMessage` 签名和语义完全没变，
所以 `App.tsx` 及以下所有文件都不需要改。

**其余每一个文件都必须与上游逐字节相同。** 想改行为请回上游改，然后重新同步整个目录。

## 同步上游的做法

```bash
REPO=<open-code-review 仓库根，即本仓库>
UP="$REPO/extensions/vscode"      # 前端唯一源头
DST="$REPO/extensions/idea/frontend"  # 本副本

# 1. 先留一份改写过的 bridge
cp "$DST/src/webview/bridge.ts" /tmp/ocr-bridge.ts

# 2. 整体覆盖
rm -rf "$DST/src/shared" "$DST/src/webview"
cp -R "$UP/src/shared"  "$DST/src/shared"
cp -R "$UP/src/webview" "$DST/src/webview"

# 3. 把 bridge 放回去
cp /tmp/ocr-bridge.ts "$DST/src/webview/bridge.ts"

# 4. 更新上面那张表里的 commit，然后跑漂移检查
cd "$REPO/extensions/idea" && ./gradlew test && (cd frontend && npm test && npm run build)
```

第 4 步的 `./gradlew test` 里有两个专门盯着这次同步的用例：

- `ProvidersTest` —— 从 `src/shared/providers.ts` 里正则出全部预置 provider 名，
  和 Kotlin 侧 `Providers.kt` 的清单对比。上游加了新 provider 而宿主没跟上，这里会红。
- `WebviewHtmlTest` —— 从 `src/webview` 里正则出全部 `--vscode-*` 变量，
  和 Kotlin 侧 `IdeaTheme.kt` 的映射表对比。上游用了新的主题变量而宿主没给值，
  界面上会出现"某块颜色没了"，这里会红。

## 构建

```bash
cd frontend
npm install          # 只需一次
npm run build        # 产物 → ../src/main/resources/webview/{webview.js,configPanel.js}
npm run watch        # 改前端时开着，配合 IDEA 里重开工具窗即可看到效果
npm test             # 上游那份 store.test.ts
npm run typecheck
```

`./gradlew build` 会自动跑上面的 `npm install` + `npm run build`（见根目录
`build.gradle.kts` 里的 `frontendInstall` / `frontendBuild`）。
机器上没有 node、或者只想编 Kotlin 时用 `-PskipFrontend=true` 跳过。

## 与上游的运行时差异（宿主侧补齐，前端不必知道）

1. **CSS 变量**：前端用的是 `--vscode-*`。IDEA 里没有这些变量，
   宿主在 HTML 里注入一段 `:root { --vscode-*: ... }`，值取自当前 IDEA 主题
   （`IdeaTheme.kt`），所以跟随 Darcula / Light 自动变化。
2. **CSP 与 nonce**：上游 HTML 带 CSP 头和 nonce。JCEF 这边脚本是内联进本地
   `loadHTML` 的页面，没有远端资源可加载，nonce 没有意义，因此没有照搬。
3. **配置面板的窗口**：上游是编辑器里的一个 webview tab，
   IDEA 侧是一个非模态对话框（`JcefConfigPanelHost`）。
   前端收到的消息序列完全一致。
