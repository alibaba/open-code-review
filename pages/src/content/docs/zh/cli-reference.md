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
  ocr review --background "Focus on auth" --background-file ./docs/requirements.md  Review with context
  ocr review -B ./docs/requirements.md                                              Review with context file
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

## 全局参数

所有命令均可使用，且放在子命令前后皆可
（`ocr --color=never review` 与 `ocr review --color=never` 等价）。

| 参数 | 默认值 | 说明 |
|---|---|---|
| `--color <auto\|always\|never>` | `auto` | 何时输出 ANSI 颜色。`auto` 仅在 stdout 是终端时着色，因此管道或重定向得到的是纯文本。`always` 会在管道中保留颜色（便于配合 `\| less -R`）。 |

当 stdout 不是终端时，文本输出始终为纯文本，因此可以安全地通过管道传递：

```bash
ocr review --commit HEAD | gh issue comment 123 --body-file -
```

`TERM=dumb` 同样会关闭颜色。

## 命令总览

| 命令 | 别名 | 作用 |
|---|---|---|
| `ocr review` | `ocr r` | 运行代码评审并输出评论。 |
| `ocr scan` | `ocr s` | 无需 Git diff，扫描完整文件。 |
| `ocr rules check <file>` | — | 显示某文件路径适用哪条规则及其来源。 |
| `ocr config set <key> <value>` | — | 将一个配置值持久化到 `~/.opencodereview/config.json`。 |
| `ocr config unset <key>` | — | 清除一个已保存的配置值（`provider`、`max_tokens`、`effort`、`custom_providers.<name>`、`mcp_servers.<name>`）。 |
| `ocr config provider` | — | 交互式 provider 配置 TUI。 |
| `ocr config model` | — | 交互式 model 选择 TUI。 |
| `ocr llm test` | — | 发送一条简短 chat 请求以验证配置的端点。 |
| `ocr llm providers` | — | 列出所有内置 LLM provider。 |
| `ocr session list` | `ocr sessions list`, `ocr session ls` | 列出已保存的评审会话。 |
| `ocr session show <id>` | `ocr sessions show <id>` | 查看单个会话及其逐文件检查点。 |
| `ocr session comments <id>` | `ocr sessions comments <id>` | 输出单个会话中记录的评审评论。 |
| `ocr session compare <before> <after>` | `ocr session diff <before> <after>` | 对比两个会话的问题：新增、仍存在、已解决、未评审。 |
| `ocr viewer` | — | 启动用于历史评审会话的本地 Web UI（`localhost:5483`）。 |
| `ocr version` | — | 打印版本、commit、平台、构建日期与 GitHub URL。 |

`ocr` 和 `ocr -h` 打印顶层用法。每个子命令也接受 `-h` / `--help`。

## `ocr review`

主命令。解析 Git diff，把语义相关的文件分组，为每组分发一个子 agent，收集评审评论并打印。

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
| `--preview` | `-p` | `false` | 运行过滤流水线但跳过 LLM。打印文件列表与排除原因。支持 `--format json`；不支持 `--format sarif`（预览没有已完成的发现可供输出）。 |
| `--no-filter` | — | `false` | 保留所有评审评论，并跳过每个文件的 `REVIEW_FILTER_TASK` LLM 后处理调用。 |
| `--resume <session-id>` | — | — | 从之前兼容的区间或单 commit 评审会话恢复。 |
| `--format <fmt>` | `-f` | `text` | `text`（人类可读）、`json`（机器可读的评论数组）或 `sarif`（用于 GitHub Code Scanning 的 SARIF 2.1.0 报告）。 |
| `--output <path>` | `-o` | 标准输出 | 将评审结果写入 UTF-8 文件（`-` 表示标准输出）。首次写入时惰性创建文件，运行失败不会截断已有文件；文本格式自动剥离 ANSI 颜色码。 |
| `--audience <who>` | — | `human` | `human` 流式输出进度行（`--format` 为 `json`/`sarif` 时输出到 stderr，使 stdout 保持为单个可解析文档）；`agent` 完全抑制进度行，只打印最终摘要 / JSON。 |
| `--background <text>` | `-b` | — | 注入 plan + main prompt 的可选需求 / 业务上下文。 |
| `--background-file <path>` | `-B` | — | 用作评审背景的 Markdown 文件路径。与 `--background` 同时设置时会合并两者。 |
| `--exclude <patterns>` | — | — | 逗号分隔的 gitignore 风格排除模式；与 `rule.json` 的 excludes 合并。 |
| `--concurrency <n>` | — | `8` | 并行评审的最大文件数。 |
| `--timeout <minutes>` | — | `15` | 每文件截止时间。`0` 关闭超时。按 effort 轮数线性缩放（如 low/medium/high 分别为 15/30/45 分钟）。 |
| `--rule <path>` | — | — | 自定义 JSON 评审规则文件路径。覆盖项目级与全局 `rule.json`。 |
| `--max-tools <n>` | — | 模板默认 | 每文件最大工具调用轮数。`0` 用模板默认（`100`）；1–49 会被上调到 `50`；解析后的值只在**大于**模板默认值时才生效（即只能上调，不能下调）。 |
| `--max-tokens <n>` | — | 配置或模板默认 | 每文件**提示词** token 上限（review 默认 `200000`）。覆盖本次运行已保存的 `max_tokens` 设置。不影响输出上限——那由 `MAX_COMPLETION_TOKENS`（`16384`）单独控制。 |
| `--max-tokens-budget <n>` | — | `0`（无限制） | 限制本次评审的输入 + 输出 token 总量。超出预算后停止分发，并仍会发布部分结果。 |
| `--effort <level>` | — | 配置或 `medium` | 评审投入档位：`low` = 1 轮 main 循环，`medium` = 2 轮（默认），`high` = 3 轮。轮数越多召回越高、耗时与 token 也越多。可用 `ocr config set effort <level>` 持久化。 |
| `--provider <name>` | — | — | 为本次运行选择已配置的 provider。支持 `providers` 和 `custom_providers` 中的名称。 |
| `--model <name>` | — | — | 为本次运行覆盖已解析出的 LLM model（如 `claude-opus-4-6`）。 |
| `--max-git-procs <n>` | — | `16` | 并发 git 子进程的最大数。 |
| `--tools <path>` | — | 内嵌 | 自定义 JSON 工具配置文件路径。覆盖内嵌工具定义。 |

