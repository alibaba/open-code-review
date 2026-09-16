---
title: ACP
sidebar:
  order: 6
---

在兼容 ACP 的客户端会话中运行 OCR 审查和扫描。`ocr-acp` 适配器通过
Agent Client Protocol（ACP）连接编辑器与本地 OCR CLI，在会话中展示进度、
审查结果和文件位置。它提供修复建议，不会修改你的文件。

ACP 是连接编辑器与 Agent 的开放协议。客户端将适配器作为独立本地进程启动，
通过 stdio 交换 JSON-RPC 消息。适配器校验请求后调用 `ocr review` 或
`ocr scan`，再将 OCR 的结构化输出转换为客户端更新。
OCR 的评审引擎和模型配置与适配器保持独立。

## 准备工作 {#prerequisites}

- 安装 [OCR CLI](../../installation/)，配置好[审查模型](../../configuration/)，
  并用 `ocr llm test` 检查连接。
- 准备包含 `acp/` 目录的源码、Git 和 Go 1.23+，用于构建适配器。
  如果还要从源码构建主项目的 OCR CLI，则需要 Go 1.25.5+。
- 使用支持 ACP v1 stdio 和自定义 Agent 启动命令的客户端。macOS arm64 已有本地自动化测试；
  Linux 上的 `aa7d4e7` 已通过 [ACP CI](https://github.com/alibaba/open-code-review/actions/runs/34986645165)，
  包括进程清理、race 检测和官方 Python ACP SDK smoke 测试。当前不支持 Windows。

适配器与 OCR CLI 分别构建。下面使用本地构建方式，不依赖 OCR npm 包提供
预装的 ACP 二进制。

## 构建并连接客户端 {#build-and-connect-your-client}

在仓库根目录运行：

```bash
make -C acp build
command -v ocr
```

构建产物是 `dist/ocr-acp`。把输出的 `ocr` 绝对路径填到下面的
`--ocr-binary` 参数中。

在客户端中注册自定义 ACP Agent，名称填 `OpenCodeReview`，传输方式选择
ACP v1 stdio，并配置以下启动命令：

```bash
/absolute/path/open-code-review/dist/ocr-acp --ocr-binary /absolute/path/ocr
```

请将两个可执行文件替换为绝对路径，并选择项目的工作目录。具体字段名称因客户端
而异，请参考其 ACP 配置说明。新建 OpenCodeReview 会话后即可使用 slash 命令，
无需单独的解析模型；OCR 本身仍需要审查模型。

## 运行审查或扫描 {#run-a-review-or-scan}

在 OpenCodeReview 会话中发送：

| 需求 | 命令 |
| --- | --- |
| 审查暂存、未暂存及未跟踪的变更 | `/review` |
| 最多进行一轮审查 | `/review --effort low` |
| 审查当前检出的最新提交 | `/review --commit HEAD` |
| 比较两个已有的引用 | `/review --from main --to HEAD` |
| 扫描目录 | `/scan --path internal/agent` |
| 扫描多个路径 | `/scan --path internal/agent,internal/config` |

请将示例中的引用和路径替换为项目实际存在的值。review 要求工作目录位于 Git
仓库中，结果路径以仓库根目录为基准；scan 以会话工作目录为基准。
`--effort low` 最多执行一轮审查，`medium` 最多两轮，`high` 最多三轮。
没有新增问题时，审查可能提前停止。
省略该参数时沿用 OCR 的配置。减少轮数可能漏掉问题，请按需权衡速度与质量。

在支持 ACP 命令提示的客户端中输入 `/`，可以查看可用命令和参数提示，
但不提供动态分支或路径补全。若客户端附加了本地文件链接，请连同 `/scan`
一起发送，并省略 `--path`。文件链接不能与 `/review` 或显式扫描路径混用；
不支持或有歧义的链接会被拒绝，不会自动扩大扫描范围。

适配器仅接受约定的 CLI 参数，不支持任意透传：

- review：`--commit`、`--from`、`--to`、`--effort`、`--no-filter`、
  `--background`、`--background-file`。
- scan：`--path`、`--batch`、`--no-plan`、`--no-dedup`、`--no-summary`、
  `--background`。

审查整个变更集请用 `/review`，不支持 `--staged`。会话命令也不接受
`--format`、`--audience` 等输出参数，以及 `--repo`、`--model`、
`--provider` 等覆盖参数。审查模型请通过 OCR 本身配置。

## 启用自然语言请求 {#enable-natural-language-requests}

解析模型将自然语言请求转换为 review/scan 命令，或在信息不足时发起澄清。
它与 OCR 的审查模型分别配置；客户端自身的模型选择不会配置这两个模型。

将以下环境设置加入 Agent 的 `env` 对象，把模型和密钥占位值替换为提供方的实际值：

```json
{
  "OCR_ACP_PARSER_PROVIDER": "openai",
  "OCR_ACP_PARSER_MODEL": "YOUR_TOOL_CALLING_MODEL",
  "OCR_ACP_PARSER_API_KEY": "YOUR_PARSER_API_KEY"
}
```

此示例使用兼容 OpenAI 的 Chat Completions 协议。使用 Anthropic Messages
协议时，将提供方设为 `anthropic`。使用兼容网关时，额外设置
`OCR_ACP_PARSER_BASE_URL` 为其 API 基础地址。密钥放在本地设置中，或通过
Agent 进程的环境变量传入，不要提交到共享的项目配置。
图形应用不一定继承你在终端中导出的环境变量。

基础地址的路径规则因协议而异：

| 协议 | `OCR_ACP_PARSER_BASE_URL` 示例 | 实际请求地址 |
| --- | --- | --- |
| `openai` | `https://gateway.example/v1` | `https://gateway.example/v1/chat/completions` |
| `anthropic` | `https://gateway.example` | `https://gateway.example/v1/messages` |

也可以填写以 `/chat/completions` 或 `/v1/messages` 结尾的对应完整地址，适配器不会再次追加该后缀。
Anthropic 的基础地址若以 `/v1` 结尾，会拼出 `/v1/v1/messages`；请按网关的实际接口路径填写。

| 环境变量 | 启动参数 | 用途 |
| --- | --- | --- |
| `OCR_ACP_PARSER_PROVIDER` | `--parser-provider` | `openai` 或 `anthropic` |
| `OCR_ACP_PARSER_MODEL` | `--parser-model` | 支持工具调用的模型 |
| `OCR_ACP_PARSER_BASE_URL` | `--parser-base-url` | 可选的 API 基础地址覆盖 |
| `OCR_ACP_PARSER_API_KEY` | 无 | 解析模型密钥，仅通过环境变量传入 |

启动参数优先于对应环境变量。解析器不读取 OCR 配置或 `OCR_LLM_*` 变量。
当前支持的解析协议为 `openai` 和 `anthropic`，不支持
`openai-responses` 与 `anthropic-bedrock`。

Anthropic 解析请求会显式关闭 Thinking，以支持必需的 `submit_intent`
工具调用。这不会改变 OCR 审查模型的 Thinking 设置。

配置后新建会话，可以尝试：

```text
审查我当前工作区的变更。
审查当前检出的最新提交。
扫描 internal/agent，找出潜在问题。
```

请求有歧义时，先回答澄清问题再执行。模型会被要求使用你的语言提问和说明；
固定校验消息和报告标签仍为英文。解析模型不可用时，slash 命令仍然可用。
如果解析配置不完整，适配器会在启动时报出明确错误。

“当前检出的最新提交”对应 `/review --commit HEAD`。会话只保留一份待澄清请求；
连续澄清可保留多个兼容的已知字段，明确的新值覆盖旧值，不会混入其他审查类型的字段。
请求完成或取消后清空该状态，不支持跨会话恢复。

## 查看结果与取消任务 {#read-results-and-cancel-work}

**Command:** 展示正在执行的命令。**OCR progress** 保存工作目录和有长度上限的
纯文本日志尾部，展开即可查看。日志更新保留在该条目中，不会反复铺满会话正文。
条目初始展开还是折叠由客户端决定。

问题反馈只在最终消息正文中展示一次，包含严重级别、分类、问题说明和建议代码，
不再额外生成 finding 卡片或导航按钮。文件存在且位置有效时，可以通过正文链接
打开对应位置；文件不存在、行号越界或路径超出工作根目录时，会保留问题文本，
不提供跳转。具体渲染和点击行为取决于客户端版本。

最终结果展示可用的摘要、部分失败信息、总 token 和 OCR 报告的耗时。
token 总数累计了多次模型请求，不是新生成回答的 token 数。

使用客户端的取消控件停止当前请求，适配器会取消任务并等待受管进程清理。
若要限制包含解析在内的整轮耗时，可在 Agent 的 `args` 数组中加入
`"--turn-timeout", "10m"`。默认值为 `0`，不限制整轮耗时；
解析请求有独立的默认 15 秒限制。超时后适配器会提示重试。

## 排错 {#troubleshooting}

| 现象 | 检查方法 |
| --- | --- |
| Agent 无法启动 | 检查两个可执行文件的绝对路径和执行权限。用 `ocr llm test` 检查 OCR 配置；删除不完整的解析配置或补齐必填值。 |
| slash 命令可用，自然语言不可用 | 单独配置 `OCR_ACP_PARSER_*` 环境变量，检查提供方、模型、密钥和网关地址。 |
| 自然语言解析超时 | 解析默认限时 15 秒，此时 OCR 尚未启动。根据诊断中的请求阶段、HTTP 状态及提供方响应定位原因；增大 `--turn-timeout` 不会延长解析器自身期限。可重试，或使用 `/review`、`/scan` 绕过解析模型。 |
| 命令或参数被拒绝 | 使用上文支持的参数。`/review --path` 应改为 `/scan --path`；提交或区间审查须使用已有的 Git 引用。 |
| 审查失败或只返回部分结果 | 展开 OCR progress 查看诊断，并阅读最终失败信息。检查 OCR 模型配置及提供方可用性。 |
| 结果中的位置无法点击 | 检查文件是否位于本次操作的根目录内，以及行号是否有效。退回纯文本展示是预期行为。 |
| 任务耗时太长 | 取消任务、查看进度详情，或设置整轮超时。review 可用 `--effort low` 减少审查轮数。 |
| 重新构建后仍是旧行为 | 检查配置中的二进制路径，并新建 Agent 会话；若仍复用旧进程，重启 Agent 或客户端。 |

连接异常时，查看客户端的 ACP 日志和适配器的 stderr 诊断信息。
分享日志前，请移除密钥和私有源码内容。

## 升级与支持范围 {#upgrade-and-support-boundaries}

更新源码后重新运行 `make -C acp build`，再新建 OpenCodeReview 会话。
正在运行的进程和旧消息不会自动加载更新。目前不支持会话恢复；新会话不保留之前
待回答的澄清状态。

适配器当前使用本地 stdio 通信，不支持 HTTP、多工作区根目录、自动修改文件以及
图片或音频请求。其他 ACP 客户端需要分别验证兼容性；协议测试通过不代表每个
客户端的界面行为都已验证。

## 另见 {#see-also}

- [配置](../../configuration/)——OCR 审查模型和提供方设置。
- [CLI 参考](../../cli-reference/)——review 和 scan 参数详情。
- [ACP 简介](https://agentclientprotocol.com/get-started/introduction)——Agent Client Protocol 官方介绍。
