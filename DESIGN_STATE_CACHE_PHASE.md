# 详细设计：对话历史管理、Phase 字段与缓存策略

> 关联：`PLAN_RESPONSES_SUPPORT.md` 第 3 点（Phase / 无状态张力）与第 5 点（`store=false` + `PromptCacheKey` 交互）。
> 本文深入分析 OpenAI Responses API 中与**对话状态**、**缓存**、**Phase 字段**相关的参数取舍，列出每个选项的修改面、优缺点和风险，供最终决策。
> 状态：**待决策（DRAFT）** — 需要在实施前选定一个方案组。

---

## 0. 问题域概述

OpenAI Responses API（`/v1/responses`）相比 Chat Completions 引入了若干与「对话历史管理」和「缓存」相关的新概念。OCR 的 Agent 主循环（`internal/llmloop/loop.go`）是一个**多轮工具调用循环**，每个文件内会进行 N 轮 LLM 请求。这些新概念直接影响：

1. **对话状态**：每轮是重放全部历史，还是用 `previous_response_id` 引用服务端状态？
2. **Phase 字段**：gpt-5.3-codex+ 模型在 assistant 消息上标注 `commentary` / `final_answer`，重放时需保留——但 OCR 的 `Message` 结构没有这个字段。
3. **`store` 参数**：服务端是否留存响应？影响隐私和缓存。
4. **`PromptCacheKey`**：显式缓存桶 key——由调用方在 session 开始时计算一次（`sha256(instructions + firstUser)[:32]`），经 `ChatRequest.CacheKey` 透传。在 `store=false` 下是否生效？

这四个问题**高度耦合**，不能单独决策。下文逐一展开，最后给出耦合矩阵与推荐方案。

---

## 1. 现有架构基线（修改面分析的前提）

### 1.1 多轮调用流程

OCR 的 LLM 调用分为四类（均在 `internal/agent/agent.go`）：

| 阶段 | 入口 | 调用模式 | 对话历史 |
|---|---|---|---|
| Plan | `executePlan` (:743) | **单轮** | 从模板构建 messages，一次性请求 |
| 主循环 | `runner.RunPerFile` (:490) → `loop.go:153` | **多轮** | messages 跨轮追加，含工具调用/结果 |
| Summary | `executeReviewFilter` (:505) 等 | **单轮** | 从模板构建，一次性请求 |
| Re-location | 类似 summary | **单轮** | 从模板构建，一次性请求 |

**关键观察**：只有**主循环**是多轮的；其余阶段每轮独立、无跨轮状态。因此对话历史管理问题**仅存在于主循环**。

### 1.2 主循环的消息管理（`loop.go`）

```
初始 messages (来自模板 + diff + plan)
    │
    ▼
┌─── loop ───────────────────────────────────┐
│ │ messages → CompletionsWithCtx(req)       │
│ │ resp.Content() / resp.ToolCalls()        │
│ │ executeToolCall(...)                     │
│ │ addNextMessage(content, calls, results)  │ ← 追加 assistant + tool 消息
│ │ [token 压缩: soft 60% / warn 80%]        │
│ └──────────────────────────────────────────┘
```

- 每轮把**完整 `messages` 切片**发给 client（`loop.go:166-171`）。
- Client（`OpenAIClient` / `AnthropicClient`）是**无状态的**：不持有 conversation ID，每次请求自包含。
- `addNextMessage`（:405）用 `llm.NewToolCallMessage(content, toolCalls)` / `llm.NewTextMessage("assistant", content)` 重建 assistant 消息，追加到 `messages`。
- **assistant 消息的重建路径**是 Phase 信息丢失的根因（详见 §3）。

### 1.3 现有 OpenAI client 的缓存实践

- **Chat Completions**：无显式 cache key 字段；依赖 OpenAI 服务端自动 prefix caching（同一 prompt 前缀自动命中）。
- **Anthropic client**：使用 `cache_control: ephemeral` 标记 system prompt 最后一个 block（`client.go:653`），显式控制缓存断点。
- 两种 client 均为无状态，靠「重放完整历史 + 服务端 prefix 匹配」获得缓存收益。

---

## 2. 状态管理：`previous_response_id` vs 无状态重放

### 2.1 选项矩阵

