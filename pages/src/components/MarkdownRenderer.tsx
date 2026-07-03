import React, { useMemo, useEffect, useRef, useState, useCallback } from 'react';
import ReactDOM from 'react-dom';
import { Marked, Renderer } from 'marked';
import DOMPurify from 'dompurify';
import mermaid from 'mermaid';
import { useTranslation } from '../i18n';
import copyIcon from '../assets/icons/icon-copy.svg';
import { createHeadingIdGenerator } from '../utils/headingId';

// Initialize mermaid with dark theme
mermaid.initialize({
  startOnLoad: false,
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
  const [toastVisible, setToastVisible] = useState(false);

  const handleCopy = useCallback((text: string) => {
    if (navigator.clipboard && window.isSecureContext) {
      navigator.clipboard.writeText(text).then(() => {
        setToastVisible(true);
      }).catch(() => fallbackCopy(text));
    } else {
      fallbackCopy(text);
    }
  }, []);

  const fallbackCopy = (text: string) => {
    const textarea = document.createElement('textarea');
    textarea.value = text;
    textarea.style.position = 'fixed';
    textarea.style.opacity = '0';
    document.body.appendChild(textarea);
    textarea.select();
    const success = document.execCommand('copy');
    document.body.removeChild(textarea);
    if (success) setToastVisible(true);
  };

  useEffect(() => {
    if (!toastVisible) return;
    const timer = setTimeout(() => setToastVisible(false), 1200);
    return () => clearTimeout(timer);
  }, [toastVisible]);

  const html = useMemo(() => {
    // Custom renderer to generate heading IDs matching the TOC extraction logic
    const renderer = new Renderer();
    const getHeadingId = createHeadingIdGenerator();
    renderer.heading = function ({ text, depth }: { text: string; depth: number }) {
      const id = getHeadingId(text);
      // Escape id attribute value to prevent XSS
      const safeId = id.replace(/"/g, '&quot;');
      return `<h${depth} id="${safeId}">${text}</h${depth}>\n`;
    };
    // Strip trailing newlines from code blocks to avoid empty line at bottom
    renderer.code = function ({ text, lang, escaped }: { text: string; lang?: string; escaped?: boolean }) {
      const trimmed = text.replace(/^\n+|\n+$/g, '');
      const content = escaped ? trimmed : trimmed.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
      const langClass = lang ? ` class="language-${lang}"` : '';
      const isMultiline = trimmed.includes('\n');
      const alignItems = isMultiline ? 'flex-start' : 'center';
      return `<pre style="align-items:${alignItems};"><code${langClass}>${content}</code></pre>\n`;
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

      // Create copy button matching reference HTML
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

    Promise.all(Array.from(mermaidBlocks).map(async (block) => {
      const pre = block.parentElement;
      if (!pre) return;
      const code = block.textContent || '';
      try {
        const id = `mermaid-diagram-${crypto.randomUUID()}`;
        const { svg } = await mermaid.render(id, code);
        if (cancelled) return;
        // Replace the <pre> with rendered SVG
        const wrapper = document.createElement('div');
        wrapper.className = 'mermaid-rendered';
        wrapper.innerHTML = svg;
        pre.replaceWith(wrapper);
      } catch (e) {
        if (cancelled) return;
        // If rendering fails, show the code block normally
        (block as HTMLElement).style.display = 'block';
        console.warn('[Mermaid] render failed:', e);
      }
    })).catch((e) => {
      console.warn('[Mermaid] unexpected render error:', e);
    });

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
        <div className={`copy-toast${toastVisible ? ' visible' : ''}`}>
          {t('quickstart.copied')}
        </div>,
        document.body
      )}
    </>
  );
};

export default MarkdownRenderer;
