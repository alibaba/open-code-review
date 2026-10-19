# レビューワークフロープラグイン（Review Workflow Plugins）

[English](../review-workflow-plugins.md) | [简体中文](review-workflow-plugins.zh-CN.md) | [日本語](review-workflow-plugins.ja-JP.md) | [한국어](review-workflow-plugins.ko-KR.md) | [Русский](review-workflow-plugins.ru-RU.md)

パッケージのインストール、実行権限、およびランタイムアダプターの関係については、[ネイティブプラグインのアーキテクチャ設計](../plugin-architecture.md) を参照してください。

コードレビューにおいて特定の実行順序や完了基準が必要な場合（例えば、変更点を検査した後に、候補となる指摘を別の段階で再検証し、最終的な完了レポートを作成する場合など）に、**レビューワークフロープラグイン** を使用します。パッケージは各段階を宣言し、モデルからの提出物を検証します。OCR ホストは diff、ルール、ツール、およびトークン予算を提供します。

## single-pass サンプルの実行

Python 3 が `python` として利用可能な環境で、サンプルをインストールし、作業ツリーの変更に対してプレビューを実行します：

```sh
ocr plugin install ./examples/ocr-plugins/single-pass
ocr review --review-plugin single-pass --preview
```

モデルプロバイダーが設定され、レビュー対象の変更がある状態で以下を実行します：

```sh
ocr review --review-plugin single-pass --format json --output review.json
```

このサンプルは 1 つの段階で構成されています。モデルに指摘を求め、OCR が公開する前にプラグインが提出内容を検証します。`review.json` 内の `review_plugin` で完了ステータスとレポートを確認できます。インストールのみではパッケージコードは実行されず、プレビュー（`--preview`）でも Python プロセスやモデルへの問い合わせは行われません。

上記はユーザー単位でインストールされたパッケージを使用しています。リポジトリでプロジェクトプラグインが宣言されている場合は、後述のプロジェクトワークフローを使用して宣言・承認してください。

## プロジェクトでの選択と実行権限

