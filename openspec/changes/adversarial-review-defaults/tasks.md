# Tasks

## 1. 模板与会话类型

- [x] 1.1 新增 `adversarial_task_system.md` 与 `adversarial_task_user.md`,并在 `task_template.json` 注册 `ADVERSARIAL_TASK` 会话。验证:`TestLoadDefault_AdversarialTask` 通过(消息数、占位符、不含 `{{plan_guidance}}`)。
- [x] 1.2 `Template` 与 `templateManifest` 增加可选 `AdversarialTask` 字段,`LoadDefault` 经 `resolveOptionalConversation` 加载,`ApplyLanguage` 覆盖该会话。验证:`TestApplyLanguage` 扩展分支通过。

## 2. 循环与编排

- [x] 2.1 抽出共享的 `runConversation`,新增 `RunAdversarialTask` 与 `session.AdversarialTask`,会话记录与缓存亲和键按 task type 分桶。验证:`make test` 中 llmloop 既有用例全绿(主路径行为不变)。
- [x] 2.2 `executeGroupSubtask` 增加第三阶段并实现 `executeGroupAdversarialPass`:跳过闸门(模板缺失、聚合预算、confirmed 上限)、prompt 预算检查、baseline 隔离 filter、失败仅记 warning。验证:`internal/agent/adversarial_test.go` 7 个用例通过。
- [x] 2.3 `checkPromptBudget` 参数 `round int` 改为 `phase string`,retry 报表新增 "Adversarial review" 阶段。验证:预算告警文案按 phase 区分,retry 分组显示正确。

## 3. 文档

- [x] 3.1 README 及 4 个 i18n 版本新增特性说明;`cli-reference.md` 5 个语言版本新增对抗性评审小节。验证:`TestCLIReferenceDocumentsAdversarialPass` 5 个子用例通过。

## 4. 遗留收尾

- [x] 4.1 复核并修正 `internal/agent/agent.go` 对抗性入口处注释。结果与最初判断相反:经自审复核,round-2+ 的 break 必然发生在更早轮次已置 `completed = true` 之后(`completed` 只置不清,`ReviewRounds()` 下限 1),原注释"该路径仅未来循环改动才会引入"是准确的;曾将其改错的中间版本由工作区自审(本 change 自带的对抗性 pass)发现并驳回。最终注释改为说明 `completed` 检查的防御性质。验证:控制流复核 + `make check` 通过 + 自审该项清零。
- [x] 4.2 补一条不设 `SkipFilter` 的对抗性用例,覆盖 baseline 隔离:标准轮次已过滤的评论不重复送审、pass 新增评论经 filter 后计入新增数。验证:该用例通过,且能捕获 baseline 被误用的回归。
- [x] 4.3 行为冒烟:构造一个含已知"缺失型"失效的小 fixture(如非幂等操作外包重试、超时分支缺失 nil 判空)外加一处纯格式改动,跑一次真实 `ocr review --audience agent`,确认对抗性 pass 能产出标准轮次未覆盖的发现、纯格式改动不招致实质性发现。验证:会话 JSONL 的 adversarial_task 分桶中出现新增发现(或明确记录 pass 零新增的事实结论)。
- [x] 4.4 提交前门禁:`make check`、`make test`,以及工作区自审 `ocr review --audience agent --background "briefly summarize the background requirements"`;PR 描述披露 AI/LLM 使用与 telemetry 属性 round→phase 的破坏性。验证:全部命令通过,自审无 critical/high 发现。
- [x] 4.5 区间评审(`ocr review --from main --to HEAD`)发现 `telemetry.md` 5 个语言版本仍把 `token.threshold.exceeded` 的属性记为 `round`:更新为 `phase`("round N" / "adversarial"),并补记 `adversarial.loop` span 行与四个 `adversarial.*` 事件行(`main.loop` span 的 `round` 属性未改名,保持不动)。验证:5 个文件的属性表与代码 emission 逐项一致,`make check` 通过。
