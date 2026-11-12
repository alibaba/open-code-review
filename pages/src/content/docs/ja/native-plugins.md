---
title: ネイティブプラグイン
sidebar:
  order: 8
---

ネイティブプラグインは追加のレビュールールをローカルディレクトリにまとめます。
OCR はディレクトリをユーザー単位のスナップショットとしてコピーし、有効な
プラグインを全リポジトリの `ocr review` と `ocr scan` に適用します。
パッケージ内のファイルはデータとして読み込みます。

## インストールと確認

OCR のソースチェックアウトから同梱のサンプルをインストールします。

```bash
ocr plugin install ./examples/ocr-plugins/review-checklist
ocr plugin list
ocr plugin show review-checklist
ocr plugin enable review-checklist
ocr rules check src/main.go
```

インストール直後は無効です。`--enable` を付けると同時に有効化できます。
`ocr rules check` は一致した各プラグインの ID、バージョン、ルールパックのパス、
パターンと、結合されたルール本文を表示します。すべてのプラグインサブコマンドは
`--json` に対応します。例：`ocr plugin list --json`。

有効化する前にルール本文を確認してください。モデルのレビュープロンプトに入ります。
インストール元には `ocr-plugin.json` を含むローカルディレクトリを指定します。
URL、GitHub リポジトリ参照、ZIP や tar.gz アーカイブは受け付けません。

## パッケージ形式

サンプルは次の 3 ファイルで構成されます。

```text
review-checklist/
  ocr-plugin.json
  rules.json
  checklist.md
```

`ocr-plugin.json`：

```json
{
  "schema_version": 1,
  "id": "review-checklist",
  "name": "Review Checklist",
  "version": "1.0.0",
  "description": "Additional review checks for error handling and resource ownership.",
  "rules": ["rules.json"],
  "requires": {"features": ["rules"]}
}
```

| フィールド | 要件 |
|---|---|
| `schema_version` | 必須。値は `1`。 |
| `id` | 必須。小文字 ASCII 英字、数字、`.`、`_`、`-` の 1～64 文字。先頭と末尾は英字か数字。Windows の予約名は不可。 |
| `name`、`version` | 必須。空白だけでない文字列。 |
| `description` | 任意。`ocr plugin show` で表示する説明。 |
| `rules` | 必須。パッケージ相対の `.json` パスを並べた、空ではなく重複のない順序付きリスト。 |
| `requires.ocr_min_version` | 任意。`1.0.0` などの完全なセマンティックバージョン。OCR のビルドバージョンと比較。 |
| `requires.features` | 任意。必要な機能のリスト。対応する値は `rules`。 |
| `metadata` | 任意。解釈や実行をせず保持する JSON オブジェクト。 |

マニフェストの未知のフィールドや重複 JSON キーは拒否されます。互換性は
インストール時、有効化時、有効なプラグインの読み込み時に確認します。`dev` ビルドは
`ocr_min_version` を満たせないため、開発ビルドで使うパッケージでは省略します。

`rules.json`：

```json
{
  "rules": [
    {
      "path": "**/*.{go,js,mjs,ts,tsx,py}",
      "rule": "checklist.md"
    }
  ]
}
```

`checklist.md` にレビューの指示を記述します。

```text
Check error propagation, cleanup, and resource ownership.
Report only concrete failure paths in the changed code.
Verify the relevant caller and explain the trigger and user-visible consequence.
```

`rule` には本文を直接書くこともできます。Markdown 参照はルールパックのファイルを
基準に解決し、パッケージ内に収まる必要があります。パスには `/` を使います。
ルールパックにはレビュールールを記述し、`include` と `exclude` はプロジェクト
またはユーザーのルールで設定します。プラグイン内の空でない両リストは拒否されます。

## ルールの結合

OCR はまず `--rule`、プロジェクト、グローバル、システムの順に基本ルールを解決します。
次に有効なプラグインを ID の昇順、各マニフェストの `rules` 順に追加します。
各ルールパックでは最初に一致した項目が適用されるため、複数のパックから同じ
ファイルに指示を追加できます。

プラグインルールは常に追加され、パック内の `merge_system_rule` は効果を持ちません。
レビュー対象は既存のファイルフィルターが決めます。パターン、フィルタリング、
基本の優先順位は[レビュールール](../review-rules/)を参照してください。

## 無効化・置換・削除

```bash
ocr plugin disable review-checklist
ocr plugin remove review-checklist
ocr plugin install ./examples/ocr-plugins/review-checklist --enable
```

置換するには、インストール済みの ID を先に削除します。元ディレクトリの変更は
再インストール後に反映されます。状態は `~/.opencodereview/plugins/state.json`、
スナップショットは `plugins/packages/` に保存されます。削除は登録を解除し、
実行中のレビューが使うスナップショットを保持します。

再開時の構成識別情報には有効なマニフェストとパッケージ内容が含まれます。
同じ内容を再インストールすると識別情報は維持され、内容を変更すると以前の
プラグイン構成で得た結果は再利用されなくなります。

パッケージの上限は 64 MiB、ディレクトリを含め 4,096 項目です。マニフェストは
1 MiB、読み込み時の各ルールパック JSON は 512 KiB が上限です。シンボリックリンク、
特殊ファイル、大文字小文字だけが異なる名前の衝突、Windows の予約ファイル名、
パッケージ外へのパスは拒否されます。スナップショットは変更せず、元ディレクトリを
編集して再インストールしてください。

コマンド一覧は [CLI リファレンス](../cli-reference/#ocr-plugin)を参照してください。
