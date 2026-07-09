import DOMPurify from 'dompurify';

/**
 * Shared utility to generate heading IDs from text.
 * Used by both extractHeadings (DocsPage TOC) and MarkdownRenderer (heading renderer)
 * to ensure consistent anchor IDs.
 */
export function generateHeadingId(text: string): string {
  // Strip HTML tags via DOMPurify (a single-pass regex is unreliable and can be
  // bypassed by nested tags). Keeps text content, decodes HTML entities so the
  // TOC side (raw markdown) and renderer side (marked HTML output) agree.
  const plain = DOMPurify.sanitize(text, { ALLOWED_TAGS: [], ALLOWED_ATTR: [] })
    .replace(/[`*_\[\]()]/g, '')
    .trim();
  return plain.toLowerCase().replace(/[^a-z0-9\u4e00-\u9fff]+/g, '-').replace(/^-|-$/g, '');
}
