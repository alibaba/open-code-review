# 计划：为 OCR 增加 OpenAI Responses API 支持

> 状态：**已确认（FINAL）** — 所有架构决策已拍板，单 PR 实施。
> 目标：在现有 LLM Provider 系统中新增 `openai-responses` 协议，使 OCR 可经 OpenAI Responses API（`/v1/responses`）进行代码评审，支持 GPT-5.x / o-系列等模型。**同时**引入 protocol 常量规范化（`openai` 保持不变以实现完全向前兼容），并为未来 `anthropic-vertex` 等 provider 预留命名空间。

---

## 0. 决策记录（已拍板）

| # | 决策 | 结论 |
|---|---|---|
| 1 | 新增内置 responses provider | **否**。本期只保证「新增使用 responses 格式的 provider 的能力」（扩展性），不新增内置项。 |
| 2 | 多轮状态管理 | **无状态**。每轮发送完整 input，不使用 `previous_response_id`；不动 loop 状态机。 |
| 3 | reasoning items 回传 | **先不回传**。`reasoning_content` 仅用于显示；记为观测项。 |
| 4 | `store` 字段 | **强制 `store=false`**。`PromptCacheKey` 由调用方（loop / agent）在 session 开始时**计算一次** `sha256(instructions + firstUserMessage)[:32]`，经 `ChatRequest.CacheKey` 透传给 client；client 不重复计算。**风险与取舍见 §5**。 |
| 5+6 | 配置块 + 环境变量 | **方案 A**：统一引入 `protocol` 字段 + `OCR_LLM_PROTOCOL`；`use_anthropic` / `OCR_USE_ANTHROPIC` 降级为兼容回退。**protocol 命名重构见 §1**。 |
| 7 | URL 处理 | **resolver 不处理 `openai-responses` URL**（与 openai 一致），归一化全部放 `NewOpenAIResponsesClient`。详见 §3.5。 |
| 8 | `finish_reason` 映射 | **粗粒度**：`completed`→`stop`、`incomplete`→`length`、有工具调用→`tool_calls`、兜底 `stop`。 |
| 9 | tokenizer `encodingForModel` | **本次不改**。 |
| 10 | PR 拆分 | **单 PR** 完成全部变更（内部按提交序列组织，见 §7）。 |

---

## 1. Protocol 命名重构（核心改动）

### 1.1 规范 protocol 值

| 规范值 | 含义 | 分发 client |
|---|---|---|
| `anthropic` | Anthropic Messages API（直连） | `AnthropicClient` |
| `openai` | OpenAI Chat Completions | `OpenAIClient` |
| `openai-responses` | OpenAI Responses API（新增） | `OpenAIResponsesClient` |

### 1.2 别名（向后兼容）

- `openai-chat-completions` → 归一化为 `openai`（仅供本分支测试期产出的配置；存量配置均为 `openai`，无需转换）
- 存量配置 `"protocol": "openai"`、`use_anthropic=false`、`OCR_USE_ANTHROPIC=false` 行为不变。

### 1.3 预留命名空间（本期不实现）

- `anthropic-vertex`（Vertex AI 上的 Claude，认证/endpoint 不同）：**校验拒绝**并返回友好错误「暂未实现，后续支持」。
- 命名约定 `<vendor>-<flavor>`，文档说明，便于后续扩展。

### 1.4 归一化与校验（新增 helper）

**`internal/llm/protocol.go`（新文件）**

```go
const (
    ProtocolAnthropic            = "anthropic"
    ProtocolOpenAIChatCompletions = "openai"
    ProtocolOpenAIResponses      = "openai-responses"
)

// NormalizeProtocol 归一化别名（openai-chat-completions → openai）。
// 空串原样返回（由调用方决定默认）。未知名原样返回，交由 ValidateProtocol 报错。
func NormalizeProtocol(raw string) string { ... }

// ValidateProtocol 接受三个规范名；拒绝其它（含 anthropic-vertex，提示暂未实现）。
// 注意：本函数**不**接受别名 "openai-chat-completions"——调用方必须先经 NormalizeProtocol 归一化。
func ValidateProtocol(p string) error { ... }

// IsAnthropicProtocol 报告是否为 anthropic 协议（仅规范名）。
func IsAnthropicProtocol(p string) bool { return p == ProtocolAnthropic }
```

