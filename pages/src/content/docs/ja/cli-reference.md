---
title: CLI リファレンス
sidebar:
  order: 6
---

各 `ocr` サブコマンド、引数、終了時の挙動に関する完全なリファレンスです。

## グローバルな使い方

```text
OpenCodeReview - AI-Powered Code Review CLI

Usage:
  ocr [command]

Commands:
  review, r    Start a code review
  rules        Inspect and debug review rules
  config       Manage configuration settings
  llm          LLM utility commands
  viewer       Start the WebUI session viewer
  session, sessions  List and inspect saved review sessions
  version      Show version information

Examples:
  ocr review --from master --to dev        Review diff range
  ocr review --commit abc123               Review a single commit
  ocr review --background "Focus on auth" --background-file ./docs/requirements.md  Review with context
  ocr review -B ./docs/requirements.md                                              Review with context file
  ocr config provider                      Interactive provider setup
  ocr config model                         Interactive model selection
  ocr config set llm.model opus-4-6        Set a config value
  ocr llm test                             Test LLM connectivity
  ocr llm providers                        List built-in providers
  ocr session list                         List saved review sessions
  ocr version                              Show version info

Use "ocr review -h" for more information about review.
Use "ocr rules -h" for more information about rules.
Use "ocr config" for more information about config.
Use "ocr llm" for more information about LLM utilities.
Use "ocr session -h" for more information about session inspection.

GitHub: https://github.com/alibaba/open-code-review
```

## グローバルフラグ

すべてのコマンドで利用でき、サブコマンドの前後どちらでも指定できます
(`ocr --color=never review` と `ocr review --color=never` は同じ意味です)。

| フラグ | デフォルト | 説明 |
|---|---|---|
| `--color <auto\|always\|never>` | `auto` | ANSI カラーを出力する条件。`auto` は stdout が端末のときだけ着色するため、パイプやリダイレクトではプレーンテキストになります。`always` はパイプ越しでも着色を維持します (`\| less -R` などに便利)。 |

stdout が端末でない場合、テキスト出力は常にプレーンになるため、安全にパイプできます:

```bash
ocr review --commit HEAD | gh issue comment 123 --body-file -
```

`TERM=dumb` でもカラーは無効になります。

## コマンド一覧

| コマンド | エイリアス | 役割 |
|---|---|---|
| `ocr review` | `ocr r` | コードレビューを実行してコメントを出力します。 |
| `ocr scan` | `ocr s` | Git diff を必要とせず、ファイル全体をスキャンします。 |
| `ocr rules check <file>` | — | あるファイルパスにどのルールが適用され、その出所はどこかを表示します。 |
| `ocr config set <key> <value>` | — | 設定値を `~/.opencodereview/config.json` に永続化します。 |
| `ocr config unset <key>` | — | 保存済みの設定値をクリアします（`provider`、`max_tokens`、`effort`、`custom_providers.<name>`、`mcp_servers.<name>`）。 |
| `ocr config provider` | — | 対話的なプロバイダー設定 TUI。 |
| `ocr config model` | — | 対話的な model 選択 TUI。 |
| `ocr llm test` | — | 短い chat リクエストを送信し、設定されたエンドポイントを検証します。 |
| `ocr llm providers` | — | 組み込みの LLM プロバイダーをすべて一覧表示します。 |
| `ocr session list` | `ocr sessions list`, `ocr session ls` | 保存されたレビューセッションを一覧表示します。 |
| `ocr session show <id>` | `ocr sessions show <id>` | 1つのセッションとファイル単位のチェックポイントを表示します。 |
| `ocr session comments <id>` | `ocr sessions comments <id>` | 1つのセッションに記録されたレビューコメントを表示します。 |
| `ocr session compare <before> <after>` | `ocr session diff <before> <after>` | 2つのセッションの指摘を比較します：新規・継続・解決済み・未レビュー。 |
| `ocr viewer` | — | 過去のレビューセッション用のローカル Web UI を起動します（`localhost:5483`）。 |
| `ocr version` | — | バージョン、commit、プラットフォーム、ビルド日、GitHub URL を出力します。 |

`ocr` および `ocr -h` はトップレベルの使い方を出力します。各サブコマンドも `-h` / `--help` を受け付けます。

## `ocr review`

