---
title: Introducing the Open Code Review Blog System
date: 2026-07-01
tags: [announcement, guide]
summary: Learn how the Open Code Review blog system works and how you can contribute articles using simple Markdown files managed in GitHub.
author: lizhengfeng101
---

## Why We Built This Blog

Open Code Review has many complex design decisions behind it — why certain features exist, why others were deliberately left out, and why we chose one approach over another.
When we choose one architecture over another, when we decide NOT to implement something, when we find a better approach after hitting a wall — we want these stories to be recorded.
That's why we created this blog — to share the design thinking and philosophy behind OCR's features.

Beyond OCR internals, we'll also share insights on cutting-edge AI techniques we explore and apply — prompt engineering strategies, model evaluation methods, and lessons learned from building AI-powered developer tools at scale.

## What Is This Blog

This blog is built into the Open Code Review website. All articles are managed as Markdown files in the GitHub repository — no CMS, no database, merge a PR and it's live.

If you can write Markdown and have deep insights on technology, you can publish here.

## How It Works

### File-Based Content

Each blog post is a `.md` file stored under `pages/src/content/blog/`. The directory structure looks like:

```
pages/src/content/blog/
├── index.ts          # Content registry
├── en/
│   └── my-post.md   # English version
├── zh/
│   └── my-post.md   # Chinese version
└── ja/
    └── my-post.md   # Japanese version
```

### Frontmatter Metadata

Every post starts with a YAML frontmatter block that defines its metadata:

```yaml
---
title: Your Article Title
date: 2026-07-10
tags: [tag1, tag2]
summary: A one-line description shown on the list page.
author: Your Name
---
```

| Field | Required | Description |
|-------|----------|-------------|
| title | Yes | Article title displayed on list and detail pages |
| date | Yes | Publication date in YYYY-MM-DD format, used for sorting |
| tags | No | Array of tags for filtering |
| summary | No | Brief description shown in the article card |
| author | No | Author name |

### Multi-Language Support

The blog supports English, Chinese, and Japanese. Place translated versions of the same article in the corresponding language directory with the same filename.

## How to Contribute a Post

### Step 1: Create Your Markdown File

Create a new `.md` file under `pages/src/content/blog/en/` (and optionally `zh/` and `ja/` for translations). Choose a slug-friendly filename like `my-article-topic.md`.

### Step 2: Register the Post

Open `pages/src/content/blog/index.ts` and:

1. Add your slug to the `BlogSlug` union type
2. Import your markdown file(s)
3. Add the entry to each language's post record

```typescript
// Add to BlogSlug type
export type BlogSlug =
  | 'introducing-ocr-blog'
  | 'my-article-topic';  // your new slug

// Add imports
import enMyArticle from './en/my-article-topic.md';
import zhMyArticle from './zh/my-article-topic.md';

// Add to post records
const enPosts: Record<BlogSlug, string> = {
  'introducing-ocr-blog': enIntroducingOcr,
  'my-article-topic': enMyArticle,
};
```

If your article needs images, place them under `pages/public/images/blog/<your-slug>/`:

```
pages/public/images/blog/
└── my-article-topic/
    ├── screenshot.png
    └── diagram.svg
```

Then reference them in markdown with an absolute path:

```markdown
![OCR Homepage](/images/blog/my-article-topic/screenshot.png)
```

Here's an example of how an image renders:

![Open Code Review Homepage](/images/blog/introducing-ocr-blog/demo.png)

### Step 3: Submit a PR

Submit a Pull Request on [GitHub](https://github.com/alibaba/open-code-review). Once the PR is merged, your article will go live within minutes.

## Features

The blog system provides:

- **Tag filtering** — readers can filter posts by tag on the list page
- **Full-text search** — Cmd+K opens a search modal
- **Table of contents** — detail pages show a right-side TOC with scroll tracking
- **Responsive design** — works on desktop and mobile
- **Dark theme** — consistent with the rest of the OCR website

## Writing Tips

To make your article easier to publish, please follow these tips:

- Use `##` and `###` headings — they appear in the right-side table of contents
- Keep summaries under 100 characters for clean card display
- Tags should be lowercase and concise
- Code blocks with language hints get syntax highlighting
- Internal links use relative paths like `(/docs/quickstart)`

We look forward to your insights!
