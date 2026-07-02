import React, { useState, useEffect, useRef, useCallback } from 'react';
import ReactDOM from 'react-dom';
import { useTranslation } from '../i18n';
import Footer from '../components/Footer';
import { useResponsive } from '../hooks/useResponsive';
import copyIcon from '../assets/icons/icon-copy.svg';
import docDownloadIcon from '../assets/icons/doc-download.svg';
import docCheckCircleIcon from '../assets/icons/doc-check-circle.svg';
import docEditIcon from '../assets/icons/doc-edit.svg';
import docContentsIcon from '../assets/icons/doc-contents.svg';
import {
  SparkContrastView2Line,
  SparkHistoryLine,
  SparkDocumentLine,
  SparkCode02Line,
  SparkAgentLine,
  SparkVisibleLine,
  SparkScanLine,
  SparkTargetLine,
  SparkFileCodeLine,
} from '@agentscope-ai/icons';

/* Toast - same as QuickStartSection */
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

interface Section {
  id: string;
  labelKey: string;
}

const sectionDefs: Section[] = [
  { id: 'overview', labelKey: 'docs.overview' },
  { id: 'install', labelKey: 'docs.install' },
  { id: 'config', labelKey: 'docs.config' },
  { id: 'review', labelKey: 'docs.review' },
  { id: 'scan', labelKey: 'docs.scan' },
  { id: 'viewer', labelKey: 'docs.viewer' },
  { id: 'mcp', labelKey: 'docs.mcp' },
  { id: 'env', labelKey: 'docs.env' },
];

/* ─── Code block matching reference: black bg, 1px border, rounded 6px, copy icon right ─── */
const CodeBlock: React.FC<{ code: string; onCopy?: () => void }> = ({ code, onCopy }) => (
  <div
    style={{
      display: 'flex',
      alignSelf: 'stretch',
      justifyContent: 'space-between',
      alignItems: 'flex-start',
      background: '#000000',
      borderRadius: 6,
      padding: '4px 16px',
      border: '1px solid rgba(255,255,255,0.16)',
    }}
  >
    <pre style={{ margin: 0, fontFamily: 'Menlo, Monaco, monospace', fontSize: 13, lineHeight: '24px', color: 'rgba(255,255,255,0.8)', whiteSpace: 'pre-wrap', wordBreak: 'break-all', flex: 1 }}>
      {code}
    </pre>
    {onCopy && (
      <div
        onClick={onCopy}
        style={{ paddingTop: 4, paddingBottom: 4, marginLeft: 12, cursor: 'pointer', flexShrink: 0 }}
      >
        <img src={copyIcon} alt="copy" style={{ width: 16, height: 16 }} />
      </div>
    )}
  </div>
);

/* ─── Icon box (32x32, rgba(255,255,255,0.04) bg, rounded 6px) ─── */
const IconBox: React.FC<{ icon: string }> = ({ icon }) => (
  <div style={{ width: 32, height: 32, display: 'flex', flex: 'none', justifyContent: 'center', alignItems: 'center', background: 'rgba(255,255,255,0.04)', borderRadius: 6 }}>
    <img src={icon} alt="" style={{ width: 16, height: 16 }} />
  </div>
);

/* ─── Spark Icon box (same style, wraps React icon component) ─── */
const SparkIconBox: React.FC<{ children: React.ReactNode }> = ({ children }) => (
  <div style={{ width: 32, height: 32, display: 'flex', flex: 'none', justifyContent: 'center', alignItems: 'center', background: 'rgba(255,255,255,0.04)', borderRadius: 6, color: 'rgba(255,255,255,0.8)' }}>
    {children}
  </div>
);