| | 选项 A：无状态重放（当前计划，决策 2） | 选项 B：`previous_response_id` 状态链 | 选项 C：混合（首轮重放 + 后续状态链） |
|---|---|---|---|
| **每轮发送** | 完整 input（全部历史 items） | 仅新增 items + `previous_response_id` | 首轮完整，后续仅新增 |
| **`store`** | `false`（无需留存） | `true`（强制，服务端须留存） | `true` |
| **跨轮状态** | 无（client 无状态） | response_id 须跨轮携带 | response_id 须跨轮携带 |
| **带宽** | 高（重放全部历史） | 低（仅增量） | 中 |
| **缓存** | 靠 prefix 匹配（不确定） | 服务端自动（确定） | 服务端自动（确定） |
| **Phase 字段** | 丢失（需额外处理，见 §3） | 服务端保留（天然兼容） | 服务端保留 |
| **隐私** | 最佳（不留存） | 最差（全部对话留存在服务端） | 差 |
| **response_id 过期** | 无此问题 | 有（30 天，但长会话可能截断） | 有 |
| **代理/网关兼容** | 最佳 | 依赖代理支持 Responses API 完整语义 | 同 B |
| **并行/分布式** | 天然支持 | response_id 链不可并行 | 不可并行 |

### 2.2 修改面对比

#### 选项 A：无状态重放（当前计划）

**修改的文件：**
- `internal/llm/responses_client.go`（新增）：`buildResponsesParams` 每轮把完整 `messages` 转为 input items
- `internal/llm/client.go`：分发 switch
- `internal/llm/protocol.go`（新增）、`resolver.go`、`providers.go`、`config_cmd.go`、`provider_tui.go`、`provider_cmd.go`

**不修改的文件：**
- ✅ `internal/llmloop/loop.go` — 无状态方案不需要 loop 感知 response_id
- ✅ `internal/agent/agent.go` — 四阶段调用方式不变
- ✅ `internal/llm/client.go` 的 `ChatRequest` / `Message` 结构 — 不增字段

**侵入性：低。** 所有改动集中在 client 层和配置层，loop / agent 零感知。

#### 选项 B：`previous_response_id` 状态链

**在选项 A 基础上额外修改：**

| 文件 | 改动 |
|---|---|
| `internal/llm/client.go` | `ChatRequest` 增 `PreviousResponseID string` 字段（**全协议共享结构**） |
| `internal/llmloop/loop.go` | `Runner` 增 `lastResponseID string` 字段；每轮调用后从 `ChatResponse` 提取 ID 并存入 Runner；每轮构建 `ChatRequest` 时填入；`RunPerFile` 入口重置 ID（新文件新链） |
| `internal/llmloop/loop.go` | `addNextMessage` 逻辑改变：不再追加完整 assistant 消息到 `messages`（只追加新增的 tool 结果 items）；或维护两套消息表示 |
| `internal/llm/client.go` | `ChatResponse.ID` 已有字段（`json:"-"`），需确保 Responses client 正确填充 |
| `internal/agent/agent.go` | Plan / Summary / Re-location 阶段也需决定是否用状态链（目前都是单轮，可不参与） |
| `internal/llmloop/loop_test.go` | 所有测试需适配新的状态传递 |
| `internal/llm/client_test.go` | `ChatRequest` 新字段的相关测试 |

**侵入性：高。** 破坏 loop 的「无状态」不变量；`ChatRequest` 是全协议共享结构，为 Responses-only 特性加字段是污染；loop 测试全部需改。

#### 选项 C：混合

修改面与 B 基本一致，但逻辑更复杂（首轮特殊处理），不推荐——复杂度增加但收益不明显。

### 2.3 推荐

**维持选项 A（无状态重放）。** 理由：
1. 侵入性最低（loop / agent / 共享结构零改动）；
2. 与现有 Anthropic / OpenAI client 的缓存模式一致（重放 + prefix 匹配）；
3. 隐私最优；
4. Phase 和缓存的短板可通过 §3 / §4 的独立决策弥补，不值得为此推翻无状态。

---

## 3. Phase 字段：codex 系模型的 assistant 消息标注

### 3.1 背景

SDK `ResponseOutputMessage`（`responses/response.go:16084`）定义：