メインコマンドです。Git diff を解析し、意味的に関連するファイルをグループにまとめ、グループごとにサブエージェントをディスパッチし、レビューコメントを収集して出力します。

### 概要

```text
ocr review [flags]
ocr r      [flags]   (alias)
```

引数を何も渡さない場合、OCR は**ワークスペースモード**で動作します。カレントディレクトリのリポジトリ内にある staged + unstaged + untracked のすべての変更をレビューします。

### 引数

| 引数 | 短縮形 | デフォルト | 説明 |
|---|---|---|---|
| `--repo <path>` | — | カレントディレクトリ | Git リポジトリのルート。 |
| `--from <ref>` | — | — | diff の開始 ref（例: `main`）。 |
| `--to <ref>` | — | — | diff の終了 ref（例: `feature-branch`）。設定すると OCR は `merge-base(from, to)..to` を計算します。 |
| `--commit <sha>` | `-c` | — | 単一の commit をレビューします（その親との差分）。 |
| `--preview` | `-p` | `false` | フィルタリングのパイプラインを実行しますが LLM はスキップします。ファイル一覧と除外理由を出力します。`--format json` に対応しています。`--format sarif` はサポートされていません（プレビューには出力する完了した指摘がありません）。 |
| `--no-filter` | — | `false` | すべてのレビューコメントを保持し、ファイルごとの `REVIEW_FILTER_TASK` LLM 後処理呼び出しをスキップします。 |
| `--resume <session-id>` | — | — | 以前の互換性のある範囲または単一 commit レビューセッションから再開します。 |
| `--format <fmt>` | `-f` | `text` | `text`（人間が読みやすい形式）、`json`（機械可読なコメント配列）または `sarif`（GitHub Code Scanning 用の SARIF 2.1.0 レポート）。 |
| `--output <path>` | `-o` | 標準出力 | レビュー結果を UTF-8 ファイルに書き込みます（`-` は標準出力を表します）。初回書き込み時に遅延作成されるため、実行が失敗しても既存のファイルは変更されません。テキスト形式では ANSI カラーコードが自動的に削除されます。 |
| `--audience <who>` | — | `human` | `human` は進捗行をストリーム出力します（`--format` が `json`/`sarif` の場合は stderr に出力し、stdout は解析可能な単一ドキュメントのままになります）。`agent` は進捗行を完全に抑制し、最終サマリー / JSON のみを出力します。 |
| `--background <text>` | `-b` | — | plan + main prompt に注入する、任意の要件 / 業務コンテキスト。 |
| `--background-file <path>` | `-B` | — | レビューの背景として使用する Markdown ファイルのパス。`--background` も指定した場合は両方を結合します。 |
| `--exclude <patterns>` | — | — | 除外する gitignore 形式のパターン（カンマ区切り）。`rule.json` の excludes とマージされます。 |
| `--concurrency <n>` | — | `8` | 並行してレビューするファイルの最大数。 |
| `--timeout <minutes>` | — | `15` | ファイルごとの締め切り時間。`0` でタイムアウトを無効化します。effort ラウンド数に応じて線形にスケールします（例: low/medium/high で 15/30/45 分）。 |
| `--rule <path>` | — | — | カスタム JSON レビュールールファイルのパス。プロジェクトレベルおよびグローバルの `rule.json` を上書きします。 |
| `--max-tools <n>` | — | テンプレートのデフォルト | ファイルごとの最大ツール呼び出し回数。`0` はテンプレートのデフォルト（`100`）を使用します。1〜49 は `50` に引き上げられます。解決後の値はテンプレートのデフォルトを**上回る場合にのみ**適用されます（引き上げのみ可能で、引き下げはできません）。 |
| `--max-tokens <n>` | — | 設定またはテンプレートのデフォルト | ファイルごとの**プロンプト**トークン上限（review のデフォルトは `200000`）。この実行で保存済みの `max_tokens` 設定を上書きします。出力の上限には影響しません。そちらは `MAX_COMPLETION_TOKENS`（`16384`）が個別に制御します。 |
| `--max-tokens-budget <n>` | — | `0`（無制限） | レビュー全体の入力 + 出力トークン使用量を制限します。予算を超えると処理の割り当てを停止し、部分的な結果は引き続き公開されます。 |
| `--effort <level>` | — | 設定または `medium` | レビューの労力プリセット: `low` = main ループ 1 ラウンド、`medium` = 2 ラウンド（デフォルト）、`high` = 3 ラウンド。ラウンドが多いほど recall は上がりますが、時間とトークンも増えます。`ocr config set effort <level>` で永続化できます。 |
| `--provider <name>` | — | — | 今回の実行で設定済み provider を選択します。`providers` と `custom_providers` の両方の名前を使用できます。 |
| `--model <name>` | — | — | 今回の実行で解決済みの LLM model を上書きします（例: `claude-opus-4-6`）。 |
| `--max-git-procs <n>` | — | `16` | 並行 git サブプロセスの最大数。 |
| `--tools <path>` | — | 埋め込み | カスタム JSON ツール設定ファイルのパス。埋め込みのツール定義を上書きします。 |
| `--post-to-pr` | — | `false` | レビューの指摘を一致する GitHub pull request に投稿します。範囲モード（`--from` と `--to`）および GitHub token が必要です。 |
| `--github-token <token>` | — | — | pull-request コメントの投稿に使用する GitHub token。省略時は環境変数 `GITHUB_TOKEN` を使用します。 |

