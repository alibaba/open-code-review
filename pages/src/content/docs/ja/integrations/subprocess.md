---
title: Direct Subprocess
sidebar:
  order: 3
---

shell を通じて `ocr` を呼び出し、JSON を解析します。これは最も低レベルの統合経路です——本サイトの他の方法はすべて最終的にこれに帰着します。[Agent Skill](../agent-skill/) と [Command](../claude-code/) の方法は、呼び出し側の agent にこの作業を行わせるための prompt テンプレートです。[CI/CD](../ci/) のレシピは、スクリプトから同じことを行う GitHub Actions と GitLab CI のパイプラインです——agent のオーケストレーションは関与せず、サブプロセス呼び出し、JSON 解析、コメントの PR / MR への貼り戻しのみです。カスタムスクリプト、LangChain ツール、その他まだカバーされていないフレームワークから OCR を呼び出す場合は、このページを直接使ってください。

## Bash

```bash
result=$(ocr review --format json --audience agent)
status=$(echo "$result" | jq -r '.status')
total=$(echo "$result" | jq '.comments | length')
echo "Status: $status — $total comments"
echo "$result" | jq -r '.comments[] | "\(.path):\(.start_line) — \(.content)"'
```

## Python

```python
import json, subprocess

proc = subprocess.run(
    ["ocr", "review", "--format", "json", "--audience", "agent",
     "--from", "origin/main", "--to", "HEAD",
     "--background", pr_description],
    capture_output=True, text=True, check=True,
)
data = json.loads(proc.stdout)
for c in data["comments"]:
    if c["start_line"] > 0:
        post_line_comment(c["path"], c["start_line"], c["content"])
```

## JSON の構造

OCR は単一のトップレベル**オブジェクト**を出力します（裸の配列ではありません）。以下は、1 件の発見を含む完全な `success` の外殻です。

```json
{
  "status": "success",
  "summary": {
    "files_reviewed": 1,
    "comments": 1,
    "total_tokens": 12770,
    "input_tokens": 12450,
    "output_tokens": 320,
    "elapsed": "9s"
  },
  "comments": [
    {
      "path": "internal/cache/store.go",
      "content": "Concurrent map access without a lock — wrap reads and writes with `sync.RWMutex` to avoid a race on the shared cache.",
      "start_line": 42,
      "end_line": 47,
      "existing_code": "func (s *Store) Get(k string) string {\n    return s.m[k]\n}",
      "suggestion_code": "func (s *Store) Get(k string) string {\n    s.mu.RLock()\n    defer s.mu.RUnlock()\n    return s.m[k]\n}",
      "thinking": "The struct exposes `m map[string]string` without a guarding mutex, and Get/Set are called from concurrent request handlers."
    }
  ]
}
```

### トップレベルのフィールド

| フィールド | 型 | 常に存在 | 説明 |
|---|---|---|---|
| `status` | string | はい | `success`、`completed_with_warnings`、`completed_with_errors`、`skipped` のいずれか。 |
| `message` | string | いいえ | 短い人間可読のサマリー。空の実行やスキップ時に設定されます（例：`"No comments generated. Looks good to me."`）。 |
| `summary` | object | いいえ | 実行の集計。完了した実行時に存在し、`skipped` 時は省略されます。フィールドは下記を参照。 |
| `comments` | array | はい | 空の場合があります。各コメントのスキーマは下記を参照。 |
| `warnings` | array | いいえ | 1 つ以上のサブ agent が失敗またはスキップされた場合にのみ存在します。スキーマは下記を参照。 |

### summary の構造（`summary`）

