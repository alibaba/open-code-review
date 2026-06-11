# 三 Tab 文件列表按模式加载 — 设计文档

日期:2026-06-09

## 背景

侧边栏「新建审查」有三个 Tab:工作区、分支对比、单次提交。当前实现存在两个问题:

1. **文件列表不分模式**:`GitService.getState(mode)` 无论哪个 mode,文件列表永远返回工作区 `git status --porcelain` 的结果。分支对比和单次提交根本没有计算各自的 diff 文件,三个 Tab 显示同一份工作区列表。
2. **假的文件选择**:`FileList.tsx` 每行渲染一个写死 `checked` 的 checkbox,没有 `onChange`、没有状态、提交时也不收集选中路径。是纯装饰,且 `ocr` CLI 不支持按文件过滤审查。

## 约束

`ocr` CLI 只支持三种审查粒度,**无任何按文件路径过滤的参数**:

- `ocr review` → 工作区全部变更
- `ocr review --from X --to Y` → 分支 merge-base 差异
- `ocr review --commit <sha>` → 单个 commit

因此文件选择无法真正生效。

## 决策

1. **三个 Tab 统一为「只展示、不可选」的文件列表**,完全对齐 CLI 能力。移除装饰性 checkbox。
2. **每个 Tab 展示与其审查范围一致的 diff 文件列表**(补上当前缺失的 per-mode 加载逻辑)。
3. 交互参考 source tree 的逻辑(按上下文罗列对应文件),**不做树形目录 UI**,保持平铺列表。

## 各 Tab 文件列表来源

| Tab | git 命令 | 触发时机 |
|------|---------|---------|
| 工作区 | `git status --porcelain`(已有) | 切到该 Tab |
| 分支对比 | `git diff --name-status <from>...<to>`(三点 merge-base) | 两个分支都选定后 |
| 单次提交 | `git show --name-status --format= <sha>` | 选中某个 commit 后 |

分支对比用三点 diff,与 `ocr` 的 merge-base mode 一致,保证展示列表和实际审查范围对得上。

## 改动点

### 1. `src/extension/services/GitService.ts`
- 新增 `getBranchDiff(from, to): Promise<FileChange[]>` — 运行 `git diff --name-status from...to`。
- 新增 `getCommitFiles(sha): Promise<FileChange[]>` — 运行 `git show --name-status --format= <sha>`。
- 两者都用根目录 + `runGit`,失败走 `trace()` 日志通道。

### 2. `src/extension/services/gitMap.ts`
- 新增 `parseNameStatus(out): FileChange[]` — 解析 `--name-status` 输出(`A/M/D/R` + 路径;`R` 带相似度和两个路径,取目标路径)。映射到现有 `FileChange.status`。

### 3. `src/shared/messages.ts`
- `WebviewToHost` 新增:`{ type: 'getModeFiles'; mode: ReviewMode; from?: string; to?: string; commit?: string }`。
- `HostToWebview` 新增:`{ type: 'modeFiles'; mode: ReviewMode; files: FileChange[] }`。

### 4. `src/extension/providers/SidebarProvider.ts`
- 处理 `getModeFiles`:按 mode 调用对应 GitService 方法,回 `modeFiles`。

### 5. `src/webview/store.ts` + `App.tsx`
- 新增状态字段保存当前 mode 对应的文件列表(如 `modeFiles: FileChange[]`)。
- 处理 `modeFiles` 消息更新状态。

### 6. `src/webview/views/IdleView.tsx`
- 分支对比:`from` 和 `to` 都有值时,自动发 `getModeFiles` 请求。
- 单次提交:选中 `commit` 时,自动发 `getModeFiles` 请求。
- 工作区:维持现状(切 tab 时通过现有 `getGitState`/init 已加载 `workspaceFiles`)。
- `FileList` 数据源从写死的 `gitState.workspaceFiles` 改为「当前 mode 对应列表」:工作区用 `workspaceFiles`,其余用 `modeFiles`。

### 7. `src/webview/components/FileList.tsx`
- 删除 checkbox,改为纯展示行。保留 `待审查文件 (N)` 标签和状态 badge。

## 不做(YAGNI)

- 不做文件勾选/选择逻辑(CLI 不支持)。
- 不改 `buildReviewArgs`(三种 mode 的 CLI 参数已正确)。
- 不做树形目录 UI,保持平铺。

## 验证

- 工作区:有未提交改动时列出,与 `git status` 一致。
- 分支对比:选两分支后列表 = `git diff --name-status from...to`。
- 单次提交:选 commit 后列表 = 该 commit 改动文件。
- 三个 Tab 均无 checkbox,文件不可选。
