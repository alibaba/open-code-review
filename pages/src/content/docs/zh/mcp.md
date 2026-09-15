---
title: MCP 服务器
sidebar:
  order: 10
---

## 以服务器为中心的管理与导入

管理器使用一个全屏终端会话。**CONFIGURED SERVERS（已配置服务器）**与 **MANAGEMENT ACTIONS（管理操作）**分区显示，按 **Tab** 切换区域。添加、导入、编辑和权限页面会替换当前页面，不会堆叠到下方。取消后回到原选中项且不保存。长预览用 **PgUp/PgDn** 滚动；任何页面都可按 **Ctrl-C** 退出并恢复原终端画面。**Q** 仅在导航页面退出，在输入框中仍可正常输入。

`ocr mcp` 直接打开服务器列表。先选择服务器，再管理工具、连接和权限；**Esc** 返回上一级，**Q** 退出，操作结束后回到原服务器。浏览列表不会连接。配置状态不是在线状态：发现结果标注为“本次管理会话的最近检查”，不会冒充持续在线。

进入 **Tools and individual permissions** 可以离线查看已配置工具。**Test connection / refresh tools** 在启动或连接前确认，只读取工具目录，新发现的工具默认未选择。每个工具显示实际生效权限，包含上级 deny、禁用状态和本次发现的定义变化。停用工具可离线完成；接受新定义需要重新连接，并将该工具重置为 `ask`。review 启动时仍会重新核验指纹。

选择 **Import connection (JSON / TOML)**，从文件导入或私密粘贴配置。粘贴内容不回显，**Ctrl-S** 进入预览，**Esc** 取消。支持 Cursor JSON 的 `mcpServers` 和 Codex TOML 的 `mcp_servers`。

```bash
ocr mcp import [file] [--yes]
ocr mcp import ./mcp.json
ocr mcp import ./one-server.toml --yes
```

每次只导入一个连接，保持禁用、零工具，不复制授权、不执行 setup、不联网。同名连接不覆盖，交互导入可改名。输入上限 1 MiB、64 个服务器；非交互导入只接受单个服务器并要求 `--yes`，`-` 表示从标准输入读取。

明文 env/header 值不会复制：环境变量值变成 `${VARIABLE_NAME}`，明文 header 变成 `${OCR_MCP_HEADER_NAME}`。预览会列出连接前需要设置的引用。header 环境变量应包含完整值，必要时包括 Bearer 前缀；Codex 的 `bearer_token_env_var` 会自动转换成 Bearer 引用。命令参数和 URL 中的连接值会保留，敏感值在预览中隐藏，请勿在其中嵌入凭据。

暂不支持 OAuth、自定义工作目录（`cwd`）、辅助命令和其他未支持字段，遇到这些字段会拒绝导入而不是默默忽略。导入后选择 **Edit connection**，确认发现、勾选工具并保存，才完成启用；执行前的独立授权机制不变。

OpenCodeReview 可以在 `ocr review` 中使用 **Model Context Protocol（模型上下文协议，MCP）**
服务器提供的工具。`ocr scan` 不会加载 MCP。

MCP server 是外部代码，它提供的工具元数据也不可信。因此，添加连接不会自动向模型
暴露全部工具。OCR 会先发现工具目录，再由你明确选择工具白名单和执行权限。

## 两道安全关卡

每次 MCP 调用都必须独立通过两道关卡：

1. **能力可见性。** 全局和 server 必须启用；工具必须在该 server 的 `tools` 白名单中；
   保存的定义指纹还必须与本次发现一致。满足这些条件后，模型才能看到它。
2. **执行授权。** 在真正发送 `tools/call` 前，OCR 会再次检查同一个 server、工具、指纹、
   context 和最终权限。`ask` 打开终端审批；`allow` 只跳过提示。

`tools` 缺失或为空都表示**零工具**，绝不表示全部工具。`allow` 权限和 server 自己声明的
annotation 都不能扩大白名单。拒绝、取消、超时、输入流关闭或终端不可交互时，不会发送
`tools/call`。

## 交互向导

在终端中打开管理器：