**调用时机**：resolver 各 strategy 在产出 `ResolvedEndpoint` 前、`tryProviderConfig`/`tryCustomProvider` 读取 `entry.Protocol` 后调用 `NormalizeProtocol`；校验统一走 `ValidateProtocol`。底层 switch 只面对规范名。

> **`openai-chat-completions` 别名在 `ValidateProtocol` 中的处理——方案比选：**
>
> | 方案 | 做法 | 优点 | 缺点 |
> |---|---|---|---|
> | **A（采用）：严格分离** | `ValidateProtocol` 只认规范名；调用方必须 `ValidateProtocol(NormalizeProtocol(raw))` | 职责单一；别名映射只在 `NormalizeProtocol` 一处；错误信息精确 | 调用方忘记先 Normalize 会导致 `"openai-chat-completions"` 被拒（但 resolver 所有路径已统一 Normalize，不会遗漏） |
> | B：内部宽容 | `ValidateProtocol` 内部先 `NormalizeProtocol(p)` 再 switch | 调用方不可能误用 | 隐式接受非规范输入；若调用方未同时 Normalize，下游 switch 可能拿到别名而非规范名 |
> | C：合并为 `ValidateAndNormalize(p) (string, error)` | 一次调用产出已校验的规范名 | 无歧义；原子操作 | 改变函数签名（`error` → `(string, error)`）；两个职责合并 |
>
> **选定方案 A**：与 §1.4 的职责划分一致（`NormalizeProtocol` 管别名、`ValidateProtocol` 管白名单），resolver 调用链已保证 `Normalize → Validate → 用规范名` 的顺序。

### 1.5 默认行为（未显式指定 protocol，保持向前兼容）

| 配置来源 | 默认规则 |
|---|---|
| legacy `llm` 块（无 `protocol` 字段） | `use_anthropic=true|nil` → `anthropic`；`false` → `openai` |
| 内置 provider（providers.go registry） | registry 内 `Protocol` 字段改为规范名（`openai` / `anthropic`） |
| custom provider | protocol 必填；接受规范名 + `openai-chat-completions` 别名 |
| 环境变量（无 `OCR_LLM_PROTOCOL`） | `OCR_USE_ANTHROPIC=true|未设` → `anthropic`；`false` → `openai` |
| `llm.protocol` / `OCR_LLM_PROTOCOL` 非空 | 优先级最高（归一化后采用） |

### 1.6 用户可见变化

- `ocr llm providers` 输出的 PROTOCOL 列：`openai`（值不变，仅从硬编码字符串改为常量引用）。
- `ocr config provider` TUI：protocol 选项展示规范名三项（见 §3.6）。

---

## 2. 现状回顾（改动基线）

- `LLMClient` 接口：`internal/llm/client.go:32`
- 工厂分发：`internal/llm/client.go:195` `NewLLMClient(ep)`（`ep.Protocol == "anthropic"` → Anthropic，否则 → OpenAI）
- 数据结构：`ChatRequest`(:315)、`Message`(:42)、`ChatResponse`(:139)、`ToolCall`(:118)
- Resolver：`internal/llm/resolver.go`；协议校验 `:290`；`"anthropic"` 分支 `:330`/`:358`；`ensureMessagesSuffix` `:633`
- 内置 provider：`internal/llm/providers.go`（12 处 `Protocol: "openai"`，1 处 `"anthropic"`）
- 配置：`cmd/opencodereview/config_cmd.go`（`ProviderEntry` :190、`Config` :211、`LlmConfig` :222、`applyProviderField` :382）
- TUI：`cmd/opencodereview/provider_tui.go:53` `cpProtocols`
- Agent 主循环：`internal/llmloop/loop.go`（`NewToolCallMessage`/`NewToolResultMessage`，依赖 `ToolCall.ID`↔`ToolCallID` 配对）
- SDK：`github.com/openai/openai-go/v3 v3.41.0`，`responses` 子包已提供 `client.Responses.New`，无需升级。

---

## 3. 详细改动清单

### 3.1 新增 `OpenAIResponsesClient`

**新文件 `internal/llm/responses_client.go`**

`NewOpenAIResponsesClient(cfg ClientConfig) *OpenAIResponsesClient`：
- 复用 `openai.NewClient`，URL 归一化见 §3.5。
- 设置 `WithAPIKey`、`WithMaxRetries(5)`、`WithHeader("User-Agent", ...)`、`WithRequestTimeout`、`ExtraHeaders`。

