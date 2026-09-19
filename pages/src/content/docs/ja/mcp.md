---
title: MCP サーバー
sidebar:
  order: 10
---

## サーバー中心の管理とインポート

管理画面は単一の全画面端末セッションで動作します。**CONFIGURED SERVERS** と **MANAGEMENT ACTIONS** は別々の領域で、**Tab** で切り替えます。追加・インポート・編集・権限の画面は現在の画面を置き換え、下に積み重なりません。取消後は保存せず元の選択項目に戻ります。長いプレビューは **PgUp/PgDn** でスクロールできます。どの画面でも **Ctrl-C** で終了し、元の端末画面に戻ります。**Q** による終了はナビゲーション画面のみで、入力欄では文字として使えます。

`ocr mcp` はサーバー一覧を開きます。サーバーを選び、ツール・接続・権限を管理します。**Esc** で親画面へ戻り、**Q** で終了します。操作後も選択したサーバーに戻ります。一覧を開いても接続しません。設定状態はオンライン状態ではなく、接続確認の結果はこの管理セッションの最終確認として表示されます。

**Tools** で設定済みツールをオフラインで確認できます。**Test connection** は起動・接続前に確認し、カタログだけを取得します。新しいツールは未選択です。実効権限には上位 deny、無効状態、検出した定義変更が反映されます。ツールの無効化はオフラインでも可能です。新しい定義の承認には再接続が必要で、そのツールは `ask` に戻ります。review 起動時に指紋を再検証します。

**Import (JSON / TOML)** ではファイルまたは非表示の貼り付けを使用できます。**Ctrl-S** でプレビュー、**Esc** で取消します。Cursor JSON の `mcpServers` と Codex TOML の `mcp_servers` に対応します。

```bash
ocr mcp import [file] [--yes]
ocr mcp import ./mcp.json
ocr mcp import ./one-server.toml --yes
```

1 回に 1 接続だけを無効状態・ツールなしで保存します。権限はコピーせず、setup の実行や通信も行いません。同名を上書きせず、対話時は名前を変更できます。上限は 1 MiB、64 サーバーです。非対話では単一サーバーと `--yes` が必要です。`-` は標準入力です。

平文 env/header 値はコピーせず、`${VARIABLE_NAME}` または `${OCR_MCP_HEADER_NAME}` 参照に変換します。接続前に設定すべき変数をプレビューします。header 変数は必要な Bearer 接頭辞を含む完全な値です。`bearer_token_env_var` は Bearer 参照に変換されます。コマンド引数と URL は保持され、プレビューでは機密値を隠します。そこに認証情報を埋め込まないでください。

OAuth、作業ディレクトリ `cwd`、ヘルパーコマンド等の未対応フィールドは拒否します。導入後は **Edit connection** で接続を確認し、ツールを選択して保存します。実行前の独立認可は変わりません。

OpenCodeReview は `ocr review` 中に **Model Context Protocol（MCP）** server の
ツールを利用できます。`ocr scan` は MCP を読み込みません。

MCP server は外部コードであり、公開されるツールのメタデータも信頼されません。
server を追加しただけですべてのツールが公開されることはありません。OCR はまず
カタログを発見し、その後ユーザーが明示的なツール allowlist と実行権限を選びます。

## 2 つのセキュリティゲート

すべての MCP 呼び出しは、独立した 2 つのゲートを通過する必要があります。

1. **能力の可視性。** MCP 全体と server が有効で、ツールが server の `tools`
   allowlist にあり、保存済みの定義 fingerprint が発見結果と一致する場合に限り、
   モデルから見えるようになります。
2. **実行時認可。** `tools/call` の直前に、OCR は同じ server、ツール、fingerprint、
   context、最終権限を再検査します。`ask` は端末で確認し、`allow` はその確認だけを
   省略します。

`tools` が空または未指定なら、意味は**ツールなし**であり、全ツールではありません。
`allow` も server 自身が宣言する annotation も allowlist を広げません。拒否、キャンセル、
タイムアウト、EOF、対話不能な端末では `tools/call` は送信されません。

## ガイド付きセットアップ

端末で manager を開きます。

```bash
ocr mcp
```

追加 wizard を直接始めることもできます。

```bash
ocr mcp add docs
```