> モード引数は排他です: `--from`/`--to` を渡すか、`--commit` を渡すか、いずれも渡さない（ワークスペースモード）かのいずれかです。
> 混在させるとそのままエラーになります。
> `--resume` は範囲または単一 commit レビューのみ対応し、`--preview` とは併用できません。

### 実行単位の LLM 選択

`review` と `scan` はどちらも `--provider` と `--model` を受け付けます。
これらの上書きは現在の呼び出しだけに適用され、保存済み設定は変更しません:

```bash
ocr review --provider anthropic --model claude-opus-4-6 --format json
ocr scan --provider openai --model gpt-5.4 --format json
```

明示的な `--provider` は通常のソース解決より先に、保存済みの `providers` または
`custom_providers` からエントリを選択します。`--provider` を指定しない場合、OCR は従来の
ソース順序を維持します: 保存済み設定、完全な `OCR_LLM_*` 環境設定、完全な Claude Code
環境設定、shell rc ファイルの順です。`--model` は選ばれたソース内の model を上書きしますが、
ソース順序は変更しません。不完全な戦略は別の戦略と混合されず、次へフォールバックします。
選択された組み込み provider の認証情報は、対応する環境変数から引き続き取得できます。

### モード

#### ワークスペースモード（デフォルト）

```bash
ocr review
```

OCR は 2 つの git コマンドからワークツリーの変更を組み立てます:

- `git diff HEAD` で追跡済みの変更を取得します（staged + unstaged をまとめて `HEAD` と比較。空の場合は `git diff --staged` にフォールバック）
- `git ls-files --others --exclude-standard` で untracked ファイルを取得し、ディスクから読み込んでファイル全体の新規追加として扱います

これは通常、commit 前に確認したい内容そのものです。より小さな範囲が必要なら、選択的に stage してください。

#### 範囲モード

```bash
ocr review --from main --to feature-branch
```

OCR は `merge-base(main, feature-branch)..feature-branch` を計算するため、feature ブランチが*導入した* diff だけが表示されます。ブランチを切ったあとに `main` へ入った無関係な変更は含まれません。

##### 指摘を GitHub pull request に投稿する

`--post-to-pr` で投稿を明示的に有効化します。`--from` と `--to` の両方を指定する
範囲モードが必要です。token は `--github-token` で渡すか、CLI に `GITHUB_TOKEN`
から読み込ませます。

CLI は base ブランチとレビュー済み head commit が一致する唯一の open pull request
を検索するため、pull-request 番号を指定する必要はありません。位置を解決する際、OCR は
各指摘が変更ファイルの旧側と新側のどちらから得られたかを記録します。投稿時には
pull-request diff の行インベントリも作成します。取得元が新側で、範囲全体が検証済みの
完全な右側 diff hunk に含まれる指摘だけが inline コメントになります。旧側の指摘、
取得元の side 情報がない指摘、無効な範囲、完全な patch から検証できない範囲は
pull-request summary に保持されます。side が判明している場合、数値上の行範囲が重なる
だけでは指摘は曖昧になりません。

Inline コメントと summary は、決定的で安全に再試行できる batch として送信されます。
各書き込みの直前に、OCR は pull request が open のままであること、base ブランチと
head が一致すること、現在の base tip と head の merge-base が完了したレビューに記録
された正確な commit であることを確認します。base tip の移動は merge-base が変わらない
場合に限り許可されます。既存の inline または fallback シーケンスは、指摘を重複させず、
すでに summary へ fallback した inline batch を再試行せずに再開されます。

