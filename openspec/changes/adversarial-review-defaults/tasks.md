# Tasks

## 1. Prompt 合并

- [ ] 1.1 编辑 `internal/config/template/prompts/main_task_system.md`,在现有 Capabilities 小节之后新增三个小节(内容依据 design 决策 2):**Review Priorities**(spec 中的失效类别清单)、**Finding Bar**(仅实质性发现、四问、以 diff 与工具输出为依据——不虚构文件、行号或代码路径)、**Calibration**(强发现优先、安全即零发现、后续轮次不灌水)。既有的 Role、Capabilities、Strict Focus Rules、Reply limit 文本逐字节保留。验证:`make check` 通过,且 diff 审查确认未修改或删除任何既有行。

## 2. 内容守卫测试

- [ ] 2.1 在 `internal/config/template/template_test.go` 新增测试,断言 `LoadDefault()` 返回的 `MAIN_TASK` system 消息包含三个新小节标题。验证:`make test` 在小节存在时通过;从 prompt 文件删掉任一标题后该测试必须失败。

## 3. 行为冒烟与集成门禁

- [ ] 3.1 构建一个 fixture diff,含七个失效类别中三类的已知问题(一个竞态条件、一个幂等缺口、一个缺失 nil 判空)外加一个纯格式改动;在 prompt 变更还原与生效两种状态下各跑一次 `ocr review`,逐次记录:哪些失效类别产出了发现、纯格式改动是否招致实质性发现、filter 移除计数、第 2 轮发现质量。验证:合并后失效类问题被找到;纯格式改动不产生实质性发现;第 2 轮不灌水;filter 移除率未上升到会吞掉有效缺失守卫型发现的程度(显著上升则阻塞合并,并按 design 产生后续 filter 变更)。

- [ ] 3.2 执行 AGENTS.md 规定的全部提交前门禁:`make test`、`make check`,以及对工作区跑 `ocr review --audience agent --background "briefly summarize the background requirements"` 自审。验证:全部命令通过;自审未在本变更上产生 critical 或 high 发现;PR 描述携带 AI/LLM 使用披露,commit message 为英文。