wizard は次の順で動作します。

1. `stdio` または `remote` を選択する。
2. 保存済み secret を表示せず、接続設定を入力する。
3. ローカルの executable と引数構造（credential らしい値はマスク）、またはマスク済み remote endpoint と header 名を表示する。
4. server を起動または接続する前に確認する。
5. MCP を初期化して `tools/list` の全ページだけを読み、ツールは呼び出さない。
6. 初期選択をゼロにし、利用するツールをユーザーが選ぶ。
7. マスク済みの要約を表示し、最終確認後に一度だけ atomic 保存する。

矢印で選択し、**Space** でツールをチェック、**Enter** で次へ、**Ctrl-B** で戻って編集、
**Esc / Ctrl-C** で保存せず終了します。JSON 入力は不要です。引数は 1 個ずつ追加し、
認証情報にはトークン本体ではなく環境変数名を入力します。
接続確認には **Save disabled (no connection)**（接続せず無効状態で保存）もあります。
接続失敗時は設定を編集して再試行できます。選んだツールは `ask` から始まります。
保存後に `ocr mcp permissions docs` の選択メニューで自動実行 `allow` を設定できますが、
上位の `deny` は優先されます。

最終確認の拒否やキャンセルは保存せず終了します。発見中に業務ツールを
呼ばなくても、不審なローカルプロセスの起動や remote server への接続自体が server 側の
副作用を起こす可能性があります。確認前に必ず接続 preview を点検してください。

### ローカル stdio server

```bash
ocr mcp add docs
# 選択: stdio
# Executable: npx
# 引数: -y、実際の MCP パッケージ名を 1 個ずつ入力、空行で次へ
# 環境変数名: DOCS_TOKEN; 参照元の環境変数名: DOCS_TOKEN
```

子プロセスが受け取るのは、最小限の platform 環境（`PATH`、home/temp、locale など）と
`env` で明示した項目だけです。OCR プロセスの環境全体は継承しません。
ウィザードの新しい値には `${ENV_NAME}` 参照が必須です。過去の literal 値は移行のため読めますが deprecated で、
出力では常にマスクされます。

OCR は起動前に executable と引数構造を表示し、credential らしい flag 値と URL query 値はマスクします。
管理中は確認後にのみ起動し、review
中は可視になり得るツールが 1 つもなければ server を起動しません。

### Remote server

```bash
ocr mcp add knowledge
# 選択: remote
# URL: https://mcp.example.com/v1
# Header 名: Authorization; 参照元の環境変数名: MCP_TOKEN
# 任意のプレフィックス: Bearer と末尾の空白
```

remote URL は localhost/loopback を除いて HTTPS が必須です。それ以外の HTTP は
`allow_insecure_http` の明示設定と追加の警告確認が必要です。userinfo や fragment を
含む URL や無効な header は拒否されます。transport-controlled または MCP protocol
予約済みの `Host`、`Content-Length`、MCP session/protocol header、`Accept`、
`Content-Type` は上書きできません。`Authorization` などの認証 header は設定できますが、
output では常にマスクされます。
manager で新しく入力する header 値には `${ENV_NAME}` 参照が必須です（例: `Bearer ${MCP_TOKEN}`）。
literal credential は拒否されます。

設定 header は元の same-origin endpoint にだけ送信されます。cross-origin redirect と
HTTPS から HTTP への downgrade redirect は拒否されます。CLI と JSON status は
マスク済み endpoint と header 名だけを出し、header 値や URL query 値は出しません。

## 発見とツール選択

設定を変更せずカタログを確認します。

```bash
ocr mcp discover docs
ocr mcp discover docs --json
```

発見は MCP 初期化と pagination された `tools/list` だけを行います。`tools/call`、
モデルへのツール登録、カタログ保存は行いません。TTY では再確認し、非対話環境では
`--yes` が必須です。

対話 editor または正確な名前でツールを選びます。

```bash
ocr mcp tools docs
ocr mcp tools docs --enable search_docs --enable get_page --yes
ocr mcp tools docs --disable get_page --yes
```

新しく有効にしたツールの既定権限は `ask` です。無効化すると権限と fingerprint も
削除されます。発見の上限は 64 pages、512 tools、catalog 全体 4 MiB、説明 8 KiB、
input schema 256 KiB です。不正、重複、過大なカタログは fail closed になります。