`CompletionsWithCtx(ctx, ChatRequest) (*ChatResponse, error)`：
1. `buildResponsesParams(model, req)` 转 `openai.ResponseNewParams`
2. 合并 `cfg.ExtraBody`（`WithJSONSet`）
3. `c.sdk.Responses.New(ctx, params, opts...)`
4. `mapResponsesResponse(sdkResp)` 转回 `*ChatResponse`

**`buildResponsesParams` 映射：**

| ChatRequest / Message | ResponseNewParams |
|---|---|
| 多个 `role=system` 文本 | `Instructions`（`\n\n` 拼接，`openai.String(...)`） |
| `role=user`/`assistant`（无 tool_calls） | `ResponseInputItemParamOfMessage(content, role)` |
| `role=assistant` + `ToolCalls` | 先一条 assistant message item（若有文本），再每个 `ToolCall` 一条 `ResponseInputItemParamOfFunctionCall(arguments, callID=tc.ID, name)` |
| `role=tool`（`ToolCallID`） | `ResponseInputItemParamOfFunctionCallOutput(callID=ToolCallID, output=result)` |
| `req.Tools` | `[]openai.ToolUnionParam`，`FunctionToolParam{Name, Description, Parameters, Strict:openai.Bool(false)}` 包为 `OfFunction` |
| `req.MaxTokens` (>0) | `MaxOutputTokens`（行为对齐 `OpenAIClient.buildOpenAIParams` `client.go:402-403`：仅 `>0` 时设，用 `openai.Int(int64(req.MaxTokens))`，无兜底默认值；区别于 Anthropic client 的 8192 兜底） |
| `req.Temperature` (非 nil) | `Temperature`（`openai.Float(*req.Temperature)`） |
| `req.Model` | `Model` |
| —— | `Store: openai.Bool(false)`（强制，见 §5） |
| `req.CacheKey`（非空时） | `PromptCacheKey: openai.String(req.CacheKey)`（调用方预计算一次，见 §5；为空时不设） |

复用 `msg.ExtractText()`（`client.go:85`）提取文本。

**`mapResponsesResponse` 映射：**

遍历 `sdkResp.Output`：
- 文本内容：**直接用 SDK 自带的 `sdkResp.OutputText()`**（`response.go:3754`），该方法遍历所有 output item 的 content，聚合 `type=="output_text"` 的 `Text`，返回拼接字符串 → `contentPtr`。无需手动遍历 `msg.Content` 逐元素调 `AsOutputText()`。
- `AsFunctionCall()`：`ToolCall{ID: CallID, Type:"function", Function{Name, Arguments}}`（**`ID = CallID`**，保证 loop 的 `NewToolResultMessage(call.ID, ...)` 配对）
- `AsReasoning()`（best-effort）：`ResponseReasoningItem.Summary` 是 `[]ResponseReasoningItemSummaryUnion`，需遍历每个 summary 元素，调 `.AsText().Text` 拼接为完整字符串 → `ReasoningContent`。注意不要只取 `summary[0]`。

单个 `Choice`：
- `Message{Role:"assistant", Content: *string, ReasoningContent, ToolCalls}`
- `FinishReason`（粗粒度，§决策8）：`completed`→`stop`；`incomplete`→`length`；有 function_call→`tool_calls`；兜底 `stop`
- content 复用 `stripThinkTags`（`client.go:764`）

`Usage`：`PromptTokens=InputTokens`、`CompletionTokens=OutputTokens`、`CacheReadTokens=InputTokensDetails.CachedTokens`、`TotalTokens=Usage.TotalTokens`（`CacheWriteTokens`=0）。

返回 `&ChatResponse{ID, Model: string(sdkResp.Model), Choices, Usage}`。

### 3.2 协议分发

**`internal/llm/client.go:195` `NewLLMClient`** 改为：

```go
switch ep.Protocol {
case ProtocolAnthropic:
    return NewAnthropicClient(cfg)
case ProtocolOpenAIResponses:
    return NewOpenAIResponsesClient(cfg)
default: // ProtocolOpenAIChatCompletions（防御兜底）
    return NewOpenAIClient(cfg)
}
```

### 3.3 Resolver / 校验改造

**`internal/llm/resolver.go`**

