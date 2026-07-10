---
title: Open Code Review ブログシステムの紹介
date: 2026-07-01
tags: [アナウンス, ガイド]
summary: Open Code Review ブログシステムの仕組みと、GitHub の Markdown ファイルを使って記事を書く方法を紹介します。
author: lizhengfeng101
---

## なぜこのブログを作ったのか

Open Code Review の裏には多くの複雑な設計判断があります。例えば、なぜある機能を作るのか、なぜ作らないのか、なぜこのアプローチを選んだのか。
複数のアーキテクチャから一つを選ぶとき、ある機能をあえて実装しないと決めたとき、壁にぶつかってより良いアプローチを見つけたとき——これらのストーリーを記録に残したいと考えています。
そこで、OCR の機能の裏にある設計思想や哲学を共有するために、このブログを作りました。

OCR 自体の設計共有に加えて、私たちが探求・応用している最先端の AI 技術についても発信します —— プロンプトエンジニアリング戦略、モデル評価手法、そして大規模な AI 開発者ツール構築から得た知見などです。

## このブログについて

このブログは Open Code Review ウェブサイトに組み込まれています。すべての記事は GitHub リポジトリ内の Markdown ファイルとして管理されています —— CMS もデータベースも不要で、PR をマージするだけで公開できます。

Markdown が書け、技術に対する深い洞察があれば、ここで記事を公開できます。

## 仕組み

### ファイルベースのコンテンツ管理

各ブログ記事は `pages/src/content/blog/` ディレクトリに格納された `.md` ファイルです。ディレクトリ構造は以下の通りです：

```
pages/src/content/blog/
├── index.ts          # コンテンツレジストリ
├── en/
│   └── my-post.md   # 英語版
├── zh/
│   └── my-post.md   # 中国語版
└── ja/
    └── my-post.md   # 日本語版
```

### Frontmatter メタデータ

各記事は YAML frontmatter ブロックで始まり、メタデータを定義します：

```yaml
---
title: 記事タイトル
date: 2026-07-10
tags: [タグ1, タグ2]
summary: リストページに表示される一行の説明。
author: あなたの名前
---
```

| フィールド | 必須 | 説明 |
|-----------|------|------|
| title | はい | リストページと詳細ページに表示される記事タイトル |
| date | はい | 公開日（YYYY-MM-DD 形式）、ソートに使用 |
| tags | いいえ | フィルタリング用のタグ配列 |
| summary | いいえ | 記事カードに表示される簡単な説明 |
| author | いいえ | 著者名 |

### 多言語対応

ブログは英語、中国語、日本語に対応しています。同じ記事の翻訳版を対応する言語ディレクトリに同じファイル名で配置してください。

## 記事の投稿方法

### ステップ 1：Markdown ファイルの作成

`pages/src/content/blog/en/` に新しい `.md` ファイルを作成します（オプションで `zh/` と `ja/` に翻訳版も作成）。ファイル名は `my-article-topic.md` のような slug フレンドリーな形式にしてください。

### ステップ 2：記事の登録

`pages/src/content/blog/index.ts` を開き：

1. `BlogSlug` ユニオン型にスラッグを追加
2. Markdown ファイルをインポート
3. 各言語の記事レコードにエントリを追加

```typescript
// BlogSlug 型に追加
export type BlogSlug =
  | 'introducing-ocr-blog'
  | 'my-article-topic';  // 新しいスラッグ

// インポートを追加
import enMyArticle from './en/my-article-topic.md';
import jaMyArticle from './ja/my-article-topic.md';

// 記事レコードに追加
const enPosts: Record<BlogSlug, string> = {
  'introducing-ocr-blog': enIntroducingOcr,
  'my-article-topic': enMyArticle,
};
```

記事に画像が必要な場合は、`pages/public/images/blog/<your-slug>/` に配置します：

```
pages/public/images/blog/
└── my-article-topic/
    ├── screenshot.png
    └── diagram.svg
```

Markdown で絶対パスを使って参照します：

```markdown
![OCR ホームページ](/images/blog/my-article-topic/screenshot.png)
```

以下は画像レンダリングの例です：

![Open Code Review ホームページ](/images/blog/introducing-ocr-blog/demo.png)

### ステップ 3：PR を提出

[GitHub](https://github.com/alibaba/open-code-review) で Pull Request を提出してください。PR がマージされれば、数分後に記事が公開されます。

## 機能

ブログシステムは以下の機能を提供します：

- **タグフィルタリング** — リストページでタグによる記事の絞り込みが可能
- **全文検索** — Cmd+K で検索モーダルを開く
- **目次ナビゲーション** — 詳細ページの右側に目次を表示し、スクロール位置を追跡
- **レスポンシブデザイン** — デスクトップとモバイルに対応
- **ダークテーマ** — OCR ウェブサイト全体と統一されたスタイル

## 執筆のヒント

記事がより受け入れられやすくなるよう、以下のヒントをご参考ください：

- `##` と `###` の見出しを使用してください —— 右側の目次に表示されます
- サマリーは 100 文字以内に収めると、カード表示がきれいになります
- タグは簡潔な言葉を使いましょう
- 言語指定付きのコードブロックはシンタックスハイライトされます
- 内部リンクは `(/docs/quickstart)` のような相対パスを使用してください

皆さんの深い洞察をお待ちしています！
