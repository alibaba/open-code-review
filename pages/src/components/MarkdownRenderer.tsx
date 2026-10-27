// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import React, { useMemo, useEffect, useRef } from 'react';
import ReactDOM from 'react-dom';
import { Marked, Renderer } from 'marked';
import DOMPurify from 'dompurify';
import { useTranslation } from '../i18n';
import { useTheme } from '../theme/ThemeProvider';
import { useCopyToast } from '../hooks/useCopyToast';
import copyIcon from '../assets/icons/icon-copy.svg';
import { extractHeadingInfo, generateHeadingId, parseExplicitHeadingId } from '../utils/headingId';

type Mermaid = typeof import('mermaid')['default'];

let mermaidPromise: Promise<Mermaid> | null = null;

const mermaidThemes: Record<'dark' | 'light', Parameters<Mermaid['initialize']>[0]> = {
  dark: {
    theme: 'dark',
    themeVariables: {
      primaryColor: '#1a1a2e',
      primaryTextColor: 'rgba(255,255,255,0.85)',
      primaryBorderColor: 'rgba(255,255,255,0.2)',
      lineColor: 'rgba(255,255,255,0.4)',
      secondaryColor: '#16213e',
      tertiaryColor: '#0f3460',
      background: 'var(--bg-page)',
      mainBkg: 'rgba(255,255,255,0.04)',
      nodeBorder: 'rgba(255,255,255,0.16)',
      clusterBkg: 'rgba(255,255,255,0.02)',
      titleColor: '#FFFFFF',
      edgeLabelBackground: '#000000',
    },
  },
  light: {
    theme: 'default',
    themeVariables: {
      primaryColor: '#eef2f7',
      primaryTextColor: 'rgba(10,15,30,0.85)',
      primaryBorderColor: 'rgba(10,15,30,0.2)',
      lineColor: 'rgba(10,15,30,0.4)',
      secondaryColor: '#f6f7f9',
      tertiaryColor: '#e2e8f0',
      background: 'var(--bg-page)',
      mainBkg: 'rgba(10,15,30,0.04)',
      nodeBorder: 'rgba(10,15,30,0.16)',
      clusterBkg: 'rgba(10,15,30,0.02)',
      titleColor: '#0a0f1e',
      edgeLabelBackground: '#ffffff',
    },
  },
};

let mermaidTheme: 'dark' | 'light' | null = null;

function configureMermaid(mermaid: Mermaid, theme: 'dark' | 'light') {
  if (mermaidTheme === theme) return;
  mermaidTheme = theme;
  mermaid.initialize({
    startOnLoad: false,
    // 'strict' makes mermaid sanitize its own SVG output (DOMPurify internally):
    // safe label HTML like <b>/<span> is kept, scripts/handlers are stripped.
    // This is why we can inject the returned SVG directly below without re-sanitizing.
    securityLevel: 'strict',
    ...mermaidThemes[theme],
    flowchart: {
      htmlLabels: true,
      curve: 'basis',
    },
  });
}

function loadMermaid(): Promise<Mermaid> {
  if (!mermaidPromise) {
    mermaidPromise = import('mermaid')
      .then(({ default: mermaid }) => mermaid)
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
  const { theme } = useTheme();
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

    // Render mermaid diagrams. Two target shapes: code.language-mermaid on a
    // first render of this html, and .mermaid-rendered wrappers carrying their
    // source in data-mermaid-source after a theme change — by then the
    // original <pre><code> elements no longer exist, and React will not reset
    // dangerouslySetInnerHTML whose html is unchanged, so without the wrappers
    // the diagrams could never re-render with the other palette.
    const mermaidTargets: Array<{ code: string; mount: Element; fresh: boolean }> = [];
    containerRef.current.querySelectorAll('code.language-mermaid').forEach((block) => {
      const pre = block.parentElement;
      if (pre) mermaidTargets.push({ code: block.textContent || '', mount: pre, fresh: true });
    });
    containerRef.current.querySelectorAll('.mermaid-rendered[data-mermaid-source]').forEach((wrapper) => {
      mermaidTargets.push({ code: wrapper.getAttribute('data-mermaid-source') || '', mount: wrapper, fresh: false });
    });
    if (mermaidTargets.length === 0) return;

    const renderMermaidBlocks = async () => {
      try {
        const mermaid = await loadMermaid();
        configureMermaid(mermaid, theme);
        if (cancelled) return;

        for (const { code, mount, fresh } of mermaidTargets) {
          if (!mount.isConnected || !code) continue;
          try {
            const id = `mermaid-diagram-${crypto.randomUUID()}`;
            const { svg } = await mermaid.render(id, code);
            if (cancelled) return;
            // Replace the mount (the <pre>, or the previous wrapper on a
            // re-render) with the rendered SVG. The SVG is produced by mermaid
            // with securityLevel:'strict' (configured in loadMermaid), which
            // already sanitizes its output. Re-running DOMPurify over the
            // whole SVG breaks it (namespaces, inline <style>, foreignObject
            // labels), so we inject mermaid's trusted output.
            const wrapper = document.createElement('div');
            wrapper.className = 'mermaid-rendered';
            wrapper.setAttribute('data-mermaid-source', code);
            // codeql[js/xss-through-dom] -- svg is derived from user-controlled mermaid code, but mermaid
            // renders it with securityLevel:'strict' (see loadMermaid), which sanitizes the output via
            // DOMPurify (scripts/handlers stripped). The trust boundary relies on that setting staying 'strict'.
            wrapper.innerHTML = svg;
            mount.replaceWith(wrapper);
          } catch (e) {
            if (cancelled) return;
            // A fresh block that fails falls back to showing the code. A
            // wrapper that fails to re-render keeps the previous diagram —
            // the old palette beats a blank spot.
            if (fresh) (mount.querySelector('code') as HTMLElement | null)?.style.setProperty('display', 'block');
            console.warn('[Mermaid] render failed:', e);
          }
        }
      } catch (e) {
        if (cancelled) return;
        for (const { mount, fresh } of mermaidTargets) {
          if (fresh) (mount.querySelector('code') as HTMLElement | null)?.style.setProperty('display', 'block');
        }
        console.warn('[Mermaid] failed to load:', e);
      }
    };

    void renderMermaidBlocks();

    return () => { cancelled = true; };
    // theme: re-render diagrams with the other palette when it changes
  }, [html, theme]);

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
            background: 'var(--fill-hover)',
            border: '1px solid var(--line)',
            color: 'var(--text-body)',
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