- 引入 `NormalizeProtocol` / `ValidateProtocol`（§1.4）。
- `tryProviderConfig`：
  - 内置 provider：`protocol = NormalizeProtocol(preset.Protocol)`（registry 已是规范名，幂等）。
  - 自定义 provider：读取 `entry.Protocol` → `NormalizeProtocol` → `ValidateProtocol`（错误信息列出三规范名 + 别名 `openai`）。
  - `protocol == ProtocolAnthropic` 分支（authHeader、`ensureMessagesSuffix`）保持条件语义不变（仅常量替换）；`openai` 与 `openai-responses` 均不做 resolver 级 URL 处理。
- `tryOCREnv`：
  - 新增读取 `OCR_LLM_PROTOCOL`（新常量 `envOCRLLMProtocol`）；非空 → `NormalizeProtocol` 后采用。
  - 否则按 `OCR_USE_ANTHROPIC`：`true|未设`→`anthropic`，`false`→`openai`（产出规范名，原逻辑产出 `"openai"` 的两处一并改）。
  - `OCR_LLM_PROTOCOL` 与 `OCR_USE_ANTHROPIC` 同时设置时，前者优先（文档说明）。
- `tryLegacyLlmConfig`：
  - 新增 `llm.protocol` 字段读取（见 §3.4）；非空 → 归一化优先。
  - 否则按 `use_anthropic`（同上产出规范名）。
- `tryCCEnv` / `tryShellRC`：硬编码 `Protocol: "anthropic"` 不变（已是规范名）。
- `ensureMessagesSuffix`：仅 anthropic 调用，逻辑不变。
- **authHeader 行为**（`resolver.go:330-348` 现有逻辑）：仅 `protocol == "anthropic"` 时设置 `authHeader`（`x-api-key` / `authorization`）；`openai` 和 `openai-responses` 均走 else 分支 → `authHeader = ""`。Responses API 的认证与 Chat Completions 完全一致——**SDK 的 `WithAPIKey(cfg.APIKey)` 自动设置 `Authorization: Bearer <key>`**（`requestconfig.go:664-667`，`authPreference = authCredentialPreferenceBearer`），无需任何 auth header 配置。

### 3.4 配置结构（方案 A）

**`internal/llm/resolver.go` `llmFileConfig`** 新增字段：

```go
type llmFileConfig struct {
    URL          string            `json:"url,omitempty"`
    AuthToken    string            `json:"auth_token,omitempty"`
    AuthHeader   string            `json:"auth_header,omitempty"`
    Model        string            `json:"model,omitempty"`
    Protocol     string            `json:"protocol,omitempty"`     // ← 新增：anthropic|openai|openai-responses
    UseAnthropic *bool             `json:"use_anthropic,omitempty"` // 降级为兼容回退
    TimeoutSec   int               `json:"timeout_sec,omitempty"`
    ExtraBody    map[string]any    `json:"extra_body,omitempty"`
    ExtraHeaders map[string]string `json:"extra_headers,omitempty"`
}
```

**`cmd/opencodereview/config_cmd.go`**

- `LlmConfig`（:222）新增 `Protocol string` 字段（同上 json tag）。
- `ProviderEntry`（:190）：`Protocol` 字段语义不变（已是 string），校验放开。
- `applyProviderField`（:382）`case "protocol"`（:388）：改为 `NormalizeProtocol` + `ValidateProtocol`。
- 新增 `case "llm.protocol"`：设置 `cfg.Llm.Protocol`（归一化后存规范名）。
- 既有 `case "llm.use_anthropic"` 保留（兼容）。
- 错误信息列出三规范名 + 别名 `openai`。

**优先级（legacy 块）**：`llm.protocol` 非空（归一化）> `use_anthropic`（兼容回退）。

> Manual tab（TUI）配置仍写 legacy `llm` 块；新增 `openai-responses` 选择后写入 `llm.protocol`（方案 A 让 manual 路径也能表达 responses，**覆盖原 §3.6 的「manual 不暴露」限制**）。见 §3.6 更新。

### 3.5 URL 归一化（决策 7 对齐）

**现状对齐对象：`NewOpenAIClient`（`client.go:286`）**——确保 cfg.URL 是完整端点，再剥后缀喂 SDK。

**resolver 侧**：`openai-responses` **不做任何 URL 处理**（与 `openai` 一致；仅 `anthropic` 调 `ensureMessagesSuffix`）。