ローカルのレビュー出力は GitHub への配信前に生成されます。投稿処理が失敗した場合も
出力は保持されますが、スクリプトや CI が配信失敗を成功と誤認しないよう、OCR は非ゼロの
ステータスで終了します。

Token には対象リポジトリへのアクセスと pull request の読み書き権限が必要です。
fine-grained personal access token または GitHub App では **Pull requests: Read and
write**、classic personal access token では適切な `repo` または `public_repo` scope
を付与してください。GitHub Actions では `pull-requests: write` を設定します。この権限は
inline 投稿できない指摘に使う issue コメントにも適用されます。

GitHub.com では `GITHUB_SERVER_URL` と `GITHUB_API_URL` を未設定にするか、
`https://github.com` と `https://api.github.com` を使用します。GitHub Enterprise
Server では `GITHUB_SERVER_URL` に HTTPS の server URL を設定します。既定の API は
`https://HOST/api/v3` です。明示的な `GITHUB_API_URL` も HTTPS を使用し、server URL
と同じ host でなければなりません。

```bash
GITHUB_TOKEN="$TOKEN" ocr review --from main --to feature-branch --post-to-pr
```

#### Commit モード

```bash
ocr review --commit abc123
ocr review -c abc123
```

`git show abc123` が生成する diff（すなわちその commit が導入した変更）をレビューします。

### 中断したレビューの再開

すべての `ocr review` 実行は、`~/.opencodereview/sessions/` 配下にローカル
セッションログを保存します。正常終了したテキスト出力はレビュー結果に集中し、session ID
は表示しません。保存済みセッションは `ocr session list/show` で確認でき、
`--format json` では機械可読出力に `session_id` が含まれます。範囲または単一 commit
レビューが中断された場合は、保存済みセッションを一覧表示し、同じレビュー対象に一致するセッションから再開します:

```bash
ocr session list
ocr session show <session-id>
ocr session comments <session-id>
ocr review --from main --to feature-branch --resume <session-id>
ocr review --commit abc123 --resume <session-id>
```

再開は意図的に厳密です。今回の実行が親と同じ対象をレビューする場合にのみ、
チェックポイントが再利用されます:

- ワークスペースレビューは再開できません
- レビューモードが一致する必要があります: 範囲セッションを単一 commit として
  再開することはできません
- 解決後の入力が一致する必要があります。ref の*表記*は比較しません
  (`abc1234` と `abc1234def` は同じ commit を指します) が、同じ ref が別の
  diff に解決される場合、あるいはルールやフィルタが選択ファイル集合を変えた
  場合は、部分的に再利用するのではなく再開全体を拒否します
- provider や model の変更は `--provider` / `--model` で明示的に指定する必要が
  あります。設定ファイルや環境変数経由の変更は拒否されます
- 親の実行が run manifest を持っている必要があります。入力はこれと照合して
  検証されます。ファイルの dispatch 開始後は、Ctrl-C によってレビューが正常に
  キャンセルされて manifest が書き出されるため、完了済みの checkpoint は再開時に
  再利用できます。正常に終了できなかったプロセスと run manifest より古い
  セッションには manifest がありません
- 再利用されるのは、親の manifest が結果を確定したファイルだけです。manifest が
  裏付けないチェックポイントや読み取れないチェックポイントは、そのファイルが
  もう一度レビューされるだけで、他のファイルには影響しません
- `--preview` と `--resume` は併用できません

拒否された再開は何も残しません: セッションも manifest も作らず、LLM も呼びません。

### 出力

#### Text（デフォルト、`--audience human`）

レビュー実行中は進捗行をストリーム出力し、続いてコメントごとに 1 ブロックを出力します（`path:start-end` を含む暗色の Unicode 区切りヘッダー、100 桁で折り返されたコメント本文、そして（存在する場合は）提案された置換のカラー化されたインライン diff）。実行終了時には stdout の末尾にサマリーを出力します:

