# Design

## Context

`ocr review` 对每个评审组运行一段 agentic 循环:内嵌的 `MAIN_TASK` 对话(system prompt 在 `internal/config/template/prompts/main_task_system.md`,user prompt 在 `main_task_user.md`)驱动 LLM 通过只读工具收集上下文、经 `code_comment` 提交发现、以 `task_done` 结束。每组最多运行 `MAX_REVIEW_ROUNDS` 轮,已确认评论经 `{{confirmed_comments}}` 回喂。其后是独立的 filter 阶段(`review_filter_task_user.md`):只移除其断言被 diff 证伪的评论,受 protected-subjects 否决(内存安全、并发、链接与声明一致性、行为/兼容性变更、未使用参数)与"证据不足即放行"的偏向约束。模板经 `go:embed` 内嵌清单(`task_template.json`)加载并引用 prompt 文件;`TestLoadDefault_PlaceholdersPresent` 与 `TestLoadDefault_FieldsPopulated` 已断言占位符、消息数与标量,但没有任何测试钉住主任务 system prompt 的小节。`ocr scan` 流水线使用独立的 `scan_template.json`。动机见 proposal.md。

## Goals / Non-Goals

**目标:**

- 每一次默认 `ocr review` 都主动猎取列明的失效类别、满足实质性门槛、执行校准——CLI 面零变化。
- 合并保留既有的客观语气、文件范围约束、工具循环与输出结构。

**非目标:**

- 不提供切换该姿态的 flag、配置键或模板包选择机制。
- 不改动 `ocr scan`、delegate 模式、分组/plan 阈值、filter 阶段。
- 不是 ROADMAP 的 "Ultra Mode"(预算/轮数扩展仍是独立规划项)。
- 不提供 `--stance` 式逃生舱(探索阶段已被用户否决)。

## Decisions

1. **并入默认 system prompt,而非新建模板包。**
   该姿态是静态、全局、常开的;`main_task_system.md` 正是这一层。仿照 `scan_template.json` 建第二个模板包,等于为一个姿态复制整段对话,还会引入用户明确不想要的 flag 或配置键。落选方案:flag 选择的双模板包(违背本变更的初衷)、规则引擎(见决策 3)、user prompt(其承载的是每次运行的可变内容,而姿态是常量)。

2. **取其实质,不取其人设。**
   codex-plugin-cc 的对抗性 prompt 贡献其实质——攻击面优先级、四问实质性门槛、grounding、校准——但不搬 "break confidence" 的人设。人设适合用户主动发起的挑战式评审;作为日常默认会拿召回换噪音。校准小节("变更安全就明说")使合并后的姿态与既有的"避免评论正确代码"指令保持兼容。

3. **规则引擎不是这份清单的正确归属。**
   `system_rules.json` 的条目是按文件模式逐文件解析的评审清单;攻击面清单是全局姿态,不是模式匹配规则。放进规则引擎会把姿态与规则解析优先级(`--rule` > repo > user > system)纠缠在一起。

4. **plan 与 filter 阶段保持不动。**
   filter 本就只移除被事实证伪的评论,且有 protected-subjects 否决与"证据不足即放行"的偏向,更大胆的 grounded 发现天然能存活。plan 阈值保持不变,因为默认开启 plan 阶段会让每次评审都付出延迟,而合并后的 prompt 已交付同等收益。两者都会在冒烟中被度量(见下),把"应该没问题"变成数据。

5. **用内容存在性测试守住这些小节。**
   `template_test.go` 新增轻量断言:加载出的 `MAIN_TASK` system 内容包含三个新小节。现有测试只断言别处的占位符与非空内容(`TestLoadDefault_PlaceholdersPresent`、`TestLoadDefault_FieldsPopulated`),删掉这些小节后现有测试依然全绿——新断言补的就是这个缺口。

## Risks / Trade-offs

- [更大胆的发现抬高日常评审的评论量] → 校准小节从设计上封顶灌水;冒烟度量前后的评论量变化,让权衡被观察到而非被假设。
- [filter 可能吞掉站得住的缺失守卫型发现] → Ground A 移除的是"所述代码不在其 subject 文件 diff 中"的评论;缺失守卫型发现描述的代码**存在**(那个裸奔的调用点),理论上应存活——但这是推理不是实测。冒烟跟踪 filter 移除率;若上升幅度显著,以独立变更跟进 filter prompt 微调。
- [后续轮次为避免重复已确认发现而灌水] → 校准小节明确禁止后续轮次编造发现;冒烟专门检查第 2 轮质量,因为 `{{confirmed_comments}}` 回喂正是压力来源。
- [合并的小节被静默漂移或删除] → 上述内容存在性测试。

## Migration Plan

单一内嵌 prompt 变更:无配置迁移、无数据迁移、无发布开关。回滚即还原一个文件。发布说明注明 `ocr review` 默认采用对抗性姿态。合并前验证:`make check`、`make test`,外加行为冒烟——在含已知竞态/幂等问题的 fixture diff 上,于变更前后各跑一次 `ocr review`,对比发现质量、第 2 轮行为与 filter 移除率。
