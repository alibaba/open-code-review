# OpenCodeReviewへのコントリビューション

OpenCodeReviewへのコントリビューションに興味を持っていただきありがとうございます！タイポの修正、バグ報告、新機能の実装など、あらゆる貢献が重要です。

[English Version](CONTRIBUTING.md) | [简体中文版](CONTRIBUTING.zh-CN.md) | [한국어](CONTRIBUTING.ko-KR.md) | [Русский](CONTRIBUTING.ru-RU.md)

## 行動規範

このプロジェクトに参加することで、敬意と包摂性のある環境を維持することに同意したことになります。すべてのやり取りにおいて、親切かつ建設的であるよう心がけてください。

## コントリビューションの方法

コードを書く以外にも、さまざまな貢献の方法があります：

- **バグ報告** — 何か壊れているものを見つけましたか？再現手順を添えてissueを開いてください。
- **機能提案** — 改善のアイデアがありますか？[GitHub Discussions](https://github.com/alibaba/open-code-review/discussions/categories/ideas)で会話を始めるか、[Feature Request](https://github.com/alibaba/open-code-review/issues/new?template=feature_request.yml) issueを開くことができます。
- **ドキュメントの改善** — タイポの修正、説明の明確化、例の追加など。問題の報告には[Documentation Issue](https://github.com/alibaba/open-code-review/issues/new?template=docs_report.yml)を開くこともできます。
- **プルリクエストのレビュー** — 他のコントリビューターのコードレビューを手伝ってください。
- **コードを書く** — バグ修正、機能追加、パフォーマンス改善など。

## はじめに

### 前提条件

- [Go 1.25+](https://go.dev/dl/)
- [Git](https://git-scm.com/)
- [Make](https://www.gnu.org/software/make/)

### セットアップ

```bash
# 1. GitHubでリポジトリをフォーク

# 2. フォークをクローン
git clone https://github.com/<your-username>/open-code-review.git
cd open-code-review

# 3. upstreamリモートを追加（メインリポジトリから更新を同期するため）
git remote add upstream https://github.com/alibaba/open-code-review.git

# 4. プロジェクトをビルド
make build

# 5. テストを実行
make test
```

すべてパスすれば、コントリビューションの準備完了です。

> **注意:** `upstream`リモートはコントリビューターにとって読み取り専用です — メインリポジトリから最新の変更を取得するために使います。upstreamに直接プッシュすることはできません。すべてのコントリビューションは自分のフォーク（`origin`）にプッシュし、Pull Request経由で提出する必要があります。

## 開発ワークフロー

### ブランチ運用

`main`からフィーチャーブランチを作成します：

```bash
git checkout main
git pull upstream main
git checkout -b feat/your-feature-name
```

変更の種類を示すプレフィックスを使用してください：

| プレフィックス | 用途                                  |
| ----------- | ------------------------------------- |
| `feat/`     | 新機能                                |
| `fix/`      | バグ修正                              |
| `docs/`     | ドキュメントのみ                      |
| `refactor/` | コードのリファクタリング（動作変更なし） |
| `test/`     | テストの追加・更新                    |
| `chore/`    | ビルド、CI、ツーリングの変更          |

### コミットメッセージ

[Conventional Commits](https://www.conventionalcommits.org/)形式に従ってください：

```
<type>(<scope>): <short summary>

[optional body]
```

例：

```
feat(agent): add support for custom tool definitions
fix(llm): handle timeout errors in Anthropic API calls
docs(README): update configuration examples
```

### コード品質

変更を提出する前に、すべてのチェックをパスすることを確認してください：

```bash
# フォーマットとリント（Go標準ツーリング）
go fmt ./...
go vet ./...

# レース検出付きでテストを実行
make test

# ビルドが成功すること
make build
```

### プロジェクト構成

```
├── cmd/opencodereview/   # CLIエントリーポイント
├── internal/
│   ├── agent/            # レビューエージェントのロジック
│   ├── config/           # 設定管理
│   ├── diff/             # Git diffのパース
│   ├── llm/              # LLM APIクライアント（Anthropic & OpenAI）
│   ├── model/            # データモデル
│   ├── session/          # レビューセッション管理
│   ├── tool/             # 組み込みツール（file_read、code_searchなど）
│   ├── telemetry/        # OpenTelemetry統合
│   └── viewer/           # WebUIセッションビューアー
├── pages/                # WebUIフロントエンド
├── scripts/              # ビルド & インストールスクリプト
└── bin/                  # NPMラッパー
```

## ドキュメントへのコントリビューション

ドキュメントはOpenCodeReviewの重要な一部です。READMEファイル、インラインコードコメント、設定例、その他ユーザー向けテキストの改善を歓迎します。

### ドキュメントコントリビューションに該当するもの

- タイポ、文法エラー、リンク切れの修正
- 分かりにくい説明の明確化や不足しているコンテキストの追加
- コマンドや設定オプションの使用例の追加
- 古くなった内容の更新（機能変更後など）
- 中国語ドキュメント（`README.zh-CN.md`、`CONTRIBUTING.zh-CN.md`）の翻訳や改善

### ドキュメントのワークフロー

1. 問題を見つけたが自分で修正する予定がない場合は、[Documentation Issue](https://github.com/alibaba/open-code-review/issues/new?template=docs_report.yml)を開いてください。
2. 自分で修正したい場合は、リポジトリをフォークし、変更を加え、`docs/`ブランチプレフィックス（例：`docs/fix-config-example`）でPRを提出してください。
3. ドキュメントのみのPRにテストの変更は不要ですが、含めるコマンドやコードスニペットが正確であることを確認してください。

### ドキュメントファイル

| ファイル                | 用途                                 |
| ----------------------- | ------------------------------------ |
| `README.md`             | メインのプロジェクトドキュメント（英語） |
| `README.zh-CN.md`       | 中国語訳                             |
| `CONTRIBUTING.md`       | コントリビューションガイド（英語）   |
| `CONTRIBUTING.zh-CN.md` | コントリビューションガイド（中国語） |

## 変更の提出

### Issueを開く

大きな変更に取り組む前に、まずissueを開いてアプローチについて議論してください。これにより、作業の重複を防ぎ、コントリビューションがプロジェクトの方向性と一致することを確認できます。

バグを報告する際は、以下を含めてください：

1. OpenCodeReviewのバージョン（`ocr version`）
2. OSとアーキテクチャ
3. 再現手順
4. 期待される動作と実際の動作
5. 関連するログやエラーメッセージ

### Pull Requestのプロセス

1. **PRはフォーカスを絞る** — 1つのPRには1つの論理的な変更のみ。複数の独立した変更がある場合は、別々のPRとして提出してください。
2. **テストを書く** — 動作の変更にはテストを追加・更新してください。
3. **ドキュメントを更新する** — 変更がユーザー向けの動作に影響する場合は、関連ドキュメントを更新してください。
4. **CLAに署名する** — すべてのコントリビューターは、PRがマージされる前にContributor License Agreementに署名する必要があります（下記参照）。
5. **PRテンプレートに記入する** — 変更の内容と理由を記述してください。

### PRタイトルの形式

コミットメッセージと同じConventional Commits形式を使用してください：

```
feat(agent): add support for custom tool definitions
```

### レビュープロセス

- メンテナーがPRをレビューします。通常は数営業日以内です。
- 変更をお願いすることがあります — これは通常の協力的なプロセスであり、敵対的なものではありません。
- 承認されると、メンテナーがPRをマージします。

## Contributor License Agreement（CLA）

コントリビューションをマージする前に、すべてのコントリビューターにAlibaba Open Source Contributor License Agreementへの署名をお願いしています。これにより、プロジェクトがライセンス条項の下で配布できることが保証されます。

最初のPRを開くと、CLAボットが手順を記載したコメントを投稿します。リンクをたどって電子署名するだけです — 1分もかかりません。

## 初めてのコントリビューター

プロジェクトは初めてですか？以下のラベルが付いたissueを探してみてください：

- [`good first issue`](https://github.com/alibaba/open-code-review/labels/good%20first%20issue) — 始めるのに最適な、小さくスコープが明確なタスク。
- [`help wanted`](https://github.com/alibaba/open-code-review/labels/help%20wanted) — コミュニティの協力を歓迎するissue。

始めるのに適した領域：

- エラーメッセージやCLI出力の改善
- テストされていないコードパスへのテストの作成
- ドキュメントの改善

## コミュニティ

- **バグ報告** — [GitHub Issues](https://github.com/alibaba/open-code-review/issues)
- **機能提案** — [GitHub Discussions (Ideas)](https://github.com/alibaba/open-code-review/discussions/categories/ideas)または[Feature Request Issue](https://github.com/alibaba/open-code-review/issues/new?template=feature_request.yml)
- **質問 & ヘルプ** — OpenCodeReviewの使い方について質問があれば、お気軽に[GitHub Discussions](https://github.com/alibaba/open-code-review/discussions)で質問してください

## ライセンス

OpenCodeReviewにコントリビューションすることで、あなたのコントリビューションが[Apache License 2.0](LICENSE)の下でライセンスされることに同意したことになります。
