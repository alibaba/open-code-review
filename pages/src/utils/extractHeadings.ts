/* ─── Extract headings from markdown for right TOC ─── */
import { generateHeadingId, parseExplicitHeadingId } from './headingId';

export function extractHeadings(markdown: string): { id: string; text: string; level: number }[] {
  const headings: { id: string; text: string; level: number }[] = [];
  const lines = markdown.split('\n');
  let inCodeBlock = false;
  for (const line of lines) {
    if (line.trim().startsWith('```')) {
      inCodeBlock = !inCodeBlock;
      continue;
    }
    if (inCodeBlock) continue;
    const match = line.match(/^(#{2,3})\s+(.+)$/);
    if (match) {
      const level = match[1].length;
      const { text: headingText, id: explicitId } = parseExplicitHeadingId(match[2]);
      // Strip markdown link syntax [text](url) → text, then strip other formatting
      const text = headingText
        .replace(/\[([^\]]+)]\([^)]*\)/g, '$1')
        .replace(/[`*_[\]()]/g, '')
        .trim();
      const id = explicitId ?? generateHeadingId(text);
      headings.push({ id, text, level });
    }
  }
  return headings;
}