説明、schema、title、icon、annotation はすべて信頼できない server metadata です。
OCR はそれらを sanitize してサイズ制限します。`readOnlyHint` などは未検証の
server-provided hint として表示できるだけで、認可には使われません。

## 権限

| 値 | 意味 |
|---|---|
| `deny` | hard stop。global/server の deny は下位から上書きできません。 |
| `ask` | 条件を満たすツールを対話 review だけで表示し、未 cache の呼び出し前に確認します。 |
| `allow` | 条件を満たすツールの確認だけを省略します。ツール自体は有効にしません。 |
| `inherit` | server/tool のみ。より広い scope の値を継承します。 |

上位に `deny` がなければ、最も具体的な非 `inherit` 値を使います。

```text
tool > server > global（既定: ask）
```

永続的な `allow` は permissions command からのみ保存でき、有効なツールに最新の
discovery fingerprint がある場合に限ります。

```bash
# Global policy と approval timeout
ocr mcp permissions
ocr mcp permissions --default ask --timeout 60 --yes

# Server と正確な tool override
ocr mcp permissions docs
ocr mcp permissions docs --default inherit \
  --tool search_docs=ask --tool get_page=allow --yes
```

`approval_timeout_seconds` は global に変更でき、既定値は 60 秒、許容範囲は 1–600 秒です。

```bash
ocr config set mcp.approval_timeout_seconds 120
```

変更は次回の `ocr review` から有効です。

### 実行時の確認

`ask` prompt は server、remote tool 名、model alias、信頼できない説明、fingerprint prefix、
上限付きでマスク済みの引数 preview を表示します。選択肢は次の 4 つです。

- **Allow once**
- **Allow this review**
- **Deny once**（既定）
- **Deny this review**

review scope の判断は正確な server/tool identity を key にし、同じ review の並行 group
で共有されますが、process 終了時に消えます。実行時確認は設定を変更しません。

## ツール identity と定義変更

モデルには remote の生名ではなく qualified alias が渡されます。

```text
mcp__<server-slug>__<tool-slug>__<16-hex>
```

hash は大文字小文字を区別する server config key と remote tool 名を結びます。OCR は
immutable な alias-to-tool mapping を保持し、alias を解析して identity を復元しません。
組み込み名、別 MCP alias、registry との衝突が 1 つでもあれば、その review の全 MCP 登録が
中止され、部分的にも公開されません。組み込みツールは引き続き利用できます。

設定保存時には同時編集を検出します。フォームを開いている間に別ウィンドウが保存した場合、
古い下書きは拒否されます。設定を開き直してください。非公開の `config.json.lock` は OCR の
書き込み調整用で、認証情報を含みません。実行中は削除しないでください。旧版や手動編集は
このロックに参加しないため、同時編集を避けてください。

選択済みツールには、接続 identity、名前、sanitized 説明、canonical input schema から
作った SHA-256 fingerprint も保存されます。fingerprint が欠落または変化すると
`needs-review` となって非表示になり、`ocr mcp tools` または `ocr mcp edit` で新しい定義を
受け入れる必要があります。annotation と解決済み secret は fingerprint に含めません。

## CI と非対話環境

stdin と stderr の両方が terminal、`TERM` が `dumb` ではなく、一般的な CI 変数が true
でない場合だけ対話可能と判断します。`0`、`false`、`no`、`off` は false と扱います。
GitHub Actions、GitLab CI、Azure Pipelines、Buildkite、Jenkins、一般の `CI` は
fail closed です。

- `ask` ツールは非表示になり、実行できません。
- 明示的に有効、fingerprint 一致、`allow` のツールだけが非対話環境で実行可能ですが、
  MCP call の直前に独立 authorizer を必ず通ります。
- subcommand なしの `ocr mcp` はマスク済み status/help だけを表示し、接続も保存もしません。
- 接続または変更を行い得る command は完全な flags と `--yes` が必要です。

CI の永続 `allow` は最小の server/tool scope に限定し、credentials の権限を絞り、起動する
server package の version を pin するなど信頼を確立してください。

## 管理 command 一覧

