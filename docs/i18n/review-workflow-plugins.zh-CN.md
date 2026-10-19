# 审查工作流插件（Review Workflow Plugins）

[English](../review-workflow-plugins.md) | [简体中文](review-workflow-plugins.zh-CN.md) | [日本語](review-workflow-plugins.ja-JP.md) | [한국어](review-workflow-plugins.ko-KR.md) | [Русский](review-workflow-plugins.ru-RU.md)

关于包安装、执行授权以及运行时适配器之间的关系，请参阅[原生插件架构设计](../plugin-architecture.md)。

当代码审查需要遵循特定的执行阶段顺序和完成标准时（例如：先检查改动，再在后续阶段对候选问题进行二次验证确认，最后生成结构化结论报告），可以使用**审查工作流插件**。插件包声明各个阶段并负责校验模型的提交结果；OCR 宿主则提供 Diff 内容、审查规则、上下文工具以及 Token 预算控制。

## 运行 single-pass 示例

在此代码检出中，确保系统可用 `python`（Python 3），安装示例插件并对工作区未提交的修改执行预览：

```sh
ocr plugin install ./examples/ocr-plugins/single-pass
ocr review --review-plugin single-pass --preview
```

在已配置好模型 Provider 并有代码修改的情况下，运行：

```sh
ocr review --review-plugin single-pass --format json --output review.json
```

该示例包含一个阶段。它要求模型提交发现的问题，并在 OCR 发布评论前由插件验证提交内容。可在 `review.json` 中的 `review_plugin` 字段查看该阶段的完成状态与报告。仅安装插件不会运行包代码；预览（`--preview`）亦不会启动 Python 进程或调用大模型。

上述命令使用的是用户全局安装的插件。如果被审查的仓库声明了项目级插件，请将示例添加到项目声明中，并按下方项目工作流进行授权。显式指定全局插件不会覆盖项目锁定的插件集。

## 项目选择与执行授权

