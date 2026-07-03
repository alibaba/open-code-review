---
title: MCP サーバー
sidebar:
  order: 10
---

OCR は **Model Context Protocol（MCP）クライアント**として動作できます。1 つ以上の
外部 MCP server を指定すると、それらの server が公開するツールがレビュー
エージェントから利用できるようになり、`file_read` や `code_search` などの
[組み込みツール](../tools/)と並んで使えます。

これは MCP の**クライアント**側です。OCR は他のエージェントが呼び出す MCP server
としては動作**しません**——その方向については[統合](../integrations/#mcp)の
説明を参照してください。本ページはその逆、OCR 自身のレビュアーに能力を追加する話です。

## いつ使うか

レビュアーが diff の外にあるコンテキストを必要とするときに MCP server を導入します：

- **Issue / チケット参照**——リンクされた Jira / GitHub issue を取得させ、変更が
  述べられた要件に合致するか確認する。
- **ドキュメント / ナレッジベース**——社内 API ドキュメントやコーディング規約を
  取り込み、コメントが実際のチームルールを引用できるようにする。
- **カスタム解析**——linter、スキーマ検証器、依存関係チェッカーを、レビュアーが
  必要に応じて呼び出せるツールとして公開する。

リポジトリを読むだけでよいなら組み込みツールで十分です——MCP は checkout の**外**に
到達するためのものです。

## 仕組み

- OCR は設定された各 server に **stdio トランスポート**で接続します：server を
  サブプロセスとして起動し、その stdin/stdout 経由で MCP を話します。
- サブプロセスは**作業ディレクトリをリポジトリのルート**に設定して実行され、OCR の
  環境変数に加えて設定した `env` を継承します。
- 起動時に OCR は server のツールを列挙し、組み込みツールが使うのと同じツール
  レジストリに登録します。登録された MCP ツールは **plan フェーズと main task の
  両方**で利用できます。
- server はレビューの間ずっと稼働し続け、終了時にシャットダウンされます。

server の起動に失敗した場合（または `setup` コマンドが失敗した場合）、OCR は警告を
表示し、**それを使わずにレビューを続行**します——壊れた MCP server がレビューを
ブロックすることはありません。

## 設定

MCP server はユーザー設定ファイル（`~/.opencodereview/config.json`）の
`mcp_servers` キーの下に置きます。各エントリは任意の名前を key とし、次の
フィールドを受け付けます：

| フィールド | 型 | 必須 | 説明 |
|---|---|---|---|
| `command` | string | ✓ | MCP server を起動する実行ファイル（`npx`、`uvx`、絶対パスなど）。 |
| `args` | string 配列 | | `command` に渡す引数。 |
| `env` | string 配列 | | 追加の環境変数、`KEY=VALUE` 形式。 |
| `tools` | string 配列 | | 登録するツール名の許可リスト。空 = server が提供する全ツールを登録。 |
| `setup` | string | | server 起動前に一度実行される shell コマンド（依存関係のインストールなど）。リポジトリのルートで実行、タイムアウト 5 分。 |

### CLI を使う

`ocr config set` コマンドはこれらのフィールドを非対話的に書き込みます。配列
フィールド（`args`、`env`、`tools`）は JSON 配列文字列を受け取ります：

```bash
# 最小構成：コマンドだけ
ocr config set mcp_servers.docs.command npx

# 引数
ocr config set mcp_servers.docs.args '["-y", "@acme/docs-mcp-server"]'

# 環境変数（KEY=VALUE エントリ）
ocr config set mcp_servers.docs.env '["DOCS_TOKEN=secret", "DOCS_REGION=eu"]'

# レビュアーに公開するツールを制限
ocr config set mcp_servers.docs.tools '["search_docs", "get_page"]'

# server 起動前に実行する setup コマンド
ocr config set mcp_servers.docs.setup "npm install -g @acme/docs-mcp-server"
```

`unset` で server を削除します：

```bash
ocr config unset mcp_servers.docs
```

### 手動で編集

同じ設定を JSON で書くと：

```json
{
  "mcp_servers": {
    "docs": {
      "command": "npx",
      "args": ["-y", "@acme/docs-mcp-server"],
      "env": ["DOCS_TOKEN=secret", "DOCS_REGION=eu"],
      "tools": ["search_docs", "get_page"],
      "setup": "npm install -g @acme/docs-mcp-server"
    }
  }
}
```

## ツールのフィルタリング

デフォルトでは server が広告するすべてのツールが登録されます。server が
レビュアーに必要以上のツールを公開する場合は `tools` に許可リストを設定します——
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

- `Running setup for MCP server "x": …`——setup コマンドを実行中。
- `failed to start MCP server "x": …`——サブプロセスが 30 秒の初期化タイムアウト内に
  接続できなかったか、`command` が `PATH` にない。
- `tool "y" conflicts with built-in tool, skipping`——server のツールを改名するか、
  `tools` から外す。
- `allowed tool "y" not found in server's tool list`——`tools` の名前が server の提供
  する何にも一致しない。スペルを確認。

## 関連項目

- [ツール](../tools/)——MCP ツールが並ぶ 6 つの組み込みツール。
- [設定](../configuration/)——設定ファイル全体とすべてのキー。
- [CLI リファレンス](../cli-reference/)——`ocr config` と review のフラグ。