```text
ocr mcp
ocr mcp add [name]
ocr mcp list [--json]
ocr mcp show <name> [--json]
ocr mcp edit <name>
ocr mcp discover <name> [--json] [--yes]
ocr mcp tools <name> [--enable TOOL ...] [--disable TOOL ...] [--yes]
ocr mcp permissions [name]
ocr mcp enable <name> [--yes]
ocr mcp disable <name> [--yes]
ocr mcp remove <name> [--yes]
```

`list` と `show` は接続しません。TTY の `remove` は No が既定で、非対話環境では
`--yes` が必要です。`enable` は server switch だけを変更し、allowlist は広げません。

## 設定と移行

manager は versioned policy を `~/.opencodereview/config.json` に保存します。

global key は `mcp.version`、`mcp.enabled`、`mcp.default_permission`、
`mcp.approval_timeout_seconds` で、各接続は `mcp_servers` の下に置かれます。

```json
{
  "mcp": {
    "version": 1,
    "enabled": true,
    "default_permission": "ask",
    "approval_timeout_seconds": 60
  },
  "mcp_servers": {
    "docs": {
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "@acme/docs-mcp-server"],
      "env": ["DOCS_TOKEN=${DOCS_TOKEN}"],
      "enabled": true,
      "default_permission": "inherit",
      "tools": ["search_docs"],
      "tool_permissions": {"search_docs": "ask"},
      "tool_definition_sha256": {"search_docs": "<64-lowercase-hex>"}
    }
  }
}
```

通常の review は legacy 設定を読みますが自動書き換えしません。fingerprint のない旧式の
非空 `tools` は対話 review の `ask` に制限され、CI で自動実行できません。空または欠落した
旧 `tools` は何も公開しません。manager で初めて正常保存したときに version 1 と受け入れた
fingerprint が書かれます。

旧 `setup` field は `ocr review` から実行されません。その server を manager で保存すると
field は削除され、手動 install/build の案内が出ます。安全用 field を理解しない旧 OCR は
移行後の設定を編集しないでください。

設定は同一 directory の mode `0600` temporary file に flush し、atomic replace されます。
wizard の取消や discovery failure は元の設定を変更しません。

`0600` は Unix 系のシステムに適用されます。Windows では Unix の権限ビットではなく、
ディレクトリから継承した ACL が使われます。設定ディレクトリへのアクセスは自分のアカウントに制限してください。

## トラブルシューティング

- **`needs-review`**: `ocr mcp tools <name>` で現在の定義を確認し、権限を選び直します。
- **CI に MCP ツールがない**: `ask` は仕様どおり非表示です。必要なら最新 fingerprint と
  最小 scope の永続 `allow` を設定します。
- **発見に失敗する**: 表示された executable/引数またはマスク済み origin、必要な環境変数、
  TLS、MCP compatibility を確認します。
- **結果が大きすぎる**: aggregate 1 MiB を超える結果は拒否されます。query を絞ります。
- **server error**: MCP `isError` は成功 telemetry ではなく実行失敗です。
- **名前衝突**: server config key または remote tool 名を変更します。first-wins は使いません。

## 関連項目

- [設定](../configuration/) — すべての設定 key。
- [CLI リファレンス](../cli-reference/) — command と flag。
- [CI 統合](../integrations/ci/) — 無人 review の設定。
- [ツール](../tools/) — 組み込み review ツール。

## 端末の接続ウィザード

`ocr mcp add` は項目別入力と `Space` によるツール選択に対応します。`Enter` で次へ、`Ctrl-B` で戻り、`Esc` で中止します。権限とタイムアウト（既定 60 秒、1–600 秒）は `ocr mcp permissions` で設定します。`OCR_CONFIG_PATH` は設定の読み書きと review に同じファイルを指定します。`ocr mcp tools docs --disable write` だけなら接続も `--yes` も不要で、オフラインで権限を取り消せます。有効化には発見と接続への明示的な同意が必要です。

## 履歴と raw ログ

MCP ツールが公開される review では、`OCR_RAW_LOGGING=1` でも raw 記録を無効にします。機密ツールの引数は履歴でマスクし、引数を含み得る native payload と付随するテキスト・推論を保存しません。モデルの実行用データは維持します。子プロセスには SDK の終了タイムアウトを使い、review 終了時は待機時間を制限して並列に接続を閉じます。
