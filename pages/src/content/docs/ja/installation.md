---
title: インストール
sidebar:
  order: 4
---

`ocr` CLI をインストールするには、サポートされた 4 つの方法があります。いずれも生成されるのは同じバイナリなので、
環境に合わせて選んでください。

## NPM（推奨）

```bash
npm install -g @alibaba-group/open-code-review
```

NPM パッケージには、小さな wrapper スクリプト（`bin/ocr.js`）と
[postinstall hook](https://github.com/alibaba/open-code-review/blob/main/scripts/install.js)
が付属しており、以下を行います。

1. お使いのプラットフォームを検出します（`darwin-amd64`、`darwin-arm64`、`linux-amd64`、
   `linux-arm64`、`windows-amd64`、`windows-arm64`）。
2. GitHub Releases から一致するバイナリをダウンロードします。
3. （チェックサムデータが存在する場合は）検証し、wrapper の隣に配置します。

プラットフォーム固有の npm パッケージ（例：`@alibaba-group/ocr-darwin-arm64`）が
optional dependency としてインストールされている場合は、そのバイナリを直接使用し、ダウンロードをスキップします。

`ocr` を実行すると、wrapper はダウンロード済みのバイナリを単に `exec` するだけなので、初回実行後の実際のオーバーヘッド
はゼロです。

### 更新

```bash
npm update -g @alibaba-group/open-code-review
# または特定のバージョンに固定：
npm install -g @alibaba-group/open-code-review@<version>
```

### アンインストール

```bash
npm uninstall -g @alibaba-group/open-code-review
```

## GitHub Release バイナリ

Node.js をインストールしたくない場合は、
[releases ページ](https://github.com/alibaba/open-code-review/releases) から
静的バイナリを直接取得できます。

```bash
# macOS (Apple Silicon)
curl -Lo ocr https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-darwin-arm64
chmod +x ocr && sudo mv ocr /usr/local/bin/ocr

# macOS (Intel)
curl -Lo ocr https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-darwin-amd64
chmod +x ocr && sudo mv ocr /usr/local/bin/ocr

# Linux x86_64
curl -Lo ocr https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-linux-amd64
chmod +x ocr && sudo mv ocr /usr/local/bin/ocr

# Linux ARM64
curl -Lo ocr https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-linux-arm64
chmod +x ocr && sudo mv ocr /usr/local/bin/ocr

# Windows (AMD64)
curl -Lo ocr.exe https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-windows-amd64.exe

# Windows (ARM64)
curl -Lo ocr.exe https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-windows-arm64.exe
```

各 release では、バイナリの隣に `sha256sum.txt` も公開されており、完全性を検証できます。

```bash
curl -LO https://github.com/alibaba/open-code-review/releases/latest/download/sha256sum.txt
shasum -a 256 -c sha256sum.txt --ignore-missing
```

## インストールスクリプト（curl | sh）

GitHub Release バイナリのダウンロード（検証付き）をラップした便利なインストーラーです——CI のベース
イメージやヘッドレス環境に適しています。

```bash
curl -fsSL https://raw.githubusercontent.com/alibaba/open-code-review/main/install.sh | sh
```

2 つの環境変数を認識します。

| 変数 | デフォルト値 | 用途 |
|---|---|---|
| `OCR_INSTALL_DIR` | `/usr/local/bin` | `ocr` バイナリを配置する場所。 |
| `OCR_VERSION` | 最新 release | 特定の release tag に固定します（例：`v1.2.3`）。 |

このスクリプトは `darwin` と `linux` の `amd64` / `arm64` をサポートします。Windows では
[GitHub Release バイナリ](#github-release-binary) または [NPM](#npm-recommended)
の方法を使用してください。

## ソースからビルドする

OCR 自体を変更する場合、またはプリコンパイル済みバイナリのないプラットフォームで実行する場合にのみ、この方法が必要です。

### 前提条件

- [Go ≥ 1.25](https://go.dev/dl/)
- [Git](https://git-scm.com/)
- [Make](https://www.gnu.org/software/make/)

### ビルド

```bash
git clone https://github.com/alibaba/open-code-review.git
cd open-code-review
make build              # dist/opencodereview を生成
sudo cp dist/opencodereview /usr/local/bin/ocr
```

### 他のプラットフォーム向けにビルドする

```bash
make build-linux-amd64
make build-linux-arm64
make build-darwin-amd64
make build-darwin-arm64
make build-windows-amd64   # Windows (x86_64)
make build-windows-arm64   # Windows (ARM64)
make build-all          # 6 つすべてを一括ビルド
make sha256sum          # sha256sum.txt も生成
```

`make dist` は `clean → build-all → sha256sum` を実行し、バイナリの隣に
`VERSION` ファイルを書き込みます——これはまさに release パイプラインが実行するステップです。

### テストの実行

```bash
make test               # LC_ALL=C go test -v -race -count=1 ./...
```

## インストールの検証

バイナリがどこから来たものであっても：

```bash
ocr version             # バージョン + git commit + ビルド日時を出力
ocr --help              # トップレベルの使い方
ocr review --help       # review コマンドの完全な引数リスト
```

"command not found" エラーが出る場合は、インストール先が `$PATH` 上にあることを確認してください。

```bash
which ocr
echo $PATH
```

## OCR が状態を保存する場所

| パス | 保存内容 |
|---|---|
| `~/.opencodereview/config.json` | LLM エンドポイント、言語、テレメトリ設定（`ocr config set` で管理）。 |
| `~/.opencodereview/rule.json` | オプションのグローバルレビュールール。 |
| `~/.opencodereview/sessions/<encoded-repo-path>/<session-id>.jsonl` | レビューセッションごとのストリーミング JSONL トランスクリプト。`ocr viewer` で使用します。 |
| `~/.opencodereview/{last-update-check,update.lock,update-available}` | NPM wrapper のバックグラウンド更新チェックの状態。wrapper はより新しい release があるかをポーリングし（デフォルトで約 18 分ごと）、アップグレードの案内を表示します。`OCR_NO_UPDATE=1` で無効化するか、`OCR_UPDATE_INTERVAL`（秒）で間隔を調整します。静的バイナリはこれらのファイルを書き込みません。 |
| `<repo>/.opencodereview/rule.json` | オプションのプロジェクトレベルのレビュールール——安全にコミットできます。 |

OCR は `~/.opencodereview/` の外に書き込むことは決してありません（NPM が一時的にバイナリをダウンロードする場合を除く）。
このディレクトリを削除すれば、クリーンなアンインストールが完了します。

## 関連項目

- [クイックスタート](../quickstart/)——LLM を設定して初回のレビューを完了します。
- [設定](../configuration/)——OCR が受け入れる各環境変数と config key。
- [コントリビュート](../contributing/)——ソースからビルドし、テストを実行して開発に参加します。
