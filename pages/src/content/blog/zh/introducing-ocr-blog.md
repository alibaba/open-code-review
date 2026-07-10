---
title: Open Code Review 博客系统介绍
date: 2026-07-10
tags: [公告, 指南]
summary: 了解 Open Code Review 博客系统的工作原理，以及如何通过 GitHub 中的 Markdown 文件来撰写和发布文章。
author: lizhengfeng101
---

## 为什么要建立这个博客

Open Code Review 背后有很多复杂的设计决策，例如为什么做某些功能、为什么不做某些功能、为什么要这样做而不是那样做。
当我们在多种架构方案中做出选择时，当我们决定不实现某个功能时，当我们碰壁后找到更好的方案时，我们希望这些故事被记录下来。
因此我们建立这个博客，用于分享 OCR 功能背后的设计思想和理念。

除了 OCR 本身的设计分享，我们也会分享在探索和应用前沿 AI 技术过程中的心得 —— 提示词工程策略、模型评估方法，以及在大规模构建 AI 开发者工具中积累的经验。

## 这个博客是什么

这个博客内置于 Open Code Review 网站中。所有文章都以 Markdown 文件的形式管理在 GitHub 仓库中 —— 没有 CMS、没有数据库，合并 PR 即可发布。

只要你会写 Markdown、对技术有着深度的思考，就可以在这里发布文章。

## 工作原理

### 基于文件的内容管理

每篇博文是一个 `.md` 文件，存储在 `pages/src/content/blog/` 目录下。目录结构如下：

```
pages/src/content/blog/
├── index.ts          # 内容注册表
├── en/
│   └── my-post.md   # 英文版
├── zh/
│   └── my-post.md   # 中文版
└── ja/
    └── my-post.md   # 日文版
```

### Frontmatter 元数据

每篇文章以 YAML frontmatter 块开头，定义其元数据：

```yaml
---
title: 你的文章标题
date: 2026-07-10
tags: [标签1, 标签2]
summary: 在列表页显示的一行描述。
author: 你的名字
---
```

| 字段 | 必填 | 说明 |
|------|------|------|
| title | 是 | 在列表页和详情页显示的文章标题 |
| date | 是 | 发布日期，YYYY-MM-DD 格式，用于排序 |
| tags | 否 | 标签数组，用于筛选 |
| summary | 否 | 文章简介，显示在文章卡片中 |
| author | 否 | 作者名称 |

### 多语言支持

博客支持英文、中文和日文。将同一篇文章的翻译版本放在对应语言目录下，使用相同的文件名即可。

## 如何贡献文章

### 第一步：创建 Markdown 文件

在 `pages/src/content/blog/en/` 下创建一个新的 `.md` 文件（可选在 `zh/` 和 `ja/` 下创建翻译版本）。文件名使用 slug 友好的格式，例如 `my-article-topic.md`。

### 第二步：注册文章

打开 `pages/src/content/blog/index.ts`，然后：

1. 将你的 slug 添加到 `BlogSlug` 联合类型
2. 导入你的 markdown 文件
3. 将条目添加到各语言的文章记录中

```typescript
// 添加到 BlogSlug 类型
export type BlogSlug =
  | 'introducing-ocr-blog'
  | 'my-article-topic';  // 你的新 slug

// 添加导入
import enMyArticle from './en/my-article-topic.md';
import zhMyArticle from './zh/my-article-topic.md';

// 添加到文章记录
const enPosts: Record<BlogSlug, string> = {
  'introducing-ocr-blog': enIntroducingOcr,
  'my-article-topic': enMyArticle,
};
```

如果文章需要插图，将图片放到 `pages/public/images/blog/<你的-slug>/` 目录下：

```
pages/public/images/blog/
└── my-article-topic/
    ├── screenshot.png
    └── diagram.svg
```

然后在 markdown 中用绝对路径引用：

```markdown
![OCR 首页](/images/blog/my-article-topic/screenshot.png)
```

以下是图片渲染的示例效果：

![Open Code Review 首页](/images/blog/introducing-ocr-blog/demo.png)

### 第三步：提交 PR

在 [Github](https://github.com/alibaba/open-code-review) 提交一个 Pull Request，若 PR 被合并，几分钟后即可上线。

## 功能特性

博客系统提供以下功能：

- **标签筛选** — 读者可以在列表页按标签筛选文章
- **全文搜索** — Cmd+K 打开搜索弹窗
- **目录导航** — 详情页右侧显示目录，并跟踪滚动位置
- **响应式设计** — 适配桌面和移动端
- **暗色主题** — 与 OCR 网站整体风格一致

## 写作建议

为了您的文章更容易被发布，请遵循以下建议：

- 使用 `##` 和 `###` 标题 —— 它们会出现在右侧目录中
- 摘要控制在 100 字符以内，以获得整洁的卡片展示效果
- 标签建议使用简短的词语
- 带语言标识的代码块会获得语法高亮
- 内部链接使用相对路径，如 `(/docs/quickstart)`

期待你的思考！