```go
type ResponseOutputMessage struct {
    ID      string
    Content []ResponseOutputMessageContentUnion
    Role    constant.Assistant  // "assistant"
    Status  ResponseOutputMessageStatus
    Type    constant.Message    // "message"
    // Labels an `assistant` message as intermediate commentary (`commentary`)
    // or the final answer (`final_answer`). For models like `gpt-5.3-codex`
    // and beyond, when sending follow-up requests, preserve and resend phase
    // on all assistant messages — dropping it can degrade performance.
    Phase ResponseOutputMessagePhase  // ← 新字段，commentary | final_answer
}
```

**关键**：SDK 文档明确警告——对 gpt-5.3-codex+ 模型，**重发 assistant 消息时必须保留 Phase，否则性能降级**。

### 3.2 Phase 在无状态方案中的丢失路径

```
API 返回 ResponseOutputMessage{Phase: "commentary", Content: [...]}
        │
        ▼  mapResponsesResponse (responses_client.go)
ResponseMessage{Role:"assistant", Content: *string, ReasoningContent, ToolCalls}
        │                                      ← Phase 在此丢弃（ResponseMessage 无 Phase 字段）
        ▼  loop.go:189-190
content := resp.Content()
calls := resp.ToolCalls()
        │
        ▼  loop.go:425 / 427  addNextMessage
llm.NewToolCallMessage(content, toolCalls)  /  llm.NewTextMessage("assistant", content)
        │                                      ← Message 结构无 Phase
        ▼  下一轮 CompletionsWithCtx
buildResponsesParams → ResponseInputItemParamOfMessage(content, "assistant")
                                               ← Phase 无法回传给 API
```

### 3.3 选项矩阵

| | 选项 P1：丢弃 Phase（接受降级） | 选项 P2：Message 增 Phase 字段（穿透） | 选项 P3：改用状态链（服务端保留） |
|---|---|---|---|
| **代码侵入** | 无 | 中 | 高（同 §2 选项 B） |
| **codex 性能** | 降级 | 满分 | 满分 |
| **非 codex 模型** | 无影响 | 无影响（Phase 为空，不设） | 无影响 |
| **共享结构污染** | 无 | `Message` / `ResponseMessage` 增 Responses-only 字段 | `ChatRequest` 增字段 |
| **当前模型受影响?** | 否（GPT-5.x / o-系列当前不产生 Phase） | 否 | 否 |

### 3.4 修改面对比

#### 选项 P1：丢弃 Phase（当前计划隐含选择）

**无额外文件修改。** 在 `mapResponsesResponse` 中不读取 Phase，在 `buildResponsesParams` 中不设 Phase。

**风险**：当 OCR 未来支持 gpt-5.3-codex+ 时，需回头补 P2。但当前 GPT-5.x / o-系列不产生 Phase，**短期无实际影响**。

#### 选项 P2：Message 增 Phase 字段

| 文件 | 改动 |
|---|---|
| `internal/llm/client.go:42` | `Message` 增 `Phase string json:"-"`（json tag `-` 因为这是内部传递，不序列化到 Chat Completions / Anthropic 的请求体） |
| `internal/llm/client.go:130` | `ResponseMessage` 增 `Phase string` |
| `internal/llm/client.go:64` | `NewToolCallMessage` 增加可选 phase 参数，或新增 `NewToolCallMessageWithPhase` 构造器 |
| `internal/llm/client.go:59` | `NewTextMessage` 同上，或新增带 phase 的变体 |
| `internal/llm/responses_client.go` | `mapResponsesResponse`：提取 `AsMessage().Phase` → `ResponseMessage.Phase`；`buildResponsesParams`：assistant message item 的 Phase 非空时设置 |
| `internal/llmloop/loop.go:405` | `addNextMessage`：从 `resp` 的 Choice 中读 Phase，传给 `NewToolCallMessage` / `NewTextMessage` |
| `internal/llmloop/loop.go:166` | `RunPerFile`：当前只取 `resp.Content()` / `resp.ToolCalls()`，需额外取 Phase |
| `internal/llmloop/loop_test.go` | 适配新构造器签名 |
| `internal/llm/client_test.go` | 适配 |
| Anthropic / OpenAI client | **不修改**（Phase 字段存在但被忽略；Chat Completions / Anthropic API 无此概念） |

