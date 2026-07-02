import React, { useCallback, useState, useEffect } from 'react';
import ReactDOM from 'react-dom';
import { useTranslation } from '../i18n';
import { useResponsive } from '../hooks/useResponsive';
import ColorBends from './ColorBends';
import docDownloadIcon from '../assets/icons/doc-download-green.svg';
import copyIcon from '../assets/icons/icon-copy.svg';


const TC = {
  brand: '#756BFF',
  cmd: '#E2BA64',
  path: '#67BAFA',
  success: '#48AA84',
  action: '#D553F6',
  text: '#e4e4e7',
  dim: 'rgba(255,255,255,0.5)',
};

const terminalLines = [
  {
    num: 1,
    content: (
      <span>
        <span style={{ color: TC.success }}>$</span>
        <span style={{ color: TC.success }}> ocr </span>
        <span style={{ color: TC.success }}>review</span>
      </span>
    ),
  },
  {
    num: 2,
    content: (
      <span>
        <span style={{ color: TC.brand }}>[ocr]</span>
        <span style={{ color: TC.text }}> Reviewing </span>
        <span style={{ color: TC.path }}>5</span>
        <span style={{ color: TC.text }}> file(s) in </span>
        <span style={{ color: TC.path }}>/home/user/project</span>
      </span>
    ),
  },
  {
    num: 3,
    content: (
      <span>
        <span style={{ color: TC.brand }}>[ocr]</span>
        <span style={{ color: TC.action }}> ▶ </span>
        <span style={{ color: TC.cmd }}>file_read</span>
        <span style={{ color: TC.text }}> </span>
        <span style={{ color: TC.path }}>"internal/auth/login.go"</span>
      </span>
    ),
  },
  {
    num: 4,
    content: (
      <span>
        <span style={{ color: TC.brand }}>[ocr]</span>
        <span style={{ color: TC.success }}> ✔ </span>
        <span style={{ color: TC.cmd }}>file_read</span>
        <span style={{ color: TC.dim }}> (15ms)</span>
      </span>
    ),
  },
  {
    num: 5,
    content: (
      <span>
        <span style={{ color: TC.brand }}>[ocr]</span>
        <span style={{ color: TC.action }}> ▶ </span>
        <span style={{ color: TC.cmd }}>code_search</span>
        <span style={{ color: TC.text }}> </span>
        <span style={{ color: TC.path }}>"password.*hash"</span>
      </span>
    ),
  },
  {
    num: 6,
    content: (
      <span>
        <span style={{ color: TC.brand }}>[ocr]</span>
        <span style={{ color: TC.success }}> ✔ </span>
        <span style={{ color: TC.cmd }}>code_search</span>
        <span style={{ color: TC.dim }}> (8ms)</span>
      </span>
    ),
  },
  {
    num: 7,
    content: (
      <span>
        <span style={{ color: TC.brand }}>[ocr]</span>
        <span style={{ color: TC.text }}> Plan completed for </span>
        <span style={{ color: TC.path }}>internal/auth/login.go</span>
      </span>
    ),
  },
  {
    num: 8,
    content: (
      <span>
        <span style={{ color: TC.brand }}>[ocr]</span>
        <span style={{ color: TC.text }}> Summary: </span>
        <span style={{ color: TC.path }}>5</span>
        <span style={{ color: TC.text }}> file(s), </span>
        <span style={{ color: TC.path }}>3</span>
        <span style={{ color: TC.text }}> comment(s), ~8421 tokens, 12.5s</span>
      </span>
    ),
  },
  { num: 9, content: <span>&nbsp;</span> },
  { num: 10, content: <span style={{ color: TC.dim }}>─── internal/auth/login.go:42-45 ───</span> },
  { num: 11, content: <span style={{ color: TC.text }}>Consider using bcrypt cost factor ≥ 12 for password hashing.</span> },
  { num: 12, content: <span className="terminal-cursor" style={{ color: TC.text }}>｜</span> },
];

const INSTALL_CMD = 'npm i -g @alibaba-group/open-code-review';

