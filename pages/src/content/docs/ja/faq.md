---
title: FAQ
sidebar:
  order: 14
---

よくあるエラー、想定外の挙動、そして「これは仕様ですか？」という疑問をまとめました。ここに
あなたの問題が見つからない場合は、実行手順と完全な出力を添えて
[GitHub issue](https://github.com/alibaba/open-code-review/issues) を作成してください。

## 設定と起動

### `no valid LLM endpoint configured`

```
no valid LLM endpoint configured; one of OCR_LLM_URL/OCR_LLM_TOKEN/OCR_LLM_MODEL,
~/.opencodereview/config.json, or ANTHROPIC_BASE_URL/ANTHROPIC_AUTH_TOKEN/
ANTHROPIC_MODEL must be set
```

OCR はエンドポイント解決チェーン全体（[設定](../configuration/#既存の環境変数を再利用する)）を
たどりましたが、完全な `(URL, token, model)` の三つ組を見つけられませんでした。次のいずれかを
行ってください。

- `ocr config set llm.url …` / `llm.auth_token …` / `llm.model …` を実行して
  `~/.opencodereview/config.json` を埋める、**または**
- `OCR_LLM_URL` / `OCR_LLM_TOKEN` / `OCR_LLM_MODEL` をエクスポートする、**または**
- すでに Claude Code を使っている場合は、`ANTHROPIC_BASE_URL` / `ANTHROPIC_AUTH_TOKEN` /
  `ANTHROPIC_MODEL` をエクスポートする。

その後 `ocr llm test` で接続性を検証してからレビューを再試行してください。

### `ocr llm test` が誤ったソースを表示する

OCR は**最後**ではなく**最初**の完全な三つ組を採用します。したがって、設定ファイルに
すでに 3 つの llm.* key がすべて揃っていると、環境変数は無視されます。環境変数を有効にするには、
設定 key を削除する（ファイルを削除するか手動で unset する）か、`ocr config set` で新しい値に
切り替えてください。

### `ocr llm test` が 401 / 403 を返す

token に scope が不足している、期限切れ、あるいはプロバイダーが一致していません。Anthropic と
OpenAI は異なる auth header と URL フォーマットを使います——`llm.use_anthropic` が指し先の URL と
一致していることを確認してください。

- Anthropic: URL は `/v1/messages` で終わり、`use_anthropic=true`。
- OpenAI / OpenAI 互換: URL は `/v1/chat/completions` で終わり、
  `use_anthropic=false`。

### `not a git repository`

`ocr review` はカレントディレクトリに対して `git diff`（および untracked ファイルに対する
`git ls-files`）を実行します。Git ワークツリー内にいない場合は、早期に終了します。リポジトリに
`cd` するか、`--repo /path/to/repo` を渡してください。

## フィルタリングとルール

### ファイルがレビューされない

`ocr review --preview` を実行してください（LLM コストなし）。出力には各候補ファイルと、それが
保持されたか破棄されたかの**理由**が一覧されます。

```
src/foo.go              modified
src/foo_test.go         modified  (excluded: user_exclude)
node_modules/lib.js     added     (excluded: default_path)
imgs/logo.png           binary    (excluded: unsupported_ext)
```

5 種類の除外理由は、[ファイルフィルタリング](../review-rules/#how-files-are-filtered)のゲートに対応します。

| 理由 | 修正方法 |
|---|---|
| `binary` | 対処不要——バイナリファイルにはレビュー可能なテキストがありません。 |
| `user_exclude` | あなたの `exclude` リストからそのパターンを削除してください。 |
| `unsupported_ext` | ホワイトリストゲートを回避するため、拡張子を `include` リストに追加してください。 |
| `default_path` | ファイルを `include` に追加してください——組み込みのテストファイル除外パターンを上書きします。 |
| `deleted` | 対処不要——レビュー対象の新しい内容がありません。 |

### カスタムルールが発火しない

`ocr rules check <file-path>` を実行してください。マッチした**レイヤー**と **glob パターン**を
すべて表示します。

```
File: src/api/UserHandler.go
Source: Project (.opencodereview/rule.json)
Pattern: src/api/**/*.go
Rule: …
```

レイヤーが正しくない場合（プロジェクトルールを期待していたのに "System built-in" と表示される等）、
多くは**宣言順序**の問題です——最初にマッチしたパターンが適用されます。より具体的なルールを
`rules` 配列内で前に移動するか、glob を修正してください。

### 波括弧展開が動作しない

`bmatcuk/doublestar/v4` は `{ts,tsx}` の波括弧をサポートします。マッチしない場合は、余分な空白を
確認してください——`{ts, tsx}` のように空白があると、`tsx` に静かにマッチしなくなります。

## レビュー

### あるファイルにコメントが 0 件——本当にレビューされたのか？

[セッションビューア](../viewer/)（`ocr viewer`）を開き、セッションを見つけ、そのファイルの
`main_task` レーンを確認してください。

- ツール呼び出しあり + `task_done` で終了 → クリーンなレビュー。
- ツール呼び出しあり + ループ途中で終了 → エラーカードを探してください。
- `main_task` カードが全くない → ファイルはレビュー前にフィルタされました。上記の[フィルタリングとルール](#filtering--rules)を参照してください。

### コメントの `start_line: 0` と `end_line: 0`

OCR はコメントを diff 内の正確な行にアンカーできませんでした。よくある原因は 2 つです。

- モデルが `existing_code` を diff からそのままコピーせず、書き換えてしまった。モデルには
  そうしないよう指示されていますが、時々起きます。
- diff のフォーマットが異常（CRLF、tab/空白の混在）で、スライディングウィンドウのマッチングが
  壊れた。

コメント自体は本物です——ただ自動的に配置されなかっただけです。ほとんどのエージェント統合
（SKILL、Claude Code plugin）は `existing_code` フィールドを読み、ファイル内で自ら位置を特定します。

### Token threshold exceeded

```
[ocr] WARNING: prompt tokens (94000) exceed 80% of max_tokens(58888) for src/big.sql
```

そのファイルの初期 prompt（ルール + diff + change-files リスト）が、モデルが応答できる前に
すでに `MAX_TOKENS = 58888` の 80 % を超えました。OCR はそのファイルをスキップして続行します——
JSON モードでは `warnings` にも表示されます。

緩和策:

- 自動生成されたものなら、ファイルを `exclude` リストに追加してください。
- 大きなリファクタリングをより小さな commit に分割してください。
- 一連の小さな commit に対しては、一括のワークスペースモードではなく `--commit` モードで
  レビューしてください。

### ファイルが小さいのに plan フェーズに時間がかかる

まず `ocr review --preview` を実行してください。ファイルの `lines.changed` が
`PLAN_MODE_LINE_THRESHOLD`（デフォルト **50**）を超えると、plan フェーズが実行されます。これは
意図的なものです——大きな diff は plan の恩恵を受けます。単一のレビューでスキップするには、
より小さな diff で実行するか、埋め込みテンプレートを一時的に編集してください（上級者向け。
`--tools` の上書きが必要）。

### "Max tool requests reached"

```
[ocr] Max tool requests reached for src/foo.go.
```

モデルが 30（`MAX_TOOL_REQUEST_TIMES`）回のツール呼び出しを費やしたのに `task_done` を
呼びませんでした。その時点までに発せられたコメントは、依然として収集されレンダリングされます。
ほとんどのファイルでこうなる場合、原因は通常次のとおりです。

- モデルが「完了したら `task_done` を呼べ」という指示に従うのが苦手。より強力なモデル
  （Claude Opus など）に切り替えてください。
- あるツールがエラーを出し続け、モデルがリトライし続けている。セッション JSONL を確認してください——
  同じツール結果が繰り返されていれば、それが原因です。
- ファイルが本当に大きい、あるいはコンテキストが重く、30 回では足りない。`--max-tools <n>` で
  上げるか下げるか調整してください（例: `--max-tools 40` でより多く、`--max-tools 15` でより少なく）。
  1〜9 は 10 に引き上げられます。`0`（デフォルト）はテンプレートのデフォルト 30 を使います。

### 一部のサブエージェントが失敗しても、実行は 0 で終了する

意図的なものです。OCR はファイルごとの失敗を隔離し、1 つの問題のあるファイルが 20 ファイルの
レビュー全体を巻き込まないようにします。成功したものが*1 つでもあれば*、集計終了コードは `0` です。
完全に失敗した場合（成功したサブエージェントがゼロ）のみ非ゼロで終了します。どのファイルが
失敗したかは、JSON モードの `warnings` 配列またはテキストモードの stderr を確認してください。

### CI での実行がローカルよりずっと遅い

よくある原因は 2 つです。

- **モデルのレート制限**——制限がかかると、LLM client はバックオフしてリトライします。最初から
  制限に触れないよう、`--concurrency` を下げてください（`4` など）。
- **コールドキャッシュ**——プロバイダーが prompt キャッシュをサポートしている場合、デプロイ後の
  初回実行は恩恵を受けられません。同じウィンドウ内の後続実行はより高速になります。

## 出力と統合

### `--audience agent` でも進捗行が出る

見ているのが **stderr** でないことを確認してください。進捗メッセージは時々 stderr に出ます
（警告、エラー）。`--audience agent` が保証するクリーンな stdout は*パーサーに優しい*ものです——
すべてを遮断するにはリダイレクトしてください: `ocr review --audience agent 2>/dev/null`。

### JSON 出力が `{ "files_reviewed": 0, "comments": [] }`

ワークスペースに対象ファイルがありません。これは意図的なものです——明示的な形により、呼び出し側は
「レビュー対象がない」ことと「レビューしたファイルに指摘がない」ことを区別できます。コメントが
ゼロの正常なレビューは、通常の空配列 `[]` を返します。

### セッション JSONL はどこにある？

```
~/.opencodereview/sessions/<path-encoded-repo-path>/<session-id>.jsonl
```

リポジトリパスは、`/` と `\` を `-` に、`:` を `_` に置き換えてエンコードされます
（例: `/Users/foo/my-repo` → `Users-foo-my-repo`）。`ocr viewer` でセッションを閲覧できます。
このディレクトリを削除すると履歴が消えます。OCR は次回実行時にエンコード済みパスを再生成します。

## パフォーマンスとコスト

### どの token にどれだけ費やしたかを知るには？

テレメトリを有効にします。

```bash
ocr config set telemetry.enabled true
ocr config set telemetry.exporter console
ocr review
```

LLM 呼び出しには独自の span がありません——metric として記録されます。`ocr.llm.tokens_used`
（counter、`model` + `type` でラベル付け）、`ocr.llm.requests_total`（counter、`model`
+ `status` でラベル付け）、`ocr.llm.request_duration_seconds`（histogram、`model` でラベル付け）に
注目してください。console exporter はこれらの集計をインラインで出力します。ダッシュボードが必要な場合は、
OTLP exporter に切り替えて metrics 基盤に送ってください——[テレメトリ](../telemetry/)を参照。

### なぜ私のレビューはこんなに高価なのか？

よくある要因:

- ファイルが 50 行以上のとき plan フェーズが起動します。これはファイルごとに LLM 呼び出しを
  1 回追加します。閾値を下げるとコストを削減でき、上げると小さな PR の速度を向上できます。
- `MAX_TOOL_REQUEST_TIMES = 30` はかなり緩やかです。ラウンドを使い切るモデルは、3 ラウンドで
  終わるモデルより長い（トークンの多い）対話を生みます。より強力なモデルはより速く終える傾向が
  あります。逆に、"max tool requests reached" に対処するため `--max-tools` を上げると、ファイルごとの
  コストはおおむね線形に増加すると考えてください。
- メモリ圧縮それ自体が 1 回の LLM 呼び出しです。長いサブタスクは、レビューのラウンドに加えて、
  圧縮のラウンド分も支払うことになります。

### LLM 呼び出しを減らすには？

- `include` リストを追加して、OCR が気にしないファイルをレビューしないようにします。
- アカウントに burst-mode の課金がある場合は、`--concurrency` を下げます。
- `--background` を渡します——十分な事前コンテキストがあれば、モデルが `file_read` /
  `code_search` の往復なしに完了できることがあります。

## プライバシーとセキュリティ

### OCR は私のコードをどこかに送るのか？

OCR はあなたの **diff**（および任意の read-tool スニペット）を、設定した LLM エンドポイントに送ります。
それ以外のものは一切あなたのマシンから出ません——セッション JSONL とルールファイルはローカルにのみ
存在します。

テレメトリを有効にしている場合、`content_logging` フラグは設定レイヤーに接続されていますが、
現時点ではどのコードパスも**制御しません**——このフラグの値にかかわらず、prompt と応答の内容が
collector にエクスポートされることは決してありません。予約項目とみなしてください。本番環境では
`false` のままにしてください。詳細は[テレメトリ](../telemetry/#content-logging)を参照してください。

### LLM に送る前に secret をマスクできるか？

組み込み機能ではありません。推奨のワークフロー:

1. secret をリポジトリにコミットしない（一般的なルール）。
2. 機密情報を含むと分かっているファイルを `exclude` に追加する。
3. `git diff --no-textconv` フィルターまたは pre-commit でマスクし、secret が diff に入らないようにする。

「マスクルール」機能はロードマップにあります。
[issue トラッカー](https://github.com/alibaba/open-code-review/issues)をご覧ください。

## その他

### changelog はどこにある？

[GitHub Releases](https://github.com/alibaba/open-code-review/releases)
——各 release には Conventional Commits から生成された notes が付属しています。

### OCR は Git 以外の VCS をサポートするか？

しません。diff provider は shell 経由で `git` を呼び出します。SVN / Mercurial などには新しい
provider が必要です。Hg サポートの issue は[こちら](https://github.com/alibaba/open-code-review/issues)で
オープンになっています。

### なぜバイナリは `opencodereview` なのに CLI は `ocr` なのか？

release で配布される静的バイナリはプロジェクト名（`opencodereview`）を持ちます。NPM wrapper は
使いやすさのため `ocr` としてインストールされます。ソースからビルドすると `dist/opencodereview` が
得られます——`$PATH` 上の `ocr` としてコピーしてください。

### アンインストールするには？

```bash
npm uninstall -g @alibaba-group/open-code-review        # NPM install
sudo rm /usr/local/bin/ocr                              # binary install
rm -rf ~/.opencodereview                                # all state
```

OCR は `~/.opencodereview` の外には書き込みません（NPM がダウンロードするバイナリを除く）。
したがってこのディレクトリを削除すれば、履歴、設定、ユーザーごとのルールが消去されます。

## 関連項目

- [設定](../configuration/)——LLM エンドポイント解決と config key。
- [レビュールール](../review-rules/)——ファイルフィルターとルール解決チェーン。
- [セッションビューア](../viewer/)——過去のレビューセッションを表示する。
- [テレメトリ](../telemetry/)——token 使用量と LLM メトリクス。
