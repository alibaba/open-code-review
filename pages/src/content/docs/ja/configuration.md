---
title: 設定
sidebar:
  order: 5
---

設定ファイルは `~/.opencodereview/config.json` にあります。編集方法は 3 つあります。

- **インタラクティブ TUI** —— `ocr config provider` / `ocr config model`。ガイド付きメニューが表示されます。
- **コマンドライン** —— `ocr config set <key> <value>`。スクリプトや CI に適しています。
- **手動編集（非推奨）** —— この JSON ファイルを直接編集（次回の `ocr config set` 書き込み時に再フォーマットされます）。

## モデルを設定する

### 推奨：インタラクティブ設定

```bash
ocr config provider
```

組み込みまたはカスタムの provider を選択し、API key を入力し、model を選び、すべてを設定ファイルに保存したうえで、`ocr llm test` を 1 回実行してエンドポイントを検証します。あとで model を切り替えるには：

```bash
ocr config model
```

### 非インタラクティブ設定（CI / TUI なし環境）

`ocr config set` で同じ設定に書き込みます。

```bash
ocr config set provider                    anthropic
ocr config set model                       claude-opus-4-6
ocr config set providers.anthropic.api_key sk-ant-xxxxxxxxxx
```

### 組み込み provider

以下の provider が OCR に同梱されており、Base URL とプロトコルがプリセット
されています——選択後は API key を入力するだけです。`providers.<name>.api_key`
が未設定の場合は、対応する環境変数に自動的にフォールバックします。

| 名称 | プロトコル | Base URL | API key 環境変数 |
|---|---|---|---|
| `anthropic` | anthropic | `https://api.anthropic.com` | `ANTHROPIC_API_KEY` |
| `openai` | openai | `https://api.openai.com/v1` | `OPENAI_API_KEY` |
| `dashscope` | openai | `https://dashscope.aliyuncs.com/compatible-mode/v1` | `DASHSCOPE_API_KEY` |
| `dashscope-tokenplan` | openai | `https://token-plan.cn-beijing.maas.aliyuncs.com/compatible-mode/v1` | `DASHSCOPE_TOKENPLAN_KEY` |
| `volcengine` | openai | `https://ark.cn-beijing.volces.com/api/v3` | `ARK_API_KEY` |
| `deepseek` | openai | `https://api.deepseek.com` | `DEEPSEEK_API_KEY` |
| `tencent-tokenhub` | openai | `https://tokenhub.tencentmaas.com/v1` | `TENCENT_TOKENHUB_API_KEY` |
| `hy-tokenplan` | openai | `https://api.lkeap.cloud.tencent.com/plan/v3` | `TENCENT_HUNYUAN_TOKENPLAN_KEY` |
| `iflytek` | openai | `https://spark-api-open.xf-yun.com/v1` | `SPARK_API_KEY` |
| `kimi` | openai | `https://api.moonshot.cn/v1` | `MOONSHOT_API_KEY` |
| `z-ai` | openai | `https://open.bigmodel.cn/api/paas/v4` | `Z_AI_API_KEY` |
| `mimo` | openai | `https://api.xiaomimimo.com/v1` | `MIMO_API_KEY` |
| `minimax` | openai | `https://api.minimax.io/v1` | `MINIMAX_GLOBAL_API_KEY` |
| `minimax-cn` | openai | `https://api.minimaxi.com/v1` | `MINIMAX_API_KEY` |
| `baidu-qianfan` | openai | `https://qianfan.baidubce.com/v2` | `QIANFAN_API_KEY` |
| `siliconflow`  | openai | `https://api.siliconflow.com/v1` | `SILICONFLOW_GLOBAL_API_KEY` |
| `siliconflow-cn`  | openai | `https://api.siliconflow.cn/v1` | `SILICONFLOW_API_KEY` |
| `novita` | openai | `https://api.novita.ai/openai` | `NOVITA_API_KEY` |

### カスタム provider

上記の表にない provider 名はすべてカスタムとみなされ、少なくとも `url` と
`protocol` を指定する必要があります（`protocol` は `anthropic`、`openai`、
または `openai-responses`）。

