import React, { useState, useEffect, useMemo, useCallback, useRef } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { useTranslation } from '../i18n';
import Navbar from '../components/Navbar';
import Footer from '../components/Footer';
import MarkdownRenderer from '../components/MarkdownRenderer';
import { useResponsive } from '../hooks/useResponsive';
import {
  getBlogContent,
  getBlogMeta,
  getAllBlogMetas,
  getAllTags,
  searchBlog,
  BlogSlug,
} from '../content/blog';
import { generateHeadingId } from '../utils/headingId';
import docContentsIcon from '../assets/icons/doc-contents.svg';
import searchIcon from '../assets/icons/icon-search.svg';
import '../styles/docs-markdown.css';
import '../styles/blog.css';

const fontFamily = 'PingFang SC, -apple-system, BlinkMacSystemFont, sans-serif';

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

/* ─── Build valid slug set from blog posts ─── */
function getValidSlugs(language: string): Set<BlogSlug> {
  return new Set(getAllBlogMetas(language).map(m => m.slug));
}

const BlogPage: React.FC = () => {
  const { slug: slugParam } = useParams<{ slug?: string }>();
  const navigate = useNavigate();
  const { t, language } = useTranslation();
  const { isMobile } = useResponsive();
  const [activeTag, setActiveTag] = useState<string | null>(null);
  const [activeHeadingId, setActiveHeadingId] = useState<string>('');
  const [hoveredHeadingId, setHoveredHeadingId] = useState<string>('');
  const [searchOpen, setSearchOpen] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');
  const [searchSelectedIdx, setSearchSelectedIdx] = useState(0);
  const searchInputRef = useRef<HTMLInputElement>(null);

  const validSlugs = useMemo(() => getValidSlugs(language), [language]);
  const isDetailView = !!(slugParam && validSlugs.has(slugParam as BlogSlug));
  const activeSlug = isDetailView ? (slugParam as BlogSlug) : null;

  /* ─── List view data ─── */
  const allMetas = useMemo(() => getAllBlogMetas(language), [language]);
  const allTags = useMemo(() => getAllTags(language), [language]);
  const filteredMetas = useMemo(
    () => activeTag ? allMetas.filter(m => m.tags.includes(activeTag)) : allMetas,
    [allMetas, activeTag]
  );

  /* ─── Detail view data ─── */
  const docContent = useMemo(
    () => activeSlug ? getBlogContent(activeSlug, language) : '',
    [activeSlug, language]
  );
  const docMeta = useMemo(
    () => activeSlug ? getBlogMeta(activeSlug, language) : null,
    [activeSlug, language]
  );
  const headings = useMemo(() => extractHeadings(docContent), [docContent]);

  /* Prev/Next navigation */
  const { prevPost, nextPost } = useMemo(() => {
    if (!activeSlug) return { prevPost: null, nextPost: null };
    const idx = allMetas.findIndex(m => m.slug === activeSlug);
    return {
      prevPost: idx < allMetas.length - 1 ? allMetas[idx + 1] : null,
      nextPost: idx > 0 ? allMetas[idx - 1] : null,
    };
  }, [activeSlug, allMetas]);

  /* Track active heading via IntersectionObserver */
  useEffect(() => {
    if (!isDetailView || headings.length === 0) return;
    const observer = new IntersectionObserver(
      (entries) => {
        const visible = entries.filter(e => e.isIntersecting);
        if (visible.length > 0) {
          const top = visible.reduce((best, e) =>
            e.boundingClientRect.top < best.boundingClientRect.top ? e : best
          );
          setActiveHeadingId(top.target.id);
        }
      },
      { rootMargin: '-80px 0px -60% 0px', threshold: 0 }
    );
    const els = headings.map(h => document.getElementById(h.id)).filter(Boolean) as HTMLElement[];
    els.forEach(el => observer.observe(el));
    return () => observer.disconnect();
  }, [headings, isDetailView]);

  /* Search results */
  const searchResults = useMemo(() => searchBlog(searchQuery, language), [searchQuery, language]);

  useEffect(() => {
    setSearchSelectedIdx(0);
  }, [searchResults]);

  /* Cmd+K keyboard shortcut */
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
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

  useEffect(() => {
    if (searchOpen) {
      setTimeout(() => searchInputRef.current?.focus(), 50);
    } else {
      setSearchQuery('');
    }
  }, [searchOpen]);

  const handleSearchSelect = useCallback((slug: BlogSlug) => {
    navigate(`/blog/${slug}`);
    setSearchOpen(false);
    window.scrollTo(0, 0);
  }, [navigate]);

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

  const scrollToHeading = useCallback((id: string) => {
    const el = document.getElementById(id);
    if (el) {
      const top = el.getBoundingClientRect().top + window.scrollY - 90;
      window.scrollTo({ top, behavior: 'smooth' });
    }
  }, []);

  const navigateToPost = useCallback((slug: BlogSlug) => {
    navigate(`/blog/${slug}`);
    window.scrollTo(0, 0);
  }, [navigate]);

  /* ─── Detail View ─── */
  if (isDetailView && docMeta) {
    return (
      <div style={{ minHeight: '100vh', background: '#000000', paddingTop: 72, fontFamily, display: 'flex', flexDirection: 'column' }}>
        <Navbar />
        <div style={{ display: 'flex', justifyContent: 'center', maxWidth: 1440, margin: '0 auto', flex: 1, width: '100%' }}>
          {/* Main content */}
          <div style={{ flex: 1, minWidth: 0, padding: isMobile ? '32px 20px 80px' : '40px 48px 80px', maxWidth: isMobile ? undefined : '65%' }}>
            {/* Back link */}
            <button
              onClick={() => navigate('/blog')}
              style={{
                background: 'none',
                border: 'none',
                cursor: 'pointer',
                padding: 0,
                marginBottom: 24,
                fontSize: 14,
                color: 'rgba(255,255,255,0.5)',
                fontFamily,
                transition: 'color 0.2s',
              }}
              onMouseEnter={e => (e.currentTarget.style.color = '#2BDE5E')}
              onMouseLeave={e => (e.currentTarget.style.color = 'rgba(255,255,255,0.5)')}
            >
              {t('blog.backToList')}
            </button>

            {/* Title */}
            <h1 style={{ fontSize: 28, fontWeight: 700, color: '#FFFFFF', margin: '0 0 16px 0', lineHeight: '36px', fontFamily }}>
              {docMeta.title}
            </h1>

            {/* Metadata */}
            <div className="blog-detail-meta">
              <span>{docMeta.date}</span>
              {docMeta.author && (
                <>
                  <span className="blog-detail-meta-separator" />
                  <span>{docMeta.author}</span>
                </>
              )}
              {docMeta.tags.length > 0 && (
                <>
                  <span className="blog-detail-meta-separator" />
                  <div className="blog-detail-tags">
                    {docMeta.tags.map(tag => (
                      <span key={tag} className="blog-detail-tag">{tag}</span>
                    ))}
                  </div>
                </>
              )}
            </div>

            {/* Content */}
            <MarkdownRenderer content={docContent} />

            {/* Prev / Next */}
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginTop: 56 }}>
              {prevPost ? (
                <button
                  onClick={() => navigateToPost(prevPost.slug)}
                  style={{ background: 'none', border: 'none', cursor: 'pointer', display: 'flex', alignItems: 'center', gap: 6, padding: 0 }}
                >
                  <span style={{ fontSize: 14, color: 'rgba(255,255,255,0.5)' }}>‹</span>
                  <span style={{ fontSize: 14, fontFamily, color: 'rgba(255,255,255,0.7)', fontWeight: 400 }}>
                    {prevPost.title}
                  </span>
                </button>
              ) : <span />}
              {nextPost ? (
                <button
                  onClick={() => navigateToPost(nextPost.slug)}
                  style={{ background: 'none', border: 'none', cursor: 'pointer', display: 'flex', alignItems: 'center', gap: 6, padding: 0 }}
                >
                  <span style={{ fontSize: 14, fontFamily, color: 'rgba(255,255,255,0.7)', fontWeight: 400 }}>
                    {nextPost.title}
                  </span>
                  <span style={{ fontSize: 14, color: 'rgba(255,255,255,0.5)' }}>›</span>
                </button>
              ) : <span />}
            </div>
          </div>

          {/* Right TOC */}
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
                  {t('blog.toc')}
                </span>
              </div>
              <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                {headings.map((h) => {
                  const isActive = h.id === activeHeadingId;
                  const isHovered = h.id === hoveredHeadingId;
                  return (
                    <button
                      key={h.id}
                      onClick={() => scrollToHeading(h.id)}
                      onMouseEnter={() => setHoveredHeadingId(h.id)}
                      onMouseLeave={() => setHoveredHeadingId('')}
                      style={{
                        background: 'none',
                        border: 'none',
                        cursor: 'pointer',
                        textAlign: 'left',
                        fontSize: 14,
                        fontFamily,
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
      </div>
    );
  }

  /* ─── List View ─── */
  return (
    <div style={{ minHeight: '100vh', background: '#000000', paddingTop: 72, fontFamily, display: 'flex', flexDirection: 'column' }}>
      <Navbar />
      <div style={{ maxWidth: 820, margin: '0 auto', padding: isMobile ? '32px 20px 80px' : '48px 48px 80px', flex: 1, width: '100%' }}>
        {/* Header */}
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 32 }}>
          <h1 style={{ fontSize: 28, fontWeight: 700, color: '#FFFFFF', margin: 0, fontFamily }}>
            {t('blog.title')}
          </h1>
          {/* Search button */}
          {!isMobile && (
            <button
              onClick={() => setSearchOpen(true)}
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: 8,
                padding: '8px 12px',
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
              <img src={searchIcon} alt="" style={{ width: 16, height: 16, opacity: 0.6 }} />
              <span>{t('blog.search.placeholder')}</span>
              <span style={{ fontSize: 13, color: 'rgba(255,255,255,0.3)', fontFamily: '-apple-system, BlinkMacSystemFont, sans-serif', lineHeight: 1, marginLeft: 8 }}>{navigator.platform?.includes('Mac') ? '⌘K' : 'Ctrl+K'}</span>
            </button>
          )}
        </div>

        {/* Tag filters */}
        {allTags.length > 0 && (
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8, marginBottom: 32 }}>
            <button
              className={`blog-tag ${activeTag === null ? 'blog-tag--active' : ''}`}
              onClick={() => setActiveTag(null)}
            >
              {t('blog.allTags')}
            </button>
            {allTags.map(tag => (
              <button
                key={tag}
                className={`blog-tag ${activeTag === tag ? 'blog-tag--active' : ''}`}
                onClick={() => setActiveTag(tag)}
              >
                {tag}
              </button>
            ))}
          </div>
        )}

        {/* Post list */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
          {filteredMetas.length === 0 && (
            <div style={{ padding: '48px 0', textAlign: 'center', color: 'rgba(255,255,255,0.4)', fontSize: 14 }}>
              {activeTag ? t('blog.noPostsForTag') : t('blog.noPosts')}
            </div>
          )}
          {filteredMetas.map(meta => (
            <div
              key={meta.slug}
              className="blog-card"
              onClick={() => navigateToPost(meta.slug)}
            >
              <div className="blog-card-header">
                <h2 className="blog-card-title">{meta.title}</h2>
                <span className="blog-card-date">{meta.date}</span>
              </div>
              {meta.summary && (
                <p className="blog-card-summary">{meta.summary}</p>
              )}
              <div className="blog-card-footer">
                <div className="blog-card-tags">
                  {meta.tags.map(tag => (
                    <span key={tag} className="blog-detail-tag">{tag}</span>
                  ))}
                </div>
                {meta.author && <span className="blog-card-author">{meta.author}</span>}
              </div>
            </div>
          ))}
        </div>
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
            <div style={{ display: 'flex', alignItems: 'center', padding: '12px 16px' }}>
              <img src={searchIcon} alt="" style={{ width: 16, height: 16, flexShrink: 0, opacity: 0.6 }} />
              <input
                ref={searchInputRef}
                type="text"
                value={searchQuery}
                onChange={e => setSearchQuery(e.target.value)}
                onKeyDown={handleSearchKeyDown}
                placeholder={t('blog.search.placeholder')}
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
            <div style={{ maxHeight: 400, overflowY: 'auto', padding: searchQuery ? '8px 0' : '0' }}>
              {searchQuery && searchResults.length === 0 && (
                <div style={{ padding: '24px 16px', textAlign: 'center', color: 'rgba(255,255,255,0.4)', fontSize: 14 }}>
                  {t('blog.search.noResults')}
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
                    background: idx === searchSelectedIdx ? 'rgba(255,255,255,0.08)' : 'transparent',
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
          </div>
        </div>
      )}
    </div>
  );
};

export default BlogPage;
