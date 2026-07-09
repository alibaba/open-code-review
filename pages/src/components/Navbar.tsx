import React, { useState, useRef, useEffect } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { useTranslation } from '../i18n';
import { useResponsive } from '../hooks/useResponsive';
import socialIcon from '../assets/icons/icon-github.svg';
import brandIcon from '../assets/images/brandicon.svg';

import type { Language } from '../i18n/types';

const LANG_OPTIONS: { value: Language; label: string }[] = [
  { value: 'en', label: 'English' },
  { value: 'zh', label: '中文' },
  { value: 'ja', label: '日本語' },
];

const navTabs = [
  { path: '/', labelKey: 'navbar.features' },
  { path: '/benchmark', labelKey: 'navbar.benchmark' },
  { path: '/quickstart', labelKey: 'navbar.quickstart' },
  { path: '/docs', labelKey: 'navbar.docs' },
];

const Navbar: React.FC = () => {
  const { language, setLanguage, t } = useTranslation();
  const { isMobile } = useResponsive();
  const location = useLocation();
  const navigate = useNavigate();
  const [langOpen, setLangOpen] = useState(false);
  const langRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (langRef.current && !langRef.current.contains(e.target as Node)) setLangOpen(false);
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, []);

  const currentPath = location.pathname;

  return (
    <nav
      style={{
        width: '100%',
        height: isMobile ? 56 : 72,
        display: 'flex',
        justifyContent: 'center',
        alignItems: 'center',
        borderBottom: '1px solid rgba(61,61,61,0.6)',
        backdropFilter: 'blur(10px)',
        WebkitBackdropFilter: 'blur(10px)',
        position: 'fixed',
        top: 0,
        left: 0,
        zIndex: 100,
        willChange: 'transform',
      }}
    >
      <div
        style={{
          width: '100%',
          maxWidth: 1440,
          height: isMobile ? 56 : 72,
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          padding: isMobile ? '0 16px' : '0 32px',
        }}
      >
        {/* Logo */}
        <div
          style={{ display: 'flex', alignItems: 'center', cursor: 'pointer' }}
          onClick={() => navigate('/')}
        >
          <img src={brandIcon} alt="Open Code Review" style={{ height: isMobile ? 20 : 24 }} />
        </div>

        {/* Nav Tabs - hidden on mobile */}
        {!isMobile && (
          <div style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
            {navTabs.map((tab) => {
              const isActive = currentPath === tab.path;
              return (
                <button
                  key={tab.path}
                  onClick={() => navigate(tab.path)}
                  style={{
                    padding: '8px 16px',
                    borderRadius: 8,
                    border: 'none',
                    background: 'transparent',
                    cursor: 'pointer',
                    transition: 'background 0.2s',
                  }}
                >
                  <span
                    style={{
                      color: '#FFFFFF',
                      fontSize: 14,
                      lineHeight: '20px',
                      opacity: isActive ? 1 : 0.6,
                      fontWeight: isActive ? 500 : 400,
                    }}
                  >
                    {t(tab.labelKey)}
                  </span>
                </button>
              );
            })}
          </div>
        )}

        {/* Right section */}
        <div style={{ display: 'flex', justifyContent: 'flex-start', alignItems: 'center', gap: 16 }}>
          {/* Language Switcher */}
          <div ref={langRef} style={{ position: 'relative', display: 'flex', alignItems: 'center' }}>
            <button
              onClick={() => setLangOpen(v => !v)}
              style={{
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                background: 'none',
                border: '1px solid #FFFFFF',
                borderRadius: 4,
                cursor: 'pointer',
                opacity: 0.6,
                padding: 0,
                width: 18,
                height: 18,
                boxSizing: 'border-box' as const,
              }}
            >
              <span style={{
                fontSize: 10,
                fontWeight: 600,
                color: '#FFFFFF',
                lineHeight: '18px',
                textAlign: 'center' as const,
                width: '100%',
                fontFamily: language === 'ja' ? "'Hiragino Sans', sans-serif" : "'PingFang SC', -apple-system, sans-serif",
              }}>
                {language === 'en' ? 'En' : language === 'zh' ? '中' : 'あ'}
              </span>
            </button>
            {langOpen && (
              <div
                style={{
                  position: 'absolute',
                  top: '100%',
                  right: 0,
                  marginTop: 8,
                  background: '#1a1a1a',
                  border: '1px solid rgba(255,255,255,0.15)',
                  borderRadius: 8,
                  padding: 4,
                  zIndex: 200,
                  minWidth: 100,
                }}
              >
                {LANG_OPTIONS.map(opt => (
                  <button
                    key={opt.value}
                    onClick={() => { setLanguage(opt.value); setLangOpen(false); }}
                    style={{
                      display: 'block',
                      width: '100%',
                      padding: '8px 12px',
                      background: opt.value === language ? 'rgba(255,255,255,0.08)' : 'transparent',
                      border: 'none',
                      borderRadius: 6,
                      color: opt.value === language ? '#fff' : 'rgba(255,255,255,0.6)',
                      fontSize: 13,
                      textAlign: 'left' as const,
                      cursor: 'pointer',
                    }}
                  >
                    {opt.label}
                  </button>
                ))}
              </div>
            )}
          </div>
          <a
            href="https://github.com/alibaba/open-code-review"
            target="_blank"
            rel="noopener noreferrer"
            style={{ display: 'flex', alignItems: 'center', opacity: 0.6 }}
          >
            <img src={socialIcon} alt="Social" style={{ width: 22, height: 22 }} />
          </a>
          <button
            onClick={() => navigate('/quickstart')}
            style={{
              height: 32,
              display: 'flex',
              justifyContent: 'center',
              alignItems: 'center',
              gap: 6,
              padding: '4px 12px',
              background: '#ffffff',
              border: '1px solid #EBEBEB',
              borderRadius: 6,
              color: 'rgba(0,0,0,0.77)',
              fontSize: isMobile ? 12 : 14,
              fontWeight: 500,
              cursor: 'pointer',
            }}
          >
            {t('navbar.getStarted')}
          </button>
        </div>
      </div>
    </nav>
  );
};

export default Navbar;