```bash
ocr mcp
```

也可以直接进入添加向导：

```bash
ocr mcp add docs
```

向导依次完成：

1. 选择 `stdio` 或 `remote` 连接；
2. 收集连接信息，且不回显已保存的 secret；
3. 展示本地 executable 和参数结构（疑似凭据的参数值会脱敏），或脱敏后的远程 endpoint 与 header 名；
4. 在启动或连接 server 前再次确认；
5. 只初始化 MCP 并读取完整分页 `tools/list`，不调用工具；
6. 默认不选择任何工具，由你勾选要启用的工具；
7. 展示脱敏汇总，最终确认后才原子保存一次。

方向键选择，**Space** 勾选工具，**Enter** 继续，**Ctrl-B** 返回修改，**Esc / Ctrl-C**
取消且不保存。不需要输入 JSON：参数逐项添加，凭据只填写环境变量名称，不粘贴 token。
连接预览页也提供 **Save disabled without connecting**（不连接，保存为禁用）。
连接失败会留在向导内，可以返回修改或重试。勾选工具后默认 `ask`；保存后通过
`ocr mcp permissions docs` 用选择菜单设置自动运行 `allow`，上级 `deny` 仍不能被覆盖。

拒绝最终确认或取消会退出且不保存。不可信本地进程的启动
或对远程 server 的连接本身仍可能在 server 侧产生副作用，即使发现阶段不调用业务工具；
因此务必先检查连接预览。

### 本地 stdio server

```bash
ocr mcp add docs
# 选择：stdio
# Executable：npx
# 参数：先输入 -y，回车，再输入实际的 MCP 包名，最后空行继续
# 环境变量名称：DOCS_TOKEN；来源环境变量名称：DOCS_TOKEN
```

子进程只收到精简的平台环境（`PATH`、home/temp、locale 等）以及 `env` 中明确列出的变量，
不会继承 OCR 进程的完整环境。向导中的新值必须使用 `${ENV_NAME}` 引用。历史明文值可以
迁移读取，但已废弃，并会在所有输出中被掩码。

OCR 会在启动前显示 executable 和参数结构，同时掩码疑似凭据的 flag 值及 URL query 值。
管理命令只有在确认后才启动 server；review
期间若没有任何可能可见的工具，也不会启动 server。

### 远程 server

```bash
ocr mcp add knowledge
# 选择：remote
# URL：https://mcp.example.com/v1
# Header 名称：Authorization；来源环境变量名称：MCP_TOKEN
# 可选前缀：Bearer，后面保留一个空格
```

远程 URL 默认必须是 HTTPS，localhost/loopback 例外。连接其他主机的明文 HTTP 必须显式
设置 `allow_insecure_http`，并再次确认风险。带 userinfo 或 fragment 的 URL 会被拒绝；
transport 控制或 MCP 协议保留的 header（例如 `Host`、`Content-Length`、MCP
session/protocol header、`Accept`、`Content-Type`）不能被覆盖。认证 header（例如
`Authorization`）可以配置，但输出中始终脱敏。
通过管理器新输入的 header 值必须使用 `${ENV_NAME}` 引用（例如 `Bearer ${MCP_TOKEN}`）；
明文凭据会被拒绝。

配置的 header 只会发送给原始同源地址。OCR 拒绝跨源重定向和 HTTPS 降级到 HTTP 的
重定向。CLI 与 JSON 状态只显示脱敏 endpoint 和 header 名，不显示 header 值或 URL
query 的值。

## 发现与工具选择

只查看目录而不修改配置：

```bash
ocr mcp discover docs
ocr mcp discover docs --json
```

发现阶段只执行 MCP 初始化和分页 `tools/list`，不会调用 `tools/call`、不会把工具交给模型，
也不会保存目录。TTY 会再次确认；非交互进程必须显式传入 `--yes`。

通过交互编辑器或精确名称选择工具：

```bash
ocr mcp tools docs
ocr mcp tools docs --enable search_docs --enable get_page --yes
ocr mcp tools docs --disable get_page --yes
```

