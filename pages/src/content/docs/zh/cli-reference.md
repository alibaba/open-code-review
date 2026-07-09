---
title: CLI 参考
sidebar:
  order: 6
---

每个 `ocr` 子命令、参数与退出行为的完整参考。

## 全局用法

```text
OpenCodeReview - AI-Powered Code Review CLI

Usage:
  ocr [command]

Commands:
  review, r    Start a code review
  rules        Inspect and debug review rules
  config       Manage configuration settings
  llm          LLM utility commands
  viewer       Start the WebUI session viewer
  session, sessions  List and inspect saved review sessions
  version      Show version information

Examples:
  ocr review --from master --to dev        Review diff range
  ocr review --commit abc123               Review a single commit
  ocr config provider                      Interactive provider setup
  ocr config model                         Interactive model selection
  ocr config set llm.model opus-4-6        Set a config value
  ocr llm test                             Test LLM connectivity
  ocr llm providers                        List built-in providers
  ocr session list                         List saved review sessions
  ocr version                              Show version info

Use "ocr review -h" for more information about review.
Use "ocr rules -h" for more information about rules.
Use "ocr config" for more information about config.
Use "ocr llm" for more information about LLM utilities.
Use "ocr session -h" for more information about session inspection.

GitHub: https://github.com/alibaba/open-code-review
```

## 命令总览

| 命令 | 别名 | 作用 |
|---|---|---|
| `ocr review` | `ocr r` | 运行代码评审并输出评论。 |
| `ocr rules check <file>` | — | 显示某文件路径适用哪条规则及其来源。 |
| `ocr config set <key> <value>` | — | 将一个配置值持久化到 `~/.opencodereview/config.json`。 |
| `ocr config unset custom_providers.<name>` | — | 删除一个自定义 provider（若它是当前启用的，则清空启用的 `provider`/`model`）。 |
| `ocr config provider` | — | 交互式 provider 配置 TUI。 |
| `ocr config model` | — | 交互式 model 选择 TUI。 |
| `ocr llm test` | — | 发送一条简短 chat 请求以验证配置的端点。 |
| `ocr llm providers` | — | 列出所有内置 LLM provider。 |
| `ocr session list` | `ocr sessions list`, `ocr session ls` | 列出已保存的评审会话。 |
| `ocr session show <id>` | `ocr sessions show <id>` | 查看单个会话及其逐文件检查点。 |
| `ocr viewer` | — | 启动用于历史评审会话的本地 Web UI（`localhost:5483`）。 |
| `ocr version` | — | 打印版本、commit、平台、构建日期与 GitHub URL。 |

`ocr` 和 `ocr -h` 打印顶层用法。每个子命令也接受 `-h` / `--help`。

## `ocr review`

主命令。解析 Git diff，分发 per-file 子 agent，收集评审评论并打印。

### 概要

```text
ocr review [flags]
ocr r      [flags]   (alias)
```

若不传任何参数，OCR 以**工作区模式**运行——评审当前目录所在仓库中所有 staged +
unstaged + untracked 变更。

### 参数

| 参数 | 简写 | 默认 | 说明 |
|---|---|---|---|
| `--repo <path>` | — | 当前目录 | Git 仓库根。 |
| `--from <ref>` | — | — | diff 起始 ref（如 `main`）。 |
| `--to <ref>` | — | — | diff 结束 ref（如 `feature-branch`）。设置后 OCR 计算 `merge-base(from, to)..to`。 |
| `--commit <sha>` | `-c` | — | 评审单个 commit（相对其父）。 |
| `--preview` | `-p` | `false` | 运行过滤流水线但跳过 LLM。打印文件列表与排除原因。 |
| `--resume <session-id>` | — | — | 从之前兼容的区间或单 commit 评审会话恢复。 |
| `--format <fmt>` | `-f` | `text` | `text`（人类可读）或 `json`（机器可读的评论数组）。 |
| `--audience <who>` | — | `human` | `human` 流式输出进度行；`agent` 静默 stdout，只打印最终摘要 / JSON。 |
| `--background <text>` | `-b` | — | 注入 plan + main prompt 的可选需求 / 业务上下文。 |
| `--concurrency <n>` | — | `8` | 并行评审的最大文件数。 |
| `--timeout <minutes>` | — | `10` | 每文件截止时间。`0` 关闭超时。 |
| `--rule <path>` | — | — | 自定义 JSON 评审规则文件路径。覆盖项目级与全局 `rule.json`。 |
| `--max-tools <n>` | — | 模板默认 | 每文件最大工具调用轮数。`0` 用模板默认（`30`）；1–9 会被上调到 `10`；任何 `≥ 10` 的值都覆盖模板默认（即使小于 `30`）。 |
| `--model <name>` | — | — | 为本次评审覆盖已解析出的 LLM model（如 `claude-opus-4-6`）。 |
| `--max-git-procs <n>` | — | `16` | 并发 git 子进程的最大数。 |
| `--tools <path>` | — | 内嵌 | 自定义 JSON 工具配置文件路径。覆盖内嵌工具定义。 |

