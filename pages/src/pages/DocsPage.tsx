// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import React, { useState, useEffect, useMemo, useCallback, useRef } from 'react';
import { useParams, useNavigate, useLocation } from 'react-router-dom';
import { useTranslation } from '../i18n';
import type { Language } from '../i18n/types';
import Navbar from '../components/Navbar';
import Footer from '../components/Footer';
import MarkdownRenderer from '../components/MarkdownRenderer';
import { SearchTrigger } from '../components/SearchTrigger';
import { useResponsive } from '../hooks/useResponsive';
import { useCommandSearch, useSearchKeyboardNav } from '../hooks/useCommandSearch';
import { isChunkLoadError, reloadAfterChunkError } from '../components/ErrorBoundary';
import {
  getDocContent,
  getDocTitle,
  hasTranslation,
  isPartialLocale,
  loadLanguageContent,
  searchDocs,
  DocSlug,
  LoadedDocContent,
  LocalizedDocs,
} from '../content/docs';
import { extractHeadings } from '../utils/extractHeadings';
import docContentsIcon from '../assets/icons/doc-contents.svg';
import searchIcon from '../assets/icons/icon-search.svg';
import '../styles/docs-markdown.css';

// marked percent-encodes non-ASCII hrefs; heading ids are raw text from
// generateHeadingId, so fragments must be decoded before lookup.
function decodeFragment(fragment: string): string {
  try {
    return decodeURIComponent(fragment);
  } catch {
    return fragment;
  }
}

// Markdown renders a frame or more after navigation, so a fragment's heading
// may not exist yet. Retry across a few frames; the returned canceller stops a
// stale chain when navigation moves on.
function scrollToFragmentWhenReady(id: string): () => void {
  let frame = 0;
  let cancelled = false;
  const tryScroll = (attempts: number) => {
    if (cancelled) return;
    const el = document.getElementById(id);
    if (el) {
      el.scrollIntoView({ behavior: 'smooth', block: 'start' });
    } else if (attempts < 10) {
      frame = requestAnimationFrame(() => tryScroll(attempts + 1));
    }
  };
  frame = requestAnimationFrame(() => tryScroll(0));
  return () => {
    cancelled = true;
    cancelAnimationFrame(frame);
  };
}

/* ─── Sidebar tree data ─── */
interface SidebarItem {
  id: string;
  labelKey: string;
  slug?: DocSlug;
  children?: SidebarItem[];
}

interface SidebarGroup {
  groupLabelKey: string;
  items: SidebarItem[];
}

const sidebarTree: SidebarGroup[] = [
  {
    groupLabelKey: 'docs.sidebar.gettingStarted',
    items: [
      { id: 'sb-quickstart', labelKey: 'docs.sidebar.quickstart', slug: 'quickstart' },
      { id: 'sb-installation', labelKey: 'docs.sidebar.installation', slug: 'installation' },
      { id: 'sb-configuration', labelKey: 'docs.sidebar.configuration', slug: 'configuration' },
    ],
  },
  {
    groupLabelKey: 'docs.sidebar.userGuide',
    items: [
      { id: 'sb-cli', labelKey: 'docs.sidebar.cliReference', slug: 'cli-reference' },
      { id: 'sb-chatgpt-preview', labelKey: 'docs.sidebar.chatgptPreview', slug: 'chatgpt-preview' },
      { id: 'sb-rules', labelKey: 'docs.sidebar.reviewRules', slug: 'review-rules' },
      { id: 'sb-arch', labelKey: 'docs.sidebar.architecture', slug: 'architecture' },
      { id: 'sb-tools', labelKey: 'docs.sidebar.tools', slug: 'tools' },
      { id: 'sb-mcp', labelKey: 'docs.sidebar.mcp', slug: 'mcp' },
      { id: 'sb-viewer', labelKey: 'docs.sidebar.viewer', slug: 'viewer' },
      { id: 'sb-telemetry', labelKey: 'docs.sidebar.telemetry', slug: 'telemetry' },
      {
        id: 'sb-integrations',
        labelKey: 'docs.sidebar.integrations',
        children: [
          { id: 'sb-agent-skill', labelKey: 'docs.sidebar.agentSkill', slug: 'agent-skill' },
          { id: 'sb-claude-code', labelKey: 'docs.sidebar.claudeCode', slug: 'claude-code' },
          { id: 'sb-copilot-cli', labelKey: 'docs.sidebar.copilotCli', slug: 'copilot-cli' },
          { id: 'sb-delegate', labelKey: 'docs.sidebar.delegate', slug: 'delegate' },
          { id: 'sb-cicd', labelKey: 'docs.sidebar.cicd', slug: 'cicd' },
          { id: 'sb-acp', labelKey: 'docs.sidebar.acp', slug: 'acp' },
        ],
      },
      { id: 'sb-contributing', labelKey: 'docs.sidebar.contributing', slug: 'contributing' },
      { id: 'sb-faq', labelKey: 'docs.sidebar.faq', slug: 'faq' },
    ],
  },
];

