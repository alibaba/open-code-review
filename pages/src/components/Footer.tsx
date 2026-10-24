// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import React, { useState, useRef, useEffect } from 'react';
import githubIcon from '../assets/icons/icon-github.svg';
import langIcon from '../assets/icons/icon-language.svg';
import { useTranslation } from '../i18n/context';
import { useResponsive } from '../hooks/useResponsive';
import type { Language } from '../i18n/types';

const LANG_OPTIONS: { value: Language; label: string }[] = [
  { value: 'en', label: 'English' },
  { value: 'zh', label: '中文' }, // allow-non-english: language options are labelled in their own language
  { value: 'ja', label: '日本語' }, // allow-non-english: language options are labelled in their own language
  { value: 'ko', label: '한국어' }, // allow-non-english: language options are labelled in their own language
  { value: 'ru', label: 'Русский' }, // allow-non-english: language options are labelled in their own language
];

const Footer: React.FC = () => {
  const { language, setLanguage, t } = useTranslation();
  const { isMobile } = useResponsive();
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, []);

  const currentLabel = LANG_OPTIONS.find(o => o.value === language)?.label ?? 'English';
  return (
    <footer
      style={{
        width: '100%',
        borderTop: '1px solid var(--line)',
        padding: isMobile ? '32px 16px' : '64px 32px',
        display: 'flex',
        justifyContent: 'center',
      }}
    >
      <div
        style={{
          width: '100%',
          display: 'flex',
          flexDirection: isMobile ? 'column' : 'row',
          justifyContent: 'space-between',
          alignItems: isMobile ? 'flex-start' : 'center',
          gap: isMobile ? 16 : 0,
          maxWidth: 1440,
          margin: '0 auto',
        }}
      >
        <a href="https://github.com/alibaba/open-code-review" target="_blank" rel="noopener noreferrer" style={{ display: 'flex', alignItems: 'center', gap: 6, textDecoration: 'none' }}>
          <img src={githubIcon} alt="" style={{ width: 18, height: 18 }} />
          <span style={{ color: 'var(--text-muted)', fontSize: 14 }}>{t('footer.brand')}</span>
        </a>
        <span style={{ color: 'var(--text-ghost)', fontSize: 13 }}>
          {t('footer.copyright')}
        </span>

        {/* Language Switcher */}
        <div ref={ref} style={{ position: 'relative' }}>
          <button
            onClick={() => setOpen(v => !v)}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 6,
              background: 'none',
              border: '1px solid var(--line)',
              borderRadius: 6,
              padding: '6px 14px',
              color: 'var(--text-muted)',
              fontSize: 13,
              cursor: 'pointer',
            }}
          >
            <img src={langIcon} alt="" style={{ width: 14, height: 14 }} />
            {currentLabel}
            <svg width="10" height="6" viewBox="0 0 10 6" fill="none" style={{ transform: open ? 'rotate(180deg)' : 'none', transition: 'transform 0.2s' }}>
              <path d="M1 1L5 5L9 1" stroke="var(--text-faint)" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round"/>
            </svg>
          </button>
          {open && (
            <div
              style={{
                position: 'absolute',
                bottom: '100%',
                left: 0,
                right: 0,
                marginBottom: 6,
                background: 'var(--surface-overlay)',
                border: '1px solid var(--line)',
                borderRadius: 8,
                padding: 4,
                zIndex: 10,
              }}
            >
              {LANG_OPTIONS.map(opt => (
                <button
                  key={opt.value}
                  onClick={() => { setLanguage(opt.value); setOpen(false); }}
                  style={{
                    display: 'block',
                    width: '100%',
                    padding: '8px 12px',
                    background: opt.value === language ? 'var(--fill-hover)' : 'transparent',
                    border: 'none',
                    borderRadius: 6,
                    color: opt.value === language ? 'var(--text-strong)' : 'var(--text-muted)',
                    fontSize: 13,
                    textAlign: 'left',
                    cursor: 'pointer',
                  }}
                >
                  {opt.label}
                </button>
              ))}
            </div>
          )}
        </div>
      </div>
    </footer>
  );
};

export default Footer;