**侵入性分析**：`Message` 是全协议共享的核心结构。加一个 `json:"-"` 的字段对 Anthropic / OpenAI client **零行为影响**（序列化时忽略，反序列化时为零值），但概念上是「共享结构承载协议特定字段」。如果未来还有更多 Responses-only 字段，这条路径会逐渐膨胀。

**SDK 回传 Phase 的方式**（需验证）：Responses API 的 input param 中，assistant 消息用 `ResponseInputItemParamOfOutputMessage`（response.go:11862）而非 `ResponseInputItemParamOfMessage`，前者接受 `status` 参数。Phase 是否通过该路径回传，需查 SDK 的 `ResponseOutputMessagePhase` 类型和对应的 param 字段——**这是实现期需确认的细节**。

#### 选项 P3：状态链（同 §2 选项 B）

修改面见 §2.2 选项 B。Phase 由服务端自动保留，无需客户端处理。

### 3.5 推荐

**短期选 P1（丢弃 Phase），在代码中留 TODO 标注。** 理由：
1. 当前目标模型（GPT-5.x / o-系列）**不产生 Phase**，P2 是纯预防性改动；
2. P2 侵入共享 `Message` 结构，在没有实际模型验证收益前不宜引入；
3. 当 gpt-5.3-codex 进入支持范围时，再按 P2 路径实施——届时 SDK 的 Phase 回传机制也更明确；
4. 在 `responses_client.go` 的 `mapResponsesResponse` 中注释：
   ```go
   // TODO(phase): ResponseOutputMessage.Phase (commentary/final_answer) is
   // currently dropped. For gpt-5.3-codex+ models, preserve and resend Phase
   // on assistant messages to avoid performance degradation. See
   // DESIGN_STATE_CACHE_PHASE.md §3.
   ```

---

## 4. `store` 参数与 `PromptCacheKey` 的交互

### 4.1 参数语义

| 参数 | 类型 | 作用 |
|---|---|---|
| `store` | `param.Opt[bool]` | `true`（默认）：服务端留存响应，可用 `previous_response_id` 引用；`false`：不留存 |
| `prompt_cache_key` | `param.Opt[string]` | 显式缓存桶 key，让服务端把相同 key 的请求归桶做 prefix 匹配 |

### 4.2 耦合关系

```
                    previous_response_id 可用?
                           │
              ┌──────── yes │ no ────────┐
              │                            │
          store=true                   store=false
              │                            │
     ┌────────┴────────┐                   │
     │                 │                   │
  prefix cache     phase/reasoning     prefix cache
  自动生效         服务端保留           生效? ← 核心不确定点
     │                                     │
     │                           ┌─────────┴─────────┐
     │                           │                   │
     │                     PromptCacheKey        无 cache key
     │                     被服务端使用?          (纯靠前缀匹配)
     │                           │
     │                     生效? 不确定
```

### 4.3 `store=false` 下 prefix caching 的不确定性

**问题**：OpenAI 官方文档对「`store=false` 时服务端 prefix caching 是否仍生效」表述不明确。

**已知事实**：
- Chat Completions API 无 `store` 参数，prefix caching 是自动的、无条件的。
- Responses API 引入 `store` 是为了隐私控制（不留存 = 不存储响应对象）。
- 逻辑上有两种可能：
  - **可能 1**：`store=false` 仅控制响应对象留存，prefix caching 仍独立工作（因为 caching 是 infra 层面的 KV cache，与响应对象存储是两回事）。
  - **可能 2**：`store=false` 同时禁用 prefix caching（因为缓存命中需要某种形式的留存）。
- **`PromptCacheKey` 在 `store=false` 下是否被使用**：更不确定。如果服务端不缓存，key 就无意义。

### 4.4 选项矩阵

| | 方案 SC1：`store=false` + 预计算 cache key（当前计划） | 方案 SC2：`store=true` + 无 cache key | 方案 SC3：`store=true` + 预计算 cache key | 方案 SC4：`store=false` + 无 cache key |
|---|---|---|---|---|
| **隐私** | 最佳（不留存） | 差（留存） | 差（留存） | 最佳 |
| **prefix caching** | 不确定 | **确定生效** | **确定生效** | 不确定 |
| **PromptCacheKey** | 不确定是否生效 | 不需要（自动缓存） | 确定生效 | 不适用 |
| **previous_response_id** | 不可用 | 可用（但不用） | 可用（但不用） | 不可用 |
| **代码改动** | `ChatRequest`+1 字段 + 调用方计算 key | 改一行（`store=true`） | SC1 + 改一行 | 去掉 cache key 通路 |
| **网关兼容** | 最佳 | 网关需支持留存 | 同 SC2 | 最佳 |

