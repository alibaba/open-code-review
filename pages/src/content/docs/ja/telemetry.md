---
title: テレメトリ
sidebar:
  order: 11
---

OCR には第一級の **OpenTelemetry** サポートが付属しています。各レビュー実行は、構造化された span、
metric、event を生成します。collector に接続すれば、これらのデータは「agent は時間をどこに費やしたか？」、
「各モデルのコストはどうか？」、「なぜこの実行は失敗したか？」に答えるのに十分です。

## 概要

テレメトリは**デフォルトで無効**です。有効にすると、OCR は以下をエクスポートします：

- **Span**——3 つのパイプラインレベルの span（`review.run`、`diff.parse`、
  `subtask.execute.<file>`）に加え、各決定ポイントのイベントごとに短命な `event.*` span を 1 つ。
- **Metric**——レビュー所要時間、レビューされたファイル数、生成されたコメント数、LLM リクエスト / token / レイテンシ、
  ツール呼び出し / レイテンシの集約カウントとヒストグラム。
- **Event**——span 内の離散的なイベント。`plan.skipped`、
  `token.threshold.exceeded`、`review.started` など。

2 種類の exporter がサポートされています：

| Exporter | 使用する場面 |
|---|---|
| `console` | 個人利用 / デバッグ。span を整形して stdout に出力します。 |
| `otlp` | システム統合。任意の OTLP 互換 collector（Jaeger、Tempo、OTel Collector、Datadog Agent……）に送信します。 |

## テレメトリを有効にする

LLM エンドポイントと同様に、テレメトリは永続化された config または環境変数で設定できます——競合する場合は環境変数が優先されます。

### 設定ファイルによる方法

```bash
ocr config set telemetry.enabled        true
ocr config set telemetry.exporter       otlp
ocr config set telemetry.otlp_endpoint  localhost:4317
ocr config set telemetry.content_logging false
```

`~/.opencodereview/config.json` での結果：

```json
{
  "telemetry": {
    "enabled": true,
    "exporter": "otlp",
    "otlp_endpoint": "localhost:4317",
    "content_logging": false
  }
}
```

### 環境変数による方法

```bash
export OCR_ENABLE_TELEMETRY=1
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317   # implies exporter=otlp
export OTEL_EXPORTER_OTLP_PROTOCOL=grpc             # default. NOTE: only grpc is currently
                                                    # implemented; http/protobuf and http/json
                                                    # are accepted but not yet wired up.
export OTEL_SERVICE_NAME=open-code-review-prod      # optional; default: open-code-review
export OCR_CONTENT_LOGGING=0                        # reserved / currently a no-op (see Content logging)
```

`OTEL_EXPORTER_OTLP_ENDPOINT` を設定すると `exporter=otlp` も強制されます——一度きりの
`OTEL_EXPORTER_OTLP_ENDPOINT=… ocr review` 実行に適しています。

## 何がエクスポートされるか

### Span

1 回のレビューの完全な span ツリー：

```
review.run
├── diff.parse
├── event.review.started                   (decision-point event)
├── subtask.execute.<file1>
│   ├── event.plan.skipped                 (when changes are below threshold)
│   ├── event.plan.failed                  (when plan phase errored)
│   ├── event.token.threshold.exceeded     (when prompt > 80% of max_tokens)
│   └── event.subtask.error                (when the subtask errored)
├── subtask.execute.<file2>
└── …
```

LLM の往復とツールの実行は個別の span としては発行**されません**——metric（下記参照）にのみ現れます。
決定ポイントのイベントは、短命な `event.<name>` span として現在の context にアタッチされます。

各 span は有用な属性を保持します：

