# Spec Delta

## Purpose

定义 `ocr review` 的对抗性评审 pass:每个文件组在标准评审完成后默认获得一次独立的对抗性会话,挑战已被接受的判断并报告标准轮次遗漏的问题;其产出经同一过滤通道,且任何失败都不影响该组的评审结果。

## ADDED Requirements

### Requirement: 默认流水线包含对抗性评审 pass

每个文件组的标准评审成功完成后,除尽力而为契约列出的跳过条件外,`ocr review` 应(SHALL)默认为该组运行恰好一次独立的对抗性会话,不得要求任何命令行 flag 或配置键。对抗性会话应(SHALL)接收与主任务相同的评审上下文(评审规则、组外变更文件、组内 diff、需求背景)以及标准轮次已确认的发现清单作为勿重复上下文;其 prompt 不得依赖 plan 阶段的输出。

#### Scenario: 裸调用即携带该 pass

- **WHEN** 用户不带任何与对抗性评审相关的 flag 运行 `ocr review`
- **THEN** 每个完成标准评审的文件组都运行一次对抗性会话

#### Scenario: preview 不运行该 pass

- **WHEN** 用户运行 `ocr review --preview`
- **THEN** 不运行对抗性会话(preview 不执行任何 LLM 调用)

#### Scenario: 标准评审未完成的组不运行

- **WHEN** 某文件组的标准评审因停止条件(轮数上限、token 预算、错误)未能完成,或该组从未被派发(派发前被预算或超时截断)
- **THEN** 该组不运行对抗性会话

#### Scenario: 标准轮次零发现时 pass 仍运行

- **WHEN** 某组的标准评审完成且确认清单为空(未产出任何确认发现,包括提交的评论全部被过滤的情况)
- **THEN** 该组仍运行对抗性会话,且其 prompt 省略"已确认发现"段

### Requirement: 对抗性发现经同一过滤通道

对抗性会话应(SHALL)通过与主任务相同的 `code_comment` 通道提交评论。`ocr review` 应(SHALL)在对抗性会话结束后对该 pass 新增的评论运行 review filter,且不得将标准轮次已过滤过的评论重复送审。

#### Scenario: 新增评论被过滤

- **WHEN** 对抗性 pass 提交的评论被 review filter 判定为被 diff 证伪
- **THEN** 该评论不出现在最终评审结果中

#### Scenario: 标准轮次评论不重复送审

- **WHEN** 对抗性 pass 结束并运行 filter
- **THEN** 仅标准轮次结束之后新增的评论被送审

### Requirement: 对抗性评论遵守相同的范围规则

对抗性 pass 不得放松既有的文件范围约束:其提交的评论应(SHALL)只针对当前评审组内的文件,评论输出结构(severity、category、行定位)与主任务的评论保持一致。`ocr scan` 流水线不得受该 pass 影响。

#### Scenario: 评论只针对组内文件

- **WHEN** 对抗性会话对当前评审组之外的文件形成了判断
- **THEN** 它不提交针对该文件的评论

#### Scenario: scan 不受影响

- **WHEN** 用户运行 `ocr scan`
- **THEN** scan 使用独立的 scan 模板,不运行对抗性 pass

### Requirement: pass 尽力而为且不影响组状态

对抗性 pass 应(SHALL)为尽力而为:聚合 token 预算耗尽、确认发现达到上限、渲染后 prompt 超出预算、LLM 错误或对话中途停止时,运行不得改变该组的完成状态、失败分类或退出码,且跳过或失败原因应(SHALL)可通过运行日志或 telemetry 观察。模板未配置该会话时 pass 静默不运行(默认模板恒配置该会话)。

#### Scenario: 会话失败保留标准发现

- **WHEN** 某组的对抗性会话返回错误
- **THEN** 该组保持完成状态,标准评审的发现全部保留,并记录一条 warning

#### Scenario: 聚合预算耗尽时跳过

- **WHEN** 轮到某组运行对抗性 pass 时聚合 token 预算已耗尽
- **THEN** 跳过该组的对抗性 pass 并记录跳过原因,组状态不变

### Requirement: 空结果合法

对抗性会话在未发现可报告问题时应(SHALL)能够以完成信号(`task_done`)正常结束;零新增发现是该 pass 的合法结果,不得被视为失败。

#### Scenario: 代码通过挑战

- **WHEN** 对抗性会话未发现可报告的新问题并以 `task_done` 结束
- **THEN** 该 pass 以零新增发现正常完成

### Requirement: 输出语言与会话归属

对抗性 pass 产出的评论应(SHALL)遵循配置的输出语言,与主任务的评论一致。其 LLM 请求与会话记录应(SHALL)以 `adversarial_task` 类型单独分桶,与主任务的请求区分开。

#### Scenario: 输出语言一致

- **WHEN** 用户配置中文输出并运行 `ocr review`
- **THEN** 对抗性 pass 产出的评论以中文撰写

#### Scenario: 会话记录分桶

- **WHEN** 查看某组的会话记录或 retry 报表
- **THEN** 对抗性 pass 的请求归属于 adversarial_task 类型,可与其他阶段的请求区分
