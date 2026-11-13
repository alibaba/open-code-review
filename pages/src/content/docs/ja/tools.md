---
title: ツール
sidebar:
  order: 9
---

OCR には、レビュー中に LLM が呼び出すための **6 つのツール**が組み込まれています。本ページでは、各ツールの用途、入力
schema、および入出力の例を記載します。基本となる機械可読の定義は
[`internal/config/toolsconfig/tools.json`](https://github.com/alibaba/open-code-review/blob/main/internal/config/toolsconfig/tools.json)
にあります。

## 各フェーズでのツールの利用可否

タスクテンプレートは共有定義から名前でツールを選択します。`task_template.json` の
`MAIN_TASK.tools` はモデルに送る呼び出し可能なツールの順序付きリストです。
`PLAN_TASK.tool_references` は計画 prompt に参考テキストとしてのみ展開されます。
計画は呼び出し可能なツールを持たない単一リクエストのままで、どちらのフィールドも定義を変更しません。

`scan_template.json` は main task のツールを独立して選択し、全ファイルスキャンには diff がないため
`file_read_diff` を含めません。省略または空のリストはツールを選択せず、カタログ全体を有効にはしません。
重複する名前は最初の出現順で一度だけ含まれます。未知の参照、重複するカタログ項目、無効な定義は
タスクの初期化時にエラーになります。

デフォルトの diff レビューでの選択は次のとおりです：

| ツール           | Plan | Main | 用途                                                             |
| ---------------- | ---- | ---- | ---------------------------------------------------------------- |
| `task_done`      | ✗    | ✓    | 「完了しました」を示す——ループを終了します。                     |
| `code_comment`   | ✗    | ✓    | 行範囲 + 提案を伴うレビューコメントを 1 件発行します。           |
| `file_read`      | ✓    | ✓    | 選択した比較側にあるファイルの一部を読み取ります。             |
| `file_read_diff` | ✓    | ✓    | 別のファイルの diff を読み取り、ファイル横断の懸念を確認します。 |
| `file_find`      | ✓    | ✓    | ファイル名のキーワードでファイルを特定します。                   |
| `code_search`    | ✓    | ✓    | リポジトリ全体の grep（リテラルまたは正規表現）。                |

`task_done` と `code_comment` は plan フェーズでは意図的に**利用できません**：plan は読み取り専用です。

> **コンテキストツールは読み取り専用のコンテキストであり、コメント対象ではありません。** `main_task` prompt は、
> *他の*ファイル内の発見についてコメントすることを明確に禁止しています。`file_read`、`file_read_diff`、`file_find`、
> `code_search` が存在するのは、モデルが現在のファイルの diff をよりよく理解するためであり——そのコンテキストを収集する際に
> 見つかったいかなる問題も、設計上無視されます。ファイル横断の懸念は、**現在のファイルの diff** で観測可能な場合にのみ
> コメントとして現れます。

組み込みツールの定義は OCR とともに配布され、実行時には上書きできません。
プロジェクトの指針には[レビュールール](../review-rules/)、外部の業務コンテキストには
[MCP](../mcp/) を使用してください。カスタムツールファイルを使用していた場合は
[移行ガイド](#customizing-tools)を参照してください。

## 比較する側 {#comparison-sides}

commit および range レビューでは、再開したセッションを含め、`file_read`、`code_search`、`file_find`
に任意の `side` パラメーターが追加されます。省略するか `"new"` を指定すると、
既定の動作と出力形式を維持します。`"old"` を指定すると、同じ比較の旧側で
ファイルの読み取り、検索、列挙ができます。

```json
{"file_path": "src/foo.go", "start_line": 10, "end_line": 80, "side": "old"}
{"search_text": "OldAPI", "file_patterns": ["src/"], "side": "old"}
{"query_name": "foo.go", "side": "old"}
```

| レビュー入力 | 新側 | 旧側 |
| --- | --- | --- |
| `--from BASE --to HEAD` | 固定された head コミット | 固定した両端の merge-base。対象ブランチの最新コミットではない |
| `--commit COMMIT` | 固定されたコミット | 最初の親コミット。マージコミットも同様 |
| ルートコミット | 固定されたコミット | 空のツリー |

diff とこれら 3 つのツールは、同じ不変の比較端点を共有します。パスと行番号は
選択した側に属します。名前を変更したファイルは旧側では旧パスを指定してください。
ツールは名前を自動変換せず、別バージョンへフォールバックもしません。削除済みの
ファイルも旧側から読み取れます。ルートコミットの旧側にはファイルがありません。
読み取りはファイルが存在しないことを報告し、検索と列挙は空の結果を返します。
Git オブジェクトが利用できない場合や Git 操作の失敗はエラーであり、空のツリーとは扱いません。
浅いクローンの境界にあるコミットをレビューするには、親オブジェクトがローカルに
存在していても、先に必要な履歴を取得してください。これにより Git diff と旧側の
ツールが同じ比較基準を使用します。

旧側の結果は `SIDE: old` と `REVISION: <commit-sha>` で始まります。空のツリーでは
`SOURCE: empty-tree` を使用します。旧側のエラーも参照元を示します。既存の行数と
一致件数の制限は両側に適用されます。受け付ける値は文字列 `"new"` と `"old"` のみで、
null、空文字列、任意の Git ref は無効です。

ワークスペースのレビューとスキャンは、既存の定義、prompt、既定のツール出力形式を
維持します。`side` は公開せず、指定された場合は拒否します。共有の `tools.json`
カタログは current-only 定義のソースであり、`side` を宣言しません。比較レビューでは
初期化時に `WithComparisonSides` でカタログから呼び出し用の定義と計画用の参照を派生し、
基本定義を変更せずに `side` の追加と説明の更新を行います。

commit または range レビューの再開時には現在の組み込みツールを使用し、選択された
patch が変わっていなくても、解決済みの新旧端点が元と一致する必要があります。
どちらのツリーでも、他の箇所のコンテキストが以前のレビューに影響した可能性があるためです。
異なる ref 名でも同じ端点に解決されれば再開できます。いずれかの端点が変わった場合や、
旧セッションに比較を検証するための端点情報が不足している場合は、新しいレビューを
開始してください。完了済みファイルの結果は再利用し、未完了のファイルは現在のツールで
レビューします。すべてのファイルを現在の機能で再確認するには、新しいレビューを開始してください。

## `task_done`

main ループを終了します。

```json
{
  "name": "task_done",
  "input": { "state": "DONE" }
}
```

| フィールド | 必須 | 意味                                                                                                                                             |
| ---------- | ---- | ------------------------------------------------------------------------------------------------------------------------------------------------ |
| `state`    | はい | `DONE`（デフォルト）または `FAILED`。`FAILED` は「利用可能なツールでは本当に完了できない」ことを示します——ほとんど常に正しい選択ではありません。 |

agent は `task_done` を受け取ると、LLM の呼び出しを停止し、累積した `code_comment`
呼び出しの処理を開始します。`task_done` は（結果がセッションログに記録される前に）即座に返されるため、`state` の値は受理されますが
**永続化されず**——終了コードにも影響しません。

## `code_comment`

1 件または複数のレビューコメントを発行します。各コメントはコードスニペット（`existing_code`）にアンカーされ、
OCR が行番号を自動計算できるようにします。

### Schema

```json
{
  "name": "code_comment",
  "input": {
    "path": "string — optional, override the file path for this comment",
    "comments": [
      {
        "content": "string — the comment in the configured language",
        "existing_code": "string — snippet from the diff to anchor on",
        "suggestion_code": "string — optional fix snippet",
        "thinking": "string — optional, the model's reasoning for this comment"
      }
    ]
  }
}
```

`comments` は配列なので、モデルは 1 回のツール呼び出しで複数のコメントを発行できます。`content` と
`existing_code` は必須です。`suggestion_code` は任意ですが、提供が推奨されます。`path` はトップレベルの任意の上書きで——
省略すると、agent は現在レビュー中のファイルを注入します。モデルが省略しても agent が自動的に `path` を注入するため、
モデルが明示的に設定する必要はほとんどありません。`thinking`（コメントごと）はモデルの推論を捕捉し、コメントに保持されます。
OCR はモデルが現在のターンで出力した推論内容で自動的に補完します（推論内容がない場合は空のまま）。JSON 出力に含めます（ターミナル出力には表示されません）。

> **`thinking` はランタイム専用フィールドです。** OCR はこれを解析して保存しますが、モデルに渡す
> `code_comment` schema には意図的に**含めていません**（`tools.json` には `content`、
> `existing_code`、`suggestion_code` のみがあります）。より高性能なモデルが依然として `thinking` ブロックを発行した場合も
> 永続化されます。ほとんどのモデルは発行しませんが、問題ありません。

### アンカーアルゴリズム

OCR は**動的なスライディングウィンドウ**を使って diff 内で `existing_code` テキストを検索します。マッチは順に試行されます：

1. **hunk の新側**——連続する **context + added** 行（deleted のみでも unchanged のみでもない）の一群で、
   新ファイルの行番号を得ます。失敗した場合、OCR は **hunk の旧側**——context +
   deleted 行——を再試行し、旧ファイルの行番号を得ます。
2. **ファイル全体の新規スキャン**——hunk マッチがない場合、OCR は変更後のファイル全体を 1 行ずつスキャンして連続マッチを探します
   （`resolveFromFileContent`）。
3. **再位置特定タスク**——より複雑な diff でテキストマッチが依然として失敗した場合、OCR は
   `RE_LOCATION_TASK` prompt を実行し、モデルにスニペットの再アンカーを依頼します。

マッチは**空白に対して非依存**です：比較前に行を trim し、diff の `+`/`-` マーカーを取り除くため、インデントが
正確に一致する必要はありません。最後の手段として、コメントは `start_line=0` で配信され、ユーザーに「問題は本物だが自分で位置を特定してほしい」と伝えます。

### 例

```json
{
  "comments": [
    {
      "content": "`tx.Rollback()` is never deferred — early returns leak the transaction.",
      "existing_code": "tx, err := db.Begin()\nif err != nil {\n    return err\n}",
      "suggestion_code": "tx, err := db.Begin()\nif err != nil {\n    return err\n}\ndefer tx.Rollback()"
    }
  ]
}
```

## `file_read`

既定ではファイルの**変更後**の形式で一定範囲の行を読み取ります。
commit および range レビューでは、任意で[比較する側](#comparison-sides)も指定できます。

### Schema

```json
{
  "name": "file_read",
  "input": {
    "file_path": "src/foo.go",
    "start_line": 10,
    "end_line": 80
  }
}
```

| フィールド   | 必須   | デフォルト   | 説明                             |
| ------------ | ------ | ------------ | -------------------------------- |
| `file_path`  | はい   | —            | リポジトリルートからの相対パス。 |
| `start_line` | いいえ | `1`          | 1 始まりのインデックス。         |
| `end_line`   | いいえ | ファイル末尾 | 端点を含む。                     |

### 出力

```
File: src/foo.go (Total lines: 220)
IS_TRUNCATED: false
LINE_RANGE: 10-80
10|package foo
11|
12|import (
13|    "fmt"
…
```

各行の内容には、1 始まりの行番号と `|` 区切り文字が前置され、モデルが後続の `code_comment` 呼び出しで
行番号を正確に参照できるようにします。

### 制限

- **1 回の呼び出しで最大 500 行。** より大きな範囲は切り詰められ、`IS_TRUNCATED: true` が設定され、
  `Note: Results truncated to 500 lines. Please narrow your line range.` が追記されます。
- 既定ではファイルの**変更後バージョン**を読み取ります。commit および range レビューでは、
  `side: "old"` で旧側の完全な内容を読み取れます。変更箇所を見るには `file_read_diff` を使用します。

モデルが周辺のコンテキストを必要とする場合（diff 内でしか見えない関数についてコメントする際など）、diff の
hunk ヘッダー `@@ -x,y +m,n @@` から範囲を計算すべきです——通常は `m-50` から `m+n+50` まで。

## `file_read_diff`

同一の変更セット内の 1 つまたは複数の*他の*ファイルの diff を読み取ります——コメントが関連ファイルの
更新有無に依存する場合に有用です。

### Schema

```json
{
  "name": "file_read_diff",
  "input": {
    "path_array": ["src/api/handler.go", "src/db/queries.go"]
  }
}
```

### 出力

```
==== FILE: src/api/handler.go ====
--- a/src/api/handler.go
+++ b/src/api/handler.go
@@ -10,1 +10,2 @@
- old line
+ new line 1
+ new line 2

==== FILE: src/db/queries.go ====
@@ -5,1 +5,1 @@
- query := "SELECT *"
+ query := "SELECT id"
```

変更セットに含まれていないパスは静かに捨てられるわけではありません。
`Error: diff not found for: <path>` の中で名前が挙げられ、見つかったパスの diff は
そのエラーより先に返されます。他のファイルの diff がある実行では、メッセージはそれらを
列挙し（`Diffs exist for: ...`）、スペルミスのパスを修正できるようにします。diff が
まったくない実行では、代わりに `file_read` を使うよう案内します。空の `path_array` は
`Error: no files found` を返します。

## `file_find`

リポジトリ相対パスまたはファイル名のキーワード（部分文字列マッチ）でリポジトリ内のファイルを検索します。

### Schema

```json
{
  "name": "file_find",
  "input": {
    "query_name": "UserService",
    "case_sensitive": false
  }
}
```

| フィールド       | 必須   | デフォルト | 説明                                                                                                                      |
| ---------------- | ------ | ---------- | ------------------------------------------------------------------------------------------------------------------------- |
| `query_name`     | はい   | —          | 各ファイルの**リポジトリ相対パス**に対して部分文字列マッチを行います（`/` と `\` の両方のパス区切り文字を受け付けます）。 |
| `case_sensitive` | いいえ | `false`    | `true` に設定すると大文字小文字を厳密に区別してマッチします。                                                             |

候補セットは、ワークスペースモードでは `git ls-files --cached --others --exclude-standard` から、
区間 / commit モードでは `git ls-tree -r --name-only <ref>` から得られます。拡張子のないファイルは
スキップされますが、`Makefile`、`Dockerfile`、`LICENSE`、`Vagrantfile`、
`Containerfile`、`BUILD`、`WORKSPACE` は例外です。

### 出力

改行区切りのパスのリスト：

```
src/main/java/com/example/UserService.java
src/test/java/com/example/UserServiceTest.java
src/main/java/com/example/internal/UserServiceImpl.java
```

マッチするファイルがない場合（または `query_name` が空の場合）、ツールはリテラル文字列
`// The file was not found` を返します。

### 制限

最大 **100** 件のマッチを返します。超過分は静かに切り詰められます。モデルがより広範な検索を必要とする場合は、
`code_search` を使用すべきです。

## `code_search`

リポジトリ全体の全文検索。`git grep` によって駆動されるため、`pathspec` 構文を理解し、
`.gitignore` に従います。

### Schema

```json
{
  "name": "code_search",
  "input": {
    "search_text": "TODO|FIXME",
    "file_patterns": ["*.go", ":(exclude)vendor/"],
    "case_sensitive": false,
    "use_perl_regexp": true
  }
}
```

| フィールド        | 必須   | デフォルト     | 説明                                                             |
| ----------------- | ------ | -------------- | ---------------------------------------------------------------- |
| `search_text`     | はい   | —              | リテラル文字列または PCRE パターン（`use_perl_regexp` を参照）。 |
| `file_patterns`   | いいえ | リポジトリ全体 | pathspec エントリの配列。除外には `:(exclude)pat` を使用します。 |
| `case_sensitive`  | いいえ | `false`        | —                                                                |
| `use_perl_regexp` | いいえ | `false`        | `true` の場合、`search_text` は正規表現として扱われます。        |

### 出力

結果はファイルごとにグループ化されます。各グループは `File: <path>` と `Match lines: <n>` で始まり、続いて各ヒットが
`line|content` の 1 行で表されます：

```
File: path/to/example.java
Match lines: 2
433|      String name = toolRequest.get().getName();
438|      logToolRequest(newPath, tool, toolRequest.get());

File: path/to/other.java
Match lines: 1
22|      var req = new ToolRequest();
```

マッチがない場合、ツールはリテラル文字列 `No matches found` を返します。

### pathspec クイックリファレンス

| 目的                      | `file_patterns`                                                    |
| ------------------------- | ------------------------------------------------------------------ |
| 単一ファイル              | `["src/main.go"]`                                                  |
| すべての Go ファイル      | `["*.go"]`                                                         |
| テストを除くすべての Go   | `["*.go", ":(exclude)*_test.go"]`                                  |
| 単一のディレクトリのみ    | `["src/api/"]`                                                     |
| 複数の種類、vendor を除外 | `["*.go", "*.ts", ":(exclude)vendor/", ":(exclude)node_modules/"]` |

### 制限

- `git grep --max-count 100` によってファイルごとのヒット数上限を **100** に設定するため、複数ファイルにまたがる
  合計出力は 100 を超える可能性があります。ファイルごとの上限に達した場合、出力の前に
  `Note: The results have been truncated. Only showing first 100 results.` が付加されます。
- 空 / 空白のみの `search_text` は、各行に展開されるのではなく `Error: search_text is blank` を返します。
- ワークスペースモードは**現在のワークツリー**を検索し、区間 / commit モードは解決された対象の ref を検索します
  （`FileReader.Ref` が位置引数として `git grep` に渡されます）。

## ツールの実行とエラー

ツールは agent ループ内で同期的に実行されますが、2 つの例外があります：

- `code_comment` は **CommentWorkerPool** にディスパッチされ、ループが行の解析 + リフレクションでブロックしないようにします。
- `task_done` はショートサーキットします——即座に返され、いかなる provider も呼び出しません。

ツールがエラーになった場合（ネットワーク障害、引数の形式エラー、ファイル未検出）、結果は通常のツール結果としてモデルに配信され、
テキストは `"Error: file not found: src/missing.go"` のような形になります。モデルはその後、再試行するか、ファイルを変更するか、
`task_done` を呼ぶかを決定します。

ツール名がレジストリに存在しない場合、OCR はクラッシュせず定数 `tool.NotAvailableMsg` を返し、
そのツールが利用できないことをモデルに伝えます。

## ツールのカスタマイズ制限と移行 {#customizing-tools}

OCR は専用のレビュー Agent を使用します。コアのワークフローと組み込みツールは、
ユーザー定義のレジストリではなく、テストとベンチマークで一体として検証します。

**移行ガイド：** `ocr review` と `ocr scan` の両方から `--tools` を削除しました。
従来の呼び出しは `unknown flag: --tools` で失敗します。スクリプト、CI ジョブ、
shell エイリアスから、このフラグとファイル引数を削除してください。

- プロジェクト固有の指針には[レビュールール](../review-rules/)または `--background`
  を使用します（`review` は `--background-file` もサポートします）。
- 要件やナレッジベースなどの外部業務コンテキストには [MCP サーバー](../mcp/)
  を設定します。現時点では `review` のみが MCP に接続し、`scan` は未対応です。
  MCP は同名の組み込みツールを置き換えられません。
- 組み込みツールの無効化、説明、schema、フェーズ別の公開設定の変更に、実行時の代替手段はありません。
  コアの変更にはソース編集、再ビルド、適切なテストとベンチマーク検証が必要です。

`--max-tools` は引き続きツール呼び出しのラウンド数を制御します。
MCP 設定の `mcp_servers.<name>.tools` は外部ツールを絞り込むもので、削除したフラグとは無関係です。
既存のセッションファイルは読めますが、カスタムツール定義を使用したセッションは再開せず、
新しいレビューを開始してください。

## 関連項目

- [アーキテクチャ](../architecture/)——agent ループがどのようにツールを駆動するか。
- [レビュールール](../review-rules/)——LLM に何に注目すべきかを伝えます。
- [セッションビューア](../viewer/)——過去のレビューで実際にどのツールがトリガーされたかを確認します。
