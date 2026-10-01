---
title: MCP サーバー
sidebar:
  order: 10
---

OCR は **Model Context Protocol（MCP）クライアント**として動作できます。1 つ以上の
外部 MCP server を指定すると、それらの server が公開するツールがレビュー
エージェントから利用できるようになり、`file_read` や `code_search` などの
[組み込みツール](../tools/)と並んで使えます。

## いつ使うか

レビュアーが diff の外にあるコンテキストを必要とするときに MCP server を導入します：

- **Issue / チケット参照** —— リンクされた Jira / GitHub issue を取得させ、変更が
  述べられた要件に合致するか確認する。
- **ドキュメント / ナレッジベース** —— 社内 API ドキュメントやコーディング規約を
  取り込み、コメントが実際のチームルールを引用できるようにする。
- **カスタム解析** —— linter、スキーマ検証器、依存関係チェッカーを、レビュアーが
  必要に応じて呼び出せるツールとして公開する。

リポジトリを読むだけでよいなら組み込みツールで十分です —— MCP は checkout の外に
到達するためのものです。

## 設定

#### ローカル MCP server を追加する

`ocr config set` コマンドはこれらのフィールドを非対話的に書き込みます。配列
フィールド（`args`、`env`、`tools`）は JSON 配列文字列を受け取ります：

```bash
# 最小構成：コマンドだけ
ocr config set mcp_servers.docs.command npx

# 引数
ocr config set mcp_servers.docs.args '["-y", "@acme/docs-mcp-server"]'

# レビュアーに公開するツールを制限
ocr config set mcp_servers.docs.tools '["search_docs", "get_page"]'

# server 起動前に実行する setup コマンド
ocr config set mcp_servers.docs.setup "npm install -g @acme/docs-mcp-server"

# 環境変数（KEY=VALUE エントリ）
ocr config set mcp_servers.docs.env '["DOCS_TOKEN=secret", "DOCS_REGION=eu"]'
```

#### リモート MCP server を追加する

**Streamable HTTP** に対応した server では、`type` を `remote` にし、ローカル
コマンドの代わりに `url` を指定します。`url` だけでは足りません：デフォルトの
type は `stdio` です。

既存の接続を上書きしないよう、新しい server 名を使ってください：

```bash
ocr config set mcp_servers.search.type remote
ocr config set mcp_servers.search.url https://mcp.example.com/mcp
ocr config set mcp_servers.search.tools '["search", "fetch"]'
```

これらのコマンドは接続をユーザー設定に保存します。次回のレビューで OCR が接続し、
`search` と `fetch` を組み込みツールと並べてエージェントに提供します。ツールの
許可リストにより、server が提供しうるそれ以外のツールはレビューに入りません。
他の設定済み server とレビュー設定は変更されません。

設定後、エージェントはレビュー中に呼び出しごとの確認なしでこれらのツールを
使えます。ツール引数 —— 検索クエリ、要求する URL、エージェントが添える
コンテキスト —— はあなたのマシンを離れ、そのエンドポイントの運営者に届きます。
これはユーザー設定なのでリポジトリを跨いで適用されます：外部リクエストが許される
場所でのみ有効にし、リクエストに秘密情報、非公開コード、社内 URL を含めないで
ください。サードパーティのサービスに接続する前に、運営者のプライバシーポリシーと
利用規約を確認してください。

#### MCP server を削除する

`unset` で server を削除します：

```bash
ocr config unset mcp_servers.docs
```

MCP server はユーザー設定ファイル（`~/.opencodereview/config.json`）の `mcp_servers` キーの下に置きます。

