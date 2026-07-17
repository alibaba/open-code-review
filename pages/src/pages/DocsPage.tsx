import React, { useState, useEffect, useMemo, useCallback, useRef } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { useTranslation } from '../i18n';
import Navbar from '../components/Navbar';
import Footer from '../components/Footer';
import MarkdownRenderer from '../components/MarkdownRenderer';
import { useResponsive } from '../hooks/useResponsive';
import { getDocContent, getDocTitle, DocSlug, searchDocs } from '../content/docs';
import { generateHeadingId } from '../utils/headingId';
import docContentsIcon from '../assets/icons/doc-contents.svg';
import searchIcon from '../assets/icons/icon-search.svg';
import '../styles/docs-markdown.css';

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
          { id: 'sb-delegate', labelKey: 'docs.sidebar.delegate', slug: 'delegate' },
          { id: 'sb-subprocess', labelKey: 'docs.sidebar.subprocess', slug: 'subprocess' },
          { id: 'sb-cicd', labelKey: 'docs.sidebar.cicd', slug: 'cicd' },
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
    <path d="M7.5 5L12.5 10L7.5 15" stroke="rgba(255,255,255,0.4)" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
  </svg>
);

/* ─── Extract headings from markdown for right TOC ─── */
function extractHeadings(markdown: string): { id: string; text: string; level: number }[] {
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
      // Strip markdown link syntax [text](url) → text, then strip other formatting
      const text = match[2]
        .replace(/\[([^\]]+)\]\([^)]*\)/g, '$1')
        .replace(/[`*_\[\]()]/g, '')
        .trim();
      const id = generateHeadingId(text);
      headings.push({ id, text, level });
    }
  }
  return headings;
}

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
  /* Active doc slug is derived from the URL param, falling back to quickstart */
  const activeSlug: DocSlug =
    slugParam && validSlugs.has(slugParam as DocSlug) ? (slugParam as DocSlug) : 'quickstart';
  const [expandedItems, setExpandedItems] = useState<Record<string, boolean>>({ 'sb-integrations': false });
  const [activeHeadingId, setActiveHeadingId] = useState<string>('');
  const [hoveredHeadingId, setHoveredHeadingId] = useState<string>('');
  const [searchOpen, setSearchOpen] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');
  const [searchSelectedIdx, setSearchSelectedIdx] = useState(0);
  const searchInputRef = useRef<HTMLInputElement>(null);
  const { t, language } = useTranslation();
  const { isMobile } = useResponsive();
  const contentRef = React.useRef<HTMLDivElement>(null);

  const fontFamily = 'PingFang SC, -apple-system, BlinkMacSystemFont, sans-serif';

  /* Get markdown content for current doc */
  const docContent = useMemo(() => getDocContent(activeSlug, language), [activeSlug, language]);
  const docTitle = useMemo(() => getDocTitle(activeSlug, language), [activeSlug, language]);
  const headings = useMemo(() => extractHeadings(docContent), [docContent]);

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
      const id = href.slice(1);
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
      const anchor2 = href.split('#')[1];
      if (anchor2) {
        const tryScroll = (attempts: number) => {
          const el = document.getElementById(anchor2);
          if (el) {
            el.scrollIntoView({ behavior: 'smooth', block: 'start' });
          } else if (attempts < 10) {
            requestAnimationFrame(() => tryScroll(attempts + 1));
          }
        };
        requestAnimationFrame(() => tryScroll(0));
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

  /* Check if a sidebar item or its children is active */
  const isItemActive = useCallback((item: SidebarItem): boolean => {
    if (item.slug === activeSlug) return true;
    if (item.children) {
      return item.children.some(child => child.slug === activeSlug);
    }
    return false;
  }, [activeSlug]);

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

  /* Search results */
  const searchResults = useMemo(() => searchDocs(searchQuery, language), [searchQuery, language]);

  /* Reset selection when results change */
  useEffect(() => {
    setSearchSelectedIdx(0);
  }, [searchResults]);

  /* ⌘K / Ctrl+K keyboard shortcut */
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
        // Don't intercept when focused in other input/textarea (unless search is already open)
        const activeEl = document.activeElement;
        if (activeEl && (activeEl.tagName === 'INPUT' || activeEl.tagName === 'TEXTAREA') && !searchOpen) return;
        e.preventDefault();
        setSearchOpen(prev => !prev);
      }
      if (e.key === 'Escape') {
        setSearchOpen(false);
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [searchOpen]);

  /* Focus input when search opens */
  useEffect(() => {
    if (searchOpen) {
      setTimeout(() => searchInputRef.current?.focus(), 50);
    } else {
      setSearchQuery('');
    }
  }, [searchOpen]);

  /* Handle search result selection */
  const handleSearchSelect = useCallback((slug: DocSlug) => {
    navigateToDoc(slug);
    setSearchOpen(false);
  }, [navigateToDoc]);

  /* Keyboard navigation in search modal */
  const handleSearchKeyDown = useCallback((e: React.KeyboardEvent) => {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      setSearchSelectedIdx(prev => Math.min(prev + 1, searchResults.length - 1));
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      setSearchSelectedIdx(prev => Math.max(prev - 1, 0));
    } else if (e.key === 'Enter' && searchResults.length > 0) {
      e.preventDefault();
      handleSearchSelect(searchResults[searchSelectedIdx].slug);
    }
  }, [searchResults, searchSelectedIdx, handleSearchSelect]);

  return (
    <div style={{ minHeight: '100vh', background: '#000000', paddingTop: 72, fontFamily }}>
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
            <button
              onClick={() => setSearchOpen(true)}
              style={{
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'space-between',
                width: '100%',
                padding: '8px 12px',
                marginBottom: 20,
                background: 'rgba(255,255,255,0.04)',
                border: '1px solid rgba(255,255,255,0.12)',
                borderRadius: 8,
                cursor: 'pointer',
                color: 'rgba(255,255,255,0.4)',
                fontSize: 14,
                fontFamily,
                outline: 'none',
                transition: 'border-color 0.15s',
              }}
              onMouseEnter={e => (e.currentTarget.style.borderColor = 'rgba(255,255,255,0.25)')}
              onMouseLeave={e => (e.currentTarget.style.borderColor = 'rgba(255,255,255,0.12)')}
            >
              <span style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                <img src={searchIcon} alt="" style={{ width: 16, height: 16, opacity: 0.6 }} />
                {t('docs.search.placeholder')}
              </span>
              <span style={{ fontSize: 13, color: 'rgba(255,255,255,0.3)', fontFamily: '-apple-system, BlinkMacSystemFont, sans-serif', lineHeight: 1 }}>⌘K</span>
            </button>

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
                  <span style={{ flexShrink: 0, fontSize: 14, fontWeight: 600, color: '#ffffff', fontFamily, letterSpacing: '0.04em', textTransform: 'uppercase' }}>
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
                          background: isActive ? 'rgba(43, 222, 94, 0.12)' : 'transparent',
                        }}
                      >
                        <div style={{ display: 'flex', flex: 1, alignItems: 'center', gap: 8 }}>
                          <span style={{
                            flexShrink: 0,
                            fontSize: 14,
                            fontFamily,
                            fontWeight: isActive ? 500 : 400,
                            color: isActive ? '#2BDE5E' : 'rgba(255,255,255,0.7)',
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
                              background: childActive ? 'rgba(43, 222, 94, 0.12)' : 'transparent',
                            }}
                          >
                            <div style={{ display: 'flex', flex: 1, alignItems: 'center', gap: 8 }}>
                              <span style={{
                                flexShrink: 0,
                                fontSize: 14,
                                fontFamily,
                                fontWeight: childActive ? 500 : 400,
                                color: childActive ? '#2BDE5E' : 'rgba(255,255,255,0.7)',
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
          {/* Doc title */}
          <h1 style={{ fontSize: 28, fontWeight: 700, color: '#FFFFFF', margin: '0 0 32px 0', lineHeight: '36px', fontFamily }}>
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
                <span style={{ fontSize: 14, color: 'rgba(255,255,255,0.5)' }}>‹</span>
                <span style={{ fontSize: 14, fontFamily, color: 'rgba(255,255,255,0.7)', fontWeight: 400 }}>
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
                <span style={{ fontSize: 14, fontFamily, color: 'rgba(255,255,255,0.7)', fontWeight: 400 }}>
                  {t(nextDoc.labelKey)}
                </span>
                <span style={{ fontSize: 14, color: 'rgba(255,255,255,0.5)' }}>›</span>
              </button>
            ) : <span />}
          </div>
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
              <span style={{ fontSize: 14, fontWeight: 500, color: 'rgba(255,255,255,0.5)', letterSpacing: '0.05em', position: 'relative', top: 1 }}>
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
                      color: isActive ? '#2BDE5E' : isHovered ? 'rgba(255,255,255,0.85)' : 'rgba(255,255,255,0.5)',
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
              background: '#141414',
              border: '1px solid rgba(255,255,255,0.12)',
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
                  color: '#ffffff',
                  fontSize: 14,
                  fontFamily,
                }}
              />
            </div>
            {/* Results */}
            <div style={{ maxHeight: 400, overflowY: 'auto', padding: searchQuery ? '8px 0' : '0' }}>
              {searchQuery && searchResults.length === 0 && (
                <div style={{ padding: '24px 16px', textAlign: 'center', color: 'rgba(255,255,255,0.4)', fontSize: 14 }}>
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
                    background: idx === searchSelectedIdx ? 'rgba(255, 255, 255, 0.08)' : 'transparent',
                    border: 'none',
                    cursor: 'pointer',
                    textAlign: 'left',
                    outline: 'none',
                    transition: 'background 0.1s',
                  }}
                  onMouseEnter={() => setSearchSelectedIdx(idx)}
                >
                  <div style={{ color: '#ffffff', fontSize: 14, fontWeight: 500, fontFamily, marginBottom: 4 }}>
                    {result.title}
                  </div>
                  <div style={{ color: 'rgba(255,255,255,0.4)', fontSize: 12, fontFamily, lineHeight: '18px', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
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
                borderTop: '1px solid rgba(255,255,255,0.08)',
                fontSize: 12,
                color: 'rgba(255,255,255,0.35)',
                fontFamily,
              }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 16 }}>
                  <span style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
                    <kbd style={{ background: 'rgba(255,255,255,0.85)', borderRadius: 3, padding: '0px 3px', fontSize: 9, color: '#000000' }}>↑</kbd>
                    <kbd style={{ background: 'rgba(255,255,255,0.85)', borderRadius: 3, padding: '0px 3px', fontSize: 9, color: '#000000' }}>↓</kbd>
                    {t('docs.search.hint.select')}
                  </span>
                  <span style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
                    <kbd style={{ background: 'rgba(255,255,255,0.85)', borderRadius: 3, padding: '0px 3px', fontSize: 9, color: '#000000' }}>↵</kbd>
                    {t('docs.search.hint.open')}
                  </span>
                </div>
                <span style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
                  <kbd style={{ background: 'rgba(255,255,255,0.85)', borderRadius: 3, padding: '0px 3px', fontSize: 9, color: '#000000' }}>esc</kbd>
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