**`NewOpenAIResponsesClient` 侧**（新 helper `ensureResponsesEndpoint`）：

```go
baseURL := strings.TrimRight(cfg.URL, "/")
if !strings.HasSuffix(baseURL, "/responses") {
    baseURL = baseURL + "/responses"
}
cfg.URL = baseURL
sdkBaseURL := strings.TrimSuffix(baseURL, "/responses")
```

行为契约（与 `TestNewOpenAIClient_URLNormalization` 对仗）：

| 输入 | cfg.URL |
|---|---|
| `https://api.openai.com/v1` | `…/v1/responses` |
| `https://api.openai.com/v1/` | `…/v1/responses` |
| `https://api.openai.com/v1/responses` | `…/v1/responses` |
| `https://api.openai.com/v1/responses/` | `…/v1/responses` |
| `https://api.openai.com` | `…/responses` |

兼容网关（Azure、第三方代理）行为与现有 openai client 等价。

### 3.6 TUI（`ocr config provider`）

**`cmd/opencodereview/provider_tui.go:53`**

```go
var cpProtocols = []string{
    llm.ProtocolAnthropic,
    llm.ProtocolOpenAIChatCompletions,
    llm.ProtocolOpenAIResponses,
}
```

- 该切片同时驱动 Custom 表单（`cpStepProtocol`）与 Manual 表单（`manualStepProtocol`）。
- 默认 `cpProtocolIdx=0`（anthropic）、`manualProtocolIdx=0` 保持。
- **Manual tab 现可表达 responses**：选中 `openai-responses` 后写入 `cfg.Llm.Protocol`（方案 A，无需再限制 manual 仅二协议）。
- `applyManualConfig`（`provider_cmd.go:92`）：
  - 优先用 `result.protocol`（规范名）写入 `cfg.Llm.Protocol`；
  - 当 protocol 为 `anthropic`/`openai` 时，仍同步设置 `use_anthropic`（兼容旧读取路径，双写无害）；
  - `result()`（:1481）中 protocol 取 `cpProtocols[idx]` 自动带规范名。

### 3.7 内置 provider registry

**`internal/llm/providers.go`**

- 12 处 `Protocol: "openai"` → `Protocol: llm.ProtocolOpenAIChatCompletions`（值不变，仍为 `"openai"`，仅从硬编码字符串改为常量引用）。
- 1 处 `Protocol: "anthropic"` → `Protocol: llm.ProtocolAnthropic`（值不变，用常量）。
- 顶部注释补充「支持的 Protocol 规范值」清单，便于后续新增 provider（呼应决策 1：保证扩展能力）。

> **内置 provider protocol vs 用户覆盖的优先级**：`tryProviderConfig`（`resolver.go:282-284`）中，`entry.Protocol != ""` 时**用户在 config 里设置的 `providers.X.protocol` 优先于 registry preset**。因此 `config set providers.openai.protocol openai-responses` 能真正改变内置 `openai` provider 的协议——registry 改规范名只是「用户未覆盖时的默认值」，二者叠加无冲突。

### 3.8 扩展性保证（决策 1）

- 未来新增「使用 responses 格式」的内置 provider：只需在 `providers.go` registry 加一条 `{Protocol: llm.ProtocolOpenAIResponses, BaseURL: ...}`，即被 `ocr config provider` 选用、被 `NewLLMClient` 正确分发，**零额外代码**。
- 未来 `anthropic-vertex`：实现 `AnthropicVertexClient` + 在 `NewLLMClient` switch 加 case + `ValidateProtocol` 放开 + registry 可选加项。
- URL 归一化在 client 内统一，新 provider 的 BaseURL 无论带不带端点后缀都工作。

---

## 4. 环境变量（决策 6，与配置联合）

**新增** `OCR_LLM_PROTOCOL`（常量 `envOCRLLMProtocol`）：`anthropic` | `openai` | `openai-responses`。

**resolver 优先级（env 路径）：**
1. `OCR_LLM_PROTOCOL` 非空 → `NormalizeProtocol` 后采用；
2. 否则 `OCR_USE_ANTHROPIC`：`true|未设`→`anthropic`，`false`→`openai`。

**文档更新**（README 环境变量表）：
- 新增 `OCR_LLM_PROTOCOL` 行。
- `OCR_USE_ANTHROPIC` 标注「兼容字段，优先使用 `OCR_LLM_PROTOCOL`」。