/* ─── Chevron icon for expandable items ─── */
const ChevronIcon: React.FC<{ expanded: boolean }> = ({ expanded }) => (
  <svg width="16" height="16" viewBox="0 0 20 20" fill="none" style={{ flexShrink: 0, transition: 'transform 0.2s', transform: expanded ? 'rotate(90deg)' : 'rotate(0deg)' }}>
    <path d="M7.5 5L12.5 10L7.5 15" stroke="var(--text-ghost)" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
  </svg>
);

/* ─── Flat ordered list of all doc slugs for prev/next navigation ─── */
function buildFlatDocList(): { slug: DocSlug; labelKey: string }[] {
  const list: { slug: DocSlug; labelKey: string }[] = [];
  for (const group of sidebarTree) {
    for (const item of group.items) {
      if (item.slug) {
        list.push({ slug: item.slug, labelKey: item.labelKey });
      }
      if (item.children) {
        for (const child of item.children) {
          if (child.slug) {
            list.push({ slug: child.slug, labelKey: child.labelKey });
          }
        }
      }
    }
  }
  return list;
}

const flatDocList = buildFlatDocList();
const validSlugs = new Set<DocSlug>(flatDocList.map(d => d.slug));

/* Dev-time invariant: every sidebar slug maps to exactly one URL, so duplicates
 * (two menu entries sharing a slug) would silently collide. Fail loudly in dev. */
if (process.env.NODE_ENV !== 'production' && validSlugs.size !== flatDocList.length) {
  const slugs = flatDocList.map(d => d.slug);
  const dupes = [...new Set(slugs.filter((s, i) => slugs.indexOf(s) !== i))];
  throw new Error(
    `[docs] Duplicate sidebar slug(s) detected: ${dupes.join(', ')} — each doc must have a unique slug for routing.`
  );
}