const HeroSection: React.FC = () => {
  const { t } = useTranslation();
  const { isMobile, isTablet } = useResponsive();
  const [toastVisible, setToastVisible] = useState(false);
  const [toastMessage, setToastMessage] = useState('');

  const showToast = (message: string) => {
    setToastMessage(message);
    setToastVisible(true);
  };

  const fallbackCopy = (text: string) => {
    const textarea = document.createElement('textarea');
    textarea.value = text;
    textarea.style.position = 'fixed';
    textarea.style.opacity = '0';
    document.body.appendChild(textarea);
    textarea.select();
    const success = document.execCommand('copy');
    document.body.removeChild(textarea);
    if (success) {
      showToast(t('hero.copied'));
    } else {
      showToast(t('hero.copyFailed'));
    }
  };

  const handleCopy = useCallback(async (text: string) => {
    if (navigator.clipboard && window.isSecureContext) {
      try {
        await navigator.clipboard.writeText(text);
        showToast(t('hero.copied'));
      } catch {
        fallbackCopy(text);
      }
    } else {
      fallbackCopy(text);
    }
  }, [t]);

  useEffect(() => {
    if (!toastVisible) return;
    const timer = setTimeout(() => setToastVisible(false), 1200);
    return () => clearTimeout(timer);
  }, [toastVisible]);

  return (
    <>
    <section
      style={{
        width: '100vw',
        marginLeft: 'calc(-50vw + 50%)',
        height: isMobile ? 820 : isTablet ? 800 : 960,
        position: 'relative',
        overflow: 'hidden',
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
      }}
    >
      {/* Shader Background */}
      <ColorBends
        style={{
          position: 'absolute',
          left: 0,
          top: 0,
          width: '100%',
          height: '100%',
          zIndex: 0,
        }}
        colors={['#0d750d', '#042e04', '#066020']}
        rotation={90}
        speed={0.23}
        scale={1.2}
        frequency={1}
        warpStrength={1}
        mouseInfluence={1}
        noise={0.33}
        parallax={0.45}
        iterations={1}
        intensity={0.8}
        bandWidth={6}
        transparent
      />

      {/* Gradient overlay */}
      <div
        style={{
          position: 'absolute',
          left: 0,
          bottom: 0,
          width: '100%',
          height: 276,
          background: 'linear-gradient(180deg, rgba(0,0,0,0) 0%, #000000 100%)',
          zIndex: 1,
        }}
      />

      {/* Content */}
      <div
        style={{
          position: 'relative',
          zIndex: 2,
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          paddingTop: isMobile ? 100 : 180,
          paddingLeft: isMobile ? 20 : 0,
          paddingRight: isMobile ? 20 : 0,
          gap: isMobile ? 24 : 32,
          maxWidth: isMobile ? '100%' : 742,
        }}
      >
        {/* Install Badge */}
        <div
          style={{
            width: 'auto',
            height: 32,
            background: 'rgba(0,0,0,0.8)',
            borderRadius: 500,
            border: '1px solid rgba(255,255,255,0.16)',
            display: 'flex',
            alignItems: 'center',
            gap: 8,
            padding: '0 12px',
            marginBottom: isMobile ? 8 : 0,
          }}
        >
          <img src={docDownloadIcon} alt="" style={{ width: 16, height: 16, flexShrink: 0 }} />
          <p className="install-text-shimmer" style={{ fontSize: 12, fontWeight: 400, margin: 0, letterSpacing: '0.2px', whiteSpace: 'nowrap' }}>
            {INSTALL_CMD}
          </p>
          <img
            src={copyIcon}
            alt="Copy"
            style={{ width: 16, height: 16, cursor: 'pointer', flexShrink: 0 }}
            onClick={() => handleCopy(INSTALL_CMD)}
          />
        </div>

        {/* Title */}
        <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center' }}>
          <h1
            style={{
              color: '#FFFFFF',
              fontSize: isMobile ? 28 : isTablet ? 36 : 48,
              fontWeight: 500,
              textAlign: 'center',
              lineHeight: isMobile ? '34px' : isTablet ? '42px' : '52px',
              letterSpacing: '0.96px',
              margin: 0,
            }}
          >
            {t('hero.title').split('\n').map((line, i, arr) => (
              <React.Fragment key={i}>
                {line}
                {i < arr.length - 1 && <br />}
              </React.Fragment>
            ))}
          </h1>
          <p
            style={{
              color: 'rgba(255,255,255,0.6)',
              fontSize: isMobile ? 14 : 16,
              textAlign: 'center',
              lineHeight: '24px',
              marginTop: 16,
              maxWidth: isMobile ? '100%' : 742,
            }}
          >
            {t('hero.description')}
          </p>
        </div>

        {/* Buttons */}
        <div style={{ display: 'flex', gap: 8 }}>
          <a
            href="#quickstart"
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
              fontSize: 14,
              fontWeight: 500,
              textDecoration: 'none',
            }}
          >
            {t('hero.quickStart')}
          </a>
          <a
            href="#/docs"
            style={{
              height: 32,
              display: 'flex',
              justifyContent: 'center',
              alignItems: 'center',
              padding: '4px 12px',
              background: 'rgba(0,0,0,0.9)',
              borderRadius: 6,
              color: '#fff',
              fontSize: 14,
              border: '1px solid rgba(255,255,255,0.16)',
              textDecoration: 'none',
            }}
          >
            {t('hero.learnMore')}
          </a>
        </div>

        {/* Terminal */}
        <div
          style={{
            width: '100%',
            maxWidth: isMobile ? '100%' : isTablet ? 560 : 692,
            borderRadius: 8,
            overflow: 'hidden',
            border: '1px solid rgba(255,255,255,0.08)',
          }}
        >
          {/* Terminal header */}
          <div
            style={{
              display: 'flex',
              justifyContent: 'space-between',
              alignItems: 'center',
              background: 'rgba(17,17,17,0.5)',
              borderTopLeftRadius: 8,
              borderTopRightRadius: 8,
              padding: '8px 15px',
            }}
          >
            <span style={{ color: 'rgba(255,255,255,0.6)', fontSize: 13, fontFamily: 'Menlo, monospace' }}>
              {t('hero.terminal')}
            </span>
          </div>
          {/* Terminal body */}
          <div
            style={{
              padding: '10px 0',
              background: 'rgba(255,255,255,0.08)',
              backdropFilter: 'blur(20px)',
              borderBottomLeftRadius: 8,
              borderBottomRightRadius: 8,
              overflowX: 'hidden',
            }}
          >
            {terminalLines.map((line) => (
              <div
                key={line.num}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 10,
                  padding: '5px 0',
                }}
              >
                <div
                  style={{
                    width: 38,
                    display: 'flex',
                    alignItems: 'center',
                    paddingLeft: 15,
                    flexShrink: 0,
                  }}
                >
                  <span style={{ width: 19, color: 'rgba(255,255,255,0.3)', fontSize: 'clamp(10px, 1.8vw, 13px)', fontFamily: 'Menlo, monospace' }}>
                    {line.num}
                  </span>
                </div>
                <span style={{ fontSize: 'clamp(10px, 1.8vw, 15px)', fontFamily: 'Menlo, monospace', lineHeight: '20px', whiteSpace: 'nowrap' }}>
                  {line.content}
                </span>
              </div>
            ))}
          </div>
        </div>
      </div>
    </section>
    {toastVisible && ReactDOM.createPortal(
      <div style={{
        position: 'fixed',
        top: 88,
        left: '50%',
        transform: 'translateX(-50%)',
        background: 'rgba(255,255,255,0.1)',
        border: '1px solid rgba(255,255,255,0.2)',
        color: 'rgba(255,255,255,0.85)',
        padding: '5px 14px',
        borderRadius: 6,
        fontSize: 12,
        zIndex: 9999,
        backdropFilter: 'blur(8px)',
      }}>
        {toastMessage}
      </div>,
      document.body
    )}
    </>
  );
};

export default HeroSection;