| Span | 主要な属性 |
|---|---|
| `review.run` | `error`（実行失敗時に設定） |
| `diff.parse` | `files.changed`、`lines.inserted`、`lines.deleted` |
| `subtask.execute.<file>` | `file.path`、`lines.changed`、`lines.inserted`、`lines.deleted` |
| `event.review.started` | `file.count`、`review.count`、`repo.dir` |
| `event.plan.skipped` | `file.path`、`lines.changed`、`threshold` |
| `event.plan.failed` | `file.path`、`message` |
| `event.token.threshold.exceeded` | `file.path`、`tokens`、`max_tokens` |
| `event.subtask.error` | `file.path`、`error` |

### Metric

OCR は OTel meter を通じて数値 metric を記録します——カウントとヒストグラムで、collector が下流で集約します：

| Metric | 種類 | 単位 | ラベル |
|---|---|---|---|
| `ocr.review.duration_seconds` | histogram | `s` | — |
| `ocr.files_reviewed_total` | counter | — | — |
| `ocr.comments_generated_total` | counter | — | — |
| `ocr.llm.requests_total` | counter | — | `model`、`status`（`ok` / `error`） |
| `ocr.llm.request_duration_seconds` | histogram | `s` | `model` |
| `ocr.llm.tokens_used` | counter | — | `model`、`type`（現在は常に `total`） |
| `ocr.tool.calls_total` | counter | — | `tool.name`、`status`（`ok` / `error`） |
| `ocr.tool.execution_duration_seconds` | histogram | `s` | `tool.name` |

### Event

イベントは決定ポイントで短命な `event.<name>` span としてトリガーされます。完全な一覧：

| イベント | 意味 |
|---|---|
| `review.started` | diff がロードされた。何ファイルをレビューするか判明している。 |
| `no.files.changed` | diff の解析で 0 ファイルとなった。 |
| `plan.skipped` | あるファイルが `PLAN_MODE_LINE_THRESHOLD` を下回った。 |
| `plan.failed` | plan フェーズでエラー。main ループは plan なしで実行される。 |
| `token.threshold.exceeded` | 初期 prompt token が `MAX_TOKENS` の 80 % を超えた。ファイルはスキップされる。 |
| `subtask.error` | あるファイルごとのサブタスクでエラー——`Error` span ステータスとして発行される。 |

これにより、ユーザーが気づく前に、レビュー品質の低下を早期に検出してアラートを出すことができます。

## コンテンツログ

テレメトリは LLM トラフィックの**形状**（カウント、所要時間、ステータス）をエクスポートしますが、実際の prompt や
レスポンスは**決して**エクスポートしません。OCR は LLM のメッセージ内容を span や event に付加しようとはしません——プロセスを離れるデータは上記に
記載された metric / event schema に限られ、それ以外は一切ありません。

`content_logging` の config key（および `OCR_CONTENT_LOGGING=1` の環境上書き）は設定レイヤーに接続されていますが、
現時点では prompt 内容を発出するいかなるコードパスも**制御しません**。このフラグは予約済みとみなしてください。

LLM に送信された、または LLM から返された内容を検査する必要がある場合は、[セッションビューア](../viewer/)が読み取るローカルの
JSONL トランスクリプトを使用してください。これらは完全に `~/.opencodereview/` 配下のディスク上に存在し、決して collector に送られません。

## レシピ

### ローカルデバッグ用の console exporter

```bash
ocr config set telemetry.enabled true
ocr config set telemetry.exporter console
ocr review --commit HEAD
```

span が人間可読な形式で stdout に出力されます。長い実行の出力を見るには `less` にパイプできます。

### OTel Collector + Tempo + Prometheus

```yaml
# otel-collector-config.yaml
receivers:
  otlp:
    protocols: { grpc: { endpoint: 0.0.0.0:4317 } }

exporters:
  otlp/tempo:
    endpoint: tempo:4317
    tls: { insecure: true }
  prometheus:
    endpoint: 0.0.0.0:9464

service:
  pipelines:
    traces:  { receivers: [otlp], exporters: [otlp/tempo] }
    metrics: { receivers: [otlp], exporters: [prometheus] }
```

