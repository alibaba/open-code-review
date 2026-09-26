# Design

## Context

`ocr review` 对每个文件组运行:Plan 阶段(按阈值可选)→ 主循环(最多 `MAX_REVIEW_ROUNDS` 轮,每轮以 `{{confirmed_comments}}` 回喂已确认发现,每轮结束后以 `baseline` 边界只对新增评论运行 review filter)。模板经 `go:embed` 内嵌清单(`task_template.json`)加载并引用 prompt 文件;会话记录与 telemetry 按 session task type 分桶,task type 同时参与 provider prompt 缓存亲和键的构造。动机见 proposal.md。

## Goals / Non-Goals

**目标:**

- 每个完成标准评审的文件组默认获得一次独立的对抗性第二意见,无 flag、无配置键。
- 对抗性产出的质量约束与主任务一致:同样的 `code_comment` 通道、同样的 review filter、同样的输出语言。
- pass 失败绝不改变组的评审结果。

**Non-Goals:**

- 不提供控制该 pass 的 flag、配置键或开关。
- 不把该 pass 纳入 `--effort` 预设的调节范围:`ApplyEffort` 只覆写 `MaxReviewRounds`,对抗性 pass 每组恒为单次对话,与轮数档位无关。
- 不改动 `ocr scan`、delegate 模式、分组/plan 阈值与主循环轮数。
- 不做 ROADMAP "Ultra Mode" 式的预算/轮数扩展。
- 不要求对抗性会话复用主对话的 provider prompt 缓存。

## Decisions

1. **独立对话,而非并入 main_task_system.md。** 同一段对话里模型刚提交完自己的发现,让它回头质疑自己刚接受的判断效果打折;独立对话没有这个包袱,且标准轮次的确认清单成为一次性的干净输入("这些别再说"),而主循环第 2 轮是边找边积累。对抗性语气也被隔离在末段,日常主 prompt 的中立姿态保持不变。落选方案:把对抗性实质并入默认 system prompt(本项目此前的草案方向,已废弃——它把对抗性语气带进每一次合作式评审,且没有"勿重复"整段对话语义)。

2. **保留对抗性人设;实质取自 openai/codex-plugin-cc 的 adversarial review prompt,取实质不取姿态。** system prompt 明确 "You are an adversarial code reviewer"……"demanding proof, not inventing problems"。人设适合一段立场明确的末段对话;配套两条校准约束:无证据不报(报告前必须用上下文工具读相关代码)与空结果合法(`task_done` 结束)。与 codex 原文的取舍:攻击面清单取其 `<attack_surface>` 七类中的五类拆并重组(鉴权/信任边界、竞态与顺序、数据丢失与回滚、失效处理与资源泄漏、兼容性);校准取其 `<calibration_rules>`"安全即零发现"与 `<grounding_rules>`"不虚构证据"。刻意不搬:`<operating_stance>` 的 "break confidence"/"Default to skepticism" 姿态(换成"第二双眼睛、要证据"的人设)、`<finding_bar>` 四问门槛、`<structured_output_contract>` JSON 契约(ocr 经 `code_comment` 工具输出);类别上丢弃 observability gaps、schema drift、empty-state/null/降级依赖、stale state/re-entrancy,并新增 codex 没有的 "Design choices" 挑战条目;`## Capabilities`、`## Strict Focus Rules` 与逐文件覆盖规则沿用 ocr 自家 `main_task_system.md` 的骨架。

3. **剔除 plan 引导。** plan 是标准评审自定的覆盖计划,喂给第二意见会让它顺着同一思路走,破坏独立性。`buildAdversarialTaskMessages` 不替换 `{{plan_guidance}}`,`TestLoadDefault_AdversarialTask` 断言 user prompt 不含该占位符。

4. **复用 runConversation,filter 走 baseline 隔离。** 对抗性 pass 是一次对话自然增长(内部工具循环驱动),不像主循环每轮重新渲染 messages;把 `RunMainTask` 函数体抽成 `runConversation(..., taskType)` 后两者共用同一套工具调用轮上限(`MAX_TOOL_REQUEST_TIMES`)、grace round 与预算语义,仅分桶不同。会话结束后以 `baseline`(标准轮次留下的评论数边界)调 `executeGroupReviewFilter`,其 `from` 语义保证只过滤索引 >= 边界的新增评论——标准轮次的评论不重复送审。

5. **尽力而为的失败语义。** 入口条件 `lastStop == nil && completed`:只在标准评审成功完成的组上运行。失败与跳过路径只经运行日志与 telemetry 观察(`adversarial.skipped` / `adversarial.failed` / `adversarial.stopped` / `adversarial.completed`;其中 LLM 错误、prompt 超预算与中途触顶路径另记 `recordWarning`),不触碰 `err` 与 `lastStop`。模板未配置该会话时裸返回、零输出——默认模板恒配置该会话,CLI 又无模板覆盖途径,该路径仅库调用可达。理由:标准发现已在 collector 里,第二意见是增值环节,不能让锦上添花的功能把成功的组变成失败。

6. **预算与 telemetry 的参数化。** `checkPromptBudget` 的 `round int` 改为 `phase string`("round N" / "adversarial"),telemetry 属性随之从 `round` 改名 `phase`;retry 报表新增 "Adversarial review" 阶段,避免落到兜底显示。

## Risks / Trade-offs

- [每组一次完整 agentic 对话的成本] → 文档如实标注;成本与组的数量成正比而非与文件数。
- [pass 花费计入共享 `--max-tokens-budget`,可能挤占后续组覆盖] → 中途触顶会置 `budgetExceeded` 并记录 warning;语义上这是"覆盖率截断"而非单纯多花钱,文档已说明。若需要隔离,以独立变更考虑给 pass 单独预算。
- [telemetry 属性 round→phase 改名破坏既有看板] → 已知破坏性;在 PR 描述中披露。
- [错误路径下部分提交的对抗性评论未经 filter 即进入结果] → 与主循环第 1 轮出错时的既有行为一致,沿用取舍;如需收紧以独立变更跟进。
- [测试固定 SkipFilter: true,baseline 隔离逻辑无覆盖] → 收尾任务补一条走 filter 的用例。

## Migration Plan

实现已随 `performance` 分支落地(提交 6750a1e),无配置迁移、无数据迁移、无发布开关。本 change 落地后:完成 tasks 中的收尾项(注释修正、filter 隔离测试、行为冒烟、`make check` / `make test` / `ocr review` 自审),随后 `openspec sync` 将 `review-prompting` 同步为项目第一个正式 spec,再 archive 本 change。回滚即还原该 feature 提交。