### 4.5 `PromptCacheKey`（调用方预计算方案）的技术取舍

**当前计划**：`PromptCacheKey = sha256(instructions + firstUserMessage)[:32]`，由调用方（loop / agent）在 session 开始时计算一次，经 `ChatRequest.CacheKey` 透传给 client。

**为什么不用原 P1 方案（client 内每轮从 instructions 计算）**：
- 原 P1 的 key 仅基于 instructions，所有文件共享同一 system prompt → 同一 cache key，无法区分文件维度。
- 原 P1 每轮在 `buildResponsesParams` 内重新扫描 messages + 计算 sha256，虽然开销极低，但逻辑上不优雅。

**预计算方案的设计要点**：

| 维度 | 说明 |
|---|---|
| **key 来源** | `sha256(instructions + "\x00" + firstUserMessage)[:32]`——null-byte 分隔避免拼接歧义 |
| **计算位置** | 调用方（`loop.go:RunPerFile` 入口 / 各单轮调用点），通过 `llm.ComputeCacheKey(messages)` helper |
| **计算频率** | 每 session 仅一次（主循环）/ 每请求一次（单轮调用，天然只有一次） |
| **传递方式** | `ChatRequest.CacheKey string \`json:"-"\``——`json:"-"` 保证 Chat Completions / Anthropic client 忽略此字段 |
| **client 行为** | `buildResponsesParams` 读 `req.CacheKey`：非空 → 设 `PromptCacheKey`；空 → 不设。不扫描 messages、不计算 hash |

**前提依赖**：预计算 key 仅在 `store=false` 且 prefix caching 仍生效的场景下有意义。如果 `store=false` 禁用了一切缓存，cache key 就是死代码（无害但无用）。

### 4.6 验证计划（必须执行）

**在 PR 实现后、合并前，用真实 key 做对照实验：**

| 实验 | store | PromptCacheKey | 预期观察 |
|---|---|---|---|
| 基线 | `true` | 不设 | `cached_tokens` 应在第 2+ 轮显著 > 0 |
| SC1 | `false` | 设（预计算） | 观察 `cached_tokens`：若 > 0 且与基线接近 → cache key 有效；若 = 0 → 无效 |
| SC4 | `false` | 不设 | 观察 `cached_tokens`：若 > 0 → store=false 不影响 prefix caching；若 = 0 → 影响 |

**决策树（基于实验结果）：**

```
SC4 的 cached_tokens > 0?
├── Yes → store=false 不影响 prefix caching
│   └── SC1 的 cached_tokens > 0?
│       ├── Yes → PromptCacheKey 在 store=false 下生效，保留预计算 key（当前计划 OK）
│       └── No  → PromptCacheKey 在 store=false 下无效，改用 SC4（去掉 cache key，简化）
└── No  → store=false 禁用 prefix caching
    └── 回到决策点：
        ├── 接受（隐私优先，无缓存）→ SC4
        ├── 改 store=true（缓存优先）→ SC2 或 SC3
        └── 改用 previous_response_id（推翻无状态）→ §2 选项 B + store=true
```

### 4.7 推荐

**实施期采用 SC1（当前计划），但必须在合并前跑完 §4.6 验证。** 根据 `cached_tokens` 实测结果决定最终方案。理由：
1. SC1 是隐私最优的方案；
2. 如果验证显示 `store=false` 禁用了缓存，可一行切换到 SC2（`store=true`），代价仅是隐私降级；
3. cache key 通路代码量小（`ChatRequest` +1 字段 + 各调用点 +1 行 `llm.ComputeCacheKey(...)` 调用），即使最终证明无效，移除也无成本；
4. 不应在不确定的阶段提前复杂化（如直接选状态链）。

---

## 5. 耦合决策矩阵（综合视图）

下表展示四个维度（状态 / Phase / store / cache key）的组合方案：