リポジトリで管理されるワークフローの場合は、`ocr plugin add <source>` で宣言し、`.opencodereview/plugins.json` と `plugins.lock.json` をコミットした上で、`ocr plugin sync --frozen` で同期インストールします。`--review-plugin <id>` で選択する前に、ロックされたパッケージを確認して `ocr plugin trust` でローカル実行権限を付与してください。`ocr plugin preflight --json` を実行すると、ワークフローを開始したりモデルを呼び出したりすることなく、インストール内容、互換性、ローカル認証状態を検証できます。プロジェクトプラグインが宣言されている場合、ロックされたセットからのみ選択可能です。詳細は [ネイティブ OCR プラグインガイド](native-plugins.ja-JP.md#project-plugins) を参照してください。

レビューワークフロープラグインは、ステージ計画、プロンプト指示、完了スキーマ、検証、およびステージ間状態を保持します。OCR は diff、ルール、モデル/ツール実行、セッション、トークン計算、コメント処理、出力を担当します。

単一実行でのワークフロー選択：

```sh
ocr review --review-plugin PACKAGE_ID --preview
ocr review --review-plugin PACKAGE_ID --format json --output review.json
```

プラグインのインストールや有効化だけではワークフローは実行されません。`--preview` はマニフェストを読み込むだけで実行ファイルは起動しません。実際のレビュー実行時には、ローカル MCP サーバーと同様にユーザーのローカル権限で実行ファイルが起動されます。`--review-plugin` で明示的に選択した場合、グローバルで無効化されていても該当プラグインのルールと MCP サーバーが読み込まれます。

## マニフェスト構成（Manifest）

```json
{
  "schema_version": 1,
  "id": "example-review",
  "name": "Example Review",
  "version": "1.0.0",
  "review": {
    "protocol": "stdio-v1",
    "command": "${plugin_dir}/bin/${os}-${arch}/reviewer${exe}",
    "args": ["--policy", "${plugin_dir}/policy.md"],
    "stages": ["inspect", "verify"]
  }
}
```

`command` は実行可能ファイルのパスであり、シェルによる解釈は行われません。コマンドと引数では `${plugin_dir}`、`${os}`、`${arch}`、`${exe}`（Windows は `.exe`、Unix は空）が展開されます。PATH 上の `python` などのランタイムも指定可能です。同梱の Python サンプルは `python` コマンドを想定しています。環境が `python3` のみの場合は、パッケージング前にマニフェストを `python3` に変更してください。

プラグインはレビューワークフロー、ルール、MCP を自由に組み合わせられます。ステージ ID は一意の識別子であり、1〜64 個の順序付きステージを宣言できます。

インストール済みスナップショットが、その実行のルールとワークフローの両方を提供します。パッケージを更新しても実行中のスナップショットは変わりません。旧実験形式の `engine`/`prompt` のみを扱うホストは更新が必要です。ホストは旧形式のインストール情報を読み、一覧表示、削除、更新を行えます。管理出力では更新が必要と表示され（JSON の `review.update_required`）、実行案内は表示されません。旧形式を実行しようとすると更新を求めます。

## 互換性

ワークフローは任意の `requires.ocr_min_version` と `requires.features` を宣言できます。例は `["rules", "mcp", "stdio-v1"]` です。[ホスト要件](native-plugins.ja-JP.md#compatibility-and-host-requirements) を参照してください。未対応の要件はプロセス起動前に拒否されます。ワークフロー API は実験段階です。既存の `stdio-v1` は同じ `begin`/`submit` を使い、必須ハンドシェイクや JSON-RPC への移行はありません。未知の応答エンベロープフィールドはエラーです。`report` と完了引数の構造はプラグインが定義します。互換性のないエンベロープ変更には新しいプロトコルまたはメッセージのバージョンが必要です。

## stdio-v1 プロトコル仕様

OCR はファイルグループごとに一つのプロセスを起動し、レビュー対象のリポジトリを作業ディレクトリにします。プロセスはそのグループのステージ間で状態を保持します。要求と応答は一行の UTF-8 JSON オブジェクトで、上限は 4 MiB です。一つの要求に対して応答は必ず一つだけ返し、診断は stderr に出力します。各応答に `"version": 1` が必要です。空でない `error` 文字列は要求を実行できないことを示し、グループを停止します。

### ステージ開始（`begin`）

各ステージ開始前に OCR が送信します：

```json
{"version":1,"method":"begin","stage":"inspect","paths":["main.go"],"severities":["critical","high","medium","low"],"categories":["bug","security","performance","maintainability","test","style","documentation","other"]}
```

プラグインはタスクと初期レポートを返します：

```json
{"version":1,"task":{"instructions":"Inspect the selected code.","input":"Check the public return contract.","completion":{"description":"Submit the result.","parameters":{"type":"object","properties":{"checked":{"type":"boolean"}},"required":["checked"]}}},"report":{"pending":["inspection"]}}
```

OCR は instructions と input を通常の diff/ルールのコンテキストに追加し、コンテキスト圧縮後も保持します。`instructions` と `input` は別々のモデルメッセージになるため、どちらも空白以外の文字列が必要です。モデルは OCR のコンテキストツールを引き続き使えます。`task_done` の定義は `completion` から取得します。直接の `code_comment` は渡さず、公開する指摘をプラグインの検証に通します。

プロトコルの要求にモデル認証情報は含めません。ただし実行ファイルは通常のプロセス環境を継承し、そこにはプロバイダーのトークンやプロキシ設定が含まれる場合があります。ユーザー権限で読める設定ファイルにもアクセスできます。このリリースには実行時サンドボックスがないため、信頼する作者のワークフローだけをインストールして選択してください。

### モデル提出（`submit`）

モデルが `task_done` を呼び出すと、OCR は引数をそのまま転送します：

```json
{"version":1,"method":"submit","stage":"inspect","arguments":{"checked":true}}
```

提出を受け入れるレスポンスでは `accepted` と `complete` のブール値を返します：

```json
{"version":1,"accepted":true,"complete":true,"report":{"checked":true},"comments":[]}
```

レポートの構造と完了条件はプラグインが定義します。`accepted:true` と `complete:false` を返すとグループは未完了として停止します。`feedback` は未完了の理由として保存され、モデルの修正を再開しません。受理済みのコメントは未完了の場合も通常の公開・フィルタリングに進みます。コメントは選択されたパスを参照し、空でない内容と `begin` で渡された severity/category の値を使う必要があります。その他のフィールドは `existing_code`、`suggestion_code`、`start_line`、`end_line`、`thinking` です。

モデルが修正できる提出物には `accepted:false`、空でない `feedback`、最新の `report` を返し、コメントは含めません。OCR は既存の要求上限内でフィードバックをモデルに返します。有効な途中結果はプラグイン自身が保持してください。クラッシュ、不正な応答、必須ブール値の欠落は実行を停止し、モデルの誤りとして再試行しません。

## 完了状態と出力

前のステージで問題が見つからなくても、宣言されたすべてのステージを完了する必要があります。`--effort` はプラグインの計画を短縮しません。既存のコンテキスト、ツールラウンド、タイムアウト設定が適用され、グループのタイムアウトは宣言されたステージ数を対象とします。ユーザーがトークン予算を指定すると、未使用の枠を残りのステージで共有します。追加の総トークン制限は設けません。

JSON の `review_plugin` は `plugin`、`group`、`stage`、`complete`、任意の `token_limit`、プラグイン定義の `report`、実行時の `error` を持ちます。後でモデルやプロセスが失敗しても、最後のレポートを保持します。完了または中断した各ステージは、テキスト出力時もセッションに `plugin_review` を保存します。`ocr session show SESSION_ID --json` の `review_plugin` 配列で確認できます。応答が `report` を省略すると前のレポートを保持します。必須作業が未完了ならカバレッジに記録され、CLI は非ゼロで終了します。通常レビューの終了動作は変わりません。

ワークフローのレビューは新しい呼び出しが必要です。`--resume` は任意のプロセス状態をまだ復元できません。JSON とセッションの記録は引き続き確認できます。

Python の実装は [single-pass の例](../../examples/ocr-plugins/single-pass) を参照してください。[Behavior Review](https://github.com/Qiyuanqiii/ocr-behavior-review) は三つのステージと証拠の保持を実装した独立した実行ファイルです。
