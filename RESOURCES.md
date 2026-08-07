# OpenCodeReview Review Runtime Resources

## Knowledge

- [Review command orchestration](cmd/opencodereview/review_cmd.go)
  Primary source for loading context, resolving the LLM runtime, registering tools, constructing the agent, running it, and emitting the result.
- [Diff review agent](internal/agent/agent.go)
  Primary source for diff parsing, per-file dispatch, prompt placeholder substitution, plan phase, main loop, filtering, and coverage.
- [Shared LLM tool loop](internal/llmloop/loop.go)
  Primary source for the repeated completion → tool execution → tool result message cycle, task completion, failures, and context compression.
- [Tool declarations](internal/config/toolsconfig/tools.json)
  Source of the JSON schemas exposed to the model, including task_done, code_comment, file_read, code_search, and file_read_diff.
- [Prompt manifest](internal/config/template/task_template.json)
  Shows which prompt files and runtime limits form the review template.
- [Review prompts](internal/config/template/prompts/main_task_system.md)
  The system instructions that define review scope and tell the model when to use tools.
- [Tool registry construction](cmd/opencodereview/review_cmd.go)
  Shows the built-in Provider implementations that execute file_read, file_find, file_read_diff, code_search, and code_comment.
- [Session history and manifest](internal/session)
  Records prompts, model responses, tool calls, comments, checkpoints, and terminal coverage state.
- [Architecture decisions](docs/adr/0003-bounded-review-tool-and-token-budgets.md)
  Explains why the review loop has bounded tool rounds and token budgets.

## Wisdom (Communities)

- 暫無。這一階段先以 repository 的 primary source 和測試行為為準，之後再補 harness 對照與實務經驗。

## Gaps

- 尚未建立 harness 與 open-code-review 的逐欄位對照表。
- 尚未用一次實際 session log 驗證從 LLM response 到 persisted session_end 的完整序列。
