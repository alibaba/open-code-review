import React, { useState, useCallback, useEffect } from 'react';
import ReactDOM from 'react-dom';
import { useTranslation } from '../i18n';
import { useResponsive } from '../hooks/useResponsive';
import { useSectionTitleStyle } from '../hooks/useResponsiveStyle';
import copyIcon from '../assets/icons/icon-copy.svg';
import chevronDown from '../assets/icons/icon-chevron-down.svg';
import chevronRight from '../assets/icons/icon-chevron-right.svg';
import playIcon from '../assets/icons/icon-play.svg';

/* Toast */
const Toast: React.FC<{ message: string; visible: boolean }> = ({ message, visible }) =>
  ReactDOM.createPortal(
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
        opacity: visible ? 1 : 0,
        transition: 'opacity 0.15s ease',
        zIndex: 9999,
        backdropFilter: 'blur(8px)',
      }}
    >
      {message}
    </div>,
    document.body
  );

const CodeBlock: React.FC<{ label: string; code: string; multiline?: boolean; onCopy: (text: string) => void }> = ({ label, code, multiline, onCopy }) => (
  <div style={{ display: 'flex', flexDirection: 'column' }}>
    <p style={{ color: 'rgba(255,255,255,0.5)', fontSize: 12, margin: '0 0 6px 0' }}>{label}</p>
    <div
      style={{
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: multiline ? 'flex-start' : 'center',
        background: '#000000',
        borderRadius: 6,
        padding: '4px 16px',
        border: '1px solid rgba(255,255,255,0.16)',
      }}
    >
      <pre style={{ color: 'rgba(255,255,255,0.8)', fontSize: 13, fontFamily: 'Menlo, monospace', margin: 0, whiteSpace: 'pre-wrap', lineHeight: '22px' }}>
        {code}
      </pre>
      <div
        onClick={() => onCopy(code)}
        style={{ paddingTop: 4, paddingBottom: 4, cursor: 'pointer', flexShrink: 0 }}
      >
        <img src={copyIcon} alt="copy" style={{ width: 16, height: 16 }} />
      </div>
    </div>
  </div>
);

