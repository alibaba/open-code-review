# Proposal

## Why

`ocr review` 的默认评审是合作式中立姿态:主任务 prompt 评审"改了什么",并要求"避免评论正确的代码"。竞态、鉴权缺口、数据丢失与回滚缺失、资源泄漏这类高代价失效,在 diff 里往往表现为"缺失的东西"(没写的锁、判空、回滚),这类问题在该姿态下被系统性低估。多轮回喂已确认发现的机制还会带来反向压力:后续轮次被告知"别重复、再找真问题",容易产出低价值发现凑数。因此把对抗性第二意见作为默认流水线的一部分加入——不新增 flag、不新增配置键。

## What Changes

- 新增 `ADVERSARIAL_TASK` 模板会话(`adversarial_task_system.md` + `adversarial_task_user.md`,经 `task_template.json` 清单引用、`go:embed` 加载)。模板字段可选,缺失时静默跳过该 pass。
- 新增 session task type `adversarial_task`。`internal/llmloop` 把 `RunMainTask` 的函数体重构为共享的 `runConversation`,新增 `RunAdversarialTask` 复用同一套工具循环、轮数上限、grace round 与聚合预算语义,仅会话分桶与 provider 缓存亲和键不同。
- `ocr review` 流水线为每个文件组新增第三阶段:标准评审(Plan → 主循环)成功完成后,运行一次独立的对抗性对话。输入与主任务相同(system rule、组外变更文件、组内 diff、需求背景),外加标准轮次的确认清单作为"勿重复"上下文;刻意不含 plan 引导。
- 对抗性会话经同一条 `code_comment` 通道提交评论、进同一个 collector;随后以 `baseline` 边界只对本 pass 新增的评论运行 review filter,标准轮次已过滤的评论不重复送审。
- 尽力而为契约:聚合 token 预算耗尽、确认发现达到 30 条上限、渲染后 prompt 超过 `max_tokens` 的 80%、LLM 错误或对话中途停止时,只经运行日志与 telemetry 记录原因,不改变组的完成状态或退出码。模板未配置 `ADVERSARIAL_TASK` 时 pass 静默不运行(默认模板恒配置该会话,CLI 无模板覆盖途径,该路径仅库调用可达)。
- 配套对齐:`ApplyLanguage` 覆盖新会话(pass 产出用户可见评论,必须遵守输出语言配置);retry 报表新增 "Adversarial review" 阶段;`checkPromptBudget` 的 `round int` 参数改为 `phase string`,telemetry 事件 `token.threshold.exceeded` 的属性随之从 `round` 改为 `phase`(注意:按该属性聚合的既有看板需同步调整)。
- CLI 面零变化:无 flag、无配置键;`--preview` 不执行 LLM 调用,因此不运行该 pass。

## Capabilities

### New Capabilities

- `review-prompting`:`ocr review` 的对抗性评审 pass 行为契约——何时运行、输入什么、产出如何过滤、失败如何处理,以及输出语言与会话归属约束。

### Modified Capabilities

(无——这是项目的第一个 spec。)

## Impact

- **代码**:`internal/agent/agent.go`(`executeGroupAdversarialPass`、`buildAdversarialTaskMessages`、`checkPromptBudget` 参数化)、`internal/llmloop/loop.go`(`runConversation` 抽取与 `RunAdversarialTask`)、`internal/config/template/`(`template.go`、`task_template.json`、`prompts/adversarial_task_system.md`、`prompts/adversarial_task_user.md`)、`internal/session/history.go`、`cmd/opencodereview/output.go`。
- **测试**:`internal/agent/adversarial_test.go`(7 个用例:正常路径、失败保状态、预算耗尽跳过、模板缺失跳过、对话中途预算停止、确认发现达上限跳过、prompt 超预算跳过)、`internal/config/template/template_test.go`(`TestLoadDefault_AdversarialTask`、`TestApplyLanguage` 扩展)、`cmd/opencodereview/cli_reference_compare_docs_test.go`(5 个语言版本的文档断言)。
- **文档**:README 及其 4 个本地化版本各加一条特性说明;`cli-reference.md` 5 个语言版本新增对抗性评审小节;`telemetry.md` 5 个语言版本将 `token.threshold.exceeded` 的属性从 `round` 更新为 `phase`("round N" / "adversarial"),并补记 `adversarial.loop` span 与 `adversarial.skipped` / `failed` / `stopped` / `completed` 四个事件。
- **成本**:每个文件组约多一次完整 agentic 对话;花费计入共享的 `--max-tokens-budget`,预算紧张时会挤占后续文件组的覆盖(表现为覆盖率截断,而非单纯多花钱)。
- **状态**:实现已在 `performance` 分支落地(提交 6750a1e)。本 change 是回顾式规格:tasks 只剩核对与收尾项,落地后经 `openspec sync` 将 `review-prompting` 同步为项目第一个正式 spec,再 archive。