> 模式参数互斥：传 `--from`/`--to`，或 `--commit`，或都不传（工作区模式）。
> 混用会直接报错。
> `--resume` 仅支持区间或单 commit 评审，不能与 `--preview` 同时使用。

### 单次运行的 LLM 选择

`review` 和 `scan` 都接受 `--provider` 与 `--model`。这些覆盖仅作用于当前调用，
不会修改已保存的配置：

```bash
ocr review --provider anthropic --model claude-opus-4-6 --format json
ocr scan --provider openai --model gpt-5.4 --format json
```

显式 `--provider` 会在常规来源解析前，从已保存的 `providers` 或 `custom_providers`
中选择条目。不传 `--provider` 时，OCR 保持原有来源顺序：已保存的配置、完整的
`OCR_LLM_*` 环境配置、完整的 Claude Code 环境配置、shell rc 文件。`--model` 会覆盖
最终选中来源中的 model，但不会改变来源顺序。不完整的策略会继续回退，而不会与其他策略
混合。选中的内置 provider 仍可从其支持的环境变量读取凭据。

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
ocr session comments <session-id>
ocr review --from main --to feature-branch --resume <session-id>
ocr review --commit abc123 --resume <session-id>
```

恢复逻辑是严格的。只有当本次运行评审的对象与父运行完全一致时，checkpoint 才会被复用：

- 工作区评审不能恢复
- 评审模式必须一致：区间会话不能以单 commit 模式恢复
- 解析后的输入必须一致。ref 的*写法*不参与比较（`abc1234` 与 `abc1234def`
  指向同一个 commit），但如果相同的 ref 现在解析到不同的 diff，或规则、过滤器
  改变了选中的文件集合，整次恢复会被拒绝，而不是部分复用
- 切换 provider 或 model 必须通过 `--provider` / `--model` 显式声明；经由配置
  文件或环境变量发生的变化一律拒绝
- 父运行必须带有 run manifest，输入正是拿它来校验的。文件派发开始后，Ctrl-C 会
  优雅取消评审并写出 manifest，因此已完成的 checkpoint 仍可恢复；未能优雅关闭的
  进程和早于 run manifest 的老 session 则没有 manifest
- 只有父 manifest 认领过的文件才会复用。manifest 未认领或已损坏的 checkpoint 只
  影响它自己那个文件——该文件重新评审一次，其余不受影响
- `--preview` 和 `--resume` 不能同时使用

被拒绝的恢复不会留下任何产物：不创建 session、不写 manifest、不调用 LLM。

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

JSON 文档始终独占 stdout。在默认的 `--audience human` 下，`[ocr]` 进度行会在评审
过程中实时输出到 **stderr**，因此既能观察长时间运行的进度，又能把 stdout 直接管道
给解析器：

```bash
ocr review --format json > result.json   # 进度仍显示在终端上
ocr review --format json | jq .summary   # stdout 是单个 JSON 文档
```

传入 `--audience agent` 可完全去掉进度行，或用 `2>/dev/null` 在 shell 层丢弃。

```json
{
  "status": "success",
  "llm": {
    "provider": "anthropic",
    "model": "claude-opus-4-6"
  },
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
| `llm` | 实际解析的 LLM 标识。规范化后的 `model` 始终存在；`provider` 仅在使用已命名的配置 provider 时存在。 |
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
  "llm": {
    "provider": "anthropic",
    "model": "claude-opus-4-6"
  },
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

## `ocr scan`

无需 Git diff 的全文件扫描。直接从工作树读取每个文件的当前内容交给
LLM 评审——适合审计陌生代码库或没有有意义 diff 的目录。

```text
ocr scan [flags]
ocr s      [flags]   (alias)
```

不传入 `--path` 时，扫描整个仓库。

### 参数

| 参数 | 简写 | 默认 | 说明 |
|---|---|---|---|
| `--path <list>` | - | 整个仓库 | 逗号分隔的仓库相对目录或文件（如 `internal/agent`、`internal/llm/client.go`）。 |
| `--exclude <patterns>` | - | - | 逗号分隔的 gitignore 风格排除模式（如 `**/generated/*,*.pb.go`）；与 `rule.json` 的 excludes 合并。 |
| `--output <path>` | `-o` | 标准输出 | 将扫描结果写入 UTF-8 文件（`-` 表示标准输出）。首次写入时惰性创建文件，运行失败不会截断已有文件；文本格式自动剥离 ANSI 颜色码。 |
| `--preview` | `-p` | `false` | 枚举并过滤文件但跳过 LLM。打印文件列表、可评审/排除数量、总行数及每个文件的排除原因。支持 `--format json`；不支持 `--format sarif`。 |

```bash
ocr scan --preview                              # 查看会扫描哪些文件
ocr scan --path internal/agent                  # 扫描单个目录
ocr scan --path internal/agent,internal/llm/client.go
ocr scan --exclude '**/generated/*,*.pb.go'
```

完整参数列表见 `ocr scan -h`。

## `ocr session`

列出和查看保存在 `~/.opencodereview/sessions/` 下的本地评审会话日志。
可用它查找 session ID、查看逐文件检查点状态，并恢复中断的区间或单 commit 评审。

```text
ocr session <sub-command>
ocr sessions <sub-command>   (alias)

Sub-commands:
  list, ls        List recent review sessions for the current repo
  show <id>       Show one session's metadata and per-file items
  comments <id>   Show the review comments recorded in one session
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

恢复的运行还会打印它所继续的父运行，若这次恢复跨了 provider 或 model，也会打印这次切换。

```bash
ocr session show <session-id>
ocr session show --json <session-id>
ocr session show --repo /path/to/repo <session-id>
```

| 参数 | 默认 | 说明 |
|---|---|---|
| `--repo <path>` | 当前目录 | 要查看会话的仓库。 |
| `--json` | `false` | 以 JSON 输出会话元数据和逐文件条目。 |

### `ocr session comments`

输出会话中保存的所有评审评论，渲染风格与 `ocr review` 的终端输出一致
（路径、行范围、严重程度标签、建议 diff）。

```bash
ocr session comments <session-id>
ocr session comments --json <session-id>
ocr session comments --severity high <session-id>
ocr session comments --severity critical,high --category bug,security <session-id>
```

| 参数 | 默认 | 说明 |
|---|---|---|
| `--repo <path>` | 当前目录 | 要查看会话的仓库。 |
| `--json` | `false` | 以 JSON 数组输出评论。 |
| `--severity <list>` | 全部 | 逗号分隔的要包含的严重程度（`critical`、`high`、`medium`、`low`）。 |
| `--category <list>` | 全部 | 逗号分隔的要包含的类别（如 `bug`、`security`）。 |

### `ocr session compare`

将两个会话的问题分为四类：**new**（仅出现在 after 会话）、**persisting**
（两个会话都有）、**resolved**（仅出现在 before 会话）、**not reviewed**
（出现在 before 会话，但 after 会话没有评审该文件，因此不计为已解决）。

匹配依据是文件路径、类别和问题代码片段，而不是行号，所以仅仅是行号发生偏移
的问题仍然算作 persisting。

```bash
ocr session compare <before-session-id> <after-session-id>
ocr session diff <before-session-id> <after-session-id>
ocr session compare --json <before-session-id> <after-session-id>
```

两个会话必须属于同一个仓库，否则命令报错。评审模式不同只会在 stderr 输出
警告，因此 `--json` 输出仍可直接用于管道。

| 参数 | 默认值 | 说明 |
|---|---|---|
| `--repo <path>` | 当前目录 | 要对比会话的仓库。 |
| `--json` | `false` | 以 JSON 输出对比结果（`new`、`persisting`、`resolved`、`not_reviewed`）。 |

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
ocr config unset <key>                     Clear a saved config value
ocr config provider                        Interactive provider setup
ocr config model                           Interactive model selection
```

- **`set`**——非交互式写入单个配置值（如
  `ocr config set effort high`）。
- **`unset`**——清除一个已保存的配置值。支持 `provider`、`max_tokens`、
  `effort`、`custom_providers.<name>` 和 `mcp_servers.<name>`。删除当前启用的
  自定义 provider 时，`provider` 和 `model` 一并被清空（运行
  `ocr config provider` 重新选择）；`ocr config unset effort` 会回到默认的
  `medium` 档位。
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
  --open <mode>      when to open the browser: auto, always, never (default: auto)

Examples:
  ocr viewer                     # start and open the browser
  ocr viewer --addr :3000        # bind to all interfaces on port 3000
  ocr viewer --open=never        # just print the URL
  ocr viewer --open=always       # force it when auto declines (piped output, WSL)
```

启动一个内嵌 HTTP 服务器，读取 `~/.opencodereview/sessions/...`，并以浏览器友好的 UI 渲染历史评审会话。见[会话查看器](../viewer/)。

`--open=auto` 在以下情况跳过打开浏览器：stdout 不是终端、设置了
`SSH_CONNECTION` 且没有转发显示环境、或 Linux 上 `DISPLAY` 与 `WAYLAND_DISPLAY`
均为空；跳过的原因会与 URL 一并打印。若 auto 拒绝但其实有可用浏览器，用
`--open=always`。

## `ocr version`

```text
ocr version
ocr --version
ocr -V
```

打印构建时写入的版本信息、短 Git commit（存在时）、平台
（`<GOOS>/<GOARCH>`）、构建日期（存在时），以及 GitHub URL
（`https://github.com/alibaba/open-code-review`）。


## ocr completion

为 `ocr` 生成 shell 补全脚本，以便在 shell 中对命令名、参数和标志进行 Tab 补全。

### Bash

仅当前会话生效：

```bash
source <(ocr completion bash)
```

持久生效（Linux）：

```bash
ocr completion bash > /etc/bash_completion.d/ocr
```

持久生效（macOS）：

```bash
ocr completion bash > $(brew --prefix)/etc/bash_completion.d/ocr
```

### Zsh

如果 shell 补全尚未启用，请先执行一次：

```bash
echo "autoload -U compinit; compinit" >> ~/.zshrc
```

然后持久加载补全：

```bash
ocr completion zsh > "${fpath[1]}/_ocr"
```

需要打开一个新的 shell 才能生效。

### Fish

仅当前会话生效：

```bash
ocr completion fish | source
```

持久生效：

```bash
ocr completion fish > ~/.config/fish/completions/ocr.fish
```

### PowerShell

仅当前会话生效：

```powershell
ocr completion powershell | Out-String | Invoke-Expression
```

持久生效 —— 先生成脚本，再从 PowerShell 配置文件中加载：

```powershell
ocr completion powershell > ocr.ps1
```

然后在 PowerShell 配置文件中添加一行以加载 `ocr.ps1`。


## man 手册页

`ocr` 提供 UNIX 手册页。安装后可用以下命令阅读：

```sh
man ocr
```

`install.sh` 安装器会自动安装该页面。如需自行生成（例如放到自定义的 `MANPATH` 下）：

```sh
ocr man <directory>
```


## 提示与注意

- `--audience agent` **并不**隐含 `--format json`。两者控制不同的事——屏蔽 UI
  vs 结构化载荷。需要二者兼得时组合使用。
- `--background` 是提升评审质量最有效的参数之一——从其他 agent 调用时，始终传入
  需求 / PR 描述。
- 某文件 diff 单独超过 `MAX_TOKENS` 的 80%（默认 `200000`）时，会在调用 LLM 前
  被丢弃。这会记录日志但不会使运行失败。
- 当一个文件组既没有单个文件的变更行数达到 `PLAN_MODE_LINE_THRESHOLD`（`50`），
  合计变更行数也没达到 `PLAN_MODE_GROUP_LINE_THRESHOLD`（`100`，仅对多文件组
  生效）时，plan 阶段会被**自动跳过**。

## 另见

- [快速开始](../quickstart/)——安装并完成首次评审。
- [配置](../configuration/)——参数背后的环境变量与 config key。
- [评审规则](../review-rules/)——`--rule` 参数与规则解析。
- [集成](../integrations/agent-skill/)——从 agent 与 CI 调用 `ocr review`。