| 方案组 | 状态 | Phase | store | CacheKey | 代码侵入 | 缓存确定性 | 隐私 | codex 兼容 |
|---|---|---|---|---|---|---|---|---|
| **★推荐** | 无状态 | 丢弃(P1) | false | 预计算 | 中低 | 待验证 | 最佳 | 降级(可接受) |
| 缓存优先 | 无状态 | 丢弃(P1) | true | 无 | 低 | 确定 | 差 | 降级 |
| 缓存+keying | 无状态 | 丢弃(P1) | true | 预计算 | 中低 | 确定 | 差 | 降级 |
| Phase 完整 | 无状态 | 穿透(P2) | false | 预计算 | 中 | 待验证 | 最佳 | 满分 |
| 全功能 | 状态链 | 服务端保留 | true | 自动 | 高 | 确定 | 最差 | 满分 |

**★ 推荐方案（当前计划）**：
- 状态：无状态重放（§2 选项 A）
- Phase：丢弃 + TODO 标注（§3 选项 P1）
- store：`false`（§4 SC1），**合并前必须跑验证**
- CacheKey：预计算（`sha256(instructions + firstUser)[:32]`，调用方计算一次经 `ChatRequest.CacheKey` 透传），**若 §4.6 验证证明无效则移除**

---

## 6. 最终修改面汇总（推荐方案）

### 6.1 新增 / 修改文件

| 文件 | 内容 | 与 Phase/store 相关的要点 |
|---|---|---|
| `internal/llm/responses_client.go`（新增） | Responses client 实现 | `buildResponsesParams`：设 `store=false` + 读 `req.CacheKey` 透传为 `PromptCacheKey`（不扫描 messages、不计算 hash）；不设 Phase。`mapResponsesResponse`：用 `sdkResp.OutputText()` 取文本；**Phase 字段加 TODO 注释** |
| `internal/llm/client.go`（修改） | `ChatRequest` 增字段 + helper | `ChatRequest` 增 `CacheKey string \`json:"-"\``；新增包级 helper `ComputeCacheKey(messages []Message) string` |
| `internal/llmloop/loop.go`（修改） | 主循环调用点 | `RunPerFile` 入口调 `llm.ComputeCacheKey(initialMessages)` 计算一次，存局部变量；每轮构建 `ChatRequest` 时填入 `CacheKey` |
| `internal/agent/agent.go`（修改） | plan / summary 调用点 | 各请求构建处调 `llm.ComputeCacheKey(messages)` 填入 `CacheKey` |
| `internal/diff/relocation.go`（修改） | re-location 调用点 | 同上 |
| `internal/scan/agent.go`（修改） | scan 调用点 ×3 | 同上 |

### 6.2 不修改的文件（及其理由）

| 文件 | 不修改理由 |
|---|---|
| `internal/llm/client.go` 的 `Message` 结构 | Phase 丢弃方案不需要 Message 增字段（Phase 维度）；`ChatRequest` 仅增 `CacheKey`（`json:"-"`，对其它协议零影响） |
| `internal/config/testconnection/*` | 通过 `NewLLMClient(ep)` 自动分发，无需感知协议 |

### 6.3 如果未来需要 Phase 支持（P2 路径）

届时需修改的文件清单（供未来参考）：
- `internal/llm/client.go`：`Message` + `ResponseMessage` 增 `Phase` 字段
- `internal/llm/responses_client.go`：`mapResponsesResponse` 提取 Phase；`buildResponsesParams` 回传 Phase
- `internal/llmloop/loop.go`：`addNextMessage` / `RunPerFile` 传递 Phase
- `internal/llmloop/loop_test.go` + `internal/llm/client_test.go`：适配
- 需验证 SDK 的 `ResponseInputItemParamOfOutputMessage` 是否支持 Phase 参数

---

## 7. 待确认项（实施期决策点）

- [ ] **§4.6 验证结果**：`store=false` 下 `cached_tokens` 是否 > 0？据此决定最终 store/CacheKey 组合。
- [ ] **SDK Phase 回传机制**：`ResponseInputItemParamOfOutputMessage` 的参数中是否有 Phase 对应字段？（影响 P2 实施可行性，不影响当前 P1 决策）
- [ ] **gpt-5.3-codex 支持时间表**：决定 P2 的优先级（如果短期不会支持，P1 的 TODO 可以长期存在）。