---

## 5. `store=false` 风险与 `PromptCacheKey` 技术取舍

> 本节作为文档单独成段，同时写入 README「Responses API 注意事项」。
> **详细设计**（含 state/Phase/store/CacheKey 的耦合矩阵、修改面对比、决策树）见独立文档：`DESIGN_STATE_CACHE_PHASE.md`。

### 5.1 采用 `store=false` 的理由与风险

- **理由**：OCR 走无状态多轮（决策 2），不使用 `previous_response_id`，无需 OpenAI 侧留存响应；`store=false` 更符合隐私最小化。
- **风险（需实测验证）**：OpenAI 文档对「`store=false` 时服务端 prefix caching 是否仍生效」表述不明确。部分信息暗示 `store=false` 可能限制或禁用自动 prefix 缓存。
- **验证步骤（PR 实现后、合并前必做）**：用真实 key，同一 prompt 分别 `store=true` / `store=false` 各跑一轮，对比 `usage.input_tokens_details.cached_tokens`。
  - 若 `store=false` 使缓存显著失效 → 回到决策点：要么接受（隐私优先）、要么改默认 `store=true`（缓存优先，附隐私告警）、要么改用 `previous_response_id` 状态模式（推翻决策 2，成本高）。

### 5.2 PromptCacheKey：调用方预计算一次方案

- **做法**：
  1. `ChatRequest` 新增 `CacheKey string \`json:"-"\`` 字段（`json:"-"` 保证 Chat Completions / Anthropic client 不序列化、不读取）。
  2. 新增包级 helper `llm.ComputeCacheKey(messages []Message) string`：从 messages 中提取 instructions（system 消息 `\n\n` 拼接）+ 第一条 user 消息文本，返回 `sha256(instructions + "\x00" + firstUser)[:32]`。
  3. 各 LLM 调用点在 **session / 请求构建时计算一次**，存为局部变量，每轮仅做字符串赋值：
     - **主循环**（`loop.go:RunPerFile`）：进入循环前调 `llm.ComputeCacheKey(initialMessages)`，存入局部变量 `cacheKey`；循环内构建 `ChatRequest` 时 `CacheKey: cacheKey`。
     - **单轮调用**（plan `agent.go:executePlan`、summary `agent.go:executeReviewFilter`、re-location `relocation.go`、scan `scan/agent.go` ×3）：在唯一的请求构建处同样调用一次。
  4. `buildResponsesParams` 直接读 `req.CacheKey`：非空 → 设 `PromptCacheKey`；空 → 不设。**client 不扫描 messages、不计算 hash。**

- **选择理由**：
  - 一个待评审文件对应一个 session，系统 prompt 跨文件完全一致；将第一条 user 消息（携带文件内容上下文）纳入 hash，使不同文件落入不同缓存桶。
  - hash 仅在 session 开始时计算一次，避免每轮重复 sha256。
  - `ChatRequest.CacheKey` 以 `json:"-"` 标记，对 Chat Completions / Anthropic client **零行为影响**（不序列化、不读取）。
  - client 层逻辑极简（透传一个字符串），职责清晰。

- **取舍 / 局限**：
  - 需修改 `ChatRequest`（全协议共享结构）增一个字段，并在 `loop.go` + 其他 5 个调用点在构建请求时计算并填入——侵入面比原 P1 方案（零管线改动）大，但改动机械、可控。
  - hash 不携带任何业务语义，无法人为区分会话。

- **与原 P1 方案的对比**：

  | 维度 | 原 P1（已弃用） | 本方案（采用） |
  |---|---|---|
  | key 来源 | `sha256(instructions)[:32]` | `sha256(instructions + firstUser)[:32]` |
  | 计算位置 | client 内 `buildResponsesParams` | 调用方（loop / agent），session 开始时 |
  | 计算频率 | 每轮重算 | 每 session 仅一次 |
  | 文件区分 | 无法（所有文件共享同一 key） | 可以（first user message 因文件而异） |
  | 管线改动 | 零 | `ChatRequest` +1 字段，5+ 调用点 +1 行 |

---

## 6. 测试计划

### 6.1 单元测试

