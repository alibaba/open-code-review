# Proposal

## Why

`ocr review` 的默认 prompt 是中立描述型的:它评审改了什么,但不会主动猎取那些代价高、危险或难以察觉的失效类别(认证与信任边界、数据丢失与不可逆状态、竞态与幂等缺口、依赖降级、迁移隐患、可观测性盲区)。挑战式评审姿态——如 openai/codex-plugin-cc 的 adversarial review 所实现的——正是为猎取这些类别而设计;其实质内容(按优先级排序的攻击面清单、带 grounding 约束的发现实质性门槛、防灌水的校准规则)与 ocr 的客观语气及其 grounded 移除式 filter 相互兼容。把这份实质并入默认 prompt,意味着每一次评审都会主动猎取这些失效类别——不新增 flag、不新增成本旋钮、不改输出契约。

## What Changes

- 把对抗性评审的实质内容并入默认评审 system prompt(`internal/config/template/prompts/main_task_system.md`),新增三个小节:
  - **Review Priorities** — 明确要求评审 agent 主动探查的失效类别清单(认证/权限/租户隔离/信任边界;数据丢失、损坏与不可逆状态;回滚安全、重试、部分失败与幂等;竞态条件、顺序假设与过期状态;空态、null、超时与依赖降级;版本漂移、schema 漂移与迁移隐患;掩盖失效的可观测性缺口)。
  - **Finding Bar** — 只报实质性发现;每条须回答:会出什么事、为什么这条代码路径脆弱、可能的影响是什么、能降低风险的具体改动;且必须以已评审的 diff 或工具输出为依据,不得虚构文件、行号或代码路径。
  - **Calibration** — 宁要一条强发现不要多条弱发现;变更确实安全就明说(零发现也是合法结果);后续评审轮次不得为凑数而编造发现。
- 保持现有客观中立语气、Strict Focus Rules、上下文工具循环、`code_comment` 输出结构(severity/category)、plan 阈值与 filter 阶段不变。
- CLI 面零变化:不新增 flag、不新增配置键,该姿态默认作用于每一次 `ocr review`。
- 明确排除:`ocr scan` 模板、规则引擎(`system_rules.json`)、强制开启 plan 阶段、review filter 的改动,以及 ROADMAP 的 "Ultra Mode"(那意味着预算/轮数扩展,仍是独立的规划项)。

## Capabilities

### New Capabilities

- `review-prompting`:`ocr review` 的默认评审姿态——评审 agent 必须主动猎取哪些失效类别、每条发现必须满足的实质性门槛、以及保证多轮评审不灌水的校准规则。

### Modified Capabilities

(无——这是项目的第一个 spec)

## Impact

- **代码**:`internal/config/template/prompts/main_task_system.md`(经 `go:embed` 内嵌;`task_template.json` 清单不变)。在 `internal/config/template/template_test.go` 增加轻量内容存在性断言,防止小节被意外删除。
- **行为**:每次 `ocr review`(`--audience agent` 与 `--audience human` 皆然)都携带新姿态;每个评审组约增加 300–400 prompt token(实现时实测确认),相对 200,000 的 `MAX_TOKENS` 预算可忽略。
- **不受影响**:`ocr scan`(独立 `scan_template.json`)、delegate 模式、分组/plan 阈值、filter 的 grounded 移除判据、评论输出 schema、插件 SKILL.md 文档、全部 CLI flag。
- **流程**:按 AGENTS.md,PR 须披露 AI/LLM 使用、commit message 用英文、`make check` 与 `make test` 通过,并在 fixture diff 上做行为前后冒烟,验证更强的发现能在 filter 中存活。
