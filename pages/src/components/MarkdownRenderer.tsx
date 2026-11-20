// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import React, { useMemo, useEffect, useRef } from 'react';
import ReactDOM from 'react-dom';
import { Marked, Renderer } from 'marked';
import DOMPurify from 'dompurify';
import { useTranslation } from '../i18n';
import { useCopyToast } from '../hooks/useCopyToast';
import copyIcon from '../assets/icons/icon-copy.svg';
import { extractHeadingInfo, generateHeadingId, parseExplicitHeadingId } from '../utils/headingId';

type Mermaid = typeof import('mermaid')['default'];

let mermaidPromise: Promise<Mermaid> | null = null;

function loadMermaid(): Promise<Mermaid> {
  if (!mermaidPromise) {
    mermaidPromise = import('mermaid')
      .then(({ default: mermaid }) => {
        mermaid.initialize({
          startOnLoad: false,
          // 'strict' makes mermaid sanitize its own SVG output (DOMPurify internally):
          // safe label HTML like <b>/<span> is kept, scripts/handlers are stripped.
          // This is why we can inject the returned SVG directly below without re-sanitizing.
          securityLevel: 'strict',
          theme: 'dark',
          themeVariables: {
            primaryColor: '#1a1a2e',
            primaryTextColor: 'rgba(255,255,255,0.85)',
            primaryBorderColor: 'rgba(255,255,255,0.2)',
            lineColor: 'rgba(255,255,255,0.4)',
            secondaryColor: '#16213e',
            tertiaryColor: '#0f3460',
            background: '#000000',
            mainBkg: 'rgba(255,255,255,0.04)',
            nodeBorder: 'rgba(255,255,255,0.16)',
            clusterBkg: 'rgba(255,255,255,0.02)',
            titleColor: '#FFFFFF',
            edgeLabelBackground: '#000000',
          },
          flowchart: {
            htmlLabels: true,
            curve: 'basis',
          },
        });
        return mermaid;
      })
      .catch((error) => {
        // Allow a later navigation to retry after a transient chunk-load failure.
        mermaidPromise = null;
        throw error;
      });
  }
  return mermaidPromise;
}

interface MarkdownRendererProps {
  content: string;
}

/**
 * Renders markdown content with dark theme styling matching the existing DocsPage design.
 * Uses `marked` to parse markdown into HTML, then renders with styled container.
 * Mermaid diagrams are rendered client-side after mount.
 */