> 模式参数互斥：传 `--from`/`--to`，或 `--commit`，或都不传（工作区模式）。
> 混用会直接报错。
> `--resume` 仅支持区间或单 commit 评审，不能与 `--preview` 同时使用。

### 模式

#### 工作区模式（默认）

```bash
ocr review
```

OCR 从两条 git 命令组装工作树变更：

- 通过 `git diff HEAD` 获取已跟踪变更（staged + unstaged 合并对比 `HEAD`；
  若为空则回退到 `git diff --staged`）
- 通过 `git ls-files --others --exclude-standard` 获取 untracked 文件，从磁盘
  读取并作为整文件新增处理

这通常是 commit 前你想要的。如需更小的范围，请选择性暂存。

#### 区间模式

```bash
ocr review --from main --to feature-branch
```

OCR 计算 `merge-base(main, feature-branch)..feature-branch`，因此你只看到
feature 分支*引入*的 diff——而非分支切出后落到 `main` 上的无关变更。

#### Commit 模式

```bash
ocr review --commit abc123
ocr review -c abc123
```

评审 `git show abc123` 产生的 diff（即该 commit 引入的变更）。

### 恢复中断的评审

每次 `ocr review` 都会在 `~/.opencodereview/sessions/` 下保存本地会话日志。
正常完成的文本输出只展示评审结果，不打印 session ID；可使用
`ocr session list/show` 查找已保存会话，或用 `--format json` 在机器可读输出中获取
`session_id`。如果区间或单 commit 评审被中断，先列出已保存会话，再从与当前评审目标一致的会话恢复：

```bash
ocr session list
ocr session show <session-id>
ocr review --from main --to feature-branch --resume <session-id>
ocr review --commit abc123 --resume <session-id>
```

恢复逻辑是严格的：

- 工作区评审不能恢复
- 区间评审必须使用相同的 `--from` 和 `--to`
- 单 commit 评审必须使用相同的 `--commit`
- `--preview` 和 `--resume` 不能同时使用

### 输出

#### Text（默认，`--audience human`）

评审运行时流式输出进度行，随后每条评论一个块（带 `path:start-end` 的暗色
Unicode 分隔头、按 100 列折行的评论正文，以及（存在时）建议替换的彩色内联
diff）。运行结束时 stdout 末尾打印一份摘要：

```
[ocr] 17 file(s) changed, reviewing 9 in /path/to/repo
[ocr] Skipping image.png — filtered by path/extension rules
[ocr]   ▶ file_read "src/foo.go"
[ocr]   ✔ file_read (12ms)
[ocr] Plan completed for src/foo.go
…

─── src/foo.go:42-47 ───
Concurrent map access without a lock — wrap with sync.RWMutex.

- m[k] = v
+ mu.Lock(); defer mu.Unlock(); m[k] = v

…
[ocr] Summary: 9 file(s) reviewed, 14 comment(s), ~21344 token(s) used (input: ~18012, output: ~3332), 1m12s elapsed
```

#### Text（agent，`--audience agent`）