```bash
ocr config set provider                             my-gateway
ocr config set custom_providers.my-gateway.url      https://gateway.internal.com/v1
ocr config set custom_providers.my-gateway.protocol openai
ocr config set custom_providers.my-gateway.model    llama-3-70b
ocr config set custom_providers.my-gateway.api_key  "$MY_API_KEY"
```

provider またはモデルが OpenAI Responses API（`/v1/responses`）を必要とする場合は、
`openai-responses` プロトコルを使用します。

```bash
ocr config set provider                                               openai-responses-gateway
ocr config set custom_providers.openai-responses-gateway.url          https://api.openai.com/v1
ocr config set custom_providers.openai-responses-gateway.protocol     openai-responses
ocr config set custom_providers.openai-responses-gateway.model        gpt-5
ocr config set custom_providers.openai-responses-gateway.api_key      "$OPENAI_API_KEY"
```

`url` には API の Base URL または完全な `/responses` エンドポイントのどちらを指定してもよく、OCR がどちらの形式も正規化します。

Ollama で動かすローカルモデルは、ローカルの OpenAI 互換エンドポイントを
指すカスタム provider にすぎません。

```bash
ocr config set provider                          ollama
ocr config set custom_providers.ollama.url       http://127.0.0.1:11434/v1
ocr config set custom_providers.ollama.protocol  openai
ocr config set custom_providers.ollama.model     qwen3:32b
ocr config set custom_providers.ollama.api_key   ollama
```