对于受仓库版本控制的工作流插件，使用 `ocr plugin add <source>` 声明依赖，提交 `.opencodereview/plugins.json` 与 `plugins.lock.json`，然后使用 `ocr plugin sync --frozen` 完成同步安装。在通过 `--review-plugin <id>` 调用前，请检查锁定的包并运行 `ocr plugin trust` 授予本地执行信任。运行 `ocr plugin preflight --json` 可在不启动工作流或请求模型的情况下，预检安装内容、版本兼容性以及本地授权状态。如果项目存在插件声明，工作流仅能从锁定的插件中选取；全局安装的工作流无法绕过项目锁定。详见[原生 OCR 插件指南](native-plugins.zh-CN.md#project-plugins)。

审查工作流插件拥有其阶段计划、提示词指令、完成 Schema、结果校验逻辑以及跨阶段状态。OCR 提供所选文件的 Diff、规则注入、模型与工具执行、会话持久化、Token 计量、评论处理和结果输出。OCR 宿主不包含任何特定插件的硬编码阶段名称或报告字段。

单次审查调用工作流：

```sh
ocr review --review-plugin PACKAGE_ID --preview
ocr review --review-plugin PACKAGE_ID --format json --output review.json
```

仅安装或启用插件**不会**自动调用其审查工作流。`--preview` 仅加载清单，不启动可执行程序或请求模型。正式运行工作流时，插件可执行程序以当前用户的本地权限运行（类似于本地 MCP Server）。显式通过 `--review-plugin` 选取时，即使该插件在全局处于禁用状态，也会在本次调用中加载其规则与 MCP Server；原有的持久化启用状态保持不变。

## 清单配置（Manifest）

```json
{
  "schema_version": 1,
  "id": "example-review",
  "name": "Example Review",
  "version": "1.0.0",
  "review": {
    "protocol": "stdio-v1",
    "command": "${plugin_dir}/bin/${os}-${arch}/reviewer${exe}",
    "args": ["--policy", "${plugin_dir}/policy.md"],
    "stages": ["inspect", "verify"]
  }
}
```

`command` 为可执行文件路径，不经过 Shell 解析。命令及参数支持 `${plugin_dir}`、`${os}`、`${arch}` 以及 `${exe}`（Windows 下为 `.exe`，其余平台为空）。直接使用 PATH 上的运行时（如 `python`）同样支持。自带的 Python 示例默认使用 `python` 命令；如果系统仅有 `python3`，请在打包或安装前将清单命令调整为 `python3`。OCR 不会自动推导解释器或静默安装运行环境。

插件包可自由组合审查工作流、规则包和 MCP 工具。阶段 ID 为唯一的标识符；清单可声明 1 到 64 个有序执行阶段。不同插件可使用不同的阶段数量、名称、Schema 和判定逻辑。

已安装的快照同时为本次运行提供规则和工作流；更新包不会改变正在运行的快照。只支持早期 `engine`/`prompt` 实验格式的宿主需要升级。宿主仍会读取旧格式的安装元数据，以便列出、删除或更新这些包；管理输出将其标记为需要更新（JSON 中的 `review.update_required`），不显示运行提示。尝试运行旧格式包会要求先更新。

## 兼容性说明

工作流插件可声明可选的 `requires.ocr_min_version` 和 `requires.features`，例如 `["rules", "mcp", "stdio-v1"]；规则详见[宿主要求](native-plugins.zh-CN.md#compatibility-and-host-requirements)。不满足的要求在进程启动前被拒绝。工作流 API 仍处于实验阶段。现有 `stdio-v1` 插件继续使用 `begin`/`submit` 消息，无强制握手或 JSON-RPC 迁移。未知的响应信封字段会报错；`report` 和完成参数保留插件定义的结构。不兼容的信封变更必须使用新的协议或消息版本，不能改变 v1 字段的含义。

## stdio-v1 协议规范

OCR 为每个文件组启动一个进程，以被审查的仓库为工作目录。进程在该组各阶段之间保留状态。请求与响应为单行 UTF-8 JSON 对象，每行最大 4 MiB。每个请求必须恰好对应一个响应；诊断信息输出到 stderr。响应必须包含 `"version": 1`。非空 `error` 字符串表示无法处理请求，将终止该文件组。

### 阶段开始（`begin`）

在每个阶段开始前，OCR 发送：

```json
{"version":1,"method":"begin","stage":"inspect","paths":["main.go"],"severities":["critical","high","medium","low"],"categories":["bug","security","performance","maintainability","test","style","documentation","other"]}
```

插件返回任务定义以及可选的初始报告：

```json
{"version":1,"task":{"instructions":"检查选定的代码。","input":"检查公共 API 的返回值约定。","completion":{"description":"提交检查结果。","parameters":{"type":"object","properties":{"checked":{"type":"boolean"}},"required":["checked"]}}},"report":{"pending":["inspection"]}}
```

OCR 将 `instructions` 与 `input` 添加到正常的 Diff/规则上下文中，并在上下文压缩时保留。两者都必须包含非空白文本，因为它们会成为独立的模型消息。模型仍可使用 OCR 的上下文工具。`task_done` 的定义来自 `completion`；直接的 `code_comment` 被隐藏，使评论必须经过插件验证器。

协议请求不传递模型凭证，但可执行程序会继承正常的进程环境，其中可能包含模型令牌和代理设置。它也能以当前用户权限读取配置文件。此版本没有运行时沙箱，请仅安装和选择可信作者的工作流。

### 模型提交（`submit`）

当模型调用 `task_done` 时，OCR 原样转发参数：

```json
{"version":1,"method":"submit","stage":"inspect","arguments":{"checked":true}}
```

接受提交的响应要求显式指定 `accepted` 和 `complete` 布尔值：

```json
{"version":1,"accepted":true,"complete":true,"report":{"checked":true},"comments":[]}
```

插件定义报告结构和完成条件。`accepted:true` 且 `complete:false` 会停止当前文件组并将其标记为未完成；`feedback` 作为未完成原因保存，不会触发模型继续修正。已接受的评论仍进入 OCR 正常的发布与过滤流程，包括未完成工作中的评论。评论必须引用本组选中的路径、包含内容，并使用 `begin` 提供的 severity/category 值。其他评论字段为 `existing_code`、`suggestion_code`、`start_line`、`end_line` 和 `thinking`。

提交有误但可由模型修正时，返回 `accepted:false`、非空 `feedback`、最新的 `report`，且不得携带评论。OCR 会在既有请求额度内将反馈交回模型。插件必须自行保留有效的部分条目。进程崩溃、畸形协议响应或缺少必需布尔字段会停止执行，不会作为模型错误重试。

## 完成状态与输出

声明的每个阶段都必须完成，即使前序阶段没有发现问题。`--effort` 不会缩短阶段计划。现有上下文、工具轮次和超时设置仍然生效，组超时覆盖声明的阶段数量。用户指定 Token 预算时，剩余阶段共享未使用额度，不额外施加总 Token 上限。

JSON 输出的 `review_plugin` 包含 `plugin`、`group`、`stage`、`complete`、可选的 `token_limit`、插件自定义的 `report` 以及执行 `error`。后续模型或进程失败时保留最近一次返回的报告。每个完成或中断的阶段都会将 `plugin_review` 记录写入会话，包括文本输出模式。通过 `ocr session show SESSION_ID --json` 的 `review_plugin` 数组可检查报告。响应省略 `report` 时保留之前的报告。未完成的必需工作进入覆盖率记录并导致非零退出；普通审查的退出行为不变。

工作流审查需要全新调用；`--resume` 尚不支持恢复任意进程内部状态。JSON 与会话记录仍可用于检查。

实现示例见 [single-pass](../../examples/ocr-plugins/single-pass)。[Behavior Review](https://github.com/Qiyuanqiii/ocr-behavior-review) 是使用三个阶段并保留证据的独立可执行插件。