const MarkdownRenderer: React.FC<MarkdownRendererProps> = ({ content }) => {
  const containerRef = useRef<HTMLDivElement>(null);
  const { t } = useTranslation();
  const { toastVisible, handleCopy } = useCopyToast();

  const html = useMemo(() => {
    // Custom renderer to generate heading IDs matching the TOC extraction logic
    const renderer = new Renderer();
    const headingIds = extractHeadingInfo(content).map(({ id }) => id);
    let headingIndex = 0;
    renderer.image = function ({ href, title, text }: { href: string; title?: string | null; text: string }) {
      const escapeAttr = (s: string) => s.replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
      const titleAttr = title ? ` title="${escapeAttr(title)}"` : '';
      return `<img src="${escapeAttr(href)}" alt="${escapeAttr(text)}"${titleAttr} />`;
    };
    renderer.heading = function ({ text, depth }: { text: string; depth: number }) {
      const { text: headingText, id: explicitId } = parseExplicitHeadingId(text);
      const id = headingIds[headingIndex++] ?? (explicitId ?? generateHeadingId(headingText));
      // Escape id attribute value to prevent XSS
      const safeId = id.replace(/"/g, '&quot;');
      return `<h${depth} id="${safeId}">${headingText}</h${depth}>\n`;
    };
    // Strip trailing newlines from code blocks to avoid empty line at bottom
    renderer.code = function ({ text, lang, escaped }: { text: string; lang?: string; escaped?: boolean }) {
      const trimmed = text.replace(/^\n+|\n+$/g, '');
      const content = escaped ? trimmed : trimmed.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
      const langClass = lang ? ` class="language-${lang}"` : '';
      return `<pre><code${langClass}>${content}</code></pre>\n`;
    };
    const instance = new Marked({ gfm: true, breaks: false, renderer });
    return DOMPurify.sanitize(instance.parse(content) as string);
  }, [content]);

  // Render mermaid diagrams and add copy buttons to code blocks after DOM update
  useEffect(() => {
    if (!containerRef.current) return;
    let cancelled = false;

    // Add copy buttons to all pre > code blocks (except mermaid)
    const preBlocks = containerRef.current.querySelectorAll('pre');
    preBlocks.forEach((pre) => {
      const codeEl = pre.querySelector('code');
      if (!codeEl || codeEl.classList.contains('language-mermaid')) return;
      if (pre.querySelector('.code-copy-btn')) return; // already added

      // Create copy button; visual styles live in docs-markdown.css
      const btn = document.createElement('div');
      btn.className = 'code-copy-btn';
      btn.innerHTML = `<img src="${copyIcon}" alt="copy" style="width:16px;height:16px;" />`;
      btn.addEventListener('click', () => {
        const text = codeEl.textContent || '';
        handleCopy(text);
      });
      pre.appendChild(btn);
    });

    // Render mermaid diagrams
    const mermaidBlocks = containerRef.current.querySelectorAll('code.language-mermaid');
    if (mermaidBlocks.length === 0) return;

    const renderMermaidBlocks = async () => {
      try {
        const mermaid = await loadMermaid();
        if (cancelled) return;

        for (const block of Array.from(mermaidBlocks)) {
          const pre = block.parentElement;
          if (!pre) continue;
          const code = block.textContent || '';
          try {
            const id = `mermaid-diagram-${crypto.randomUUID()}`;
            const { svg } = await mermaid.render(id, code);
            if (cancelled) return;
            // Replace the <pre> with rendered SVG. The SVG is produced by mermaid with
            // securityLevel:'strict' (configured in loadMermaid), which already sanitizes
            // its output. Re-running DOMPurify over the whole SVG breaks it (namespaces,
            // inline <style>, foreignObject labels), so we inject mermaid's trusted output.
            const wrapper = document.createElement('div');
            wrapper.className = 'mermaid-rendered';
            // codeql[js/xss-through-dom] -- svg is derived from user-controlled mermaid code, but mermaid
            // renders it with securityLevel:'strict' (see loadMermaid), which sanitizes the output via
            // DOMPurify (scripts/handlers stripped). The trust boundary relies on that setting staying 'strict'.
            wrapper.innerHTML = svg;
            pre.replaceWith(wrapper);
          } catch (e) {
            if (cancelled) return;
            // If rendering fails, show the code block normally
            (block as HTMLElement).style.display = 'block';
            console.warn('[Mermaid] render failed:', e);
          }
        }
      } catch (e) {
        if (cancelled) return;
        for (const block of Array.from(mermaidBlocks)) {
          (block as HTMLElement).style.display = 'block';
        }
        console.warn('[Mermaid] failed to load:', e);
      }
    };

    void renderMermaidBlocks();

    return () => { cancelled = true; };
  }, [html]);

  return (
    <>
      <div
        ref={containerRef}
        className="docs-markdown"
        dangerouslySetInnerHTML={{ __html: html }}
        style={{ width: '100%' }}
      />
      {ReactDOM.createPortal(
        <div
          style={{
            position: 'fixed',
            top: 88,
            left: '50%',
            transform: 'translateX(-50%)',
            background: 'rgba(255,255,255,0.1)',
            border: '1px solid rgba(255,255,255,0.2)',
            color: 'rgba(255,255,255,0.85)',
            padding: '5px 8px 5px 10px',
            borderRadius: 6,
            fontSize: 12,
            fontWeight: 500,
            pointerEvents: 'none',
            opacity: toastVisible ? 1 : 0,
            transition: 'opacity 0.15s ease',
            zIndex: 9999,
            backdropFilter: 'blur(8px)',
          }}
        >
          {t('quickstart.copied')}
        </div>,
        document.body
      )}
    </>
  );
};

export default MarkdownRenderer;