その後、shell で：

```bash
export OCR_ENABLE_TELEMETRY=1
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317
ocr review --from main --to feature/branch
```

Tempo を開く → `service.name=open-code-review` で検索 → 任意の trace をクリックして完全な
span ツリーを表示。

### Datadog

Datadog Agent の OTLP receiver はデフォルトで OTLP/gRPC を使用します：

```bash
export OCR_ENABLE_TELEMETRY=1
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317
export OTEL_SERVICE_NAME=open-code-review
```

span はその service name で APM 配下に現れます。LLM metric は上記のラベル付きで Metrics 配下に現れます。

### CI 実行、結果をダッシュボードへ

パイプラインステップで環境変数を注入します：

```yaml
- name: Code review
  env:
    OCR_LLM_URL: ${{ secrets.OCR_LLM_URL }}
    OCR_LLM_TOKEN: ${{ secrets.OCR_LLM_TOKEN }}
    OCR_LLM_MODEL: claude-opus-4-6
    OCR_ENABLE_TELEMETRY: "1"
    OTEL_EXPORTER_OTLP_ENDPOINT: ${{ vars.OTEL_COLLECTOR_URL }}
    OTEL_SERVICE_NAME: open-code-review-ci
  run: ocr review --from origin/main --to HEAD --audience agent
```

`OTEL_SERVICE_NAME` により、CI の trace を手動の開発実行の trace と区別できます。

## 解決の優先順位

OCR が最終的なテレメトリ設定を構築する際：

1. デフォルト（`enabled=false`、`exporter=console`、endpoint なし）。
2. `~/.opencodereview/config.json` の `telemetry.*` key。
3. 環境変数（最高優先度、ファイルを**上書き**）。

したがって、config では `telemetry.enabled=false` を保持したまま、実行ごとに
`OCR_ENABLE_TELEMETRY=1` で有効にできます。

## サンプリングとオーバーヘッド

OCR は**すべて**をエクスポートします。サンプリングの設定はありません。OTel のサンプリングは collector の責務です。典型的なレビュー
実行の場合：

- 1 つの `review.run` span + 1 つの `diff.parse` span + レビューされたファイルごとに 1 つの
  `subtask.execute.<file>` span + 各決定ポイントのイベントごとに 1 つの短命な `event.*` span。
- 10 ファイルの PR で合計およそ 15〜25 個の span。LLM の往復とツール呼び出しは metric のカウントを増やしますが、
  追加の span は作成しません。

エクスポートは**バッチ処理かつ非同期**です——テレメトリはレビューループをブロックしません。collector に到達できない場合、OCR は警告を記録して
続行します。レビューは引き続き通常の出力を生成します。

## トラブルシューティング

| 症状 | 考えられる原因 |
|---|---|
| 何もエクスポートされない | `OCR_ENABLE_TELEMETRY` / `telemetry.enabled` が未設定。デフォルトは**無効**。 |
| OTLP がローカルでは動くが本番で失敗する | OCR は現在 OTLP/gRPC のみを実装している——`OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf`（または `http/json`）は受理されるが接続されておらず、切り替えても効果はない。endpoint と、collector が gRPC をリッスンしているかを確認すること。 |
| span は表示されるが metric がない | 一部の collector はデフォルトで traces pipeline のみを有効にする。設定に `metrics` pipeline を追加すること。 |
| span に prompt がない | OCR は prompt の内容をテレメトリに付加することは決してない——[コンテンツログ](#content-logging)を参照。代わりに[セッションビューア](../viewer/)を使ってトランスクリプトを検査すること。 |

## 関連項目

- [設定](../configuration/)——`telemetry.*` 名前空間の完全な key リファレンス。
- [アーキテクチャ](../architecture/)——各 span が実際に何を計測するか。
- [OpenTelemetry ドキュメント](https://opentelemetry.io/docs/)——collector のセットアップと exporter。