Ollama は API key を無視しますが、カスタム provider は空でない `api_key` を
必要とします（カスタム provider には環境変数のフォールバックがありません）。
そのため任意のプレースホルダー値を設定してください。モデル自体はネイティブな
ツール呼び出しをサポートしている必要があります——選ぶ前に FAQ の
["No tool calls parsed"（ローカルモデル / Ollama）](../faq/#no-tool-calls-parsed-ollama)を
参照してください。

### タイムアウト（Timeouts）

各 LLM リクエストには HTTP タイムアウトがあり、デフォルトは **300 秒**です。
遅いローカルモデル（あるいは大きなファイル）では、それ以上の時間が必要になることがあります。
スコープの狭い順に、3 つの設定があります。

- `providers.<name>.timeout_sec` / `custom_providers.<name>.timeout_sec`
  ——provider ごと、秒単位。
- `llm.timeout_sec`——レガシーな `llm` セクション用、秒単位。
- `OCR_LLM_TIMEOUT` 環境変数——整数（秒単位）。すべての解決パスで設定ファイルの
  値を上書きします。

`timeout_sec` key は `ocr config set` ではサポートされていません——
`~/.opencodereview/config.json` を直接編集してください。

```json
{
  "custom_providers": {
    "ollama": { "url": "http://127.0.0.1:11434/v1", "protocol": "openai", "timeout_sec": 900 }
  }
}
```

### 追加のリトライ対象ステータスコード

一部の LLM プロバイダーでは、レート制限に対して `403` や `400` を返すなど、
一時的なエラーを標準外の 4xx ステータスコードで表すことがあります。
`retry_codes` を使うと、OCR はこれらのリクエストに対して既存の SDK の
リトライ機構を使用します。

`retry_codes` は整数の配列です。`llm.retry_codes` または
`custom_providers.<name>.retry_codes` に設定できます。`ocr config set` では、
コードをカンマ区切りで指定します。

```bash
ocr config set llm.retry_codes 403,400
ocr config set custom_providers.my-gateway.retry_codes 403,400
```

指定できるのは 4xx の HTTP ステータスコードだけです。`408`、`409`、`429` は
SDK がすでにリトライするため、設定ファイルから読み込む際には無視されます。
`ocr config set` で指定した場合は、OCR が警告を出し、これらのコードを保存しません。
5xx のレスポンスも SDK がデフォルトでリトライするため、`retry_codes` には追加できません。

### ファイルごとのプロンプト上限

OCR はデフォルトで、ファイルごとのレビューにおいて 58,888 トークンのプロンプト上限を
使用します。より大きなコンテキストウィンドウを持つモデル向けに、`max_tokens` を
保存して上限を引き上げられます。

```bash
ocr config set max_tokens 200000
```

この設定は `ocr review` と `ocr scan` の両方に適用されます。保存済みの設定を
変更せずに一度だけ上書きするには `--max-tokens` を使用します。

```bash
ocr review --max-tokens 200000
ocr scan --max-tokens 200000
```

実行時のフラグは `max_tokens` より優先されます。どちらも設定されていない場合、
OCR は組み込みのタスクテンプレートのデフォルト値を使用します。この上限はファイル
単位であり、モデルの出力トークン上限、および実行全体のトークン使用量を制限する
`--max-tokens-budget` のいずれとも独立しています。組み込みのデフォルトに戻すには
`ocr config unset max_tokens` を実行してください。

### 接続性を検証する

```bash
ocr llm test
```

### 既存の環境変数を再利用する

Claude Code の `ANTHROPIC_*` や OCR 独自の `OCR_LLM_*` 環境変数をすでに
設定している場合、OCR はそれらを自動的に認識するため、設定ファイルを書く
必要はありません。

### CC-Switch を使う

[CC-Switch](https://github.com/farion1231/cc-switch) を
[ルーティングサービス](https://www.ccswitch.io/en/docs?section=proxy&item=service)
有効で使用している場合、プロバイダーの `url` をローカルプロキシに向けるだけで、
追加設定なしで利用できます：

```bash
# Claude（Anthropic 互換）
ocr config set providers.anthropic.url http://127.0.0.1:15721

# Codex / OpenAI 互換 — そのプロバイダーの url キーを設定
ocr config set providers.<name>.url http://127.0.0.1:15721/v1
```

`api_key` は任意の値で構いません。`extra_body`（およびその他のプロバイダー固有フィールド）は
引き続き有効です。

### ベンダー固有のフィールドを送信する

一部の provider は非標準のリクエストフィールド（Bedrock 風の `thinking` など）を
必要とします。`extra_body`（各リクエストにマージされます）を使えば、ソースコードを
変更せずにそれらを送信できます。

```bash
ocr config set providers.anthropic.extra_body '{"thinking":{"type":"disabled"}}'
```

### プロンプトキャッシュのセッションアフィニティ

OCR はすべての LLM 会話ごとに、レビューセッションとその中のタスクにスコープされた
プロンプトキャッシュ・アフィニティキー（`<セッションID>-<タスク種別>-<スコープハッシュ>`）を
導出します。プロンプトキャッシュはプレフィックス単位でマッチするため、会話ごとのキーは、
実行全体を 1 つのホットキーに集中させず、成長していく各会話（例：ファイルごとの
レビューツールループ）を一貫したキャッシュノードに保ちます。キーのセッション ID
プレフィックスにより、プロバイダー側のキャッシュログを `ocr session` の記録と照合できます。

利用するには、プロバイダーがキーを期待する場所の `extra_headers` または `extra_body` の
値に `{ocr_session_key}` テンプレート変数を埋め込みます。OCR がリクエストごとにその会話の
キーに置換し、設定がなければ何も送信しません。

```bash
# OpenAI: prompt_cache_key リクエストボディフィールド
ocr config set providers.openai.extra_body '{"prompt_cache_key": "{ocr_session_key}"}'

# ヘッダーでルーティングするゲートウェイ（Cloudflare、Fireworks、Mistral は通常 x-session-affinity を使用）
ocr config set custom_providers.my-gateway.extra_headers "x-session-affinity={ocr_session_key}"
```

## レビュー言語を設定する

`language` はレビューコメントをどの言語で出力するかを決めます。未設定の場合は
デフォルトで英語になります。

```bash
ocr config set language 中文
ocr config set language English
```

## 関連項目

- [クイックスタート](../quickstart/)——最小限のセットアップと初回のレビュー。
- [CLI リファレンス](../cli-reference/)——review コマンドが受け入れる各引数。
