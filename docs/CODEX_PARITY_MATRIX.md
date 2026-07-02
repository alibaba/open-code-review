# Codex-Owned OCR Parity Matrix

状态：Phase 2–5 实现基线（2026-06-30）。

“对等”指确定性工程能力和使用场景对等，不承诺两个不同模型逐字生成相同评论。
模型质量仍需在固定 ground-truth corpus 上持续比较。
中性命令入口是 `ocr agent ...`；Codex 插件只是该确定性数据面的第一个适配层。

| 原生能力 | Codex-owned 实现 | 自动化证据 |
|---|---|---|
| workspace staged/unstaged/untracked | `ocr agent prepare` + native diff provider | `reviewbundle/prepare_test.go` |
| range merge-base / commit | immutable target resolution | `reviewbundle/target_test.go`, `prepare_test.go` |
| include/exclude/default filter | shared diff filter + native scan classifier | `reviewfilter/filter_test.go`, `scan/*_test.go` |
| layered rules | `rules.DetailResolver` + deduplicated rule table | `reviewbundle/prepare_test.go` |
| hunk/line metadata | raw patch + parsed hunks | `reviewbundle/prepare_test.go` |
| 大型 diff 分片 | `ocr agent prepare --split` + 确定性 manifest | `reviewbundle/prepare_test.go`, CLI tests |
| large-target planning contract | bundle reflection contract; Skill risk-plan gate | Skill static test |
| file read/find/diff/search | bundle-bound `ocr agent context` | `reviewbundle/context_test.go` |
| stale target detection | target/workspace/file hashes | `reviewbundle/validate_test.go`, `context_test.go` |
| line/hunk/content validation | `validate-comments` structured errors/warnings | `reviewbundle/validate_test.go` |
| suggestion safety | localization/consistency check only; no apply command | validator tests and Skill invariant |
| Markdown/text/JSON report | deterministic `ocr agent report` | `reviewbundle/report_test.go` |
| Git and non-Git scan | native scan provider | `reviewbundle/scan_test.go` |
| scan preview/filter/size/budget | manifest summaries and explicit skipped scope | `reviewbundle/scan_test.go`, CLI tests |
| none/language/directory batching | exported native grouping implementation | `scan/batch_test.go`, `reviewbundle/scan_test.go` |
| scan dedup/project summary | Codex second pass; Skill requires traceability and partial-scope disclosure | Skill static test |
| session/history | opt-in `--session-id`, correlated JSONL | `session/codex_test.go` |
| viewer | `codex-owned` records and unavailable-token semantics | `viewer/codex_test.go` |
| telemetry/trace facts | files, findings, warnings, partial, duration, context calls, validation | session tests |
| native OCR compatibility | `ocr review`/`ocr scan` code paths retained | full `go test ./...` |
| no OCR LLM in Codex path | agent commands do not load runtime/client/provider | source boundary scan |
| no source writes | only explicit bundle/report/session output writes | source boundary scan |

## Remaining release-quality benchmark

功能矩阵由自动化测试覆盖；模型输出质量需要固定 benchmark corpus、多次运行和人工
ground truth。发布门槛继续要求严重问题召回率不低于原生基线、误报率不高于原生
基线，并记录模型版本、运行次数、定位成功率和所有 partial failure。该指标不能由
单元测试伪造，也不能以“模型不同”为理由跳过。