const DocsPage: React.FC = () => {
  const { slug: slugParam } = useParams<{ slug?: string }>();
  const navigate = useNavigate();
  const { hash } = useLocation();
  /* Active doc slug is derived from the URL param, falling back to quickstart */
  const activeSlug: DocSlug =
    slugParam && validSlugs.has(slugParam as DocSlug) ? (slugParam as DocSlug) : 'quickstart';
  const [expandedItems, setExpandedItems] = useState<Record<string, boolean>>({ 'sb-integrations': true });
  const [activeHeadingId, setActiveHeadingId] = useState<string>('');
  const [hoveredHeadingId, setHoveredHeadingId] = useState<string>('');
  /* Cancels an in-flight click-triggered scroll when a newer one starts */
  const cancelPendingScroll = useRef<(() => void) | null>(null);
  const { t, language } = useTranslation();
  const { isMobile } = useResponsive();

  /* ─── Async docs content: load the selected language's bundle on demand ─── */
  type DocsLoadState =
    | { status: 'loading'; language: Language; attempt: number }
    | { status: 'ready'; content: LoadedDocContent }
    | { status: 'error'; language: Language; error: Error; attempt: number };

  /* English fallback for partial locales, tracked separately from the main
   * language so its own loading and failure can drive page and search UI. */
  type EnglishFallbackState =
    | { status: 'idle' }
    | { status: 'loading'; requestKey: string }
    | { status: 'ready'; docs: LocalizedDocs }
    | { status: 'error'; requestKey: string; error: Error; attempt: number };

  const [docState, setDocState] = useState<DocsLoadState>(() => ({ status: 'loading', language, attempt: 0 }));
  const [mainRetry, setMainRetry] = useState<{ language: Language; count: number }>({ language, count: 0 });
  const [englishFallback, setEnglishFallback] = useState<EnglishFallbackState>({ status: 'idle' });
  const [fallbackRetry, setFallbackRetry] = useState<{ requestKey: string; count: number }>({ requestKey: '', count: 0 });

  const loadedLanguage = docState.status === 'ready' ? docState.content.language : null;

  const loadedContent = useMemo<LoadedDocContent | null>(() => {
    if (docState.status !== 'ready') return null;
    return {
      language: docState.content.language,
      docs: docState.content.docs,
      english: englishFallback.status === 'ready' ? englishFallback.docs : undefined,
    };
  }, [docState, englishFallback]);

  /* Search stays synchronous over whatever content is loaded; results appear
   * once the load resolves and the page re-renders. */
  const runDocsSearch = useCallback(
    (query: string) => (loadedContent ? searchDocs(query, loadedContent) : []),
    [loadedContent],
  );

  const {
    searchOpen, setSearchOpen,
    searchQuery, setSearchQuery,
    searchSelectedIdx, setSearchSelectedIdx,
    searchInputRef,
    searchResults,
  } = useCommandSearch(runDocsSearch, language);
  const contentRef = React.useRef<HTMLDivElement>(null);

  const fontFamily = 'PingFang SC, -apple-system, BlinkMacSystemFont, sans-serif';

  /* Load the selected language's content. Cleanup flips `cancelled` so a
   * response landing after a language switch or retry is ignored. */
  useEffect(() => {
    let cancelled = false;
    const attempt = mainRetry.language === language ? mainRetry.count : 0;
    setDocState({ status: 'loading', language, attempt });
    loadLanguageContent(language)
      .then(docs => {
        if (!cancelled) setDocState({ status: 'ready', content: { language, docs } });
      })
      .catch(error => {
        if (!cancelled) setDocState({ status: 'error', language, error, attempt });
      });
    return () => { cancelled = true; };
  }, [language, mainRetry.language, mainRetry.count]);

  /* English fallback for partial locales: loaded only when the active page or
   * an active search needs the English entries — never speculatively. English
   * content is language-independent, so a load started by any trigger settles
   * the shared state no matter which trigger remains active; the attempt key
   * keeps a failed attempt from re-firing in a loop until the trigger changes
   * or the user retries. */
  const fallbackAttemptKey = useRef('');
  useEffect(() => {
    if (loadedLanguage === null || loadedLanguage === 'en') return;
    const pageNeedsFallback = !hasTranslation(activeSlug, loadedLanguage);
    const searchNeedsFallback = searchQuery.trim() !== '' && isPartialLocale(loadedLanguage);
    if (!pageNeedsFallback && !searchNeedsFallback) {
      fallbackAttemptKey.current = '';
      return;
    }
    if (englishFallback.status === 'ready' || englishFallback.status === 'loading') return;
    const key = `${loadedLanguage}:${activeSlug}:${searchNeedsFallback}`;
    const attempt = fallbackRetry.requestKey === key ? fallbackRetry.count : 0;
    if (fallbackAttemptKey.current === `${key}:${attempt}`) return;
    fallbackAttemptKey.current = `${key}:${attempt}`;
    setEnglishFallback({ status: 'loading', requestKey: key });
    loadLanguageContent('en')
      .then(docs => {
        setEnglishFallback({ status: 'ready', docs });
      })
      .catch(error => {
        setEnglishFallback({ status: 'error', requestKey: key, error, attempt });
      });
  }, [loadedLanguage, activeSlug, searchQuery, englishFallback.status, fallbackRetry.requestKey, fallbackRetry.count]);

  /* ─── Effective states for the page body and the search modal ───
   * Both read the same content: the main language bundle, plus English while
   * a partial locale needs it. "No results" is only shown once every piece of
   * content the search depends on has actually arrived. */
  const pageNeedsFallback =
    loadedLanguage !== null && loadedLanguage !== 'en' && !hasTranslation(activeSlug, loadedLanguage);
  const pageStatus: 'loading' | 'error' | 'ready' =
    docState.status !== 'ready'
      ? docState.status
      : pageNeedsFallback && englishFallback.status !== 'ready'
        ? (englishFallback.status === 'error' ? 'error' : 'loading')
        : 'ready';

  const mainChunkError = docState.status === 'error' && isChunkLoadError(docState.error);
  const fallbackChunkError = englishFallback.status === 'error' && isChunkLoadError(englishFallback.error);
  const canReloadMainChunk = mainChunkError && docState.attempt > 0;
  const canReloadFallbackChunk = fallbackChunkError && englishFallback.attempt > 0;

  const retryPageLoad = useCallback(() => {
    if (docState.status === 'error') {
      setMainRetry({ language: docState.language, count: docState.attempt + 1 });
    } else if (pageNeedsFallback && englishFallback.status === 'error') {
      setFallbackRetry({ requestKey: englishFallback.requestKey, count: englishFallback.attempt + 1 });
    }
  }, [docState, pageNeedsFallback, englishFallback]);

  const reloadPageAfterChunkError = useCallback(() => {
    reloadAfterChunkError();
  }, []);

  const searchNeedsEnglish =
    loadedLanguage !== null && loadedLanguage !== 'en'
    && searchQuery.trim() !== '' && isPartialLocale(loadedLanguage);
  const searchContentStatus: 'loading' | 'error' | 'ready' =
    docState.status === 'loading'
      ? 'loading'
      : docState.status === 'error'
        ? 'error'
        : searchNeedsEnglish && englishFallback.status !== 'ready'
          ? (englishFallback.status === 'error' ? 'error' : 'loading')
          : 'ready';

  const retrySearchContent = useCallback(() => {
    if (docState.status === 'error') {
      setMainRetry({ language: docState.language, count: docState.attempt + 1 });
    } else if (searchNeedsEnglish && englishFallback.status === 'error') {
      setFallbackRetry({ requestKey: englishFallback.requestKey, count: englishFallback.attempt + 1 });
    }
  }, [docState, searchNeedsEnglish, englishFallback]);

  /* Get markdown content for current doc */
  const docContent = useMemo(
    () => (loadedContent ? getDocContent(activeSlug, loadedContent) : ''),
    [activeSlug, loadedContent],
  );
  const docTitle = useMemo(
    () => (loadedContent ? getDocTitle(activeSlug, loadedContent) : ''),
    [activeSlug, loadedContent],
  );
  const headings = useMemo(() => extractHeadings(docContent), [docContent]);

  /* Scroll direct links after their markdown heading has rendered */
  useEffect(() => {
    const fragment = hash.startsWith('#') ? hash.slice(1) : hash;
    if (!fragment) return;
    return scrollToFragmentWhenReady(decodeFragment(fragment));
  }, [hash, docContent]);

  /* Track active heading via IntersectionObserver */
  useEffect(() => {
    if (headings.length === 0) return;
    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (entry.isIntersecting) {
            setActiveHeadingId(entry.target.id);
          }
        }
      },
      { rootMargin: '-80px 0px -60% 0px', threshold: 0 }
    );
    const els = headings.map(h => document.getElementById(h.id)).filter(Boolean) as HTMLElement[];
    els.forEach(el => observer.observe(el));
    return () => observer.disconnect();
  }, [headings]);

  /* Prev/Next navigation */
  const { prevDoc, nextDoc } = useMemo(() => {
    const idx = flatDocList.findIndex(d => d.slug === activeSlug);
    return {
      prevDoc: idx > 0 ? flatDocList[idx - 1] : null,
      nextDoc: idx < flatDocList.length - 1 ? flatDocList[idx + 1] : null,
    };
  }, [activeSlug]);

  const toggleExpand = useCallback((id: string) => {
    setExpandedItems(prev => ({ ...prev, [id]: !prev[id] }));
  }, []);

  const navigateToDoc = useCallback((slug: DocSlug) => {
    navigate(`/docs/${slug}`);
    // Scroll page to top
    window.scrollTo(0, 0);
  }, [navigate]);

  /* Intercept clicks on internal doc links and convert to SPA navigation */
  const handleContentClick = useCallback((e: React.MouseEvent<HTMLDivElement>) => {
    const target = e.target as HTMLElement;
    const anchor = target.closest('a') as HTMLAnchorElement | null;
    if (!anchor) return;
    const href = anchor.getAttribute('href');
    if (!href) return;
    // Skip external links
    if (href.startsWith('http://') || href.startsWith('https://')) return;
    // Skip pure anchors (same-page scroll)
    if (href.startsWith('#')) {
      e.preventDefault();
      const id = decodeFragment(href.slice(1));
      const el = document.getElementById(id);
      if (el) el.scrollIntoView({ behavior: 'smooth', block: 'start' });
      return;
    }
    // Parse relative paths to extract slug
    // Patterns: ../slug/, slug/, ../../slug/, ../slug/#anchor
    const pathOnly = href.split('#')[0].replace(/\/+$/, ''); // remove trailing slash & anchor
    const segments = pathOnly.split('/').filter(s => s !== '' && s !== '.' && s !== '..');
    const lastSegment = segments[segments.length - 1];
    if (!lastSegment) return;
    // Map path segment to DocSlug (ci -> cicd)
    const slugMap: Record<string, DocSlug> = { 'ci': 'cicd' };
    const slug = (slugMap[lastSegment] || lastSegment) as DocSlug;
    // Verify it's a valid doc slug
    if (validSlugs.has(slug)) {
      e.preventDefault();
      navigateToDoc(slug);
      // Handle anchor scroll after navigation with reliable retry
      const anchor2raw = href.split('#')[1];
      const anchor2 = anchor2raw ? decodeFragment(anchor2raw) : undefined;
      if (anchor2) {
        cancelPendingScroll.current?.();
        cancelPendingScroll.current = scrollToFragmentWhenReady(anchor2);
      }
    }
  }, [navigateToDoc]);

  const scrollToHeading = useCallback((id: string) => {
    const el = document.getElementById(id);
    if (el) {
      const top = el.getBoundingClientRect().top + window.scrollY - 90;
      window.scrollTo({ top, behavior: 'smooth' });
    }
  }, []);

  /* Auto-expand parent when a child is active */
  useEffect(() => {
    for (const group of sidebarTree) {
      for (const item of group.items) {
        if (item.children && item.children.some(c => c.slug === activeSlug)) {
          setExpandedItems(prev => ({ ...prev, [item.id]: true }));
        }
      }
    }
  }, [activeSlug]);

  /* Handle search result selection */
  const handleSearchSelect = useCallback((slug: DocSlug) => {
    navigateToDoc(slug);
    setSearchOpen(false);
  }, [navigateToDoc, setSearchOpen]);

  /* Keyboard navigation in search modal */
  const handleSearchKeyDown = useSearchKeyboardNav(
    searchResults, searchSelectedIdx, setSearchSelectedIdx, handleSearchSelect,
  );

  return (
    <div style={{ minHeight: '100vh', background: 'var(--bg-page)', paddingTop: 72, fontFamily }}>
      <Navbar />
      {/* Main layout: left sidebar + content + right TOC */}
      <div style={{ display: 'flex', justifyContent: 'flex-start', alignItems: 'flex-start', maxWidth: 1440, margin: '0 auto', minHeight: 'calc(100vh - 72px)' }}>

        {/* ─── Left sidebar: tree navigation ─── */}
        {!isMobile && (
          <nav style={{
            position: 'sticky',
            top: 72,
            width: 264,
            flexShrink: 0,
            height: 'calc(100vh - 72px)',
            overflowY: 'auto',
            paddingTop: 40,
            paddingBottom: 40,
            paddingRight: 12,
            paddingLeft: 24,
            borderRight: 'none',
          }}>
            {/* Search trigger button */}
            <SearchTrigger
              placeholder={t('docs.search.placeholder')}
              onClick={() => setSearchOpen(true)}
              style={{ width: '100%', justifyContent: 'space-between', marginBottom: 20 }}
            />

            {sidebarTree.map((group, gi) => (
              <div key={gi} style={{ display: 'flex', flexDirection: 'column', marginBottom: 16 }}>
                {/* Group header */}
                <div style={{
                  width: '100%',
                  display: 'flex',
                  alignItems: 'center',
                  gap: 8,
                  padding: '8px 12px 12px 12px',
                }}>
                  <span style={{ flexShrink: 0, fontSize: 14, fontWeight: 600, color: 'var(--text-strong)', fontFamily, letterSpacing: '0.04em', textTransform: 'uppercase' }}>
                    {t(group.groupLabelKey)}
                  </span>
                </div>
                {/* Group items */}
                {group.items.map((item) => {
                  const isActive = item.slug != null && item.slug === activeSlug;
                  const hasChildren = item.children && item.children.length > 0;
                  const isExpanded = expandedItems[item.id] ?? false;
                  return (
                    <React.Fragment key={item.id}>
                      <div
                        onClick={() => {
                          if (item.slug) {
                            navigateToDoc(item.slug);
                          } else if (hasChildren) {
                            toggleExpand(item.id);
                          }
                        }}
                        style={{
                          height: 36,
                          display: 'flex',
                          alignItems: 'center',
                          gap: 8,
                          borderRadius: 6,
                          padding: '10px 12px',
                          cursor: 'pointer',
                          transition: 'background 0.15s',
                          background: isActive ? 'var(--accent-soft)' : 'transparent',
                        }}
                      >
                        <div style={{ display: 'flex', flex: 1, alignItems: 'center', gap: 8 }}>
                          <span style={{
                            flexShrink: 0,
                            fontSize: 14,
                            fontFamily,
                            fontWeight: isActive ? 500 : 400,
                            color: isActive ? 'var(--accent)' : 'var(--text-soft)',
                            lineHeight: '22px',
                            transition: 'color 0.2s',
                          }}>
                            {t(item.labelKey)}
                          </span>
                        </div>
                        {hasChildren && (
                          <div
                            onClick={(e) => {
                              e.stopPropagation();
                              toggleExpand(item.id);
                            }}
                            style={{ display: 'flex', alignItems: 'center', padding: 2 }}
                          >
                            <ChevronIcon expanded={isExpanded} />
                          </div>
                        )}
                      </div>
                      {/* Children (sub-items) */}
                      {hasChildren && isExpanded && item.children!.map((child) => {
                        const childActive = child.slug != null && child.slug === activeSlug;
                        return (
                          <div
                            key={child.id}
                            onClick={() => { if (child.slug) navigateToDoc(child.slug); }}
                            style={{
                              height: 36,
                              display: 'flex',
                              alignItems: 'center',
                              gap: 8,
                              borderRadius: 6,
                              padding: '10px 12px 10px 28px',
                              cursor: 'pointer',
                              background: childActive ? 'var(--accent-soft)' : 'transparent',
                            }}
                          >
                            <div style={{ display: 'flex', flex: 1, alignItems: 'center', gap: 8 }}>
                              <span style={{
                                flexShrink: 0,
                                fontSize: 14,
                                fontFamily,
                                fontWeight: childActive ? 500 : 400,
                                color: childActive ? 'var(--accent)' : 'var(--text-soft)',
                                lineHeight: '22px',
                                transition: 'color 0.2s',
                              }}>
                                {t(child.labelKey)}
                              </span>
                            </div>
                          </div>
                        );
                      })}
                    </React.Fragment>
                  );
                })}
              </div>
            ))}
          </nav>
        )}

        {/* ─── Main content area ─── */}
        <div ref={contentRef} onClick={handleContentClick} style={{ display: 'flex', flex: 1, flexDirection: 'column', minWidth: 0, padding: isMobile ? '32px 20px 80px' : '40px 48px 80px' }}>

          {pageStatus === 'loading' && (
            <div role="status" style={{ padding: '96px 0', textAlign: 'center', color: 'var(--text-faint)', fontSize: 14, fontFamily }}>
              {t('docs.content.loading')}
            </div>
          )}
          {pageStatus === 'error' && (
            <div role="alert" style={{ padding: '96px 0', textAlign: 'center', fontSize: 14, fontFamily }}>
              <div style={{ color: 'var(--text-muted)', marginBottom: 16 }}>
                {t('docs.content.loadError')}
              </div>
              <button
                onClick={retryPageLoad}
                style={{
                  background: 'var(--accent-soft)',
                  border: 'none',
                  borderRadius: 6,
                  color: 'var(--accent)',
                  cursor: 'pointer',
                  fontSize: 14,
                  fontFamily,
                  padding: '8px 20px',
                }}
              >
                {t('docs.content.retry')}
              </button>
              {(canReloadMainChunk || canReloadFallbackChunk) && (
                <button
                  onClick={reloadPageAfterChunkError}
                  style={{
                    background: 'none',
                    border: 'none',
                    color: 'var(--accent)',
                    cursor: 'pointer',
                    display: 'block',
                    fontSize: 14,
                    fontFamily,
                    margin: '16px auto 0',
                    padding: 0,
                    textDecoration: 'underline',
                  }}
                >
                  {t('error.reload')}
                </button>
              )}
            </div>
          )}
          {pageStatus === 'ready' && (
            <>
              {/* Doc title */}
              <h1 style={{ fontSize: 28, fontWeight: 700, color: 'var(--text-strong)', margin: '0 0 32px 0', lineHeight: '36px', fontFamily }}>
                {docTitle}
              </h1>
              {/* Rendered markdown content */}
              <MarkdownRenderer content={docContent} />


              {/* ─── Prev / Next pagination ─── */}
              <div style={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            marginTop: 56,
          }}>
            {prevDoc ? (
              <button
                onClick={() => navigateToDoc(prevDoc.slug)}
                style={{
                  background: 'none',
                  border: 'none',
                  cursor: 'pointer',
                  display: 'flex',
                  alignItems: 'center',
                  gap: 6,
                  padding: 0,
                  transition: 'opacity 0.2s',
                }}
              >
                <span style={{ fontSize: 14, color: 'var(--text-faint)' }}>‹</span>
                <span style={{ fontSize: 14, fontFamily, color: 'var(--text-soft)', fontWeight: 400 }}>
                  {t(prevDoc.labelKey)}
                </span>
              </button>
            ) : <span />}
            {nextDoc ? (
              <button
                onClick={() => navigateToDoc(nextDoc.slug)}
                style={{
                  background: 'none',
                  border: 'none',
                  cursor: 'pointer',
                  display: 'flex',
                  alignItems: 'center',
                  gap: 6,
                  padding: 0,
                  transition: 'opacity 0.2s',
                }}
              >
                <span style={{ fontSize: 14, fontFamily, color: 'var(--text-soft)', fontWeight: 400 }}>
                  {t(nextDoc.labelKey)}
                </span>
                <span style={{ fontSize: 14, color: 'var(--text-faint)' }}>›</span>
              </button>
            ) : <span />}
              </div>
            </>
          )}
        </div>

        {/* ─── Right sidebar: page TOC ─── */}
        {!isMobile && headings.length > 0 && (
          <div style={{
            position: 'sticky',
            top: 72,
            width: 220,
            flexShrink: 0,
            height: 'calc(100vh - 72px)',
            overflowY: 'auto',
            overflowX: 'hidden',
            paddingLeft: 20,
            paddingRight: 24,
            paddingTop: 40,
          }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 16 }}>
              <img src={docContentsIcon} alt="" style={{ width: 20, height: 20 }} />
              <span style={{ fontSize: 14, fontWeight: 500, color: 'var(--text-faint)', letterSpacing: '0.05em', position: 'relative', top: 1 }}>
                {t('docs.toc')}
              </span>
            </div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
              {headings.map((h, i) => {
                const isActive = h.id === activeHeadingId;
                const isHovered = h.id === hoveredHeadingId;
                return (
                  <button
                    key={i}
                    onClick={() => scrollToHeading(h.id)}
                    onMouseEnter={() => setHoveredHeadingId(h.id)}
                    onMouseLeave={() => setHoveredHeadingId('')}
                    style={{
                      background: 'none',
                      border: 'none',
                      cursor: 'pointer',
                      textAlign: 'left',
                      fontSize: 14,
                      fontFamily: 'PingFang SC, -apple-system, sans-serif',
                      fontWeight: isActive ? 500 : 400,
                      color: isActive ? 'var(--accent)' : isHovered ? 'var(--text-body)' : 'var(--text-faint)',
                      lineHeight: '22px',
                      padding: 0,
                      paddingLeft: h.level === 3 ? 16 : 0,
                      transition: 'color 0.2s',
                    }}
                  >
                    {h.text}
                  </button>
                );
              })}
            </div>
          </div>
        )}
      </div>
      <Footer />

      {/* Search Modal */}
      {searchOpen && (
        <div
          style={{
            position: 'fixed',
            top: 0,
            left: 0,
            right: 0,
            bottom: 0,
            background: 'rgba(0,0,0,0.6)',
            zIndex: 9999,
            display: 'flex',
            justifyContent: 'center',
            alignItems: 'flex-start',
            paddingTop: 120,
          }}
          onClick={() => setSearchOpen(false)}
        >
          <div
            style={{
              width: 560,
              maxWidth: '90vw',
              background: 'var(--surface-raised)',
              border: '1px solid var(--line)',
              borderRadius: 12,
              overflow: 'hidden',
              boxShadow: '0 24px 48px rgba(0,0,0,0.4)',
            }}
            onClick={e => e.stopPropagation()}
          >
            {/* Search input */}
            <div style={{ display: 'flex', alignItems: 'center', padding: '12px 16px' }}>
              <img src={searchIcon} alt="" style={{ width: 16, height: 16, flexShrink: 0, opacity: 0.6 }} />
              <input
                ref={searchInputRef}
                type="text"
                value={searchQuery}
                onChange={e => setSearchQuery(e.target.value)}
                onKeyDown={handleSearchKeyDown}
                placeholder={t('docs.search.placeholder')}
                style={{
                  flex: 1,
                  marginLeft: 12,
                  background: 'transparent',
                  border: 'none',
                  outline: 'none',
                  color: 'var(--text-strong)',
                  fontSize: 14,
                  fontFamily,
                }}
              />
            </div>
            {/* Results — pending or failed content shows its own state instead
             * of a misleading "no results"; "no results" appears only once
             * every content the search depends on has arrived. */}
            <div style={{ maxHeight: 400, overflowY: 'auto', padding: searchQuery ? '8px 0' : '0' }}>

              {searchQuery && searchContentStatus === 'loading' && (
                <div style={{ padding: '24px 16px', textAlign: 'center', color: 'var(--text-faint)', fontSize: 14 }}>
                  {t('docs.search.loading')}
                </div>
              )}
              {searchQuery && searchContentStatus === 'error' && (
                <div style={{ padding: '24px 16px', textAlign: 'center', fontSize: 14 }}>
                  <span style={{ color: 'var(--text-muted)' }}>{t('docs.search.loadError')}</span>{' '}
                  <button
                    onClick={retrySearchContent}
                    style={{
                      background: 'none',
                      border: 'none',
                      color: 'var(--accent)',
                      cursor: 'pointer',
                      fontSize: 14,
                      fontFamily,
                      padding: 0,
                      textDecoration: 'underline',
                    }}
                  >
                    {t('docs.content.retry')}
                  </button>
                  {((docState.status === 'error' && isChunkLoadError(docState.error) && docState.attempt > 0)
                    || (searchNeedsEnglish && fallbackChunkError && englishFallback.attempt > 0)) && (
                    <button
                      onClick={reloadPageAfterChunkError}
                      style={{
                        background: 'none',
                        border: 'none',
                        color: 'var(--accent)',
                        cursor: 'pointer',
                        fontSize: 14,
                        fontFamily,
                        marginLeft: 12,
                        padding: 0,
                        textDecoration: 'underline',
                      }}
                    >
                      {t('error.reload')}
                    </button>
                  )}
                </div>
              )}
              {searchQuery && searchContentStatus === 'ready' && searchResults.length === 0 && (
                <div style={{ padding: '24px 16px', textAlign: 'center', color: 'var(--text-faint)', fontSize: 14 }}>

                  {t('docs.search.noResults')}
                </div>
              )}
              {searchResults.map((result, idx) => (
                <button
                  key={result.slug}
                  onClick={() => handleSearchSelect(result.slug)}
                  style={{
                    display: 'block',
                    width: '100%',
                    padding: '10px 16px',
                    background: idx === searchSelectedIdx ? 'var(--fill-hover)' : 'transparent',
                    border: 'none',
                    cursor: 'pointer',
                    textAlign: 'left',
                    outline: 'none',
                    transition: 'background 0.1s',
                  }}
                  onMouseEnter={() => setSearchSelectedIdx(idx)}
                >
                  <div style={{ color: 'var(--text-strong)', fontSize: 14, fontWeight: 500, fontFamily, marginBottom: 4 }}>
                    {result.title}
                  </div>
                  <div style={{ color: 'var(--text-ghost)', fontSize: 12, fontFamily, lineHeight: '18px', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                    {result.snippet}
                  </div>
                </button>
              ))}
            </div>
            {/* Footer hints */}
            {searchResults.length > 0 && (
              <div style={{
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'space-between',
                padding: '8px 16px',
                borderTop: '1px solid var(--line)',
                fontSize: 12,
                color: 'var(--text-ghost)',
                fontFamily,
              }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 16 }}>
                  <span style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
                    <kbd style={{ background: 'var(--fill-hover)', border: '1px solid var(--line)', borderRadius: 3, padding: '0px 3px', fontSize: 9, color: 'var(--text-strong)' }}>↑</kbd>
                    <kbd style={{ background: 'var(--fill-hover)', border: '1px solid var(--line)', borderRadius: 3, padding: '0px 3px', fontSize: 9, color: 'var(--text-strong)' }}>↓</kbd>
                    {t('docs.search.hint.select')}
                  </span>
                  <span style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
                    <kbd style={{ background: 'var(--fill-hover)', border: '1px solid var(--line)', borderRadius: 3, padding: '0px 3px', fontSize: 9, color: 'var(--text-strong)' }}>↵</kbd>
                    {t('docs.search.hint.open')}
                  </span>
                </div>
                <span style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
                  <kbd style={{ background: 'var(--fill-hover)', border: '1px solid var(--line)', borderRadius: 3, padding: '0px 3px', fontSize: 9, color: 'var(--text-strong)' }}>esc</kbd>
                  {t('docs.search.hint.close')}
                </span>
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
};

export default DocsPage;
