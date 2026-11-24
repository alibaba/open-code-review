// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

/* ─── Extract headings from markdown for right TOC ─── */
import { extractHeadingInfo } from './headingId';

export function extractHeadings(markdown: string): { id: string; text: string; level: number }[] {
  return extractHeadingInfo(markdown).filter(({ level }) => level === 2 || level === 3);
}
