/**
 * Shared utility to generate heading IDs from text.
 * Used by both extractHeadings (DocsPage TOC) and MarkdownRenderer (heading renderer)
 * to ensure consistent anchor IDs.
 */
export function generateHeadingId(text: string): string {
  // Strip HTML tags first (from marked output), then strip markdown formatting chars
  const plain = text.replace(/<[^>]+>/g, '').replace(/[`*_\[\]()]/g, '').trim();
  return plain.toLowerCase().replace(/[^a-z0-9\u4e00-\u9fff]+/g, '-').replace(/^-|-$/g, '');
}

/**
 * Creates a stateful heading ID generator that deduplicates IDs by appending
 * a numeric suffix (-1, -2, etc.) when the same heading text appears multiple times.
 * Call createHeadingIdGenerator() once per document render to track seen IDs.
 */
export function createHeadingIdGenerator(): (text: string) => string {
  const seen = new Map<string, number>();
  return (text: string): string => {
    const baseId = generateHeadingId(text);
    const count = seen.get(baseId) || 0;
    seen.set(baseId, count + 1);
    return count === 0 ? baseId : `${baseId}-${count}`;
  };
}