```
[ocr] 17 file(s) changed, reviewing 9 in /path/to/repo
[ocr] Skipping image.png — filtered by path/extension rules
[ocr]   ▶ file_read "src/foo.go"
[ocr]   ✔ file_read (12ms)
[ocr] Plan completed for src/foo.go
…

─── src/foo.go:42-47 ───
Concurrent map access without a lock — wrap with sync.RWMutex.

- m[k] = v
+ mu.Lock(); defer mu.Unlock(); m[k] = v

…
[ocr] Summary: 9 file(s) reviewed, 14 comment(s), ~21344 token(s) used (input: ~18012, output: ~3332), 1m12s elapsed
```

#### Text（agent、`--audience agent`）

コメントの出力は同じですが、内部的に静音化可能な stdout ライターを通じて進捗行が抑制されます（[`internal/stdout`](https://github.com/alibaba/open-code-review/blob/main/internal/stdout/stdout.go)）。CI / パイプライン内で別の agent に引き渡す場合に使用します。

#### JSON

```bash
ocr review --format json --audience agent
```

JSON ドキュメントは常に stdout を単独で使用します。デフォルトの `--audience human`
では、`[ocr]` 進捗行はレビュー実行中に **stderr** へストリーム出力されるため、長時間
の実行を確認しながら stdout をそのままパーサーへパイプできます:

```bash
ocr review --format json > result.json   # 進捗は端末に表示されたままです
ocr review --format json | jq .summary   # stdout は単一の JSON ドキュメントです
```

進捗行を完全に取り除くには `--audience agent` を、シェル側で破棄するには
`2>/dev/null` を使用してください。

```json
{
  "status": "success",
  "llm": {
    "provider": "anthropic",
    "model": "claude-opus-4-6"
  },
  "summary": {
    "files_reviewed": 9,
    "comments": 1,
    "total_tokens": 21344,
    "input_tokens": 18012,
    "output_tokens": 3332,
    "elapsed": "1m12s"
  },
  "comments": [
    {
      "path": "src/foo.go",
      "content": "Concurrent map access without a lock — wrap with sync.RWMutex.",
      "start_line": 42,
      "end_line": 47,
      "existing_code": "m[k] = v",
      "suggestion_code": "mu.Lock(); defer mu.Unlock(); m[k] = v",
      "thinking": "Looking at line 42, the map …"
    }
  ]
}
```

トップレベルのフィールド:

| フィールド | 説明 |
|---|---|
| `status` | `success`、`completed_with_warnings`、`completed_with_errors`、または `skipped`。 |
| `llm` | 解決された LLM の識別情報。正規化済みの `model` は常に含まれ、`provider` は名前付きの設定済み provider の場合だけ含まれます。 |
| `message` | 任意。人間が読みやすいサマリー（例: `"No comments generated. Looks good to me."`）。 |
| `summary` | 任意。実行の集計: `files_reviewed`、`comments`、`total_tokens`、`input_tokens`、`output_tokens`、`cache_read_tokens`（omitempty）、`cache_write_tokens`（omitempty）、`elapsed`。`skipped` の実行時は省略されます。 |
| `comments` | 常に存在しますが、空の場合があります。各コメントのフィールドは上記の例のとおりです。 |
| `warnings` | 任意。1 つ以上のサブエージェントが失敗した場合に存在します。各項目は影響を受けたファイルとエラーを記述します。 |
| `session_id` | 任意。永続化されたレビュー実行に含まれます。互換性のある範囲または単一 commit レビューを再試行する際に `ocr review --resume <session-id>` へ渡せます。 |
| `resume` | 任意。再開した実行で存在し、`resumed_from`、`reused_files`、`rerun_files`、`previous_model`、`current_model` を含みます。 |

レビュー対象のファイルがない場合、JSON モードは `skipped` の外殻を発行し、呼び出し側が「変更なし」と「発見なし」を区別できるようにします:

```json
{
  "status": "skipped",
  "message": "No supported files changed.",
  "llm": {
    "provider": "anthropic",
    "model": "claude-opus-4-6"
  },
  "comments": []
}
```

### 終了コード

| コード | 意味 |
|---|---|
| `0` | レビューが完了しました（コメントがゼロの場合や、致命的でない警告がある場合もあります）。 |
| `1` | 致命的エラー。引数の誤り、LLM エンドポイントを解決できない、すべてのファイルごとのサブエージェントが失敗した、などです。エラーテキストは stderr に出力されます。 |

致命的でない警告（個々のサブエージェントの失敗、あるファイルが token しきい値を超過、など）はインラインで出力されます。JSON モードでは `warnings` 配列に追加されます。

## `ocr scan`

Git diff を必要としないファイル全体のレビュー。作業ツリーから各ファイルの現在の内容を読み込み、LLM に送信します。馴染みのないコードベースや、意味のある diff がないディレクトリの監査に便利です。

```text
ocr scan [flags]
ocr s      [flags]   (alias)
```

`--path` を渡さない場合、リポジトリ全体をスキャンします。

### 引数

| 引数 | 短縮形 | デフォルト | 説明 |
|---|---|---|---|
| `--path <list>` | - | リポジトリ全体 | スキャン対象のリポジトリ相対ディレクトリまたはファイル（カンマ区切り、例: `internal/agent`、`internal/llm/client.go`）。 |
| `--exclude <patterns>` | - | - | 除外する gitignore 形式のパターン（カンマ区切り、例: `**/generated/*,*.pb.go`）。`rule.json` の excludes とマージされます。 |
| `--output <path>` | `-o` | 標準出力 | スキャン結果を UTF-8 ファイルに書き込みます（`-` は標準出力を表します）。初回書き込み時に遅延作成されるため、実行が失敗しても既存のファイルは変更されません。テキスト形式では ANSI カラーコードが自動的に削除されます。 |
| `--preview` | `-p` | `false` | LLM を呼び出さずにファイルを列挙・フィルタリングします。ファイルリスト、レビュー対象/除外数、総行数、ファイルごとの除外理由を出力します。`--format json` に対応しています。`--format sarif` はサポートされていません。 |

```bash
ocr scan --preview                              # スキャン対象を確認
ocr scan --path internal/agent                  # 単一ディレクトリをスキャン
ocr scan --path internal/agent,internal/llm/client.go
ocr scan --exclude '**/generated/*,*.pb.go'
```

完全なフラグリストは `ocr scan -h` を参照してください。

## `ocr session`

`~/.opencodereview/sessions/` 配下に保存されたローカルレビューセッションログを一覧表示・確認します。
session ID の確認、ファイル単位のチェックポイント状態の確認、中断した範囲または単一 commit
レビューの再開に使用します。

```text
ocr session <sub-command>
ocr sessions <sub-command>   (alias)

Sub-commands:
  list, ls        List recent review sessions for the current repo
  show <id>       Show one session's metadata and per-file items
  comments <id>   Show the review comments recorded in one session
```

### `ocr session list`

```bash
ocr session list
ocr session list --limit 50
ocr session list --json
```

| 引数 | デフォルト | 説明 |
|---|---|---|
| `--repo <path>` | カレントディレクトリ | セッションを一覧表示するリポジトリ。 |
| `--json` | `false` | セッションサマリーを JSON として出力します。 |
| `--limit <n>` | `20` | 一覧表示するセッション数を制限します。`0` は無制限です。 |

### `ocr session show`

再開した実行では、継続元の実行も表示されます。provider や model をまたいだ
再開の場合は、その切り替えも表示されます。

```bash
ocr session show <session-id>
ocr session show --json <session-id>
ocr session show --repo /path/to/repo <session-id>
```

| 引数 | デフォルト | 説明 |
|---|---|---|
| `--repo <path>` | カレントディレクトリ | セッションを確認するリポジトリ。 |
| `--json` | `false` | セッションのメタデータとファイル単位の項目を JSON として出力します。 |

### `ocr session comments`

セッションに保存されたすべてのレビューコメントを、`ocr review` のターミナル出力と
同じスタイル（パス、行範囲、重要度バッジ、提案 diff）で表示します。

```bash
ocr session comments <session-id>
ocr session comments --json <session-id>
ocr session comments --severity high <session-id>
ocr session comments --severity critical,high --category bug,security <session-id>
```

| 引数 | デフォルト | 説明 |
|---|---|---|
| `--repo <path>` | カレントディレクトリ | セッションを確認するリポジトリ。 |
| `--json` | `false` | コメントを JSON 配列として出力します。 |
| `--severity <list>` | すべて | 含める重要度をカンマ区切りで指定します（`critical`、`high`、`medium`、`low`）。 |
| `--category <list>` | すべて | 含めるカテゴリをカンマ区切りで指定します（例: `bug`、`security`）。 |

### `ocr session compare`

2つのセッションの指摘を4つに分類します：**new**（after セッションのみ）、
**persisting**（両方）、**resolved**（before セッションのみ）、
**not reviewed**（before セッションにあり、after セッションがそのファイルを
レビューしていないため解決済みとは数えないもの）。

照合はパス・カテゴリ・該当コード片で行い、行番号は使いません。そのため行が
ずれただけの指摘は persisting のままになります。

```bash
ocr session compare <before-session-id> <after-session-id>
ocr session diff <before-session-id> <after-session-id>
ocr session compare --json <before-session-id> <after-session-id>
```

2つのセッションは同じリポジトリのものである必要があります。異なる場合はエラー
になります。レビューモードが異なる場合は stderr に警告を出すだけなので、
`--json` の出力はそのままパイプできます。

| フラグ | デフォルト | 説明 |
|---|---|---|
| `--repo <path>` | カレントディレクトリ | 比較するセッションが属するリポジトリ。 |
| `--json` | `false` | 比較結果を JSON で出力します（`new`、`persisting`、`resolved`、`not_reviewed`）。 |

## `ocr rules`

ルールの自己確認です。サブコマンドは 1 つだけです:

```text
ocr rules check [flags] <file-path>

Flags:
  --repo <path>    Git repository root (default: current dir)
  --rule <path>    Path to a custom rule JSON file
```

与えられたファイルパスに対して、OCR は次を行います:

1. 4 層のルールチェーン（custom → project → global → system）を辿ります。
2. 最初に一致したものを採用します。
3. **出所となる層**、一致した **glob パターン**、そして解決された**ルールテキスト**を出力します。

```bash
$ ocr rules check src/main/java/com/example/Foo.java
File: src/main/java/com/example/Foo.java
Source: System built-in
Pattern: **/*.java
Rule:
────────────────────────────────────────
<contents of internal/config/rules/rule_docs/java.md>
────────────────────────────────────────
```

「なぜ自分のカスタムルールが発火しないのか？」を調査するのに使えます。優先順位の完全な説明は[レビュールール](../review-rules/)を参照してください。

## `ocr config`

key を `~/.opencodereview/config.json` に永続化し、対話的な設定 TUI を提供します。4 つのサブコマンドがあります:

```text
ocr config set <key> <value>
ocr config unset <key>                     Clear a saved config value
ocr config provider                        Interactive provider setup
ocr config model                           Interactive model selection
```

- **`set`**: 非対話的に単一の設定値を書き込みます（例: `ocr config set effort high`）。
- **`unset`**: 保存済みの設定値をクリアします。`provider`、`max_tokens`、`effort`、`custom_providers.<name>`、`mcp_servers.<name>` をサポートします。削除するものが現在有効なカスタムプロバイダーの場合、`provider` と `model` もクリアされます（`ocr config provider` を実行して再選択してください）。`ocr config unset effort` はデフォルトの `medium` プリセットに戻します。
- **`provider`**: 対話的なプロバイダー設定 TUI を起動します（追加の引数なし。非対話的には `ocr config set provider <name>` を使用してください）。
- **`model`**: 対話的な model 選択 TUI を起動します（追加の引数なし。非対話的には `ocr config set model <name>` を使用してください）。

key の完全なリファレンス、schema、例は[設定](../configuration/)を参照してください。

## `ocr llm`

LLM ユーティリティコマンドです。2 つのサブコマンドがあります:

```text
ocr llm <sub-command>

Sub-commands:
  test         Send a test conversation to the configured LLM model
  providers    List all built-in LLM providers
```

### `ocr llm test`

```text
ocr llm test
```

`ocr review` とまったく同じ方法で LLM エンドポイントを解決し、[`internal/config/testconnection/task.json`](https://github.com/alibaba/open-code-review/blob/main/internal/config/testconnection/task.json) からあらかじめ用意された chat リクエストを送信して、以下を出力します:

```
Source: <which strategy was used>
URL:    <endpoint URL>
Model:  <effective model>
<the model's reply>
✓ Connection test successful
```

非ゼロで終了した場合は、エンドポイントが完全に設定されていないか、リクエストが失敗した（ネットワーク / 認証 / モデルのエラー）ことを意味します。エラーメッセージがどのケースかを示します。

### `ocr llm providers`

```text
ocr llm providers
```

各組み込み LLM プロバイダーを 3 列のテーブルで一覧表示します:

```
Built-in providers:
  NAME        PROTOCOL    BASE URL
  ----        --------    --------
  anthropic   anthropic   https://api.anthropic.com
  …
```

続いて、`ocr config provider` で対話的に設定するか、`ocr config set provider <name>` で非対話的に設定できる旨のヒントが表示されます。

## `ocr viewer`

```text
ocr viewer [flags]

Flags:
  --addr <address>   listen address (default: localhost:5483)
  --open <mode>      when to open the browser: auto, always, never (default: auto)

Examples:
  ocr viewer                     # start and open the browser
  ocr viewer --addr :3000        # bind to all interfaces on port 3000
  ocr viewer --open=never        # just print the URL
  ocr viewer --open=always       # force it when auto declines (piped output, WSL)
```

埋め込み HTTP サーバーを起動し、`~/.opencodereview/sessions/...` を読み込んで、過去のレビューセッションをブラウザで扱いやすい UI としてレンダリングします。[セッションビューア](../viewer/)を参照してください。

`--open=auto` は、stdout が端末でないとき、`SSH_CONNECTION` が設定されていて
ディスプレイが転送されていないとき、Linux で `DISPLAY` も `WAYLAND_DISPLAY` も
ないときにブラウザを開くのを省略します。その理由は URL と併せて表示されます。
auto が見送るが実際にはブラウザに到達できる場合は `--open=always` を使います。

## `ocr version`

```text
ocr version
ocr --version
ocr -V
```

ビルド時に書き込まれたバージョン情報、短い Git commit（存在する場合）、プラットフォーム（`<GOOS>/<GOARCH>`）、ビルド日（存在する場合）、そして GitHub URL（`https://github.com/alibaba/open-code-review`）を出力します。

## ocr completion

`ocr` のシェル補完スクリプトを生成し、シェル内でコマンド名・フラグ・引数を Tab 補完できるようにします。

### Bash

現在のセッションのみ：

```bash
source <(ocr completion bash)
```

永続化（Linux）：

```bash
ocr completion bash > /etc/bash_completion.d/ocr
```

永続化（macOS）：

```bash
ocr completion bash > $(brew --prefix)/etc/bash_completion.d/ocr
```

### Zsh

シェル補完がまだ有効になっていない場合は、一度だけ実行してください：

```bash
echo "autoload -U compinit; compinit" >> ~/.zshrc
```

その後、補完を永続的に読み込みます：

```bash
ocr completion zsh > "${fpath[1]}/_ocr"
```

反映させるには新しいシェルを開始する必要があります。

### Fish

現在のセッションのみ：

```bash
ocr completion fish | source
```

永続化：

```bash
ocr completion fish > ~/.config/fish/completions/ocr.fish
```

### PowerShell

現在のセッションのみ：

```powershell
ocr completion powershell | Out-String | Invoke-Expression
```

永続化 —— スクリプトを生成し、PowerShell プロファイルから読み込みます：

```powershell
ocr completion powershell > ocr.ps1
```

その後、`ocr.ps1` を読み込む行を PowerShell プロファイルに追加してください。

## ヒントと注意点

- `--audience agent` は `--format json` を**含意しません**。両者は異なることを制御します。UI の抑制 vs 構造化されたペイロードです。両方が必要な場合は組み合わせて使用してください。
- `--background` はレビュー品質を高めるのに最も効果的な引数の 1 つです。他の agent から呼び出す際は、常に要件 / PR の説明を渡してください。
- あるファイルの diff が単独で `MAX_TOKENS` の 80%（デフォルト `200000`）を超える場合、LLM を呼び出す前に破棄されます。これはログに記録されますが、実行を失敗にはしません。
- ファイルグループ内のどのファイルも `PLAN_MODE_LINE_THRESHOLD`（`50`）に達せず、かつ合計変更行数も `PLAN_MODE_GROUP_LINE_THRESHOLD`（`100`。複数ファイルのグループにのみ適用）に達しない場合、plan 段階は**自動的にスキップ**されます。

## 関連項目

- [クイックスタート](../quickstart/): インストールして最初のレビューを完了します。
- [設定](../configuration/): 引数の背後にある環境変数と config key。
- [レビュールール](../review-rules/): `--rule` 引数とルールの解決。
- [連携](../integrations/agent-skill/): agent と CI から `ocr review` を呼び出します。