| フィールド | 型 | 必須 | 説明 |
|---|---|---|---|
| `type` | string | | ローカルのサブプロセスなら `stdio`（デフォルト）、Streamable HTTP なら `remote`。 |
| `command` | string | `stdio` では必須 | MCP server を起動する実行ファイル（`npx`、`uvx`、絶対パスなど）。 |
| `args` | string 配列 | | `command` に渡す引数（`stdio` のみ）。 |
| `url` | string | `remote` では必須 | HTTP または HTTPS の MCP エンドポイント。 |
| `headers` | object | | HTTP ヘッダー名と文字列値（`remote` のみ）。値は接続時に OCR の環境から `$VAR` または `${VAR}` を展開します。空文字列に展開された値は、空のまま送信されたり無視されたりせず、**接続を失敗させます**。匿名アクセスでは省略します。 |
| `tools` | string 配列 | | 登録するツール名の許可リスト。空 = server が提供する全ツールを登録。 |
| `setup` | string | | server 起動前に一度実行される shell コマンド（`stdio` のみ、依存関係のインストールなど）。リポジトリのルートで実行、タイムアウト 5 分。 |
| `env` | string 配列 | | サブプロセスへの追加の環境変数、`KEY=VALUE` 形式（`stdio` のみ）。 |

認証が必要なリモート server では、その server の指示に従って `headers` を設定して
ください。`ocr config set` に渡す JSON に環境変数の参照が含まれる場合は、OCR が
設定を保存する前に shell が展開してしまわないよう、シングルクォートで囲みます。
匿名アクセスを許す server では `headers` は一切不要です。

## ツールのフィルタリング

デフォルトでは server が広告するすべてのツールが登録されます。server が
レビュアーに必要以上のツールを公開する場合は `tools` に許可リストを設定します ——
ツールが少なく的確なほどエージェントは集中でき、トークンコストも下がります。
リストに含まれていて server が実際には提供しない名前は警告付きでスキップされる
ため、タイプミスは黙って無視されるのではなく stderr に表示されます。

## 名前の衝突

MCP ツール名は組み込みツールと 1 つの名前空間を共有します。server が広告する
ツール名が**組み込み / 予約**ツール（`file_read`、`code_search`、`task_done` など）や、
別の MCP server が既に登録したツールと衝突する場合、OCR はそれを**スキップ**して
警告を記録します。先に登録されたものが優先されます。こうしてツールを失わない
よう、各 server には重複しないツール名を付けてください。

## `setup` コマンド

`setup` は server サブプロセスの起動前に、リポジトリのルートから一度実行されます。
server をオンデマンドでインストールまたはビルドするのに使います：

```json
"setup": "npm install -g @acme/docs-mcp-server"
```

**5 分のタイムアウト**があります。非ゼロで終了した場合、OCR はコマンド、作業
ディレクトリ、出力を記録し、その server をスキップしてレビューを続行します。

## トラブルシューティング

すべての MCP 診断情報は **stderr** に、`[ocr]` プレフィックス付きで出力されるため、
stdout の `--format json` 出力を汚染することはありません：

- `Running setup for MCP server "x": …` —— setup コマンドを実行中。
- `failed to start MCP server "x": …` —— サブプロセスが 30 秒の初期化タイムアウト内に
  接続できなかったか、`command` が `PATH` にない。
- `remote MCP server "x" has no URL configured, skipping` —— `type` が `remote` なのに
  `url` が未設定。`url` だけ設定して `type` を忘れる場合の裏返し。
- `failed to connect to remote MCP server "x": …` —— エンドポイントが 30 秒の初期化
  タイムアウト内に接続できなかった、またはツール一覧を返せなかった。URL、ネットワーク
  到達性、必要なヘッダーを確認。
- `MCP server "x" header "h" expanded to empty value` —— `headers` の `$VAR` が OCR の
  環境に設定されていない。ヘッダーが無いものとして扱われるのではなく、接続が失敗する。
- `remote MCP server "x" returned HTTP 401 Unauthorized` —— トークンやヘッダーの設定を
  確認。
- `remote MCP server "x" returned HTTP 403 Forbidden` —— 認証情報は server に届いたが、
  必要な権限が不足している。
- `tool "y" conflicts with built-in tool, skipping` —— server のツールを改名するか、
  `tools` から外す。
- `allowed tool "y" not found in server's tool list` —— `tools` の名前が server の提供
  する何にも一致しない。スペルを確認。

起動または接続に失敗した server はスキップされ、そのツールなしでレビューが続行され
ます。

## 関連項目

- [ツール](../tools/) —— MCP ツールが並ぶ 6 つの組み込みツール。
- [設定](../configuration/) —— 設定ファイル全体とすべてのキー。
- [CLI リファレンス](../cli-reference/) —— `ocr config` と review のフラグ。