**`internal/llm/protocol_test.go`（新增）**
- `NormalizeProtocol`：`openai-chat-completions`→`openai`；空串→空；规范名幂等；未知名原样。
- `ValidateProtocol`：三规范名通过；`ValidateProtocol(NormalizeProtocol("openai-chat-completions"))` 通过（别名须先归一化）；`anthropic-vertex` 拒绝（错误含「暂未实现」）；`grpc` 拒绝。

**`internal/llm/responses_client_test.go`（新增）**
- `TestNewOpenAIResponsesClient_URLNormalization`：对仗 §3.5 契约表。
- `TestBuildResponsesParams_SystemToInstructions`：多 system → `Instructions` 拼接。
- `TestBuildResponsesParams_ToolCallItems`：assistant+tool_calls → function_call items；tool 结果 → function_call_output，`call_id` 正确。
- `TestBuildResponsesParams_Tools`：`ToolDef` → `FunctionToolParam`（`Strict:false`）。
- `TestBuildResponsesParams_StoreAndCacheKey`：`Store=false`；`req.CacheKey` 非空时透传为 `PromptCacheKey`；为空时不设。
- `TestComputeCacheKey`：`llm.ComputeCacheKey` 对 system + first user 的 hash 正确；相同 instructions + first user → 相同 key；不同 first user → 不同 key。
- `TestMapResponsesResponse_TextOnly` / `_FunctionCalls`（`ID==CallID`）/ `_Usage`（含 `CachedTokens`）/ `_Status`（粗粒度映射）。
- 用 `httptest.Server` 录制/回放（参考 `client_test.go`）。

**`internal/llm/client_test.go`（扩展）**
- `TestNewLLMClient_Dispatch`：三规范名分别得到 `*AnthropicClient`/`*OpenAIClient`/`*OpenAIResponsesClient`；`openai-chat-completions` 别名经 resolver 归一化后落到 `*OpenAIClient`。

**`internal/llm/resolver_test.go`（扩展）**
- 自定义 provider `protocol: "openai-responses"` / `"openai"` / 别名 `"openai-chat-completions"` 各自解析正确。
- `anthropic-vertex` 被拒（错误信息校验）。
- legacy `llm.protocol` 优先于 `use_anthropic`。
- `OCR_LLM_PROTOCOL` 优先于 `OCR_USE_ANTHROPIC`。
- `openai-responses` URL 未被附加 `/v1/messages` 或 `/chat/completions`。

**`internal/llm/providers_test.go`（扩展）**
- registry 内置项 Protocol 均使用规范常量（`ProtocolOpenAIChatCompletions` / `ProtocolAnthropic`）。

**`cmd/opencodereview/`（扩展）**
- `provider_cmd_test.go` / `config_dispatch_test.go`：
  - `config set providers.X.protocol openai-responses` / `openai` / `openai-chat-completions`（别名）成功。
  - `config set custom_providers.Y.protocol anthropic-vertex` 失败（暂未实现）。
  - `config set llm.protocol openai-responses` 成功写入 `cfg.Llm.Protocol`。
- `provider_tui_test.go`：`cpProtocols` 含三项规范名；Manual/Custom 表单可选到 `openai-responses`，`result().protocol` 为规范名。

### 6.2 集成 / 手动验证

- `ocr config set provider openai` + `ocr config set providers.openai.protocol openai-responses` + key + `ocr llm test` 通过。
- 别名回归：`providers.openai.protocol openai` 仍走 Chat Completions（行为不变）。
- `ocr review` 与 `ocr scan` 在 `openai-responses` 下端到端跑通（plan/主循环/summary/re-location 四阶段）；多轮工具调用 `call_id` 无失配。
- 同仓库对比 `openai` vs `openai-responses` 的评论数 / token / `cached_tokens`（呼应 §5.1 验证）。
- 自定义网关 URL 透传正常。
- `ocr viewer` 会话 JSONL 可读。

---

## 7. 实施顺序（单 PR 内提交序列）

> 决策 10：单 PR。以下为 PR 内逻辑提交顺序，便于评审与定位。