新启用的工具默认是 `ask`。禁用工具会同时删除其权限与指纹。发现最多读取 64 页、512 个
工具、4 MiB 完整目录；单个描述最多 8 KiB，input schema 最多 256 KiB。重复、非法或
超限目录全部 fail closed（失败时不授予权限）。

工具描述、schema、title、icon 和 annotation 都是不可信 server 元数据。OCR 会清理并
限制它们。`readOnlyHint` 等 annotation 最多作为“server 自报、未验证”的提示展示，
绝不会自动授权。

## 权限

| 值 | 含义 |
|---|---|
| `deny` | 硬拒绝。全局或 server 的 deny 不能被下级覆盖。 |
| `ask` | 仅在交互 review 中暴露符合条件的工具，并在每次未缓存调用前询问。 |
| `allow` | 对符合条件的工具跳过提示并授权；它本身不能启用工具。 |
| `inherit` | 仅 server/tool 可用：继承更上一级最终值。 |

如果上级没有 `deny`，采用最具体的非 `inherit` 值：

```text
tool > server > global（默认 ask）
```

持久 `allow` 只能通过 permissions 命令写入，而且工具必须已启用并具有当前有效指纹：

```bash
# 全局策略和审批超时
ocr mcp permissions
ocr mcp permissions --default ask --timeout 60 --yes

# server 与精确工具覆盖
ocr mcp permissions docs
ocr mcp permissions docs --default inherit \
  --tool search_docs=ask --tool get_page=allow --yes
```

`approval_timeout_seconds` 支持全局修改，默认 60 秒，合法范围 1–600 秒；也可直接设置：

```bash
ocr config set mcp.approval_timeout_seconds 120
```

修改在下一次 `ocr review` 生效。

### 运行时审批

`ask` 提示会显示 server、原始工具名、模型别名、不可信描述、指纹前缀，以及有界且脱敏的
参数预览。四个选项是：

- **Allow once（仅允许一次）**
- **Allow this review（本次 review 允许）**
- **Deny once（仅拒绝一次，默认）**
- **Deny this review（本次 review 拒绝）**

review 级决定以精确 server/tool 身份为 key，在并发 review group 间共享，进程退出即消失。
运行时审批永远不会修改持久配置。

## 工具身份与定义变化

模型看到的是限定别名，不是 server 的原始工具名：

```text
mcp__<server-slug>__<tool-slug>__<16-hex>
```

hash 绑定区分大小写的 server 配置 key 与远程工具名。OCR 持有不可变的 alias-to-tool
映射，不会反向解析别名。若与内置工具、另一个 MCP alias 或已有 registry 项发生任何
冲突，本次 review 的全部 MCP 注册都会中止，并保持零部分暴露；内置工具仍可使用。

保存配置时会检查并发修改。如果表单打开期间另一个窗口保存了配置，OCR 会拒绝旧草稿，
请重新打开设置后再试。私有的 `config.json.lock` 文件用于协调 OCR 写入，不含凭据；
OCR 运行期间不要删除它。旧版本客户端和手工编辑器不参与此锁，请避免与它们同时修改。

每个选中工具还保存一个 SHA-256 指纹，其输入包括连接身份、工具名、净化后的描述和
canonical input schema。指纹缺失或变化会把工具标记为 `needs-review` 并隐藏，直到通过
`ocr mcp tools` 或 `ocr mcp edit` 接受新定义。annotation 和解析后的 secret 不参与指纹。

## CI 与非交互环境

只有 stdin 与 stderr 都是终端、`TERM` 不是 `dumb`，且常见 CI 环境变量未取真值时，
OCR 才认为环境可交互。`0`、`false`、`no`、`off` 视为假；GitHub Actions、GitLab CI、
Azure Pipelines、Buildkite、Jenkins 和通用 `CI` 环境默认 fail closed。

- `ask` 工具会被隐藏，不能执行。
- 指纹匹配且明确启用的 `allow` 工具可以执行，但实际 MCP 调用前仍必须经过独立 authorizer。
- 非交互运行裸 `ocr mcp` 只输出脱敏状态和帮助，不连接、不写配置。
- 可能连接或修改状态的命令必须给齐参数并显式使用 `--yes`。

