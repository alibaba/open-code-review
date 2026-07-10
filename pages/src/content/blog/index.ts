/* Blog content index — imports all markdown files and provides lookup by slug + language */

// English posts
import enIntroducingOcr from './en/introducing-ocr-blog.md';

// Chinese posts
import zhIntroducingOcr from './zh/introducing-ocr-blog.md';

// Japanese posts
import jaIntroducingOcr from './ja/introducing-ocr-blog.md';

export type BlogSlug =
  | 'introducing-ocr-blog';

export interface BlogMeta {
  slug: BlogSlug;
  title: string;
  date: string;
  tags: string[];
  summary: string;
  author: string;
}

const enPosts: Record<BlogSlug, string> = {
  'introducing-ocr-blog': enIntroducingOcr,
};

const zhPosts: Record<BlogSlug, string> = {
  'introducing-ocr-blog': zhIntroducingOcr,
};

const jaPosts: Record<BlogSlug, string> = {
  'introducing-ocr-blog': jaIntroducingOcr,
};

const blogMap: Record<string, Record<BlogSlug, string>> = {
  en: enPosts,
  zh: zhPosts,
  ja: jaPosts,
};

function stripFrontmatter(md: string): string {
  if (md.startsWith('---')) {
    const end = md.indexOf('---', 3);
    if (end !== -1) {
      return md.slice(end + 3).trim();
    }
  }
  return md;
}

function parseFrontmatter(slug: BlogSlug, raw: string): BlogMeta {
  const defaults: BlogMeta = { slug, title: slug, date: '', tags: [], summary: '', author: '' };
  if (!raw.startsWith('---')) return defaults;
  const end = raw.indexOf('---', 3);
  if (end === -1) return defaults;
  const fm = raw.slice(3, end);

  const titleMatch = fm.match(/title:\s*(.+)/);
  const dateMatch = fm.match(/date:\s*(.+)/);
  const summaryMatch = fm.match(/summary:\s*(.+)/);
  const authorMatch = fm.match(/author:\s*(.+)/);
  const tagsMatch = fm.match(/tags:\s*\[([^\]]*)\]/);

  return {
    slug,
    title: titleMatch ? titleMatch[1].trim() : slug,
    date: dateMatch ? dateMatch[1].trim() : '',
    tags: tagsMatch ? tagsMatch[1].split(',').map(t => t.trim()).filter(Boolean) : [],
    summary: summaryMatch ? summaryMatch[1].trim() : '',
    author: authorMatch ? authorMatch[1].trim() : '',
  };
}

function getRawContent(slug: BlogSlug, language: string): string {
  const langPosts = blogMap[language] || blogMap.en;
  return langPosts[slug] || enPosts[slug] || '';
}

export function getBlogContent(slug: BlogSlug, language: string): string {
  return stripFrontmatter(getRawContent(slug, language));
}

export function getBlogMeta(slug: BlogSlug, language: string): BlogMeta {
  return parseFrontmatter(slug, getRawContent(slug, language));
}

export function getAllBlogMetas(language: string): BlogMeta[] {
  const slugs = Object.keys(blogMap.en) as BlogSlug[];
  return slugs
    .map(slug => getBlogMeta(slug, language))
    .sort((a, b) => b.date.localeCompare(a.date));
}

export function getAllTags(language: string): string[] {
  const metas = getAllBlogMetas(language);
  const tagSet = new Set<string>();
  metas.forEach(m => m.tags.forEach(t => tagSet.add(t)));
  return Array.from(tagSet).sort();
}

export function searchBlog(query: string, language: string): { slug: BlogSlug; title: string; snippet: string }[] {
  if (!query.trim()) return [];
  const langPosts = blogMap[language] || blogMap.en;
  const results: { slug: BlogSlug; title: string; snippet: string }[] = [];
  const lowerQuery = query.toLowerCase();
  const slugs = Object.keys(langPosts) as BlogSlug[];
  for (const slug of slugs) {
    const raw = langPosts[slug] || enPosts[slug] || '';
    const meta = getBlogMeta(slug, language);
    const content = stripFrontmatter(raw);
    const lowerContent = content.toLowerCase();
    const lowerTitle = meta.title.toLowerCase();
    const lowerSummary = (meta.summary || '').toLowerCase();

    let snippet = '';
    const contentIdx = lowerContent.indexOf(lowerQuery);
    if (lowerTitle.includes(lowerQuery)) {
      snippet = meta.title;
    } else if (lowerSummary.includes(lowerQuery)) {
      snippet = meta.summary || '';
    } else if (contentIdx !== -1) {
      const start = Math.max(0, contentIdx - 30);
      const end = Math.min(content.length, contentIdx + query.length + 60);
      snippet = content.slice(start, end).replace(/[#*_`\[\]()]/g, '').replace(/\n/g, ' ').trim();
      if (start > 0) snippet = '...' + snippet;
      if (end < content.length) snippet = snippet + '...';
    } else {
      continue;
    }
    results.push({ slug, title: meta.title, snippet });
  }
  return results;
}