const DocsPage: React.FC = () => {
  const [activeSection, setActiveSection] = useState('overview');
  const [toastVisible, setToastVisible] = useState(false);
  const lockedRef = useRef<string | null>(null);
  const unlockTimerRef = useRef<ReturnType<typeof setTimeout>>();
  const { t } = useTranslation();
  const { isMobile } = useResponsive();

  const sections = sectionDefs.map(s => ({ ...s, label: t(s.labelKey) }));

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
    if (success) {
      setToastVisible(true);
    } else {
      console.warn('[DocsPage] copy to clipboard failed');
    }
  };

  useEffect(() => {
    if (!toastVisible) return;
    const timer = setTimeout(() => setToastVisible(false), 1200);
    return () => clearTimeout(timer);
  }, [toastVisible]);

  useEffect(() => {
    const THRESHOLD = 160;
    const handleScroll = () => {
      if (lockedRef.current) return;
      let bestIndex = 0;
      let bestTop = -Infinity;
      for (let i = 0; i < sectionDefs.length; i++) {
        const el = document.getElementById(sectionDefs[i].id);
        if (!el) continue;
        const top = el.getBoundingClientRect().top;
        if (top <= THRESHOLD && top > bestTop) {
          bestTop = top;
          bestIndex = i;
        }
      }
      setActiveSection(sectionDefs[bestIndex].id);
    };
    window.addEventListener('scroll', handleScroll);
    return () => {
      window.removeEventListener('scroll', handleScroll);
      clearTimeout(unlockTimerRef.current);
    };
  }, []);

  const scrollToSection = (id: string) => {
    lockedRef.current = id;
    clearTimeout(unlockTimerRef.current);
    const el = document.getElementById(id);
    if (el) el.scrollIntoView({ behavior: 'smooth' });
    setActiveSection(id);
    unlockTimerRef.current = setTimeout(() => { lockedRef.current = null; }, 800);
  };

  /* ─── Shared styles ─── */
  const fontFamily = 'PingFang SC, -apple-system, BlinkMacSystemFont, sans-serif';
  const sectionTitle: React.CSSProperties = { fontSize: 20, fontWeight: 600, color: '#FFFFFF', margin: '0 0 16px 0', lineHeight: '28px', fontFamily };
  const subTitle: React.CSSProperties = { fontSize: 15, fontWeight: 600, color: '#FFFFFF', margin: '24px 0 8px 0', lineHeight: '24px', fontFamily };
  const desc: React.CSSProperties = { fontSize: 14, color: 'rgba(255,255,255,0.6)', lineHeight: '24px', margin: '0 0 12px 0', fontFamily };
  const sectionSpacing: React.CSSProperties = { marginBottom: 56, display: 'flex', flexDirection: 'column' as const, alignItems: 'stretch' };
  const mcpAddCommands = `ocr config set mcp_servers.<name>.command <command>
ocr config set mcp_servers.<name>.args '["arg1","arg2"]'
ocr config set mcp_servers.<name>.tools '["tool_name"]'
ocr config set mcp_servers.<name>.setup '<setup command>'
ocr config set mcp_servers.<name>.env '["KEY=VALUE"]'`;
  const mcpDeleteCommands = `ocr config unset mcp_servers.<name>`;
  const mcpCodeGraphCommands = `ocr config set mcp_servers.codegraph.command codegraph
ocr config set mcp_servers.codegraph.args '["serve","--mcp"]'
ocr config set mcp_servers.codegraph.tools '["codegraph_explore"]'
ocr config set mcp_servers.codegraph.setup 'codegraph init && codegraph index'`;

  return (
    <div style={{ minHeight: '100vh', background: '#000000', paddingTop: 72, fontFamily: 'PingFang SC, -apple-system, BlinkMacSystemFont, sans-serif' }}>

      {/* Main layout: content + right sidebar */}
      <div style={{ display: 'flex', justifyContent: 'flex-start', alignItems: 'flex-start', gap: 40, padding: isMobile ? '0 20px' : '0 40px', paddingRight: isMobile ? 20 : 300 }}>
        {/* Main content area */}
        <div style={{ display: 'flex', flex: 1, flexShrink: 0, justifyContent: 'center', alignItems: 'flex-start' }}>
          <div style={{ maxWidth: 1080, display: 'flex', flex: 1, flexDirection: 'column', paddingTop: 40, paddingBottom: 80 }}>
            {/* Page title "Docs" */}
            <div style={{ marginBottom: 40 }}>
              <p style={{ fontSize: 36, fontWeight: 700, color: '#FFFFFF', margin: 0, lineHeight: '44px', fontFamily: 'PingFang SC, -apple-system, BlinkMacSystemFont, sans-serif' }}>{t('navbar.docs')}</p>
            </div>

            {/* ─── Overview ─── */}
            <section id="overview" style={{ ...sectionSpacing, scrollMarginTop: 100 }}>
              <p style={sectionTitle}>{t('docs.overviewTitle')}</p>
              <p style={desc}>
                Open Code Review {t('docs.overviewDesc').replace(/<\/?code>/g, '')}
              </p>
              <p style={{ ...desc, fontWeight: 500, color: 'rgba(255,255,255,0.8)' }}>
                {t('docs.overviewFeatures')}
              </p>
              <div style={{ display: 'flex', alignSelf: 'stretch', justifyContent: 'flex-start', alignItems: 'center', background: 'rgba(255,255,255,0.04)', borderRadius: 12, padding: 16, border: '1px solid rgba(255,255,255,0.16)' }}>
                <span style={{ flexShrink: 0, color: '#2BDE5E', fontSize: 12, fontFamily: 'Menlo, monospace', lineHeight: '24px', marginRight: 12 }}>
                  {'✔\n✔\n✔\n✔\n✔\n✔'.split('\n').map((c, i) => <React.Fragment key={i}>{c}<br /></React.Fragment>)}
                </span>
                <span style={{ fontSize: 14, color: 'rgba(255,255,255,0.7)', lineHeight: '24px' }}>
                  {t('docs.overviewFeat1')}<br />
                  {t('docs.overviewFeat2')}<br />
                  {t('docs.overviewFeat3')}<br />
                  {t('docs.overviewFeat4')}<br />
                  {t('docs.overviewFeat5')}<br />
                  {t('docs.overviewFeat6')}
                </span>
              </div>
            </section>

            {/* ─── Install ─── */}
            <section id="install" style={{ ...sectionSpacing, scrollMarginTop: 100 }}>
              <p style={sectionTitle}>{t('docs.installTitle')}</p>
              {/* Install item */}
              <div style={{ display: 'flex', flexDirection: 'column', marginBottom: 16, background: 'rgba(255,255,255,0.04)', borderRadius: 12, padding: 16, border: '1px solid rgba(255,255,255,0.16)' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 12 }}>
                  <IconBox icon={docDownloadIcon} />
                  <span style={{ fontSize: 14, fontWeight: 500, color: '#FFFFFF' }}>{t('docs.installLabel')}</span>
                </div>
                <CodeBlock code="npm i -g @alibaba-group/open-code-review" onCopy={() => handleCopy('npm i -g @alibaba-group/open-code-review')} />
              </div>
              {/* Verify item */}
              <div style={{ display: 'flex', flexDirection: 'column', background: 'rgba(255,255,255,0.04)', borderRadius: 12, padding: 16, border: '1px solid rgba(255,255,255,0.16)' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 12 }}>
                  <IconBox icon={docCheckCircleIcon} />
                  <span style={{ fontSize: 14, fontWeight: 500, color: '#FFFFFF' }}>{t('docs.installVerifyLabel')}</span>
                </div>
                <CodeBlock code="ocr version" onCopy={() => handleCopy('ocr version')} />
              </div>
            </section>

            {/* ─── Configuration & Verification ─── */}
            <section id="config" style={{ ...sectionSpacing, scrollMarginTop: 100 }}>
              <p style={sectionTitle}>{t('docs.configTitle')}</p>
              <p style={desc}>{t('docs.configDesc').replace(/<\/?code>/g, '')}</p>

              <p style={subTitle}>{t('docs.configInteractive')}</p>
              <p style={desc}>{t('docs.configInteractiveDesc')}</p>
              <CodeBlock code="ocr config provider" onCopy={() => handleCopy('ocr config provider')} />

              <p style={subTitle}>{t('docs.configModelSelect')}</p>
              <p style={desc}>{t('docs.configModelSelectDesc')}</p>
              <CodeBlock code="ocr config model" onCopy={() => handleCopy('ocr config model')} />

              <p style={subTitle}>{t('docs.configListProviders')}</p>
              <p style={desc}>{t('docs.configListProvidersDesc')}</p>
              <CodeBlock code="ocr llm providers" onCopy={() => handleCopy('ocr llm providers')} />

              <p style={subTitle}>{t('docs.configManual')}</p>
              <p style={{ ...desc, fontWeight: 500, color: 'rgba(255,255,255,0.7)' }}>{t('docs.configCommand')}</p>
              <CodeBlock code={'ocr config set <key> <value>'} />

              <p style={{ ...desc, fontWeight: 500, color: 'rgba(255,255,255,0.7)', marginTop: 16 }}>{t('docs.configExample')}</p>
              <CodeBlock
                code={`ocr config set llm.url https://api.anthropic.com \\\n    && ocr config set llm.auth_token {{your-api-key}} \\\n    && ocr config set llm.model claude-opus-4-6 \\\n    && ocr config set llm.use_anthropic true  \\\n    && ocr config set language Chinese`}
                onCopy={() => handleCopy(`ocr config set llm.url https://api.anthropic.com \\\n    && ocr config set llm.auth_token {{your-api-key}} \\\n    && ocr config set llm.model claude-opus-4-6 \\\n    && ocr config set llm.use_anthropic true  \\\n    && ocr config set language Chinese`)}
              />

              <p style={subTitle}>{t('docs.configKeys')}</p>
              <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                {/* 2-column grid of config keys */}
                {[
                  [{ key: 'llm.url', desc: t('docs.configKeyUrl') }, { key: 'llm.auth_token', desc: t('docs.configKeyToken') }],
                  [{ key: 'llm.model', desc: t('docs.configKeyModel') }, { key: 'llm.use_anthropic', desc: t('docs.configKeyAnthropic') }],
                  [{ key: 'telemetry.enabled', desc: t('docs.configKeyTelemetry') }, { key: 'language', desc: t('docs.configKeyLanguage') }],
                  [{ key: 'llm.extra_body', desc: t('docs.configKeyExtraBody') }],
                ].map((row, ri) => (
                  <div key={ri} style={{ display: 'flex', gap: 4, flexWrap: isMobile ? 'wrap' : 'nowrap' }}>
                    {row.map(({ key, desc: d }) => (
                      <div key={key} style={{ display: 'flex', flex: 1, justifyContent: 'space-between', alignItems: 'center', background: '#000000', borderRadius: 6, padding: '4px 16px', border: '1px solid rgba(255,255,255,0.16)', minWidth: isMobile ? '100%' : undefined }}>
                        <p style={{ margin: 0, fontSize: 13, fontFamily: 'Menlo, monospace', color: 'rgba(255,255,255,0.8)' }}>
                          <span style={{ color: '#2BDE5E' }}>{key}</span>
                          <span style={{ color: 'rgba(255,255,255,0.4)', marginLeft: 8 }}>{d}</span>
                        </p>
                      </div>
                    ))}
                  </div>
                ))}
              </div>

              <p style={subTitle}>{t('docs.configVerify')}</p>
              <CodeBlock
                code={`# Test LLM connection\nocr llm test`}
                onCopy={() => handleCopy('ocr llm test')}
              />
              <p style={{ ...desc, marginTop: 12 }}>{t('docs.configVerifyDesc')}</p>
            </section>

            {/* ─── ocr review ─── */}
            <section id="review" style={{ ...sectionSpacing, scrollMarginTop: 100 }}>
              <p style={sectionTitle}>{t('docs.reviewTitle')}</p>
              <p style={desc}>{t('docs.reviewDesc').replace(/<\/?code>/g, '')}</p>

              <p style={subTitle}>{t('docs.reviewModes')}</p>
              {/* Workspace Mode */}
              <div style={{ marginBottom: 12, background: 'rgba(255,255,255,0.04)', borderRadius: 12, padding: 16, border: '1px solid rgba(255,255,255,0.16)' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 12 }}>
                  <IconBox icon={docEditIcon} />
                  <div style={{ display: 'flex', flex: 1, flexDirection: 'column' }}>
                    <span style={{ fontSize: 14, fontWeight: 500, color: '#FFFFFF' }}>{t('docs.reviewWorkspace')}</span>
                    <p style={{ margin: 0, fontSize: 13, color: 'rgba(255,255,255,0.5)', lineHeight: '20px' }}>{t('docs.reviewWorkspaceDesc')}</p>
                  </div>
                </div>
                <CodeBlock code="ocr review" onCopy={() => handleCopy('ocr review')} />
              </div>
              {/* Branch Diff Mode */}
              <div style={{ marginBottom: 12, background: 'rgba(255,255,255,0.04)', borderRadius: 12, padding: 16, border: '1px solid rgba(255,255,255,0.16)' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 12 }}>
                  <SparkIconBox><SparkContrastView2Line size={16} /></SparkIconBox>
                  <div style={{ display: 'flex', flex: 1, flexDirection: 'column' }}>
                    <span style={{ fontSize: 14, fontWeight: 500, color: '#FFFFFF' }}>{t('docs.reviewBranch')}</span>
                    <p style={{ margin: 0, fontSize: 13, color: 'rgba(255,255,255,0.5)', lineHeight: '20px' }}>{t('docs.reviewBranchDesc')}</p>
                  </div>
                </div>
                <CodeBlock code="ocr review --from master --to dev-ref" onCopy={() => handleCopy('ocr review --from master --to dev-ref')} />
              </div>
              {/* Single Commit Mode */}
              <div style={{ marginBottom: 12, background: 'rgba(255,255,255,0.04)', borderRadius: 12, padding: 16, border: '1px solid rgba(255,255,255,0.16)' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 12 }}>
                  <SparkIconBox><SparkHistoryLine size={16} /></SparkIconBox>
                  <div style={{ display: 'flex', flex: 1, flexDirection: 'column' }}>
                    <span style={{ fontSize: 14, fontWeight: 500, color: '#FFFFFF' }}>{t('docs.reviewCommit')}</span>
                    <p style={{ margin: 0, fontSize: 13, color: 'rgba(255,255,255,0.5)', lineHeight: '20px' }}>{t('docs.reviewCommitDesc')}</p>
                  </div>
                </div>
                <CodeBlock code="ocr review -c abc123" onCopy={() => handleCopy('ocr review -c abc123')} />
              </div>

              <p style={subTitle}>{t('docs.reviewAdvanced')}</p>
              {/* Review with Requirement Context */}
              <div style={{ marginBottom: 12, background: 'rgba(255,255,255,0.04)', borderRadius: 12, padding: 16, border: '1px solid rgba(255,255,255,0.16)' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 12 }}>
                  <SparkIconBox><SparkDocumentLine size={16} /></SparkIconBox>
                  <div style={{ display: 'flex', flex: 1, flexDirection: 'column' }}>
                    <span style={{ fontSize: 14, fontWeight: 500, color: '#FFFFFF' }}>{t('docs.reviewBackground')}</span>
                    <p style={{ margin: 0, fontSize: 13, color: 'rgba(255,255,255,0.5)', lineHeight: '20px' }}>{t('docs.reviewBackgroundDesc')}</p>
                  </div>
                </div>
                <CodeBlock code={`ocr review --background "requirement context"\nocr review -b "requirement context"`} onCopy={() => handleCopy('ocr review --background "requirement context"')} />
              </div>
              {/* JSON Output */}
              <div style={{ marginBottom: 12, background: 'rgba(255,255,255,0.04)', borderRadius: 12, padding: 16, border: '1px solid rgba(255,255,255,0.16)' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 12 }}>
                  <SparkIconBox><SparkCode02Line size={16} /></SparkIconBox>
                  <div style={{ display: 'flex', flex: 1, flexDirection: 'column' }}>
                    <span style={{ fontSize: 14, fontWeight: 500, color: '#FFFFFF' }}>{t('docs.reviewJson')}</span>
                    <p style={{ margin: 0, fontSize: 13, color: 'rgba(255,255,255,0.5)', lineHeight: '20px' }}>{t('docs.reviewJsonDesc')}</p>
                  </div>
                </div>
                <CodeBlock code={`ocr review --format json\nocr review -f json`} onCopy={() => handleCopy('ocr review --format json')} />
              </div>
              {/* Agent Mode */}
              <div style={{ marginBottom: 12, background: 'rgba(255,255,255,0.04)', borderRadius: 12, padding: 16, border: '1px solid rgba(255,255,255,0.16)' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 12 }}>
                  <SparkIconBox><SparkAgentLine size={16} /></SparkIconBox>
                  <div style={{ display: 'flex', flex: 1, flexDirection: 'column' }}>
                    <span style={{ fontSize: 14, fontWeight: 500, color: '#FFFFFF' }}>{t('docs.reviewAgent')}</span>
                    <p style={{ margin: 0, fontSize: 13, color: 'rgba(255,255,255,0.5)', lineHeight: '20px' }}>{t('docs.reviewAgentDesc')}</p>
                  </div>
                </div>
                <CodeBlock code="ocr review --audience agent" onCopy={() => handleCopy('ocr review --audience agent')} />
              </div>
              {/* Dry-Run Preview */}
              <div style={{ background: 'rgba(255,255,255,0.04)', borderRadius: 12, padding: 16, border: '1px solid rgba(255,255,255,0.16)' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 12 }}>
                  <SparkIconBox><SparkVisibleLine size={16} /></SparkIconBox>
                  <div style={{ display: 'flex', flex: 1, flexDirection: 'column' }}>
                    <span style={{ fontSize: 14, fontWeight: 500, color: '#FFFFFF' }}>{t('docs.reviewPreviewLabel')}</span>
                    <p style={{ margin: 0, fontSize: 13, color: 'rgba(255,255,255,0.5)', lineHeight: '20px' }}>{t('docs.reviewPreviewDesc')}</p>
                  </div>
                </div>
                <CodeBlock code="ocr review --preview" onCopy={() => handleCopy('ocr review --preview')} />
              </div>

              <p style={subTitle}>{t('docs.reviewFlags')}</p>
              {/* Flags table */}
              <div style={{ display: 'flex', flexDirection: 'column', borderRadius: 8, border: '1px solid rgba(255,255,255,0.16)', overflow: 'hidden' }}>
                {/* Header */}
                <div style={{ display: 'flex', borderBottom: '1px solid rgba(255,255,255,0.16)' }}>
                  <div style={{ width: 120, flexShrink: 0, padding: '10px 12px' }}><span style={{ fontSize: 13, fontWeight: 500, color: 'rgba(255,255,255,0.6)' }}>{t('docs.reviewFlagCol1')}</span></div>
                  <div style={{ flex: 1, padding: '10px 12px' }}><span style={{ fontSize: 13, fontWeight: 500, color: 'rgba(255,255,255,0.6)' }}>{t('docs.reviewFlagCol2')}</span></div>
                  <div style={{ width: 120, flexShrink: 0, padding: '10px 12px' }}><span style={{ fontSize: 13, fontWeight: 500, color: 'rgba(255,255,255,0.6)' }}>{t('docs.reviewFlagCol3')}</span></div>
                </div>
                {/* Rows */}
                {[
                  ['-c, --commit', t('docs.reviewFlag1Desc'), '—'],
                  ['--from', t('docs.reviewFlag2Desc'), '—'],
                  ['--to', t('docs.reviewFlag3Desc'), '—'],
                  ['-f, --format', t('docs.reviewFlag4Desc'), 'text'],
                  ['--repo', t('docs.reviewFlag5Desc'), t('docs.reviewFlag5Default')],
                  ['--rule', t('docs.reviewFlag6Desc'), t('docs.reviewFlag6Default')],
                  ['--concurrency', t('docs.reviewFlag7Desc'), '8'],
                  ['--timeout', t('docs.reviewFlag8Desc'), '10'],
                  ['--audience', t('docs.reviewFlag9Desc'), 'human'],
                  ['--max-tools', t('docs.reviewFlag10Desc'), t('docs.reviewFlag10Default')],
                ].map(([flag, d, def], idx, arr) => (
                  <div key={idx} style={{ display: 'flex', borderBottom: idx < arr.length - 1 ? '1px solid rgba(255,255,255,0.16)' : 'none' }}>
                    <div style={{ width: 120, height: 44, flexShrink: 0, display: 'flex', alignItems: 'center', padding: '10px 12px' }}>
                      <span style={{ fontSize: 12, fontFamily: 'Menlo, monospace', color: 'rgba(255,255,255,0.7)' }}>{flag}</span>
                    </div>
                    <div style={{ flex: 1, height: 44, display: 'flex', alignItems: 'center', padding: '10px 12px' }}>
                      <span style={{ fontSize: 13, color: 'rgba(255,255,255,0.6)' }}>{d}</span>
                    </div>
                    <div style={{ width: 120, height: 44, flexShrink: 0, display: 'flex', alignItems: 'center', padding: '10px 12px' }}>
                      <span style={{ fontSize: 12, fontFamily: 'Menlo, monospace', color: 'rgba(255,255,255,0.5)' }}>{def}</span>
                    </div>
                  </div>
                ))}
              </div>
              <p style={{ ...desc, marginTop: 16, fontSize: 12 }}>
                {t('docs.reviewNote').replace(/<\/?code>/g, '')}
              </p>
            </section>

            {/* ─── ocr scan ─── */}
            <section id="scan" style={{ ...sectionSpacing, scrollMarginTop: 100 }}>
              <p style={sectionTitle}>{t('docs.scanTitle')}</p>
              <p style={desc}>{t('docs.scanDesc').replace(/<\/?code>/g, '')}</p>

              <p style={subTitle}>{t('docs.scanVsTitle')}</p>
              <div style={{ marginBottom: 12, background: 'rgba(255,255,255,0.04)', borderRadius: 12, padding: 16, border: '1px solid rgba(255,255,255,0.16)' }}>
                <div style={{ display: 'flex', alignItems: 'flex-start', gap: 12 }}>
                  <SparkIconBox><SparkContrastView2Line size={16} /></SparkIconBox>
                  <div style={{ display: 'flex', flex: 1, flexDirection: 'column' }}>
                    <span style={{ fontSize: 14, fontWeight: 500, color: '#FFFFFF' }}>{t('docs.scanVsReviewLabel')}</span>
                    <p style={{ margin: 0, fontSize: 13, color: 'rgba(255,255,255,0.5)', lineHeight: '20px' }}>{t('docs.scanVsReview').replace(/<\/?code>/g, '')}</p>
                  </div>
                </div>
              </div>
              <div style={{ marginBottom: 24, background: 'rgba(255,255,255,0.04)', borderRadius: 12, padding: 16, border: '1px solid rgba(255,255,255,0.16)' }}>
                <div style={{ display: 'flex', alignItems: 'flex-start', gap: 12 }}>
                  <SparkIconBox><SparkScanLine size={16} /></SparkIconBox>
                  <div style={{ display: 'flex', flex: 1, flexDirection: 'column' }}>
                    <span style={{ fontSize: 14, fontWeight: 500, color: '#FFFFFF' }}>{t('docs.scanVsScanLabel')}</span>
                    <p style={{ margin: 0, fontSize: 13, color: 'rgba(255,255,255,0.5)', lineHeight: '20px' }}>{t('docs.scanVsScan').replace(/<\/?code>/g, '')}</p>
                  </div>
                </div>
              </div>

              <p style={subTitle}>{t('docs.scanUsage')}</p>
              <div style={{ marginBottom: 12, background: 'rgba(255,255,255,0.04)', borderRadius: 12, padding: 16, border: '1px solid rgba(255,255,255,0.16)' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 12 }}>
                  <SparkIconBox><SparkScanLine size={16} /></SparkIconBox>
                  <div style={{ display: 'flex', flex: 1, flexDirection: 'column' }}>
                    <span style={{ fontSize: 14, fontWeight: 500, color: '#FFFFFF' }}>{t('docs.scanUsageWhole')}</span>
                    <p style={{ margin: 0, fontSize: 13, color: 'rgba(255,255,255,0.5)', lineHeight: '20px' }}>{t('docs.scanUsageWholeDesc')}</p>
                  </div>
                </div>
                <CodeBlock code="ocr scan" onCopy={() => handleCopy('ocr scan')} />
              </div>
              <div style={{ marginBottom: 12, background: 'rgba(255,255,255,0.04)', borderRadius: 12, padding: 16, border: '1px solid rgba(255,255,255,0.16)' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 12 }}>
                  <SparkIconBox><SparkTargetLine size={16} /></SparkIconBox>
                  <div style={{ display: 'flex', flex: 1, flexDirection: 'column' }}>
                    <span style={{ fontSize: 14, fontWeight: 500, color: '#FFFFFF' }}>{t('docs.scanUsagePath')}</span>
                    <p style={{ margin: 0, fontSize: 13, color: 'rgba(255,255,255,0.5)', lineHeight: '20px' }}>{t('docs.scanUsagePathDesc')}</p>
                  </div>
                </div>
                <CodeBlock code="ocr scan --path internal/agent" onCopy={() => handleCopy('ocr scan --path internal/agent')} />
              </div>
              <div style={{ marginBottom: 12, background: 'rgba(255,255,255,0.04)', borderRadius: 12, padding: 16, border: '1px solid rgba(255,255,255,0.16)' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 12 }}>
                  <SparkIconBox><SparkFileCodeLine size={16} /></SparkIconBox>
                  <div style={{ display: 'flex', flex: 1, flexDirection: 'column' }}>
                    <span style={{ fontSize: 14, fontWeight: 500, color: '#FFFFFF' }}>{t('docs.scanUsageFile')}</span>
                    <p style={{ margin: 0, fontSize: 13, color: 'rgba(255,255,255,0.5)', lineHeight: '20px' }}>{t('docs.scanUsageFileDesc')}</p>
                  </div>
                </div>
                <CodeBlock code="ocr scan --path internal/agent/agent.go,internal/diff/scan.go" onCopy={() => handleCopy('ocr scan --path internal/agent/agent.go,internal/diff/scan.go')} />
              </div>
              <div style={{ background: 'rgba(255,255,255,0.04)', borderRadius: 12, padding: 16, border: '1px solid rgba(255,255,255,0.16)' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 12 }}>
                  <SparkIconBox><SparkVisibleLine size={16} /></SparkIconBox>
                  <div style={{ display: 'flex', flex: 1, flexDirection: 'column' }}>
                    <span style={{ fontSize: 14, fontWeight: 500, color: '#FFFFFF' }}>{t('docs.scanUsagePreviewLabel')}</span>
                    <p style={{ margin: 0, fontSize: 13, color: 'rgba(255,255,255,0.5)', lineHeight: '20px' }}>{t('docs.scanUsagePreviewDesc')}</p>
                  </div>
                </div>
                <CodeBlock code="ocr scan --preview" onCopy={() => handleCopy('ocr scan --preview')} />
              </div>

              <p style={subTitle}>{t('docs.scanBatching')}</p>
              <p style={desc}>{t('docs.scanBatchingDesc').replace(/<\/?code>/g, '')}</p>
              <div style={{ display: 'flex', flexDirection: 'column', gap: 4, marginBottom: 16 }}>
                {[
                  [t('docs.scanBatchingNone'), t('docs.scanBatchingNoneDesc')],
                  [t('docs.scanBatchingLang'), t('docs.scanBatchingLangDesc')],
                  [t('docs.scanBatchingDir'), t('docs.scanBatchingDirDesc')],
                ].map(([name, d]) => (
                  <div key={name} style={{ display: 'flex', alignSelf: 'stretch', justifyContent: 'space-between', alignItems: 'center', background: '#000000', borderRadius: 6, padding: '4px 16px', border: '1px solid rgba(255,255,255,0.16)' }}>
                    <p style={{ margin: 0, fontSize: 13, fontFamily: 'Menlo, monospace', color: 'rgba(255,255,255,0.8)' }}>
                      <span style={{ color: '#2BDE5E' }}>{name}</span>
                      <span style={{ color: 'rgba(255,255,255,0.4)', marginLeft: 12 }}>{d}</span>
                    </p>
                  </div>
                ))}
              </div>
              <CodeBlock code="ocr scan --batch by-directory" onCopy={() => handleCopy('ocr scan --batch by-directory')} />

              <p style={subTitle}>{t('docs.scanToggles')}</p>
              <p style={desc}>{t('docs.scanTogglesDesc')}</p>
              <div style={{ display: 'flex', flexDirection: 'column', gap: 4, marginBottom: 16 }}>
                {[
                  ['--no-plan', t('docs.scanTogglesPlanDesc')],
                  ['--no-dedup', t('docs.scanTogglesDedupDesc')],
                  ['--no-summary', t('docs.scanTogglesSummaryDesc')],
                ].map(([flag, d]) => (
                  <div key={flag} style={{ display: 'flex', alignSelf: 'stretch', justifyContent: 'flex-start', alignItems: 'center', background: '#000000', borderRadius: 6, padding: '8px 16px', border: '1px solid rgba(255,255,255,0.16)' }}>
                    <span style={{ fontSize: 13, fontFamily: 'Menlo, monospace', color: '#2BDE5E', flexShrink: 0, marginRight: 12 }}>{flag}</span>
                    <span style={{ fontSize: 13, color: 'rgba(255,255,255,0.6)', lineHeight: '20px' }}>{d}</span>
                  </div>
                ))}
              </div>
              <CodeBlock code="ocr scan --no-plan --no-dedup --no-summary" onCopy={() => handleCopy('ocr scan --no-plan --no-dedup --no-summary')} />

              <p style={subTitle}>{t('docs.scanBudget')}</p>
              <p style={desc}>{t('docs.scanBudgetDesc').replace(/<\/?code>/g, '')}</p>
              <CodeBlock code="ocr scan --max-tokens-budget 500000" onCopy={() => handleCopy('ocr scan --max-tokens-budget 500000')} />

              <p style={subTitle}>{t('docs.scanFlags')}</p>
              <div style={{ display: 'flex', flexDirection: 'column', borderRadius: 8, border: '1px solid rgba(255,255,255,0.16)', overflow: 'hidden' }}>
                <div style={{ display: 'flex', borderBottom: '1px solid rgba(255,255,255,0.16)' }}>
                  <div style={{ width: 160, flexShrink: 0, padding: '10px 12px' }}><span style={{ fontSize: 13, fontWeight: 500, color: 'rgba(255,255,255,0.6)' }}>{t('docs.scanFlagCol1')}</span></div>
                  <div style={{ flex: 1, padding: '10px 12px' }}><span style={{ fontSize: 13, fontWeight: 500, color: 'rgba(255,255,255,0.6)' }}>{t('docs.scanFlagCol2')}</span></div>
                  <div style={{ width: 120, flexShrink: 0, padding: '10px 12px' }}><span style={{ fontSize: 13, fontWeight: 500, color: 'rgba(255,255,255,0.6)' }}>{t('docs.scanFlagCol3')}</span></div>
                </div>
                {[
                  ['--path', t('docs.scanFlag1Desc'), t('docs.scanFlag1Default')],
                  ['--exclude', t('docs.scanFlag2Desc'), '—'],
                  ['-p, --preview', t('docs.scanFlag3Desc'), 'false'],
                  ['--max-tokens-budget', t('docs.scanFlag4Desc'), '0'],
                  ['--no-plan', t('docs.scanFlag5Desc'), 'false'],
                  ['--no-dedup', t('docs.scanFlag6Desc'), 'false'],
                  ['--no-summary', t('docs.scanFlag7Desc'), 'false'],
                  ['--batch', t('docs.scanFlag8Desc'), 'by-language'],
                  ['-f, --format', t('docs.scanFlag9Desc'), 'text'],
                  ['--concurrency', t('docs.scanFlag10Desc'), '8'],
                  ['--timeout', t('docs.scanFlag11Desc'), '10'],
                  ['--audience', t('docs.scanFlag12Desc'), 'human'],
                  ['-b, --background', t('docs.scanFlag13Desc'), '—'],
                  ['--max-tools', t('docs.scanFlag14Desc'), t('docs.scanFlag14Default')],
                  ['--max-git-procs', t('docs.scanFlag15Desc'), '16'],
                  ['--rule', t('docs.scanFlag16Desc'), '—'],
                  ['--tools', t('docs.scanFlag17Desc'), t('docs.scanFlag17Default')],
                  ['--repo', t('docs.scanFlag18Desc'), t('docs.scanFlag18Default')],
                ].map(([flag, d, def], idx, arr) => (
                  <div key={idx} style={{ display: 'flex', borderBottom: idx < arr.length - 1 ? '1px solid rgba(255,255,255,0.16)' : 'none' }}>
                    <div style={{ width: 160, flexShrink: 0, display: 'flex', alignItems: 'center', padding: '10px 12px' }}>
                      <span style={{ fontSize: 12, fontFamily: 'Menlo, monospace', color: 'rgba(255,255,255,0.7)' }}>{flag}</span>
                    </div>
                    <div style={{ flex: 1, display: 'flex', alignItems: 'center', padding: '10px 12px' }}>
                      <span style={{ fontSize: 13, color: 'rgba(255,255,255,0.6)' }}>{d}</span>
                    </div>
                    <div style={{ width: 120, flexShrink: 0, display: 'flex', alignItems: 'center', padding: '10px 12px' }}>
                      <span style={{ fontSize: 12, fontFamily: 'Menlo, monospace', color: 'rgba(255,255,255,0.5)' }}>{def}</span>
                    </div>
                  </div>
                ))}
              </div>
              <p style={{ ...desc, marginTop: 16, fontSize: 12 }}>
                {t('docs.scanNote').replace(/<\/?code>/g, '')}
              </p>
            </section>

            {/* ─── Viewer ─── */}
            <section id="viewer" style={{ ...sectionSpacing, scrollMarginTop: 100 }}>
              <p style={sectionTitle}>{t('docs.viewerTitle')}</p>
              <p style={desc}>{t('docs.viewerDesc')}</p>
              <CodeBlock code="ocr viewer" onCopy={() => handleCopy('ocr viewer')} />
              <p style={{ ...desc, marginTop: 12 }}>{t('docs.viewerNote')}</p>
            </section>

            {/* ─── MCP Server ─── */}
            <section id="mcp" style={{ ...sectionSpacing, scrollMarginTop: 100 }}>
              <p style={sectionTitle}>{t('docs.mcpTitle')}</p>
              <p style={desc}>{t('docs.mcpDesc')}</p>

              <p style={subTitle}>{t('docs.mcpConfig')}</p>
              <CodeBlock code={mcpAddCommands} onCopy={() => handleCopy(mcpAddCommands)} />
              <p style={{ ...desc, marginTop: 12 }}>{t('docs.mcpConfigLocation')}</p>

              <p style={subTitle}>{t('docs.mcpDelete')}</p>
              <CodeBlock code={mcpDeleteCommands} onCopy={() => handleCopy(mcpDeleteCommands)} />

              <p style={subTitle}>{t('docs.mcpFields')}</p>
              <div style={{ display: 'flex', flexDirection: 'column', alignSelf: 'stretch', border: '1px solid rgba(255,255,255,0.16)', borderRadius: 8, overflow: 'hidden', marginBottom: 16 }}>
                <div style={{ display: 'flex', background: 'rgba(255,255,255,0.04)' }}>
                  <div style={{ width: 140, flexShrink: 0, padding: '10px 12px' }}><span style={{ fontSize: 13, fontWeight: 500, color: 'rgba(255,255,255,0.6)' }}>{t('docs.mcpFieldCol')}</span></div>
                  <div style={{ width: 100, flexShrink: 0, padding: '10px 12px' }}><span style={{ fontSize: 13, fontWeight: 500, color: 'rgba(255,255,255,0.6)' }}>{t('docs.mcpRequiredCol')}</span></div>
                  <div style={{ flex: 1, padding: '10px 12px' }}><span style={{ fontSize: 13, fontWeight: 500, color: 'rgba(255,255,255,0.6)' }}>{t('docs.mcpDescCol')}</span></div>
                </div>
                {[
                  ['command', t('docs.mcpYes'), t('docs.mcpFieldCommandDesc')],
                  ['args', t('docs.mcpNo'), t('docs.mcpFieldArgsDesc')],
                  ['tools', t('docs.mcpNo'), t('docs.mcpFieldToolsDesc')],
                  ['setup', t('docs.mcpNo'), t('docs.mcpFieldSetupDesc')],
                  ['env', t('docs.mcpNo'), t('docs.mcpFieldEnvDesc')],
                ].map(([field, required, d], idx, arr) => (
                  <div key={field} style={{ display: 'flex', borderBottom: idx < arr.length - 1 ? '1px solid rgba(255,255,255,0.16)' : 'none' }}>
                    <div style={{ width: 140, flexShrink: 0, display: 'flex', alignItems: 'center', padding: '10px 12px' }}>
                      <span style={{ fontSize: 12, fontFamily: 'Menlo, monospace', color: 'rgba(255,255,255,0.7)' }}>{field}</span>
                    </div>
                    <div style={{ width: 100, flexShrink: 0, display: 'flex', alignItems: 'center', padding: '10px 12px' }}>
                      <span style={{ fontSize: 13, color: 'rgba(255,255,255,0.6)' }}>{required}</span>
                    </div>
                    <div style={{ flex: 1, display: 'flex', alignItems: 'center', padding: '10px 12px' }}>
                      <span style={{ fontSize: 13, color: 'rgba(255,255,255,0.6)' }}>{d}</span>
                    </div>
                  </div>
                ))}
              </div>

              <p style={{ ...desc, fontSize: 12 }}>
                {t('docs.mcpNote')}
              </p>

              <p style={subTitle}>{t('docs.mcpExample')}</p>
              <CodeBlock code={mcpCodeGraphCommands} onCopy={() => handleCopy(mcpCodeGraphCommands)} />
            </section>

            {/* ─── Claude Code Integration ─── */}
            <section id="env" style={{ ...sectionSpacing, scrollMarginTop: 100 }}>
              <p style={sectionTitle}>{t('docs.envTitle')}</p>
              <p style={desc}>
                {t('docs.envDesc').replace(/<\/?code>/g, '')}
              </p>
              <CodeBlock
                code={`export ANTHROPIC_BASE_URL=https://api.anthropic.com\nexport ANTHROPIC_AUTH_TOKEN=sk-ant-xxxxx\nexport ANTHROPIC_MODEL=claude-opus-4-6\n\n# Open Code Review auto-detects these variables ✨`}
                onCopy={() => handleCopy('export ANTHROPIC_BASE_URL=https://api.anthropic.com\nexport ANTHROPIC_AUTH_TOKEN=sk-ant-xxxxx\nexport ANTHROPIC_MODEL=claude-opus-4-6')}
              />
              <p style={{ ...desc, marginTop: 12 }}>
                {t('docs.envNote').replace(/<\/?code>/g, '')}
              </p>
            </section>
          </div>
        </div>

        {/* ─── Right sidebar: CONTENTS (fixed) ─── */}
        {!isMobile && (
          <div style={{ position: 'fixed', top: 112, right: 'max(40px, calc((100vw - 1440px) / 2 + 32px))', width: 220, zIndex: 30 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 16 }}>
              <img src={docContentsIcon} style={{ width: 20, height: 20 }} />
              <span style={{ fontSize: 12, fontWeight: 600, color: 'rgba(255,255,255,0.5)', letterSpacing: '0.05em' }}>{t('docs.toc')}</span>
            </div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
              {sections.map((s) => (
                <button
                  key={s.id}
                  onClick={() => scrollToSection(s.id)}
                  style={{
                    background: 'none',
                    border: 'none',
                    cursor: 'pointer',
                    textAlign: 'left',
                    fontSize: 14,
                    fontFamily: 'PingFang SC, -apple-system, sans-serif',
                    fontWeight: activeSection === s.id ? 500 : 400,
                    color: activeSection === s.id ? '#2BDE5E' : 'rgba(255,255,255,0.5)',
                    lineHeight: '22px',
                    padding: 0,
                    transition: 'color 0.2s',
                  }}
                >
                  {s.label}
                </button>
              ))}
            </div>
          </div>
        )}
      </div>
      <Footer />
      <Toast message={t('quickstart.copied')} visible={toastVisible} />
    </div>
  );
};

export default DocsPage;
