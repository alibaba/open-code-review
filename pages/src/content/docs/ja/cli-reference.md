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

## コマンド一覧

| コマンド | エイリアス | 役割 |
|---|---|---|
| `ocr review` | `ocr r` | コードレビューを実行してコメントを出力します。 |
| `ocr rules check <file>` | — | あるファイルパスにどのルールが適用され、その出所はどこかを表示します。 |
| `ocr config set <key> <value>` | — | 設定値を `~/.opencodereview/config.json` に永続化します。 |
| `ocr config unset custom_providers.<name>` | — | カスタムプロバイダーを削除します（現在有効なものであれば、有効な `provider`/`model` もクリアされます）。 |
| `ocr config provider` | — | 対話的なプロバイダー設定 TUI。 |
| `ocr config model` | — | 対話的な model 選択 TUI。 |
| `ocr llm test` | — | 短い chat リクエストを送信し、設定されたエンドポイントを検証します。 |
| `ocr llm providers` | — | 組み込みの LLM プロバイダーをすべて一覧表示します。 |
| `ocr session list` | `ocr sessions list`, `ocr session ls` | 保存されたレビューセッションを一覧表示します。 |
| `ocr session show <id>` | `ocr sessions show <id>` | 1つのセッションとファイル単位のチェックポイントを表示します。 |
| `ocr viewer` | — | 過去のレビューセッション用のローカル Web UI を起動します（`localhost:5483`）。 |
| `ocr version` | — | バージョン、commit、プラットフォーム、ビルド日、GitHub URL を出力します。 |

`ocr` および `ocr -h` はトップレベルの使い方を出力します。各サブコマンドも `-h` / `--help` を受け付けます。

## `ocr review`

メインコマンドです。Git diff を解析し、ファイルごとのサブエージェントをディスパッチし、レビューコメントを収集して出力します。

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
| `--preview` | `-p` | `false` | フィルタリングのパイプラインを実行しますが LLM はスキップします。ファイル一覧と除外理由を出力します。 |
| `--resume <session-id>` | — | — | 以前の互換性のある範囲または単一 commit レビューセッションから再開します。 |
| `--format <fmt>` | `-f` | `text` | `text`（人間が読みやすい形式）または `json`（機械可読なコメント配列）。 |
| `--audience <who>` | — | `human` | `human` は進捗行をストリーム出力します。`agent` は stdout を静音化し、最終サマリー / JSON のみを出力します。 |
| `--background <text>` | `-b` | — | plan + main prompt に注入する、任意の要件 / 業務コンテキスト。 |
| `--concurrency <n>` | — | `8` | 並行してレビューするファイルの最大数。 |
| `--timeout <minutes>` | — | `10` | ファイルごとの締め切り時間。`0` でタイムアウトを無効化します。 |
| `--rule <path>` | — | — | カスタム JSON レビュールールファイルのパス。プロジェクトレベルおよびグローバルの `rule.json` を上書きします。 |
| `--max-tools <n>` | — | テンプレートのデフォルト | ファイルごとの最大ツール呼び出し回数。`0` はテンプレートのデフォルト（`30`）を使用します。1〜9 は `10` に引き上げられます。`≥ 10` の値はすべてテンプレートのデフォルトを上書きします（`30` より小さくても）。 |
| `--model <name>` | — | — | 今回のレビューについて、解決済みの LLM model を上書きします（例: `claude-opus-4-6`）。 |
| `--max-git-procs <n>` | — | `16` | 並行 git サブプロセスの最大数。 |
| `--tools <path>` | — | 埋め込み | カスタム JSON ツール設定ファイルのパス。埋め込みのツール定義を上書きします。 |