1. **协议命名重构基底**：新增 `internal/llm/protocol.go` + 单测；引入 `NormalizeProtocol`/`ValidateProtocol`/常量。
2. **registry 与分发改名**：`providers.go` 改规范名常量；`client.go` `NewLLMClient` switch；`resolver.go` 各 strategy 产出规范名 + 校验。
3. **配置通路（方案 A）**：`resolver.go` `llmFileConfig` + `config_cmd.go` `LlmConfig` 增 `Protocol`；`applyProviderField` 放开；`llm.protocol` / `OCR_LLM_PROTOCOL` 优先级；TUI `cpProtocols` 三项 + manual 可表达 responses。
4. **Responses client + CacheKey 通路**：新增 `responses_client.go` + URL 归一化 + `buildResponsesParams`/`mapResponsesResponse` + `store=false`；`ChatRequest` 增 `CacheKey` 字段 + `llm.ComputeCacheKey` helper；loop / agent 各调用点在 session 开始时计算一次 cache key 并填入；接入分发。
5. **测试补全**：上述各测试文件。
6. **文档**：README（含全部译本：`README.zh-CN.md`、`README.ko-KR.md`、`README.ja-JP.md`、`README.ru-RU.md`；VS Code 扩展 `extensions/vscode/README.zh-CN.md` 按需）protocol 说明、配置示例、环境变量表、§5 注意事项；release notes 要点。

每个提交都应使 `go test ./...` / `go vet ./...` / `make build` 通过。

---

## 8. 验收清单

- [ ] 三规范名 + `openai` 别名 + `anthropic-vertex`（拒）行为符合 §1。
- [ ] `NewOpenAIResponsesClient` 实现 `LLMClient`，四阶段（plan/主循环/summary/re-location）可用。
- [ ] `ocr config provider`（TUI）、`ocr config set`、legacy `llm.protocol`、`OCR_LLM_PROTOCOL` 均可配置 `openai-responses`。
- [ ] 别名回归：存量 `openai` 配置行为不变。
- [ ] URL 归一化符合 §3.5 契约表；resolver 不处理 `openai-responses` URL。
- [ ] `store=false` + 预计算 cache key 生效（`ChatRequest.CacheKey` 透传）；§5.1 实测记录在案（缓存命中或确认退化）。
- [ ] `ocr llm test` / `ocr review` / `ocr scan` 在新协议下端到端通过；多轮 `call_id` 配对正确。
- [ ] `go test ./...` / `go vet ./...` / `make build` 通过。
- [ ] README（含译本）更新协议命名、配置示例、env 表、§5 注意事项。

---

## 9. 涉及文件清单

**新增：**
- `internal/llm/protocol.go` / `protocol_test.go`
- `internal/llm/responses_client.go` / `responses_client_test.go`

**修改：**
- `internal/llm/client.go`（分发 switch + `ChatRequest.CacheKey` 字段 + `ComputeCacheKey` helper）
- `internal/llm/resolver.go`（归一化/校验、`llm.protocol`、`OCR_LLM_PROTOCOL`、URL 处理条件常量化）
- `internal/llm/resolver_test.go`
- `internal/llm/providers.go`（registry 改规范名常量 + 注释）
- `internal/llm/providers_test.go`
- `internal/llm/client_test.go`（dispatch 测试）
- `internal/llmloop/loop.go`（`RunPerFile` 入口计算一次 cache key，每轮填入 `ChatRequest.CacheKey`）
- `internal/agent/agent.go`（plan / summary 调用点计算 cache key）
- `internal/diff/relocation.go`（re-location 调用点计算 cache key）
- `internal/scan/agent.go`（scan 调用点 ×3 计算 cache key）
- `cmd/opencodereview/config_cmd.go`（`LlmConfig.Protocol`、`applyProviderField`、`llm.protocol` case）
- `cmd/opencodereview/provider_tui.go`（`cpProtocols` 三项规范名）
- `cmd/opencodereview/provider_cmd.go`（`applyManualConfig` 双写）
- `cmd/opencodereview/provider_tui_test.go` / `provider_cmd_test.go` / `config_dispatch_test.go`
- `README.md` / `README.zh-CN.md` / `README.ko-KR.md` / `README.ja-JP.md` / `README.ru-RU.md`（及 `extensions/vscode/README.zh-CN.md` 按需）

**不修改（本期）：**
- `internal/agent/*`、`internal/scan/*`、`internal/config/testconnection/*`（均通过 `NewLLMClient(ep)` 按 `ep.Protocol` 自动分发，新增 `openai-responses` 天然兼容。注：`agent.go` / `scan/agent.go` / `relocation.go` 仅新增一行 `CacheKey: llm.ComputeCacheKey(...)` 填充，不涉及协议分发逻辑改动）
- `client.go` 的 `encodingForModel`（决策 9）
