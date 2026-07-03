---
title: 設定
sidebar:
  order: 5
---

## エンドポイントの解決

`ocr review` または `ocr llm test` が実行されると、4 つの来源を順に試し、
完全な `(URL, token, model)` の三つ組を最初に返せた来源を使用します。

| 優先度 | 来源 | 読み取る内容 |
|---|---|---|
| 1 | `~/.opencodereview/config.json` | `provider` が設定されている場合は `providers`/`custom_providers` マッピングを通じて解決します（provider が優先。[組み込み provider](#built-in-providers) を参照）。provider が設定されていない場合にのみ、レガシーの `llm` セクションにフォールバックします。 |
| 2 | OCR 環境変数 | `OCR_LLM_URL`、`OCR_LLM_TOKEN`、`OCR_LLM_MODEL`、`OCR_USE_ANTHROPIC`、`OCR_LLM_AUTH_HEADER`。 |
| 3 | Claude Code 環境変数 | `ANTHROPIC_BASE_URL`、`ANTHROPIC_AUTH_TOKEN`、`ANTHROPIC_MODEL`。 |
| 4 | シェルの rc ファイル | `~/.zshrc`、`~/.bashrc`、`~/.bash_profile`、`~/.profile` から解析された `export ANTHROPIC_*=…` 行。 |

Claude Code 風の来源については、`ANTHROPIC_BASE_URL` にバージョン付きのパス
（`/v1/...`）がない場合、OCR は自動的に `/v1/messages` を追加します。

いずれの戦略でも完全な三つ組が得られない場合、OCR は次のメッセージで終了します。

```
no valid LLM endpoint configured; one of OCR_LLM_URL/OCR_LLM_TOKEN/OCR_LLM_MODEL,
~/.opencodereview/config.json, or ANTHROPIC_BASE_URL/ANTHROPIC_AUTH_TOKEN/
ANTHROPIC_MODEL must be set
```

> 解決は、単に最初に空だった来源ではなく、最初に**エラーとなった**来源で停止します。特に注意してください。
> `config.json` に `provider` が設定されているのにその項目の設定が誤っている場合（不明な provider 名、
> 環境変数のフォールバックもなく `api_key` が欠落、`model` が欠落、カスタム provider の
> `url`/`protocol` が欠落）、OCR はそのエラーで終了し、OCR 環境変数、
> Claude Code、rc ファイルの来源にフォールバックすることは**ありません**。環境変数ベースの設定に切り替えるには、まず
> `provider` key を解除してください。

> 来源の優先順位により、設定ファイルが完全に埋まっている場合、**環境変数はいかなる値も上書きしません**。
> 環境変数を有効にするには、`~/.opencodereview/config.json` から該当する `llm.*` key を削除するか、
> `ocr config set` で新しい値に切り替えてください。

## `ocr config set` ——`~/.opencodereview/config.json` を管理する

```bash
ocr config set <key> <value>
```

`config set` は key/value ペアでファイルを変更し、schema を認識した解析を行います。インタラクティブな TUI
コマンド `ocr config provider` と `ocr config model` も同じファイルに書き込みます（
[インタラクティブ設定](#interactive-setup--ocr-config-provider--ocr-config-model) を参照）。認識される
key：

| Key | 型 | 説明 |
|---|---|---|
| `provider` | string | 現在の provider を設定します（組み込み名またはカスタム）。provider を切り替えると model がクリアされます。 |
| `model` | string | 現在の provider に model を設定します（provider 項目の下に保存。provider がない場合はトップレベルの `model` に保存）。 |
| `providers.<name>.<field>` | varies | 組み込み provider のフィールドごとの設定：`api_key`、`url`、`protocol`、`model`、`models`、`auth_header`、`extra_body`。 |
| `custom_providers.<name>.<field>` | varies | 同じフィールド。カスタム（非組み込み）provider 用。カスタム provider には少なくとも `url` と `protocol` を設定する必要があります。 |
| `llm.url` | string | エンドポイント URL。Anthropic では完全な Messages URL（例：`https://api.anthropic.com/v1/messages`）を使用します。OpenAI 互換では chat-completions URL を使用します。 |
| `llm.auth_token` | string | API key。`Authorization: Bearer …` として送信されます（OpenAI）。レガシーの Anthropic パスもデフォルトで `Authorization: Bearer …`（プリセットの `anthropic` provider ではデフォルトが `x-api-key` に変わります）。`llm.auth_header` を明示的に設定した場合にのみ `x-api-key` を使用します。 |
| `llm.auth_header` | string | Auth header 名（`x-api-key`、`authorization`、または `bearer`）。Anthropic のみで使用します。`x-api-key` を必要とする一部の Anthropic 設定で必須です。 |
| `llm.model` | string | モデル名。`[<数字>m]` サフィックスは自動的に除去されます。 |
| `llm.use_anthropic` | boolean | `true`（デフォルト）→ Anthropic Messages プロトコル。`false` → OpenAI Chat Completions。 |
| `llm.extra_body` | JSON object | ベンダー固有のリクエストフィールド。各 chat リクエストボディにマージされます。例：`'{"thinking":{"type":"disabled"}}'`。 |
| `language` | string | system prompt に追加される指示として転送されます。未設定の場合はデフォルトで `English`。[言語の選択](#choosing-a-language) を参照。 |
| `telemetry.enabled` | boolean | OpenTelemetry エクスポートのマスタースイッチ。デフォルトは無効。 |
| `telemetry.exporter` | string | `console` または `otlp`。 |
| `telemetry.otlp_endpoint` | string | OTLP collector のアドレス（例：`localhost:4317`）。 |
| `telemetry.content_logging` | boolean | エクスポートされるイベントデータに LLM の prompt / レスポンスを含めます。 |

例：

```bash
ocr config set llm.url           https://api.anthropic.com/v1/messages
ocr config set llm.auth_token    sk-ant-xxxxxxxxxx
ocr config set llm.model         claude-opus-4-6
ocr config set llm.use_anthropic true
ocr config set llm.extra_body   '{"thinking":{"type":"disabled"}}'
ocr config set language          English
ocr config set telemetry.enabled true
ocr config set telemetry.exporter otlp
ocr config set telemetry.otlp_endpoint localhost:4317

# provider ベースの設定（推奨）
ocr config set provider          anthropic
ocr config set model             claude-opus-4-6
ocr config set providers.anthropic.api_key "$ANTHROPIC_API_KEY"

# カスタム（非組み込み）provider
ocr config set provider          my-gateway
ocr config set custom_providers.my-gateway.url      https://gateway.internal.com/v1
ocr config set custom_providers.my-gateway.protocol openai
ocr config set custom_providers.my-gateway.model    llama-3-70b
ocr config set custom_providers.my-gateway.api_key   "$MY_API_KEY"
```

boolean は Go の `strconv.ParseBool` が受け付ける任意の形式（`true`、`false`、`1`、`0`、
`t`、`f`……）を受け付けます。`llm.extra_body` は正しい JSON でなければなりません。

## ファイル schema リファレンス

上記のコマンドを実行すると、`~/.opencodereview/config.json` は次のようになります。

```json
{
    "llm": {
        "url": "https://api.anthropic.com/v1/messages",
        "auth_token": "sk-ant-xxxxxxxxxx",
        "auth_header": "x-api-key",
        "model": "claude-opus-4-6",
        "use_anthropic": true,
        "extra_body": {
            "thinking": { "type": "disabled" }
        }
    },
    "language": "English",
    "telemetry": {
        "enabled": true,
        "exporter": "otlp",
        "otlp_endpoint": "localhost:4317"
    }
}
```

provider ベースの形式では、レガシーの `llm` ブロックではなく `provider`、`model`、`providers`、
`custom_providers` を使用します。

```json
{
    "provider": "anthropic",
    "model": "claude-opus-4-6",
    "providers": {
        "anthropic": {
            "api_key": "sk-ant-xxxxxxxxxx",
            "model": "claude-opus-4-6"
        }
    },
    "custom_providers": {
        "my-gateway": {
            "url": "https://gateway.internal.com/v1",
            "protocol": "openai",
            "model": "llama-3-70b",
            "models": ["llama-3-70b", "llama-3-8b"],
            "api_key": "gw-xxxxxxxxxx",
            "auth_header": "authorization"
        }
    },
    "language": "English"
}
```

`provider` が設定されている場合、解決は `providers`/`custom_providers` マッピングによって駆動されます。この設定では
レガシーの `llm` セクションは無視されます。

このファイルを手動で編集することもできますが、次回の書き込み時に `ocr config set` が `"    "` インデントで
再シリアライズします。

## インタラクティブ設定——`ocr config provider` / `ocr config model`

provider と model を選択するために key を手動で入力する手間を省くため、OCR は 2 つのインタラクティブな Bubble Tea TUI を提供します。
どちらも同様に `~/.opencodereview/config.json` を変更します。

```bash
ocr config provider
ocr config model
```

- `ocr config provider`——組み込みまたはカスタムの provider を選択し、URL / protocol /
  API key / model を入力するインタラクティブな TUI です。選択内容は config に保存され、自動的に `ocr llm test` を実行して
  エンドポイントを検証します。組み込み provider の場合、直接入力しなければ API key はその provider の環境変数から
  読み取れます（[組み込み provider](#built-in-providers) を参照）。手動設定を選んだ場合は、レガシーの
  `llm.*` ブロックに代わりに書き込みます。
- `ocr config model`——現在の provider のプリセットリスト、および
  `providers.<name>.models` / `custom_providers.<name>.models` の下でユーザーが追加した
  model からモデルを選択するインタラクティブな TUI です。先に provider を設定しておく必要があります（`ocr config provider`）。

## 組み込み provider

以下の provider が OCR に同梱されています。それぞれにプリセットの `BaseURL`、`Protocol`、および
（該当する場合）`providers.<name>.api_key` が未設定のときにフォールバックとなる API key 環境変数があります。

| 名称 | Protocol | Base URL | API key 環境変数 |
|---|---|---|---|
| `anthropic` | anthropic | `https://api.anthropic.com` | `ANTHROPIC_API_KEY` |
| `openai` | openai | `https://api.openai.com/v1` | `OPENAI_API_KEY` |
| `dashscope` | openai | `https://dashscope.aliyuncs.com/compatible-mode/v1` | `DASHSCOPE_API_KEY` |
| `dashscope-tokenplan` | openai | `https://token-plan.cn-beijing.maas.aliyuncs.com/compatible-mode/v1` | `DASHSCOPE_TOKENPLAN_KEY` |
| `volcengine` | openai | `https://ark.cn-beijing.volces.com/api/v3` | `ARK_API_KEY` |
| `deepseek` | openai | `https://api.deepseek.com` | `DEEPSEEK_API_KEY` |
| `tencent-tokenhub` | openai | `https://tokenhub.tencentmaas.com/v1` | `TENCENT_TOKENHUB_API_KEY` |
| `hy-tokenplan` | openai | `https://api.lkeap.cloud.tencent.com/plan/v3` | `TENCENT_HUNYUAN_TOKENPLAN_KEY` |
| `kimi` | openai | `https://api.moonshot.cn/v1` | `MOONSHOT_API_KEY` |
| `z-ai` | openai | `https://open.bigmodel.cn/api/paas/v4` | `Z_AI_API_KEY` |
| `mimo` | openai | `https://api.xiaomimimo.com/v1` | `MIMO_API_KEY` |
| `minimax` | openai | `https://api.minimaxi.com/v1` | `MINIMAX_API_KEY` |
| `baidu-qianfan` | openai | `https://qianfan.baidubce.com/v2` | `QIANFAN_API_KEY` |

その他の provider 名はすべてカスタムとみなされ、`custom_providers` の下で設定する必要があり、少なくとも
`url` と `protocol` が必要です。

## 環境変数リファレンス

| 変数 | 用途 |
|---|---|
| `OCR_LLM_URL` | エンドポイント URL——`llm.url` と同形。 |
| `OCR_LLM_TOKEN` | API key——`llm.auth_token` と同じ。 |
| `OCR_LLM_MODEL` | モデル名。 |
| `OCR_LLM_AUTH_HEADER` | Auth header 名（`x-api-key`、`authorization`、または `bearer`）。Anthropic のみ。`llm.auth_header` と同じ。未設定の場合はデフォルトで `authorization`。 |
| `OCR_USE_ANTHROPIC` | 未設定 → Anthropic プロトコル（デフォルト）。`true` / `1` / `yes`（大文字小文字を区別しない）に設定 → Anthropic。その他の値（`false`、`0`、`no`、スペルミス……）に設定 → OpenAI。 |
| `ANTHROPIC_BASE_URL` | Claude Code 互換の base URL。 |
| `ANTHROPIC_AUTH_TOKEN` | Claude Code 互換の API key。 |
| `ANTHROPIC_MODEL` | Claude Code 互換の model。 |
| `OCR_ENABLE_TELEMETRY` | `1` で環境変数からテレメトリを有効化します。 |
| `OTEL_SERVICE_NAME` | span/metric の service name を上書きします。 |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP collector のアドレス——同時に exporter を `otlp` に強制します。 |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | OTLP のトランスポートプロトコル（`grpc`、`http/protobuf`、または `http/json`）。デフォルトは `grpc`。 |
| `OCR_CONTENT_LOGGING` | `1` でテレメトリイベントに prompt/レスポンスを含めます。 |

各 provider の API key（`ANTHROPIC_API_KEY`、`OPENAI_API_KEY`、
`DASHSCOPE_API_KEY`……）は、組み込み provider の `api_key` フィールドが未設定のときにフォールバックとなります。
各 provider の環境変数名は [組み込み provider](#built-in-providers) の表を参照してください。

## なぜ `extra_body` があるのか

一部のホスト型 provider は、リクエストボディに非標準のフィールドを追加します（例：Bedrock 風の `thinking`、
ベンダー固有の `temperature_strategy`、ストリーミングオプション）。`llm.extra_body` は発行される各リクエストに
マージされるため、ソースコードを変更せずにこれらのフィールドを送信できます。

```bash
ocr config set llm.extra_body '{"thinking":{"type":"enabled","budget_tokens":2048}}'
```

## 言語の選択

`language` key は 1 つのことだけを制御します。それは、レビューと `ocr llm test` の prompt 内の各
system-role メッセージに追加される 1 つの指示です。注入される正確な文字列は次のとおりです。

```
\n\nAlways respond in <language>.
```

- *未設定* または空——`English` として扱われます。
- `Chinese`、`English`、またはその他任意の文字列——そのまま透過されます。

組み込みの rule docs は言語の切り替えをサポートしません。`internal/config/rules/rule_docs/` の下に埋め込まれたファイルは
固定のファイル名で読み込まれ、ほとんどが中国語で書かれています（`default.md` は英語の例外）。`language`
の設定にかかわらず、それらはそのまま prompt に現れます。したがって `language` を `English` に設定した場合、prompt
には英語の指示と大量の中国語の rule テキストが重なります——強力なモデルは指示に従って英語のコメントを生成し、
弱いモデルは中国語と英語が混在した内容を出力する可能性があります。

`language` には環境変数、CLI 引数、プロジェクトレベルの上書きがありません——設定できる唯一の場所はグローバルな
`~/.opencodereview/config.json` で、
[`ocr config set`](#ocr-config-set--managing-opencodereviewconfigjson) を通じて設定します。

```bash
ocr config set language English
```

純粋な英語の rule テキストが必要な場合は、`--rule`、`<repo>/.opencodereview/rule.json`、
または `~/.opencodereview/rule.json` を通じて独自のルールを提供してください（
[レビュールール](../review-rules/#priority-chain) を参照）。

## プロジェクトレベル vs グローバル設定

CLI 自体はグローバルに設定されます（`~/.opencodereview/config.json`）——プロジェクトレベルの LLM 設定はありません。
ただし**レビュールール**はプロジェクトレベルです。[レビュールール](../review-rules/#priority-chain) を参照してください。

## 関連項目

- [クイックスタート](../quickstart/)——最小限のセットアップと初回のレビュー。
- [CLI リファレンス](../cli-reference/)——review コマンドが受け入れる各引数。
- [テレメトリ](../telemetry/)——OTLP / console exporter への接続方法。