> モード引数は排他です: `--from`/`--to` を渡すか、`--commit` を渡すか、いずれも渡さない（ワークスペースモード）かのいずれかです。
> 混在させるとそのままエラーになります。
> `--resume` は範囲または単一 commit レビューのみ対応し、`--preview` とは併用できません。

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
ocr review --from main --to feature-branch --resume <session-id>
ocr review --commit abc123 --resume <session-id>
```

再開は意図的に厳密です:

- ワークスペースレビューは再開できません
- 範囲レビューは同じ `--from` と `--to` が必要です
- 単一 commit レビューは同じ `--commit` が必要です
- `--preview` と `--resume` は併用できません

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

```json
{
  "status": "success",
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
  "comments": []
}
```

### 終了コード

| コード | 意味 |
|---|---|
| `0` | レビューが完了しました（コメントがゼロの場合や、致命的でない警告がある場合もあります）。 |
| `1` | 致命的エラー。引数の誤り、LLM エンドポイントを解決できない、すべてのファイルごとのサブエージェントが失敗した、などです。エラーテキストは stderr に出力されます。 |

致命的でない警告（個々のサブエージェントの失敗、あるファイルが token しきい値を超過、など）はインラインで出力されます。JSON モードでは `warnings` 配列に追加されます。

## `ocr session`

`~/.opencodereview/sessions/` 配下に保存されたローカルレビューセッションログを一覧表示・確認します。
session ID の確認、ファイル単位のチェックポイント状態の確認、中断した範囲または単一 commit
レビューの再開に使用します。

```text
ocr session <sub-command>
ocr sessions <sub-command>   (alias)

Sub-commands:
  list, ls    List recent review sessions for the current repo
  show <id>   Show one session's metadata and per-file items
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

```bash
ocr session show <session-id>
ocr session show --json <session-id>
ocr session show --repo /path/to/repo <session-id>
```

| 引数 | デフォルト | 説明 |
|---|---|---|
| `--repo <path>` | カレントディレクトリ | セッションを確認するリポジトリ。 |
| `--json` | `false` | セッションのメタデータとファイル単位の項目を JSON として出力します。 |

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
ocr config unset custom_providers.<name>   Delete a custom provider
ocr config provider                        Interactive provider setup
ocr config model                           Interactive model selection
```

- **`set`**: 非対話的に単一の設定値を書き込みます。
- **`unset`**: カスタムプロバイダーを削除します。サポートされるのは `custom_providers.<name>` のみです。削除するものが現在有効なプロバイダーの場合、`provider` と `model` がクリアされます（`ocr config provider` を実行して再選択してください）。
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

Examples:
  ocr viewer                     # start on default port
  ocr viewer --addr :3000        # bind to all interfaces on port 3000
```

埋め込み HTTP サーバーを起動し、`~/.opencodereview/sessions/...` を読み込んで、過去のレビューセッションをブラウザで扱いやすい UI としてレンダリングします。[セッションビューア](../viewer/)を参照してください。

## `ocr version`

```text
ocr version
ocr --version
ocr -V
```

ビルド時に書き込まれたバージョン情報、短い Git commit（存在する場合）、プラットフォーム（`<GOOS>/<GOARCH>`）、ビルド日（存在する場合）、そして GitHub URL（`https://github.com/alibaba/open-code-review`）を出力します。

## ヒントと注意点

- `--audience agent` は `--format json` を**含意しません**。両者は異なることを制御します。UI の抑制 vs 構造化されたペイロードです。両方が必要な場合は組み合わせて使用してください。
- `--background` はレビュー品質を高めるのに最も効果的な引数の 1 つです。他の agent から呼び出す際は、常に要件 / PR の説明を渡してください。
- あるファイルの diff が単独で `MAX_TOKENS` の 80%（デフォルト `58888`）を超える場合、LLM を呼び出す前に破棄されます。これはログに記録されますが、実行を失敗にはしません。
- あるファイルの変更行数が `PLAN_MODE_LINE_THRESHOLD`（`50`）を下回る場合、plan 段階は**自動的にスキップ**されます。

## 関連項目

- [クイックスタート](../quickstart/): インストールして最初のレビューを完了します。
- [設定](../configuration/): 引数の背後にある環境変数と config key。
- [レビュールール](../review-rules/): `--rule` 引数とルールの解決。
- [連携](../integrations/): agent と CI から `ocr review` を呼び出します。