const QuickStartSection: React.FC = () => {
  const { t } = useTranslation();
  const { isMobile, isTablet } = useResponsive();
  const titleStyle = useSectionTitleStyle();
  const [toastVisible, setToastVisible] = useState(false);

  const handleCopy = useCallback((text: string) => {
    if (navigator.clipboard && window.isSecureContext) {
      navigator.clipboard.writeText(text).then(() => {
        setToastVisible(true);
      }).catch(() => {
        fallbackCopy(text);
      });
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

  return (
    <section
      id="quickstart"
      style={{ width: '100%', display: 'flex', justifyContent: 'center', padding: isMobile ? '60px 20px' : isTablet ? '80px 40px' : '80px 0', overflow: 'hidden' }}
    >
      <div style={{ width: '100%', maxWidth: 1200, display: 'flex', flexDirection: 'column', alignItems: 'center', gap: isMobile ? 32 : 48 }}>
        {/* Header */}
        <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 12 }}>
          <span style={{ color: '#2BDE5E', fontSize: 16, fontWeight: 500, letterSpacing: '0.48px' }}>
            {t('quickstart.sectionLabel')}
          </span>
          <h2 style={{ color: '#FFFFFF', fontSize: titleStyle.fontSize, fontWeight: 500, textAlign: 'center', lineHeight: titleStyle.lineHeight, letterSpacing: '0.96px', margin: 0, maxWidth: 758 }}>
            {t('quickstart.title')}
          </h2>
          <p style={{ color: 'rgba(255,255,255,0.5)', fontSize: 16, textAlign: 'center', lineHeight: '24px', margin: 0, maxWidth: 646 }}>
            {t('quickstart.subtitle')}
          </p>
        </div>

        {/* Steps */}
        <div style={{ width: isMobile ? '100%' : isTablet ? '100%' : 720, display: 'flex', flexDirection: 'column', gap: 24 }}>
          {/* Step 1 */}
          <div style={{ display: 'flex', flexDirection: 'column', background: 'rgba(255,255,255,0.04)', border: '1px solid rgba(255,255,255,0.16)', borderRadius: 8, padding: 24 }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
                <div style={{ width: 32, height: 32, display: 'flex', justifyContent: 'center', alignItems: 'center', background: 'rgba(255,255,255,0.04)', borderRadius: 6 }}>
                  <span style={{ color: 'rgba(255,255,255,0.6)', fontSize: 13, fontWeight: 500 }}>01</span>
                </div>
                <div>
                  <p style={{ color: '#FFFFFF', fontSize: 16, fontWeight: 500, margin: 0 }}>{t('quickstart.step1Title')}</p>
                  <p style={{ color: 'rgba(255,255,255,0.4)', fontSize: 13, margin: 0 }}>{t('quickstart.step1Desc')}</p>
                </div>
              </div>
              <img src={chevronDown} alt="" style={{ width: 16, height: 16 }} />
            </div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
              <CodeBlock label={t('quickstart.step1Label1')} code="npm i -g @alibaba-group/open-code-review" onCopy={handleCopy} />
              <CodeBlock label={t('quickstart.step1Label2')} code="ocr version" onCopy={handleCopy} />
            </div>
          </div>

          {/* Step 2 */}
          <div style={{ display: 'flex', flexDirection: 'column', background: 'rgba(255,255,255,0.04)', border: '1px solid rgba(255,255,255,0.16)', borderRadius: 8, padding: 24 }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
                <div style={{ width: 32, height: 32, display: 'flex', justifyContent: 'center', alignItems: 'center', background: 'rgba(255,255,255,0.04)', borderRadius: 6 }}>
                  <span style={{ color: 'rgba(255,255,255,0.6)', fontSize: 13, fontWeight: 500 }}>02</span>
                </div>
                <div>
                  <p style={{ color: '#FFFFFF', fontSize: 16, fontWeight: 500, margin: 0 }}>{t('quickstart.step2Title')}</p>
                  <p style={{ color: 'rgba(255,255,255,0.4)', fontSize: 13, margin: 0 }}>{t('quickstart.step2Desc')}</p>
                </div>
              </div>
              <img src={chevronRight} alt="" style={{ width: 16, height: 16 }} />
            </div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
              <CodeBlock label={t('quickstart.step2Label1')} code="ocr config provider" onCopy={handleCopy} />
              <CodeBlock
                label={t('quickstart.step2Label2')}
                code={`ocr config set llm.url https://api.anthropic.com \\
    && ocr config set llm.auth_token {{your-api-key}} \\
    && ocr config set llm.model claude-opus-4-6 \\
    && ocr config set llm.use_anthropic true`}
                multiline
                onCopy={handleCopy}
              />
              <CodeBlock label={t('quickstart.step2Label3')} code="ocr llm test" onCopy={handleCopy} />
            </div>
          </div>

          {/* Step 3 */}
          <div style={{ display: 'flex', flexDirection: 'column', background: 'rgba(255,255,255,0.04)', border: '1px solid rgba(255,255,255,0.16)', borderRadius: 8, padding: 24 }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
                <div style={{ width: 32, height: 32, display: 'flex', justifyContent: 'center', alignItems: 'center', background: 'rgba(255,255,255,0.04)', borderRadius: 6 }}>
                  <span style={{ color: 'rgba(255,255,255,0.6)', fontSize: 13, fontWeight: 500 }}>03</span>
                </div>
                <div>
                  <p style={{ color: '#FFFFFF', fontSize: 16, fontWeight: 500, margin: 0 }}>{t('quickstart.step3Title')}</p>
                  <p style={{ color: 'rgba(255,255,255,0.4)', fontSize: 13, margin: 0 }}>{t('quickstart.step3Desc')}</p>
                </div>
              </div>
              <img src={playIcon} alt="" style={{ width: 16, height: 16 }} />
            </div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
              <CodeBlock
                label={t('quickstart.step3Label1')}
                code={`${t('quickstart.commentReview')}
ocr review

${t('quickstart.commentBranch')}
ocr review --from main --to feature-auth

${t('quickstart.commentCommit')}
ocr review --commit abc123`}
                multiline
                onCopy={handleCopy}
              />
            </div>
          </div>
        </div>
      </div>
      <Toast message={t('quickstart.copied')} visible={toastVisible} />
    </section>
  );
};

export default QuickStartSection;