CI 中应谨慎使用持久 `allow`，让凭据只覆盖最小 server/tool 范围，并固定或以其他方式信任
将要启动的 server 包版本。

## 管理命令参考

```text
ocr mcp
ocr mcp add [name]
ocr mcp list [--json]
ocr mcp show <name> [--json]
ocr mcp edit <name>
ocr mcp discover <name> [--json] [--yes]
ocr mcp tools <name> [--enable TOOL ...] [--disable TOOL ...] [--yes]
ocr mcp permissions [name]
ocr mcp enable <name> [--yes]
ocr mcp disable <name> [--yes]
ocr mcp remove <name> [--yes]
```

`list` 与 `show` 从不连接 server。终端中的 `remove` 默认选择 No，非交互时必须传 `--yes`。
`enable` 只改变 server 开关，不会把任何工具加入白名单。

## 配置与迁移

管理器向 `~/.opencodereview/config.json` 写入版本化策略：

全局配置键是 `mcp.version`、`mcp.enabled`、`mcp.default_permission` 和
`mcp.approval_timeout_seconds`；每个具名连接位于 `mcp_servers` 下。

```json
{
  "mcp": {
    "version": 1,
    "enabled": true,
    "default_permission": "ask",
    "approval_timeout_seconds": 60
  },
  "mcp_servers": {
    "docs": {
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "@acme/docs-mcp-server"],
      "env": ["DOCS_TOKEN=${DOCS_TOKEN}"],
      "enabled": true,
      "default_permission": "inherit",
      "tools": ["search_docs"],
      "tool_permissions": {"search_docs": "ask"},
      "tool_definition_sha256": {"search_docs": "<64-lowercase-hex>"}
    }
  }
}
```

普通 review 只读取旧配置，不会自动改写。旧配置中非空 `tools` 若没有指纹，最多只能在
交互 review 中按 `ask` 使用，不能在 CI 中自动执行；旧配置中缺失或为空的 `tools` 暴露
零工具。第一次通过管理器成功保存时才写入 version 1 和接受的指纹。

旧 `setup` 字段不会再被 `ocr review` 执行。当管理器保存对应 server 时会删除这个旧字段，
并提示你手动安装或构建 server。旧版 OCR 不应继续编辑迁移后的配置，因为它不认识这些
安全字段，可能在重写配置时将其丢弃。

配置保存会在同目录创建权限 `0600` 的临时文件，flush 后原子替换旧文件。向导取消或发现
失败不会改变原配置。

## 故障排查

- **`needs-review`**：运行 `ocr mcp tools <name>`，接受当前发现的定义，再选择权限。
- **CI 中没有 MCP 工具**：`ask` 按设计会隐藏。无人值守确有需要时，使用当前指纹和最小范围
  的持久 `allow`。
- **发现失败**：检查展示的 executable/参数或脱敏 origin、必要环境变量、TLS 和 MCP 兼容性。
- **工具结果过大**：聚合结果超过 1 MiB 会被拒绝；缩小查询或让 server 返回有界结果。
- **server 返回错误**：MCP `isError` 会记为真正的执行失败，而非成功遥测。
- **名称冲突**：修改 server 配置 key 或远程工具名；OCR 不再采用 first-registration-wins。

## 另请参阅

- [配置](../configuration/)——所有配置键。
- [CLI 参考](../cli-reference/)——命令与 flag 说明。
- [CI 集成](../integrations/ci/)——无人值守 review 配置。
- [工具](../tools/)——内置 review 工具。

## 终端接入体验

`ocr mcp add` 提供逐项输入和 `Space` 工具勾选。`Enter` 继续，`Ctrl-B` 返回，`Esc` 取消。`ocr mcp permissions` 设置权限与超时（默认 60 秒，1–600 秒）。`OCR_CONFIG_PATH` 为配置读写和 review 选择同一份文件，可用于隔离体验。纯 `ocr mcp tools docs --disable write` 撤权不连接服务器，不需要 `--yes`，离线也能完成；启用工具仍需发现并明确确认连接。
