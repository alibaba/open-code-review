---
title: 会话查看器
sidebar:
  order: 10
---

`ocr viewer` 是一个小型内嵌 HTTP 服务器，以浏览器友好的 UI 渲染历史评审会话。
无外部依赖——会话直接从 OCR 在每次评审期间写入磁盘的 JSONL 文件读取。

## 启动

```bash
ocr viewer                  # binds localhost:5483
ocr viewer --addr :3000     # bind to all interfaces on port 3000
ocr viewer --addr 0.0.0.0:8080   # bind on all interfaces
```

默认地址是 `localhost:5483`。服务器在前台运行——`Ctrl+C` 停止。会话在每次请求时
从 `~/.opencodereview/sessions/` 惰性扫描，因此另一个终端里运行的评审一旦其
JSONL 文件出现就会显示。

> **DNS-rebinding 防护。** 查看器会对照 loopback 白名单
> （`localhost`、`127.0.0.1`、`::1`）检查 `Host` 头。具体的绑定主机
> （如 `--addr 192.168.1.10:5483`）会自动加入，但**通配**绑定
> （`:3000`、`0.0.0.0`、`::`）不会——此时从 LAN IP 或主机名访问 UI 会返回
> `forbidden host`。要让通配绑定可被访问，设置
> `OCR_VIEWER_ALLOWED_HOSTS` 为逗号分隔的允许主机名列表
> （如 `OCR_VIEWER_ALLOWED_HOSTS=box.local,192.168.1.10`）。

## 三个页面

查看器有三个 URL：

| URL | 看到内容 |
|---|---|
| `/` | 磁盘上有会话的所有仓库列表。 |
| `/r/{repo}` | 单个仓库的会话列表，最新在前。 |
| `/r/{repo}/{sessionID}` | 单个会话的完整详情。 |

`{repo}` 是一个路径编码字符串（分隔符 `/` 和 `\` 替换为 `-`、冒号替换为
`_`——与磁盘目录命名相同的编码）。通常你不会手动输入它——而是点击进入。

### `/`——仓库列表

对每个至少有一条会话的仓库，显示仓库路径、总会话数和最近活动时间戳。

### `/r/{repo}`——单仓库会话列表

对每个会话：ID（一个 UUID）、分支名（OCR 能检测到时）、评审模式、模型、文件数、
时长和开始时间戳。

### `/r/{repo}/{sessionID}`——会话详情

详情页是最有用的那个。它显示：

1. **头部**——diff 范围、模型、分支、总 token、运行时长。
2. **文件分组**——每个被评审文件一个块。每个文件内，五条“任务类型”泳道：

| 任务类型 | 何时出现 |
|---|---|
| `plan_task` | 运行了 plan 阶段（文件 ≥ `PLAN_MODE_LINE_THRESHOLD`）。 |
| `main_task` | 每个文件。主评审循环。 |
| `review_filter_task` | 为该文件运行了评审后评论过滤流程。 |
| `memory_compression_task` | active+compress 区超过 60 % / 80 % 预算。 |
| `re_location_task` | 某条 `code_comment` 无法锚定，回退重新定位运行。 |

每条泳道是**任务卡片**的水平条带——每个 LLM 往返一张。卡片按任务类型着色，让你
一眼看出哪些阶段主导了运行。

## 任务卡片里有什么

点击任务卡片展开。每张卡片有：

- 一行**头部**——请求号、模型徽章、token 徽章（`P:` prompt / `C:` completion，
  存在时还显示 `CR:` / `CW:` 缓存读写）、时长徽章，以及该轮失败时的错误徽章；
- **Response**——原始 assistant 响应，包括任何推理 / `thinking` 块；
- **Tool calls**——每个工具调用及其参数 + 返回结果（可折叠）。

发给模型的完整消息列表和作用域内工具定义**不**在卡片 UI 中渲染；如需要，可直接
检查 JSONL 转录（每条 `llm_request` 记录的 `messages` 字段）。

## 使用场景

查看器围绕三个工作流设计：

### “模型为什么这么说？”

在终端输出中打开一条评论，在查看器中定位该文件，沿着它的 `main_task` 泳道向下查看。
**工具调用**中包含你关心的 `code_comment` 的那张卡片，就是产出它的那一轮。卡片的
Response 显示模型推理；要确切知道发给模型的 prompt + 上下文，在 JSONL 转录中
打开该请求号的 `llm_request` 记录（其 `messages` 字段）。

### “这个文件为什么静默？”

一个**无评论**的文件，只有当模型*主动*调用 `task_done` 时才是成功评审。若泳道
显示工具调用但无 `code_comment`，那是模型主动给出的干净评审。若泳道以错误卡片结束，那是
伪装成静默的失败——应作为警告处理。

### “压缩保留 / 丢弃了什么？”

`memory_compression_task` 泳道显示每次压缩轮。其中，Response 窗格有结果摘要；
被压缩的 compress 区渲染出的 XML 在该轮 `llm_request` 的 `messages`（JSONL 转录中）。
排查“模型忘了早前上下文”这类反馈时有用——你能看到压缩是否丢弃了相关细节。

## 磁盘存储布局

查看器读取：

```
~/.opencodereview/sessions/
└── <path-encoded-repo-path>/
    └── <session-id>.jsonl
```

JSONL 文件每行是一个事件：

```json
{"type": "llm_request", "filePath": "src/foo.go", "taskType": "main_task", "request_no": 1, "messages": [{"role": "user", "content": "Review this diff…"}], "timestamp": "2026-06-02T10:15:23Z"}
{"type": "llm_response", "filePath": "src/foo.go", "taskType": "main_task", "model": "claude-sonnet-4-6", "content": "Found 2 issues…", "duration_ms": 8421, "usage": {"prompt_tokens": 12450, "completion_tokens": 320}}
{"type": "tool_call", "filePath": "src/foo.go", "tool_name": "file_read", "arguments": "{\"file_path\":\"src/foo.go\",\"start_line\":1,\"end_line\":50}", "result": "File: src/foo.go (Total lines: 220)\nIS_TRUNCATED: false\nLINE_RANGE: 1-50\n1|package foo…", "ok": true, "duration_ms": 14}
```

行是 append-only——不完整的 JSONL 意味着会话在运行中被中断，查看器会渲染已写入的
内容。

要释放磁盘空间，删除整个会话文件；查看器在下次请求时重建索引。

## 隐私

JSONL 转录包含发给 LLM 和从 LLM 收到的**一切**，包括 diff 中的任何代码。它们
完全存在于你机器的 `~/.opencodereview/` 内。OCR 不会把它们上传到任何地方。

如果你的评审包含你不想长期存储的代码，可以：

- 定期删除会话文件，或
- 在 CI 中把 `--audience agent --format json` 输出重定向到临时管道，并用临时
  `HOME` 运行，使 JSONL 不会被持久化。

OpenTelemetry exporter 是另一回事——如何让 prompt 内容不进入导出 trace 见
[遥测](../telemetry/)。

## 查看器*不*适用时

- 程序化后处理（CI、仪表盘）用 `ocr review --format json --audience agent`。
  查看器为人渲染，不为机器。
- 如需跨多会话 grep，直接对 JSONL 文件用 `jq`。UI 中暂无搜索框。

## 另见

- [架构](../architecture/)——那五种任务类型在底层实际做什么。
- [工具](../tools/)——你在 `main_task` 卡片中会看到的工具调用。