| フィールド | 型 | 説明 |
|---|---|---|
| `files_reviewed` | int | すべてのフィルタを通過し、モデルに送られたファイル数。 |
| `comments` | int | すべてのファイルにわたって出力されたコメントの総数（`comments.length` と一致）。 |
| `total_tokens` | int | 実行中の各 LLM 呼び出しの prompt + completion token の合計。 |
| `input_tokens` | int | 各 LLM 呼び出しの prompt token（キャッシュ読み取り token を含む）。 |
| `output_tokens` | int | 各 LLM 呼び出しの completion token（キャッシュ書き込み token を含む）。 |
| `cache_read_tokens` | int | 各 LLM 呼び出しのキャッシュ読み取り token の総数。ゼロの場合は省略されます（`omitempty`）。 |
| `cache_write_tokens` | int | 各 LLM 呼び出しのキャッシュ書き込み token の総数。ゼロの場合は省略されます（`omitempty`）。 |
| `elapsed` | string | 経過時間（実時間）。秒単位に丸められ、Go の `time.Duration.String()` によってフォーマットされます（例：`"1m12s"`）。 |

### 各コメントのフィールド（`comments[]`）

| フィールド | 型 | 常に存在 | 説明 |
|---|---|---|---|
| `path` | string | はい | リポジトリ相対のファイルパス。 |
| `content` | string | はい | レビューコメント（Markdown）。 |
| `start_line` | int | はい | 影響範囲の先頭行。値が `< 1` の場合、コメントに行アンカーが無いこと（ファイルレベル）を意味します——これらはインラインで貼り付けようとせず、サマリーにまとめるべきです。 |
| `end_line` | int | はい | 影響範囲の末尾行。単一行コメントの場合は `start_line` と等しくなります。 |
| `existing_code` | string | いいえ | 置き換え対象の元のコード片。diff を伴わない提案的コメントの場合は省略されます。 |
| `suggestion_code` | string | いいえ | `existing_code` の提案された置き換え。存在する場合は常に `existing_code` とペアになります。 |
| `thinking` | string | いいえ | モデルの推論の軌跡。分類やデバッグに有用です。ユーザーに表示する前に安全に破棄できます。 |

### warnings の構造（`warnings[]`）

スキップまたは一部のファイルが失敗した実行は、次のような形になります。

```json
{
  "status": "completed_with_errors",
  "message": "Some files could not be reviewed due to errors.",
  "comments": [],
  "warnings": [
    {
      "file": "src/very_long_file.go",
      "message": "diff size exceeds 80% of MAX_TOKENS; skipped",
      "type": "token_threshold_exceeded"
    },
    {
      "file": "src/broken.py",
      "message": "sub-agent failed: context deadline exceeded",
      "type": "subtask_error"
    }
  ]
}
```

| フィールド | 型 | 説明 |
|---|---|---|
| `file` | string | 警告を引き起こしたファイルのリポジトリ相対パス。 |
| `message` | string | 短い人間可読の説明。 |
| `type` | string | フィルタリング用の安定した型。現在出力されるもの：`subtask_error`（サブ agent の実行失敗）と `token_threshold_exceeded`（diff がモデルにとって大きすぎる）。 |

`warnings` が少なくとも 1 つの `subtask_error` を含む場合、`status` は
`completed_with_errors` になります。そうでない場合は `completed_with_warnings` です。

### severity / priority フィールドは無い

OCR は `severity` や `priority` フィールドを**出力しません**。[Agent Skill](../agent-skill/)
と [Command](../claude-code/) のドキュメントで見られる High/Medium/Low の分類は、呼び出し側の agent が生のコメントを受け取った後に追加したものです——`jq '.comments[].severity'` を試みないでください。それは存在しません。

## 空の結果の扱い

**対象となるファイルが無い**作業領域は `status` で報告されるため、呼び出し側は「変更なし」と「発見なし」を区別できます。

```json
{
  "status": "skipped",
  "message": "No supported files changed.",
  "comments": []
}
```

「すべてクリーン」と判断する前に、必ず `status == "skipped"` を確認してください。

## 関連項目

- [CI/CD](../ci/)——サブプロセス呼び出しの上に構築された、すぐ使える GitHub Actions と pre-commit のレシピ。
- [Agent Skill](../agent-skill/)——呼び出し側が通常のスクリプトではなく Anthropic SDK agent の場合。
