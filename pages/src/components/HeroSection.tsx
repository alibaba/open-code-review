// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import React, { Suspense, useCallback, useState, useEffect } from 'react';
import ReactDOM from 'react-dom';
import { Link } from 'react-router-dom';
import { useTranslation } from '../i18n';
import { useResponsive } from '../hooks/useResponsive';
import ColorBends from './ColorBends';
import npmIcon from '../assets/icons/npm.svg';
import appleIcon from '../assets/icons/apple.svg';
import linuxIcon from '../assets/icons/linux.svg';
import windowsIcon from '../assets/icons/windows.svg';
import copyIcon from '../assets/icons/icon-copy.svg';

const ColorBends = React.lazy(() => import(/* webpackChunkName: "color-bends" */ './ColorBends'));


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

const INSTALL_CHANNELS = [
  { key: 'npm', labelKey: 'hero.installNpm', cmd: 'npm i -g @alibaba-group/open-code-review', icons: [npmIcon] },
  { key: 'unix', labelKey: 'hero.installUnix', cmd: 'curl -fsSL https://raw.githubusercontent.com/alibaba/open-code-review/main/install.sh | sh', icons: [appleIcon, linuxIcon] },
  { key: 'windows', labelKey: 'hero.installWindows', cmd: 'irm https://raw.githubusercontent.com/alibaba/open-code-review/main/install.ps1 | iex', icons: [windowsIcon] },
];

const HeroSection: React.FC = () => {
  const { t } = useTranslation();
  const { isMobile, isTablet, isDesktop } = useResponsive();
  const twoCol = isDesktop;
  const [toastVisible, setToastVisible] = useState(false);
  const [toastMessage, setToastMessage] = useState('');
  const [showShaderBackground, setShowShaderBackground] = useState(false);

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

  useEffect(() => {
    // Wait until after the first paint before loading the heavy shader chunk.
    let secondFrame: number | undefined;
    const firstFrame = requestAnimationFrame(() => {
      secondFrame = requestAnimationFrame(() => setShowShaderBackground(true));
    });

    return () => {
      cancelAnimationFrame(firstFrame);
      if (secondFrame !== undefined) cancelAnimationFrame(secondFrame);
    };
  }, []);

  const shaderFallback = (
    <div
      style={{
        position: 'absolute',
        inset: 0,
        zIndex: 0,
        background: 'radial-gradient(circle at 50% 20%, #0d750d 0%, #042e04 38%, #000000 78%)',
      }}
    />
  );

  return (
    <>
    <section
      style={{
        width: '100vw',
        marginLeft: 'calc(-50vw + 50%)',
        height: isMobile ? 1000 : isTablet ? 900 : 860,
        position: 'relative',
        overflow: 'hidden',
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
      }}
    >
      {/* Shader Background */}
      {!showShaderBackground && shaderFallback}
      {showShaderBackground && (
        <ErrorBoundary fallback={shaderFallback}>
          <Suspense fallback={shaderFallback}>
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
          </Suspense>
        </ErrorBoundary>
      )}

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
          flexDirection: twoCol ? 'row' : 'column',
          alignItems: 'center',
          justifyContent: 'center',
          paddingTop: isMobile ? 80 : 170,
          paddingLeft: isMobile ? 20 : 40,
          paddingRight: isMobile ? 20 : 40,
          gap: twoCol ? 56 : isMobile ? 24 : 32,
          maxWidth: twoCol ? 1200 : isMobile ? '100%' : 742,
          width: '100%',
        }}
      >
        {/* Left column */}
        <div
          style={{
            display: 'flex',
            flexDirection: 'column',
            alignItems: twoCol ? 'flex-start' : 'center',
            gap: isMobile ? 20 : 24,
            flex: twoCol ? '1 1 0' : undefined,
            minWidth: 0,
            maxWidth: twoCol ? 520 : '100%',
            width: '100%',
          }}
        >
          {/* Title */}
          <div style={{ display: 'flex', flexDirection: 'column', alignItems: twoCol ? 'flex-start' : 'center', width: '100%' }}>
            <h1
              style={{
                color: '#FFFFFF',
                fontSize: isMobile ? 28 : isTablet ? 36 : 48,
                fontWeight: 500,
                textAlign: twoCol ? 'left' : 'center',
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
                textAlign: twoCol ? 'left' : 'center',
                lineHeight: '24px',
                marginTop: 16,
                maxWidth: isMobile ? '100%' : 520,
              }}
            >
              {t('hero.description')}
            </p>
          </div>

          {/* Install channels */}
          <div
            style={{
              display: 'flex',
              flexDirection: 'column',
              gap: 12,
              width: '100%',
              maxWidth: 460,
            }}
          >
            {INSTALL_CHANNELS.map((ch) => (
              <div key={ch.key} style={{ display: 'flex', flexDirection: 'column', gap: 6, width: '100%' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                  {ch.key === 'unix' ? (() => {
                    const parts = t(ch.labelKey).split('/');
                    const macLabel = (parts[0] ?? '').trim();
                    const linuxLabel = (parts[1] ?? '').trim();
                    return (
                      <>
                        <img src={ch.icons[0]} alt="" style={{ width: 14, height: 14, flexShrink: 0 }} />
                        <span style={{ fontSize: 12, fontWeight: 500, color: 'rgba(255,255,255,0.6)' }}>
                          {macLabel}{linuxLabel ? ' /' : ''}
                        </span>
                        {linuxLabel && (
                          <>
                            <img src={ch.icons[1]} alt="" style={{ width: 14, height: 14, flexShrink: 0 }} />
                            <span style={{ fontSize: 12, fontWeight: 500, color: 'rgba(255,255,255,0.6)' }}>
                              {linuxLabel}
                            </span>
                          </>
                        )}
                      </>
                    );
                  })() : (
                    <>
                      {ch.icons.map((icon, idx) => (
                        <img key={idx} src={icon} alt="" style={{ width: 14, height: 14, flexShrink: 0 }} />
                      ))}
                      <span style={{ fontSize: 12, fontWeight: 500, color: 'rgba(255,255,255,0.6)' }}>
                        {t(ch.labelKey)}
                      </span>
                    </>
                  )}
                </div>
                <div
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'space-between',
                    gap: 8,
                    height: 32,
                    padding: '0 12px',
                    background: 'rgba(0,0,0,0.8)',
                    border: '1px solid rgba(255,255,255,0.16)',
                    borderRadius: 6,
                    width: '100%',
                  }}
                >
                  <span
                    style={{
                      fontSize: 12,
                      fontFamily: 'Menlo, monospace',
                      color: 'rgba(255,255,255,0.85)',
                      whiteSpace: 'nowrap',
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                    }}
                  >
                    {ch.cmd}
                  </span>
                  <img
                    src={copyIcon}
                    alt="Copy"
                    style={{ width: 16, height: 16, cursor: 'pointer', flexShrink: 0 }}
                    onClick={() => handleCopy(ch.cmd)}
                  />
                </div>
              </div>
            ))}
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
            <Link
              to="/docs"
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
            </Link>
          </div>
        </div>

        {/* Terminal */}
        <div
          style={{
            width: '100%',
            maxWidth: isMobile ? '100%' : twoCol ? 600 : isTablet ? 560 : 692,
            flexShrink: twoCol ? 0 : undefined,
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
              padding: '10px 0 8px 0',
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
