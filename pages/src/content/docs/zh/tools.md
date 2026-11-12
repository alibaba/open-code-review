---
title: 工具
sidebar:
  order: 9
---

OCR 内置 **六个工具**，供 LLM 在评审过程中调用。本页记录每个工具的用途、输入
schema 与示例输入/输出。基础的机器可读定义位于
[`internal/config/toolsconfig/tools.json`](https://github.com/alibaba/open-code-review/blob/main/internal/config/toolsconfig/tools.json)。

## 各阶段工具可用性

任务模板按名称从共享定义中选择工具。在 `task_template.json` 中，
`MAIN_TASK.tools` 是发送给模型的有序可调用工具列表，
`PLAN_TASK.tool_references` 则仅作为参考文本渲染进规划 prompt。
规划仍然是没有可调用工具的单次请求；这两个字段都不会改变工具定义。

`scan_template.json` 独立选择主任务工具，不包含 `file_read_diff`，因为全文件扫描没有 diff。
省略或留空的列表不选择任何工具，不会启用整个目录。重复名称按首次出现的顺序去重。
未知引用、重复的目录条目和无效定义会在任务初始化时导致失败。

默认 diff 评审的选择如下：

| 工具             | Plan | Main | 用途                                   |
| ---------------- | ---- | ---- | -------------------------------------- |
| `task_done`      | ✗    | ✓    | 表示“我完成了”——终止循环。             |
| `code_comment`   | ✗    | ✓    | 发出一条带行范围 + 建议的评审评论。    |
| `file_read`      | ✓    | ✓    | 读取所选比较侧中某文件的一段。         |
| `file_read_diff` | ✓    | ✓    | 读取另一文件的 diff 以确认跨文件关切。 |
| `file_find`      | ✓    | ✓    | 按文件名关键字定位文件。               |
| `code_search`    | ✓    | ✓    | 全仓 grep（字面量或正则）。            |

`task_done` 和 `code_comment` 在 plan 阶段有意**不可用**：plan 是只读的。

> **上下文工具是只读上下文，不是评论目标。** `main_task` prompt 明确禁止对
> *其他*文件中的发现发表评论。`file_read`、`file_read_diff`、`file_find` 和
> `code_search` 的存在是为了让模型更好地理解当前文件的 diff——收集该上下文时
> 发现的任何问题按设计都会被忽略。跨文件关切只有在**当前文件 diff**中可观测
> 时，才会作为评论出现。

内置工具定义随 OCR 发布，不支持运行时覆盖。项目指引使用[评审规则](../review-rules/)，
外部业务上下文使用 [MCP](../mcp/)。曾使用自定义工具文件的用户请参阅
[迁移说明](#customizing-tools)。

## 比较两侧 {#comparison-sides}

commit 和 range 评审（包括恢复的会话）会为 `file_read`、`code_search` 和 `file_find`
提供可选参数 `side`。省略该参数或传入 `"new"`，保持默认行为和输出格式；
传入 `"old"`，即可读取、搜索或枚举同一次比较的旧侧代码：

```json
{"file_path": "src/foo.go", "start_line": 10, "end_line": 80, "side": "old"}
{"search_text": "OldAPI", "file_patterns": ["src/"], "side": "old"}
{"query_name": "foo.go", "side": "old"}
```

| 评审输入 | 新侧 | 旧侧 |
| --- | --- | --- |
| `--from BASE --to HEAD` | 固定的 head 提交 | 固定端点的 merge-base，而非目标分支的最新提交 |
| `--commit COMMIT` | 固定的提交 | 第一个父提交，合并提交也一样 |
| 根提交 | 固定的提交 | 空树 |

diff 和这三个工具共享这些不可变端点。路径和行号属于所选的一侧。对于重命名文件，
旧侧应使用旧路径；工具不会自动转换名称，也不会回退到其他版本。已删除文件仍可
从旧侧读取。根提交的旧侧没有文件：读取会报告文件不存在，搜索和枚举则返回空结果。
Git 对象不可用或 Git 操作失败时会报错，不能据此判定为空树。
对于处于浅克隆边界的提交，必须先补全所需历史再评审，即使父提交对象已存在于本地，
以确保 Git diff 与旧侧工具使用一致的比较基线。

旧侧结果以 `SIDE: old` 和 `REVISION: <commit-sha>` 开头；空树则使用
`SOURCE: empty-tree`。旧侧错误也会标明来源。现有的行数和匹配数限制对两侧
都适用。只接受字符串 `"new"` 和 `"old"`；null、空字符串和任意 Git ref 均无效。

工作区评审和扫描保留现有定义、prompt 和默认工具输出格式，不暴露 `side`，
传入时会明确拒绝。共享的 `tools.json` 目录仍是 current-only 定义的来源，
不声明 `side`。比较评审在初始化时通过 `WithComparisonSides` 从该目录派生
可调用工具定义和规划参考，添加 `side` 并更新描述，不修改基础定义。

恢复 commit 或 range 评审时使用当前内置工具，并要求解析后的新旧端点与原会话
一致，即使选中的 patch 没变：任一代码树中的其他上下文都可能影响之前的评审。
不同 ref 名称只要解析到相同端点，仍可恢复。如果任一端点变化，或旧会话缺少
足够的端点信息来验证比较，应新建评审而不是恢复。已完成文件会复用结果；
待完成文件使用当前工具评审。若要让所有文件都使用当前能力重新评审，请新建评审。

## `task_done`

终止 main 循环。

```json
{
  "name": "task_done",
  "input": { "state": "DONE" }
}
```

| 字段    | 必需 | 含义                                                                                           |
| ------- | ---- | ---------------------------------------------------------------------------------------------- |
| `state` | 是   | `DONE`（默认）或 `FAILED`。`FAILED` 表示“我确实无法用可用工具完成此事”——几乎从不是正确的选择。 |

agent 看到 `task_done` 后，停止调用 LLM 并开始处理已累积的 `code_comment`
调用。`task_done` 立即返回（在结果记入会话日志之前），因此 `state` 值会被接受
但**不**持久化——它也不影响退出码。

## `code_comment`

发出一条或多条评审评论。每条评论锚定到一个代码片段（`existing_code`），以便
OCR 自动计算行号。

### Schema

```json
{
  "name": "code_comment",
  "input": {
    "path": "string — optional, override the file path for this comment",
    "comments": [
      {
        "content": "string — the comment in the configured language",
        "existing_code": "string — snippet from the diff to anchor on",
        "suggestion_code": "string — optional fix snippet",
        "thinking": "string — optional, the model's reasoning for this comment"
      }
    ]
  }
}
```

`comments` 是数组，因此模型可以在一次工具调用中发出多条评论。`content` 和
`existing_code` 必需；`suggestion_code` 可选但建议提供。`path` 是顶层可选覆盖——
省略时，agent 会注入当前评审的文件。即便模型省略，agent 也会自动注入 `path`，因此
模型极少需要显式设置。`thinking`（按评论）捕获模型推理，保留在评论上；OCR 会用
模型当轮输出的推理内容自动回填（模型未输出推理内容时保持为空），并包含在 JSON 输出中（终端输出不渲染）。

> **`thinking` 是运行时专属字段。** OCR 会解析并存储它，但有意**不**把它列入
> 给模型的 `code_comment` schema（`tools.json` 中只有 `content`、
> `existing_code` 和 `suggestion_code`）。更强模型若仍发出 `thinking` 块，也会
> 被持久化；多数模型不会发，没问题。

### 锚定算法

OCR 用一个**动态滑动窗口**在 diff 中查找 `existing_code` 文本。匹配按序尝试：

1. **hunk 新侧**——一段连续的 **context + added** 行（不是仅有 deleted、也不是仅有
   unchanged），得到新文件行号。若失败，OCR 重试 **hunk 旧侧**——context +
   deleted 行——得到旧文件行号。
2. **全新文件扫描**——若无 hunk 匹配，OCR 对整个变更后文件逐行扫描连续匹配
   （`resolveFromFileContent`）。
3. **重新定位任务**——若文本匹配在较复杂的 diff 上仍失败，OCR 运行
   `RE_LOCATION_TASK` prompt，请模型重新锚定片段。

匹配**对空白不敏感**：比较前会 trim 行并去除 diff 的 `+`/`-` 标记，因此缩进
无需精确一致。作为最后手段，评论会以 `start_line=0` 交付，告诉用户“问题是真实的，但需自行定位”。

### 示例

```json
{
  "comments": [
    {
      "content": "`tx.Rollback()` is never deferred — early returns leak the transaction.",
      "existing_code": "tx, err := db.Begin()\nif err != nil {\n    return err\n}",
      "suggestion_code": "tx, err := db.Begin()\nif err != nil {\n    return err\n}\ndefer tx.Rollback()"
    }
  ]
}
```

## `file_read`

默认读取文件**变更后**形式的一段行。
commit 和 range 评审还支持可选的[比较侧](#comparison-sides)。

### Schema

```json
{
  "name": "file_read",
  "input": {
    "file_path": "src/foo.go",
    "start_line": 10,
    "end_line": 80
  }
}
```

| 字段         | 必需 | 默认     | 说明               |
| ------------ | ---- | -------- | ------------------ |
| `file_path`  | 是   | —        | 相对仓库根的路径。 |
| `start_line` | 否   | `1`      | 从 1 开始索引。    |
| `end_line`   | 否   | 文件末尾 | 含端点。           |

### 输出

```
File: src/foo.go (Total lines: 220)
IS_TRUNCATED: false
LINE_RANGE: 10-80
10|package foo
11|
12|import (
13|    "fmt"
…
```

每行内容以 1 起始行号和 `|` 分隔符为前缀，以便模型在后续 `code_comment` 调用中
精确引用行号。

### 限制

- **每次调用最多 500 行。** 更大范围会被截断，置 `IS_TRUNCATED: true`，并追加
  `Note: Results truncated to 500 lines. Please narrow your line range.`。
- 默认读取文件的**修改后版本**。在 commit 和 range 评审中，使用 `side: "old"`
  可读取旧侧完整内容；使用 `file_read_diff` 可查看变更片段。

当模型需要周围上下文（对某个只能在 diff 中看到的函数发表评论时），应从 diff
hunk 头 `@@ -x,y +m,n @@` 计算范围——通常 `m-50` 到 `m+n+50`。

## `file_read_diff`

读取同一变更集中一个或多个*其他*文件的 diff——当评论取决于某相关文件是否被
更新时有用。

### Schema

```json
{
  "name": "file_read_diff",
  "input": {
    "path_array": ["src/api/handler.go", "src/db/queries.go"]
  }
}
```

### 输出

```
==== FILE: src/api/handler.go ====
--- a/src/api/handler.go
+++ b/src/api/handler.go
@@ -10,1 +10,2 @@
- old line
+ new line 1
+ new line 2

==== FILE: src/db/queries.go ====
@@ -5,1 +5,1 @@
- query := "SELECT *"
+ query := "SELECT id"
```

不在变更集中的路径不会被静默丢弃：它会在 `Error: diff not found for: <path>`
中被点名，而已找到路径的 diff 仍会先于该错误返回。当本次运行对其他文件有 diff 时，
消息会列出它们（`Diffs exist for: ...`），以便纠正拼写错误的路径；当本次运行完全
没有 diff 时，消息会指引改用 `file_read`。空的 `path_array` 返回
`Error: no files found`。

## `file_find`

按仓库相对路径或文件名关键字（子串匹配）在仓库中查找文件。

### Schema

```json
{
  "name": "file_find",
  "input": {
    "query_name": "UserService",
    "case_sensitive": false
  }
}
```

| 字段             | 必需 | 默认    | 说明                                                                       |
| ---------------- | ---- | ------- | -------------------------------------------------------------------------- |
| `query_name`     | 是   | —       | 与每个文件的**仓库相对路径**做子串匹配（同时支持 `/` 和 `\` 路径分隔符）。 |
| `case_sensitive` | 否   | `false` | 设为 `true` 做精确大小写匹配。                                             |

候选集在工作区模式下来自 `git ls-files --cached --others --exclude-standard`，
在区间 / commit 模式下来自 `git ls-tree -r --name-only <ref>`。无扩展名的文件
被跳过，但 `Makefile`、`Dockerfile`、`LICENSE`、`Vagrantfile`、
`Containerfile`、`BUILD`、`WORKSPACE` 例外。

### 输出

以换行分隔的路径列表：

```
src/main/java/com/example/UserService.java
src/test/java/com/example/UserServiceTest.java
src/main/java/com/example/internal/UserServiceImpl.java
```

当没有文件匹配（或 `query_name` 为空）时，工具返回字面字符串
`// The file was not found`。

### 限制

最多返回 **100** 条匹配；超出被静默截断。若模型需要更广搜索，应改用
`code_search`。

## `code_search`

全仓全文搜索。由 `git grep` 驱动，因此理解 `pathspec` 语法并遵循
`.gitignore`。

### Schema

```json
{
  "name": "code_search",
  "input": {
    "search_text": "TODO|FIXME",
    "file_patterns": ["*.go", ":(exclude)vendor/"],
    "case_sensitive": false,
    "use_perl_regexp": true
  }
}
```

| 字段              | 必需 | 默认    | 说明                                               |
| ----------------- | ---- | ------- | -------------------------------------------------- |
| `search_text`     | 是   | —       | 字面量字符串或 PCRE 模式（见 `use_perl_regexp`）。 |
| `file_patterns`   | 否   | 全仓    | pathspec 条目数组。用 `:(exclude)pat` 做排除。     |
| `case_sensitive`  | 否   | `false` | —                                                  |
| `use_perl_regexp` | 否   | `false` | 为 `true` 时，`search_text` 作为正则处理。         |

### 输出

结果按文件分组。每组以 `File: <path>` 和 `Match lines: <n>` 开头，随后每条命中
一行 `line|content`：

```
File: path/to/example.java
Match lines: 2
433|      String name = toolRequest.get().getName();
438|      logToolRequest(newPath, tool, toolRequest.get());

File: path/to/other.java
Match lines: 1
22|      var req = new ToolRequest();
```

无匹配时，工具返回字面字符串 `No matches found`。

### pathspec 速查

| 目标                  | `file_patterns`                                                    |
| --------------------- | ------------------------------------------------------------------ |
| 单个文件              | `["src/main.go"]`                                                  |
| 所有 Go 文件          | `["*.go"]`                                                         |
| 除测试外所有 Go       | `["*.go", ":(exclude)*_test.go"]`                                  |
| 仅一个目录            | `["src/api/"]`                                                     |
| 多种类型，排除 vendor | `["*.go", "*.ts", ":(exclude)vendor/", ":(exclude)node_modules/"]` |

### 限制

- 通过 `git grep --max-count 100` 把每文件命中数上限设为 **100**，因此跨多文件的
  总输出可能超过 100。触及每文件上限时，输出前会加
  `Note: The results have been truncated. Only showing first 100 results.`。
- 空 / 仅空白的 `search_text` 返回 `Error: search_text is blank`，而不是展开成
  每一行。
- 工作区模式搜索**当前工作树**，区间 / commit 模式搜索解析出的目标 ref
  （`FileReader.Ref` 作为位置参数传给 `git grep`）。

## 工具执行与错误

工具在 agent 循环内同步执行，两个例外：

- `code_comment` 派发到 **CommentWorkerPool**，使循环不阻塞在行解析 + 反思上。
- `task_done` 短路——立即返回，不调用任何 provider。

工具出错时（网络失败、参数格式错误、文件未找到），结果作为常规工具结果交付给模型，
文本形如 `"Error: file not found: src/missing.go"`。模型再决定是重试、换文件，
还是调 `task_done`。

若工具名不在注册表中，OCR 返回常量 `tool.NotAvailableMsg` 而不是崩溃，
将未知工具调用作为工具不可用的结果反馈给模型。

## 工具定制限制与迁移 {#customizing-tools}

OCR 使用专用审查 Agent，核心工作流与内置工具通过测试和 benchmark 一同验证，
而不是由用户自行定义工具注册表。

**迁移说明：** `ocr review` 和 `ocr scan` 均已移除 `--tools`。
旧调用会报 `unknown flag: --tools`；请从脚本、CI 任务和 shell 别名中删除该参数及其文件路径。

- 项目专属审查指引使用[评审规则](../review-rules/)或 `--background`
  （`review` 还支持 `--background-file`）。
- 需求、知识库等外部业务上下文使用 [MCP 服务器](../mcp/)。目前只有 `review`
  接入 MCP，`scan` 尚未接入；MCP 不能替换同名内置工具。
- 禁用内置工具、修改其描述、schema 或阶段可见性没有运行时替代方案。
  核心变更需要修改源码、重新构建，并进行相应测试和 benchmark 验证。

`--max-tools` 仍控制工具调用轮次预算。MCP 配置项 `mcp_servers.<name>.tools`
仍用于筛选外部工具，与移除的参数无关。历史会话文件仍可读取，但对曾使用自定义工具定义的
会话，建议重新开始审查，而不是恢复旧会话。

## 另见

- [架构](../architecture/)——agent 循环如何驱动工具。
- [评审规则](../review-rules/)——告知 LLM 关注什么。
- [会话查看器](../viewer/)——查看过去评审中到底触发了哪些工具。