评论输出相同，但通过一个内部可静默的 stdout writer 屏蔽进度行
（[`internal/stdout`](https://github.com/alibaba/open-code-review/blob/main/internal/stdout/stdout.go)）。
在 CI / 流水线中交给另一个 agent 时使用。

#### JSON

```bash
ocr review --format json --audience agent
```

```json
{
  "status": "success",
  "summary": {
    "files_reviewed": 9,
    "comments": 1,
    "total_tokens": 21344,
    "input_tokens": 18012,
    "output_tokens": 3332,
    "elapsed": "1m12s"
  },
  "comments": [
    {
      "path": "src/foo.go",
      "content": "Concurrent map access without a lock — wrap with sync.RWMutex.",
      "start_line": 42,
      "end_line": 47,
      "existing_code": "m[k] = v",
      "suggestion_code": "mu.Lock(); defer mu.Unlock(); m[k] = v",
      "thinking": "Looking at line 42, the map …"
    }
  ]
}
```

顶层字段：

| 字段 | 说明 |
|---|---|
| `status` | `success`、`completed_with_warnings`、`completed_with_errors` 或 `skipped`。 |
| `message` | 可选。人类可读摘要，如 `"No comments generated. Looks good to me."`。 |
| `summary` | 可选。运行聚合：`files_reviewed`、`comments`、`total_tokens`、`input_tokens`、`output_tokens`、`cache_read_tokens`（omitempty）、`cache_write_tokens`（omitempty）、`elapsed`。`skipped` 运行时省略。 |
| `comments` | 总是存在，可能为空。每条评论的字段如上例。 |
| `warnings` | 可选。当一个或多个子 agent 失败时存在；每条描述受影响文件与错误。 |
| `session_id` | 可选。持久化的评审运行会包含该字段；重试兼容的区间或单 commit 评审时可传给 `ocr review --resume <session-id>`。 |
| `resume` | 可选。恢复运行时存在，包含 `resumed_from`、`reused_files`、`rerun_files`、`previous_model` 和 `current_model`。 |

当没有文件可评审时，JSON 模式会发一个 `skipped` 外壳，以便调用方区分“无变更”
与“无发现”：

```json
{
  "status": "skipped",
  "message": "No supported files changed.",
  "comments": []
}
```

### 退出码

| 码 | 含义 |
|---|---|
| `0` | 评审完成（可能零评论，可能有非致命警告）。 |
| `1` | 致命错误——参数错误、无法解析 LLM 端点、所有 per-file 子 agent 失败等。错误文本打印到 stderr。 |

非致命警告（单个子 agent 失败、某文件超过 token 阈值等）内联打印；JSON 模式下
会加入 `warnings` 数组。

## `ocr session`

列出和查看保存在 `~/.opencodereview/sessions/` 下的本地评审会话日志。
可用它查找 session ID、查看逐文件检查点状态，并恢复中断的区间或单 commit 评审。

```text
ocr session <sub-command>
ocr sessions <sub-command>   (alias)

Sub-commands:
  list, ls    List recent review sessions for the current repo
  show <id>   Show one session's metadata and per-file items
```

### `ocr session list`

```bash
ocr session list
ocr session list --limit 50
ocr session list --json
```

| 参数 | 默认 | 说明 |
|---|---|---|
| `--repo <path>` | 当前目录 | 要列出会话的仓库。 |
| `--json` | `false` | 以 JSON 输出会话摘要。 |
| `--limit <n>` | `20` | 限制列出的会话数量。使用 `0` 表示不限制。 |

### `ocr session show`

```bash
ocr session show <session-id>
ocr session show --json <session-id>
ocr session show --repo /path/to/repo <session-id>
```

| 参数 | 默认 | 说明 |
|---|---|---|
| `--repo <path>` | 当前目录 | 要查看会话的仓库。 |
| `--json` | `false` | 以 JSON 输出会话元数据和逐文件条目。 |

## `ocr rules`

规则自查。只有一个子命令：

```text
ocr rules check [flags] <file-path>

Flags:
  --repo <path>    Git repository root (default: current dir)
  --rule <path>    Path to a custom rule JSON file
```

对给定文件路径，OCR 会：

1. 遍历四层规则链（custom → project → global → system）。
2. 取第一条匹配。
3. 打印**来源层**、匹配的 **glob 模式**，以及解析出的**规则文本**。

```bash
$ ocr rules check src/main/java/com/example/Foo.java
File: src/main/java/com/example/Foo.java
Source: System built-in
Pattern: **/*.java
Rule:
────────────────────────────────────────
<contents of internal/config/rules/rule_docs/java.md>
────────────────────────────────────────
```

可用于排查“为什么我的自定义规则没触发？”——完整的优先级说明见
[评审规则](../review-rules/)。

## `ocr config`

将 key 持久化到 `~/.opencodereview/config.json`，并提供交互式配置 TUI。四个
子命令：

```text
ocr config set <key> <value>
ocr config unset custom_providers.<name>   Delete a custom provider
ocr config provider                        Interactive provider setup
ocr config model                           Interactive model selection
```

- **`set`**——非交互式写入单个配置值。
- **`unset`**——删除一个自定义 provider。仅支持
  `custom_providers.<name>`。若删除的是当前启用的 provider，则 `provider` 和
  `model` 被清空（运行 `ocr config provider` 重新选择）。
- **`provider`**——启动交互式 provider 配置 TUI（无额外参数；非交互式请用
  `ocr config set provider <name>`）。
- **`model`**——启动交互式 model 选择 TUI（无额外参数；非交互式请用
  `ocr config set model <name>`）。

完整的 key 参考、schema 与示例见[配置](../configuration/)。

## `ocr llm`

LLM 工具命令。两个子命令：

```text
ocr llm <sub-command>

Sub-commands:
  test         Send a test conversation to the configured LLM model
  providers    List all built-in LLM providers
```

### `ocr llm test`

```text
ocr llm test
```

以与 `ocr review` 完全相同的方式解析 LLM 端点，从
[`internal/config/testconnection/task.json`](https://github.com/alibaba/open-code-review/blob/main/internal/config/testconnection/task.json)
发送一条预置 chat 请求，并打印：

```
Source: <which strategy was used>
URL:    <endpoint URL>
Model:  <effective model>
<the model's reply>
✓ Connection test successful
```

非零退出意味着端点未完整配置，或请求失败（网络 / 鉴权 / 模型错误）。错误信息
会指明具体是哪一种。

### `ocr llm providers`

```text
ocr llm providers
```

以三列表格列出每个内置 LLM provider：

```
Built-in providers:
  NAME        PROTOCOL    BASE URL
  ----        --------    --------
  anthropic   anthropic   https://api.anthropic.com
  …
```

随后是一条提示，可用 `ocr config provider` 交互式配置，或用
`ocr config set provider <name>` 非交互式配置。

## `ocr viewer`

```text
ocr viewer [flags]

Flags:
  --addr <address>   listen address (default: localhost:5483)

Examples:
  ocr viewer                     # start on default port
  ocr viewer --addr :3000        # bind to all interfaces on port 3000
```

启动一个内嵌 HTTP 服务器，读取 `~/.opencodereview/sessions/...`，并以浏览器友好的 UI 渲染历史评审会话。见[会话查看器](../viewer/)。

## `ocr version`

```text
ocr version
ocr --version
ocr -V
```

打印构建时写入的版本信息、短 Git commit（存在时）、平台
（`<GOOS>/<GOARCH>`）、构建日期（存在时），以及 GitHub URL
（`https://github.com/alibaba/open-code-review`）。

## 提示与注意

- `--audience agent` **并不**隐含 `--format json`。两者控制不同的事——屏蔽 UI
  vs 结构化载荷。需要二者兼得时组合使用。
- `--background` 是提升评审质量最有效的参数之一——从其他 agent 调用时，始终传入
  需求 / PR 描述。
- 某文件 diff 单独超过 `MAX_TOKENS` 的 80%（默认 `58888`）时，会在调用 LLM 前
  被丢弃。这会记录日志但不会使运行失败。
- 当某文件变更行数低于 `PLAN_MODE_LINE_THRESHOLD`（`50`）时，plan 阶段会被
  **自动跳过**。

## 另见

- [快速开始](../quickstart/)——安装并完成首次评审。
- [配置](../configuration/)——参数背后的环境变量与 config key。
- [评审规则](../review-rules/)——`--rule` 参数与规则解析。
- [集成](../integrations/)——从 agent 与 CI 调用 `ocr review`。
