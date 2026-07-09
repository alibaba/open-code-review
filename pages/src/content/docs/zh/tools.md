---
title: 工具
sidebar:
  order: 9
---

OCR 内置 **六个工具**，供 LLM 在评审过程中调用。本页记录每个工具的用途、输入
schema 与示例输入/输出。完整的机器可读定义位于
[`internal/config/toolsconfig/tools.json`](https://github.com/alibaba/open-code-review/blob/main/internal/config/toolsconfig/tools.json)。

## 各阶段工具可用性

每个工具声明它在 **plan 阶段**、**main task** 还是两者中都可用：

| 工具 | Plan | Main | 用途 |
|---|---|---|---|
| `task_done` | ✗ | ✓ | 表示“我完成了”——终止循环。 |
| `code_comment` | ✗ | ✓ | 发出一条带行范围 + 建议的评审评论。 |
| `file_read` | ✗ | ✓ | 读取变更后快照中某文件的一段。 |
| `file_read_diff` | ✓ | ✓ | 读取另一文件的 diff 以确认跨文件关切。 |
| `file_find` | ✓ | ✓ | 按文件名关键字定位文件。 |
| `code_search` | ✓ | ✓ | 全仓 grep（字面量或正则）。 |

`task_done` 和 `code_comment` 在 plan 阶段有意**不可用**：plan 是只读的。

> **上下文工具是只读上下文，不是评论目标。** `main_task` prompt 明确禁止对
> *其他*文件中的发现发表评论。`file_read`、`file_read_diff`、`file_find` 和
> `code_search` 的存在是为了让模型更好地理解当前文件的 diff——收集该上下文时
> 发现的任何问题按设计都会被忽略。跨文件关切只有在**当前文件 diff**中可观测
> 时，才会作为评论出现。

要覆盖工具注册表，传入一个与内嵌定义同形的 JSON 文件路径 `--tools <path>`。
借此可以禁用工具、编辑描述，或基于已有 provider 添加新工具。

## `task_done`

终止 main 循环。

```json
{
  "name": "task_done",
  "input": { "state": "DONE" }
}
```

| 字段 | 必需 | 含义 |
|---|---|---|
| `state` | 是 | `DONE`（默认）或 `FAILED`。`FAILED` 表示“我确实无法用可用工具完成此事”——几乎从不是正确的选择。 |

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
模型极少需要显式设置。`thinking`（按评论）捕获模型推理，保留在评论上，但不会
在最终评审输出中显示。

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

读取文件**变更后**形式的一段行。

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

| 字段 | 必需 | 默认 | 说明 |
|---|---|---|---|
| `file_path` | 是 | — | 相对仓库根的路径。 |
| `start_line` | 否 | `1` | 从 1 开始索引。 |
| `end_line` | 否 | 文件末尾 | 含端点。 |

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
- 只读取文件的**修改后版本**。要看旧版本，用 `file_read_diff`。

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

若某路径不在变更集中，该条目被静默省略。若请求的路径**都不**在变更集中，工具
返回 `Error: diff not found for the requested paths`；空的 `path_array` 返回
`Error: no files found`。

## `file_find`

按文件名关键字（子串匹配）在仓库中查找文件。

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

| 字段 | 必需 | 默认 | 说明 |
|---|---|---|---|
| `query_name` | 是 | — | 与每个文件的 **basename**（最后一个 `/` 之后的部分）做子串匹配，而非全路径。 |
| `case_sensitive` | 否 | `false` | 设为 `true` 做精确大小写匹配。 |

候选集在工作区模式下来自 `git ls-files --cached --others --exclude-standard`，
在区间 / commit 模式下来自 `git ls-tree -r --name-only <ref>`。无扩展名的文件
被跳过，但 `Makefile`、`Dockerfile`、`LICENSE`、`Vagrantfile`、
`Containerfile` 例外。

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

| 字段 | 必需 | 默认 | 说明 |
|---|---|---|---|
| `search_text` | 是 | — | 字面量字符串或 PCRE 模式（见 `use_perl_regexp`）。 |
| `file_patterns` | 否 | 全仓 | pathspec 条目数组。用 `:(exclude)pat` 做排除。 |
| `case_sensitive` | 否 | `false` | — |
| `use_perl_regexp` | 否 | `false` | 为 `true` 时，`search_text` 作为正则处理。 |

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

| 目标 | `file_patterns` |
|---|---|
| 单个文件 | `["src/main.go"]` |
| 所有 Go 文件 | `["*.go"]` |
| 除测试外所有 Go | `["*.go", ":(exclude)*_test.go"]` |
| 仅一个目录 | `["src/api/"]` |
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

若工具名不在注册表中，OCR 返回常量 `tool.NotAvailableMsg` 而不是崩溃。这使得
（通过 `--tools`）运行时禁用工具是安全的。

## 自定义工具

两种扩展方式：

### 1. 禁用工具

复制 `tools.json`，删掉不想要的条目，然后运行：

```bash
ocr review --tools ./my-tools.json
```

例如，想要一个从不读额外上下文的“仅评论”评审器，只保留 `code_comment` 和
`task_done`。

### 2. 重新描述工具

保留 `name`（provider 内部按 name 查找）但更改 `description` 以引导模型。这是
注入项目专属指引最简单的方式——如“使用 `file_read` 时，始终读取变更附近至少
30 行。”

> 添加**新**工具*名*需要在 Go 侧接入；见 `internal/tool/definitions.go` 及
> `internal/tool/` 下的 provider。仅靠 JSON 文件无法添加新行为。

## 另见

- [架构](../architecture/)——agent 循环如何驱动工具。
- [评审规则](../review-rules/)——告知 LLM 关注什么。
- [会话查看器](../viewer/)——查看过去评审中到底触发了哪些工具。
