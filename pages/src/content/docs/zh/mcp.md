---
title: MCP 服务器
sidebar:
  order: 10
---

OCR 可以作为 **Model Context Protocol（MCP）客户端**。你把它指向一个或多个外部
MCP server，这些 server 暴露的工具就会提供给审查 agent —— 与 `file_read`、
`code_search` 等[内置工具](../tools/)并列。

## 何时使用

当审查器需要 diff 之外的上下文时，就该引入 MCP server：

- **Issue / 工单查询** —— 让 agent 拉取关联的 Jira / GitHub issue，核对变更是否
  符合声明的需求。
- **文档 / 知识库** —— 拉取内部 API 文档或编码规范，让评论引用真正的团队约定。
- **自定义分析** —— 把 linter、schema 校验器或依赖检查器暴露为工具，供审查器按需
  调用。

如果你只需要读仓库本身，内置工具就够了 —— MCP 是为了触达 checkout 之外的东西。

## 配置

#### 添加本地 MCP server

`ocr config set` 命令以非交互方式写入这些字段。数组字段（`args`、`env`、`tools`）
接受 JSON 数组字符串：

```bash
# 最小配置：只给命令
ocr config set mcp_servers.docs.command npx

# 参数
ocr config set mcp_servers.docs.args '["-y", "@acme/docs-mcp-server"]'

# 限制暴露给审查器的工具
ocr config set mcp_servers.docs.tools '["search_docs", "get_page"]'

# server 启动前运行的 setup 命令
ocr config set mcp_servers.docs.setup "npm install -g @acme/docs-mcp-server"

# 环境变量（KEY=VALUE 条目）
ocr config set mcp_servers.docs.env '["DOCS_TOKEN=secret", "DOCS_REGION=eu"]'
```

#### 添加远程 MCP server

对于支持 **Streamable HTTP** 的 server，把 `type` 设为 `remote`，并给出 `url` 而不是
本地命令。只设 `url` 是不够的：默认类型是 `stdio`。

使用一个新的 server 名称，以免这些命令覆盖已有的连接：

```bash
ocr config set mcp_servers.search.type remote
ocr config set mcp_servers.search.url https://mcp.example.com/mcp
ocr config set mcp_servers.search.tools '["search", "fetch"]'
```

这些命令会把连接保存到你的用户配置中。下一次审查时，OCR 会连接该 server，并把
`search` 与 `fetch` 与内置工具一并提供给 agent。工具白名单会把 server 可能提供的
其他工具挡在审查之外。其他已配置的 server 和你的审查设置保持不变。

配置完成后，agent 在审查过程中调用这些工具不会逐次征求同意。工具参数 —— 搜索
查询、请求的 URL，以及 agent 附带的任何上下文 —— 都会离开你的机器，抵达该端点的
运营方。由于这是用户级配置，它对所有仓库生效：只在允许对外请求的场景启用它，且不要
在请求中包含密钥、私有代码或内网 URL。接入第三方服务前，请先了解其隐私政策与服务
条款。

#### 移除 MCP server

用 `unset` 移除某个 server：

```bash
ocr config unset mcp_servers.docs
```

MCP server 配置在用户配置文件（`~/.opencodereview/config.json`）的 `mcp_servers` 键下。

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `type` | string | | `stdio`（默认）表示本地子进程，`remote` 表示 Streamable HTTP。 |
| `command` | string | `stdio` 必填 | 启动 MCP server 的可执行文件（如 `npx`、`uvx`、绝对路径）。 |
| `args` | string 数组 | | 传给 `command` 的参数（仅 `stdio`）。 |
| `url` | string | `remote` 必填 | HTTP 或 HTTPS 的 MCP 端点。 |
| `headers` | object | | HTTP 请求头名称与字符串值（仅 `remote`）。值在连接时会从 OCR 的环境中展开 `$VAR` 或 `${VAR}`；若某个值展开为空，会**导致连接失败**，而不是发送空值或丢弃该请求头。匿名访问时省略。 |
| `tools` | string 数组 | | 要注册的工具名白名单。为空 = 注册该 server 提供的全部工具。 |
| `setup` | string | | server 启动前运行一次的 shell 命令（仅 `stdio`，如安装依赖）。在仓库根目录运行，超时 5 分钟。 |
| `env` | string 数组 | | 额外的子进程环境变量，`KEY=VALUE` 形式（仅 `stdio`）。 |

对于需要认证的远程 server，请按该 server 的说明配置 `headers`。当传给
`ocr config set` 的 JSON 中含有环境变量引用时，请用单引号包裹，以免 shell 在 OCR
保存配置之前就把它们展开。允许匿名访问的 server 完全不需要 `headers`。

## 过滤工具

默认注册 server 声明的每个工具。当 server 暴露的工具超出审查器所需时，用 `tools`
设一个白名单 —— 更少、更精准的工具能让 agent 更专注，也降低 token 成本。白名单里
server 实际没有提供的名字会被跳过并给出警告，因此拼写错误会显示在 stderr 上，而不是
悄无声息地什么都不做。

## 名称冲突

MCP 工具名与内置工具共享同一个命名空间。如果某个 server 声明的工具名与**内置/保留**
工具（`file_read`、`code_search`、`task_done` 等）冲突，或与另一个 MCP server 已
注册的工具冲突，OCR 会**跳过**它并记录警告。先注册者胜出；为各 server 使用互不相同
的工具名，以免因此丢失工具。

## `setup` 命令

`setup` 在 server 子进程启动前、从仓库根目录运行一次。用它来按需安装或构建 server：

```json
"setup": "npm install -g @acme/docs-mcp-server"
```

它有 **5 分钟超时**。若非零退出，OCR 会记录命令、工作目录和输出，然后跳过该 server
并继续审查。

## 排错

所有 MCP 诊断信息都输出到 **stderr**，以 `[ocr]` 前缀标记，因此绝不会污染 stdout 上
的 `--format json` 输出：

- `Running setup for MCP server "x": …` —— 正在执行 setup 命令。
- `failed to start MCP server "x": …` —— 子进程未在 30 秒初始化超时内连接成功，或
  `command` 不在 `PATH` 中。
- `remote MCP server "x" has no URL configured, skipping` —— `type` 设为了 `remote`
  却没有设 `url`；这是「设了 `url` 却没设 `type`」的另一面。
- `failed to connect to remote MCP server "x": …` —— 端点未在 30 秒初始化超时内连接
  成功或列出工具。检查 URL、网络可达性，以及所需的请求头。
- `MCP server "x" header "h" expanded to empty value` —— `headers` 中的某个 `$VAR` 在
  OCR 的环境里没有设置。这会导致连接失败，而不会被当作「该请求头不存在」处理。
- `remote MCP server "x" returned HTTP 401 Unauthorized` —— 检查 token 或请求头配置。
- `remote MCP server "x" returned HTTP 403 Forbidden` —— 凭据已到达 server，但缺少所需
  权限。
- `tool "y" conflicts with built-in tool, skipping` —— 重命名该 server 的工具，或将其
  从 `tools` 中去掉。
- `allowed tool "y" not found in server's tool list` —— `tools` 中的名字与 server 提供
  的任何工具都不匹配；检查拼写。

启动或连接失败的 server 会被跳过；审查会在没有它的工具的情况下继续。

## 另见

- [工具](../tools/) —— MCP 工具与之并列的六个内置工具。
- [配置](../configuration/) —— 完整的配置文件与每个键。
- [CLI 参考](../cli-reference/) —— `ocr config` 与 review 参数。
