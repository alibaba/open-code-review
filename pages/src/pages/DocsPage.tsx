import React, { useState, useEffect, useRef } from 'react';
import Navbar from '../components/Navbar';
import { useTranslation } from '../i18n';

interface Section {
  id: string;
  labelKey: string;
}

const sectionDefs: Section[] = [
  { id: 'overview', labelKey: 'docs.overview' },
  { id: 'install', labelKey: 'docs.install' },
  { id: 'config', labelKey: 'docs.config' },
  { id: 'review', labelKey: 'docs.review' },
  { id: 'viewer', labelKey: 'docs.viewer' },
  { id: 'env', labelKey: 'docs.env' },
];

const CodeBlock: React.FC<{ code: string; copied?: boolean; onCopy?: () => void; copyLabel?: string }> = ({ code, copied, onCopy, copyLabel }) => (
  <div className="relative group/code">
    <div className="code-block rounded-xl p-4 overflow-x-auto group-hover/code:border-brand-500/30 transition-colors duration-300">
      <pre className="font-mono text-xs text-brand-400 whitespace-pre">{code}</pre>
    </div>
    {onCopy && (
      <button
        onClick={onCopy}
        className="absolute top-2 right-3 text-slate-600 hover:text-brand-400 transition-colors text-xs flex items-center gap-1 opacity-60 group-hover/code:opacity-100"
      >
        <i className={`fa-solid ${copied ? 'fa-check text-brand-400' : 'fa-copy'}`}></i>
        {copied ? '' : (copyLabel || '')}
      </button>
    )}
  </div>
);

const DocSection: React.FC<{ id: string; title: string; children: React.ReactNode }> = ({ id, title, children }) => (
  <section id={id} className="mb-16 scroll-mt-24">
    <h2 className="text-2xl font-bold text-white mb-6 pb-2 border-b border-dark-600/30">{title}</h2>
    {children}
  </section>
);

const DocsPage: React.FC = () => {
  const [activeSection, setActiveSection] = useState('overview');
  const [mobileTocOpen, setMobileTocOpen] = useState(false);
  const [copiedIndex, setCopiedIndex] = useState<string | null>(null);
  const lockedRef = useRef<string | null>(null);
  const { t } = useTranslation();

  const sections = sectionDefs.map(s => ({ ...s, label: t(s.labelKey) }));

  const handleCopy = (code: string, key: string) => {
    if (navigator.clipboard?.writeText) {
      navigator.clipboard.writeText(code).then(() => {
        setCopiedIndex(key);
        setTimeout(() => setCopiedIndex(null), 2000);
      });
    } else {
      const textarea = document.createElement('textarea');
      textarea.value = code;
      textarea.style.position = 'fixed';
      textarea.style.opacity = '0';
      document.body.appendChild(textarea);
      textarea.select();
      document.execCommand('copy');
      document.body.removeChild(textarea);
      setCopiedIndex(key);
      setTimeout(() => setCopiedIndex(null), 2000);
    }
  };

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

  const unlockTimerRef = useRef<ReturnType<typeof setTimeout>>();

  const scrollToSection = (id: string) => {
    lockedRef.current = id;
    clearTimeout(unlockTimerRef.current);
    const el = document.getElementById(id);
    if (el) {
      el.scrollIntoView({ behavior: 'smooth' });
      window.history.pushState(null, '', `#${id}`);
    }
    setActiveSection(id);
    unlockTimerRef.current = setTimeout(() => {
      lockedRef.current = null;
    }, 800);
  };

  return (
    <div className="min-h-screen bg-dark-900 relative noise-overlay pt-16">
      <div className="absolute inset-0 pointer-events-none">
        <div className="absolute top-0 left-1/4 w-[800px] h-[600px] rounded-full bg-brand-500/[0.02] blur-[120px]"></div>
      </div>

      <Navbar />

      {/* Mobile TOC toggle */}
      <div className="lg:hidden fixed top-16 right-4 z-50">
        <button
          className="text-slate-400 hover:text-white transition-colors text-sm flex items-center gap-2 bg-dark-900/80 backdrop-blur-xl border border-dark-600/30 rounded-lg px-3 py-1.5"
          onClick={() => setMobileTocOpen(!mobileTocOpen)}
        >
          <i className="fa-solid fa-list-ul"></i>
          {t('docs.toc')}
        </button>
      </div>

      {/* Mobile TOC dropdown */}
      {mobileTocOpen && (
        <div className="lg:hidden fixed inset-0 z-[60] bg-black/60" onClick={() => setMobileTocOpen(false)}>
          <div
            className="bg-dark-900 border-r border-dark-600/30 w-64 max-h-full overflow-y-auto pt-16 pb-8 px-4"
            onClick={(e) => e.stopPropagation()}
          >
            <ul className="space-y-1">
              {sections.map((s) => (
                <li key={s.id}>
                  <button
                    onClick={() => { scrollToSection(s.id); setMobileTocOpen(false); }}
                    className={`block w-full text-left px-3 py-2 rounded-lg text-sm transition-colors ${
                      activeSection === s.id ? 'text-brand-400 bg-brand-500/10 font-medium' : 'text-slate-400 hover:text-white hover:bg-dark-800/50'
                    }`}
                  >
                    {s.label}
                  </button>
                </li>
              ))}
            </ul>
          </div>
        </div>
      )}

      <div className="max-w-7xl mx-auto px-6 py-12 flex gap-12 relative z-10">
        {/* Sidebar TOC — desktop */}
        <aside className="hidden lg:block w-56 flex-shrink-0 sticky top-24 self-start max-h-[calc(100vh-120px)] overflow-y-auto">
          <p className="text-slate-500 text-xs font-mono uppercase tracking-widest mb-4">{t('docs.toc')}</p>
          <ul className="space-y-1 border-l border-dark-600/20 pl-4">
            {sections.map((s) => (
              <li key={s.id}>
                <button
                  onClick={() => scrollToSection(s.id)}
                  className={`w-full text-left block py-1.5 text-sm transition-all border-l-2 -ml-4 pl-4 ${
                    activeSection === s.id
                      ? 'text-brand-400 border-brand-500 font-medium'
                      : 'text-slate-500 border-transparent hover:text-slate-300 hover:border-slate-700'
                  }`}
                >
                  {s.label}
                </button>
              </li>
            ))}
          </ul>
        </aside>

        {/* Main content */}
        <main className="flex-1 min-w-0 max-w-3xl">
          {/* Overview */}
          <DocSection id="overview" title={t('docs.overviewTitle')}>
            <p className="text-slate-300 leading-relaxed mb-4">
              <code className="text-brand-400 bg-dark-800/50 px-1.5 py-0.5 rounded text-sm font-mono">Open Code Review</code>{' '}
              <span dangerouslySetInnerHTML={{ __html: t('docs.overviewDesc') }} />
            </p>
            <div className="glass rounded-xl p-5 mb-6">
              <p className="text-slate-400 text-sm mb-3"><strong className="text-white">{t('docs.overviewFeatures')}</strong></p>
              <ul className="space-y-2 text-sm text-slate-400">
                {(['docs.overviewFeat1', 'docs.overviewFeat2', 'docs.overviewFeat3', 'docs.overviewFeat4', 'docs.overviewFeat5', 'docs.overviewFeat6'] as const).map((key) => (
                  <li key={key} className="flex items-start gap-2"><i className="fa-solid fa-check text-brand-500 mt-1 text-xs"></i>{t(key)}</li>
                ))}
              </ul>
            </div>
          </DocSection>

          {/* Install */}
          <DocSection id="install" title={t('docs.installTitle')}>
            <div className="space-y-4 mb-8">
              <div className="feature-card rounded-xl p-4 glass">
                <h4 className="text-white font-semibold mb-2 flex items-center gap-2">
                  <i className="fa-solid fa-download text-brand-400 text-sm"></i>
                  {t('docs.installLabel')}
                </h4>
                <CodeBlock
                  code="npm i -g @alibaba-group/open-code-review"
                  copied={copiedIndex === 'install'}
                  onCopy={() => handleCopy('npm i -g @alibaba-group/open-code-review', 'install')}
                  copyLabel={t('docs.copy')}
                />
              </div>
              <div className="feature-card rounded-xl p-4 glass">
                <h4 className="text-white font-semibold mb-2 flex items-center gap-2">
                  <i className="fa-solid fa-circle-check text-brand-400 text-sm"></i>
                  {t('docs.installVerifyLabel')}
                </h4>
                <CodeBlock
                  code="ocr version"
                  copied={copiedIndex === 'install-verify'}
                  onCopy={() => handleCopy('ocr version', 'install-verify')}
                  copyLabel={t('docs.copy')}
                />
              </div>
            </div>
          </DocSection>

          {/* Config */}
          <DocSection id="config" title={t('docs.configTitle')}>
            <p className="text-slate-300 leading-relaxed mb-6" dangerouslySetInnerHTML={{ __html: t('docs.configDesc') }} />

            <h3 className="text-lg font-semibold text-white mb-3">{t('docs.configInteractive')}</h3>
            <p className="text-slate-400 text-sm mb-3">{t('docs.configInteractiveDesc')}</p>
            <div className="space-y-3 mb-6">
              <CodeBlock
                code="ocr config provider"
                copied={copiedIndex === 'config-provider'}
                onCopy={() => handleCopy('ocr config provider', 'config-provider')}
                copyLabel={t('docs.copy')}
              />
            </div>
            <div className="space-y-3 mb-6">
              <p className="text-slate-500 text-xs font-mono mb-2 ml-1">{t('docs.configModelSelect')}</p>
              <p className="text-slate-400 text-sm mb-3">{t('docs.configModelSelectDesc')}</p>
              <CodeBlock
                code="ocr config model"
                copied={copiedIndex === 'config-model'}
                onCopy={() => handleCopy('ocr config model', 'config-model')}
                copyLabel={t('docs.copy')}
              />
            </div>
            <div className="space-y-3 mb-8">
              <p className="text-slate-500 text-xs font-mono mb-2 ml-1">{t('docs.configListProviders')}</p>
              <p className="text-slate-400 text-sm mb-3">{t('docs.configListProvidersDesc')}</p>
              <CodeBlock
                code="ocr llm providers"
                copied={copiedIndex === 'config-providers-list'}
                onCopy={() => handleCopy('ocr llm providers', 'config-providers-list')}
                copyLabel={t('docs.copy')}
              />
            </div>

            <h3 className="text-lg font-semibold text-white mb-3">{t('docs.configManual')}</h3>

            <h4 className="text-base font-medium text-slate-300 mb-3">{t('docs.configCommand')}</h4>
            <CodeBlock code="ocr config set &lt;key&gt; &lt;value&gt;" />

            <h4 className="text-base font-medium text-slate-300 mb-3 mt-6">{t('docs.configExample')}</h4>
            <div className="space-y-3 mb-8">
              <CodeBlock
                code={`ocr config set llm.url https://api.anthropic.com \\\n    && ocr config set llm.auth_token {{your-api-key}} \\\n    && ocr config set llm.model claude-opus-4-6 \\\n    && ocr config set llm.use_anthropic true  \\\n    && ocr config set language Chinese`}
                copied={copiedIndex === 'config-examples'}
                onCopy={() => handleCopy(`ocr config set llm.url https://api.anthropic.com \\\n    && ocr config set llm.auth_token {{your-api-key}} \\\n    && ocr config set llm.model claude-opus-4-6 \\\n    && ocr config set llm.use_anthropic true  \\\n    && ocr config set language Chinese`, 'config-examples')}
                copyLabel={t('docs.copy')}
              />
            </div>

            <h3 className="text-lg font-semibold text-white mb-3">{t('docs.configKeys')}</h3>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
              {[
                { key: 'llm.url', descKey: 'docs.configKeyUrl' },
                { key: 'llm.auth_token', descKey: 'docs.configKeyToken' },
                { key: 'llm.model', descKey: 'docs.configKeyModel' },
                { key: 'llm.use_anthropic', descKey: 'docs.configKeyAnthropic' },
                { key: 'llm.extra_body', descKey: 'docs.configKeyExtraBody' },
                { key: 'language', descKey: 'docs.configKeyLanguage' },
                { key: 'telemetry.enabled', descKey: 'docs.configKeyTelemetry' },
              ].map(({ key, descKey }) => (
                <div key={key} className="rounded-lg bg-dark-800/40 px-3 py-2 border border-dark-600/20">
                  <code className="text-brand-400 font-mono text-sm">{key}</code>
                  <span className="text-slate-500 text-sm ml-2">{t(descKey)}</span>
                </div>
              ))}
            </div>

            <h3 className="text-lg font-semibold text-white mb-3 mt-8">{t('docs.configVerify')}</h3>
            <CodeBlock
              code={`# Test LLM connection\nocr llm test`}
            />
            <p className="text-slate-400 text-sm mt-4">
              {t('docs.configVerifyDesc')}
            </p>
          </DocSection>

          {/* Review */}
          <DocSection id="review" title={t('docs.reviewTitle')}>
            <p className="text-slate-300 leading-relaxed mb-6" dangerouslySetInnerHTML={{ __html: t('docs.reviewDesc') }} />

            <h3 className="text-lg font-semibold text-white mb-3">{t('docs.reviewModes')}</h3>
            <div className="space-y-4 mb-8">
              <div className="feature-card rounded-xl p-4 glass">
                <h4 className="text-white font-semibold mb-2 flex items-center gap-2">
                  <i className="fa-solid fa-pen-to-square text-brand-400 text-sm"></i>
                  {t('docs.reviewWorkspace')}
                </h4>
                <p className="text-slate-400 text-sm mb-2">{t('docs.reviewWorkspaceDesc')}</p>
                <CodeBlock code="ocr review" />
              </div>
              <div className="feature-card rounded-xl p-4 glass">
                <h4 className="text-white font-semibold mb-2 flex items-center gap-2">
                  <i className="fa-solid fa-code-branch text-brand-400 text-sm"></i>
                  {t('docs.reviewBranch')}
                </h4>
                <p className="text-slate-400 text-sm mb-2">{t('docs.reviewBranchDesc')}</p>
                <CodeBlock
                  code="ocr review --from master --to dev-ref"
                  copied={copiedIndex === 'review-branch'}
                  onCopy={() => handleCopy('ocr review --from master --to dev-ref', 'review-branch')}
                  copyLabel={t('docs.copy')}
                />
              </div>
              <div className="feature-card rounded-xl p-4 glass">
                <h4 className="text-white font-semibold mb-2 flex items-center gap-2">
                  <i className="fa-solid fa-code-commit text-brand-400 text-sm"></i>
                  {t('docs.reviewCommit')}
                </h4>
                <p className="text-slate-400 text-sm mb-2">{t('docs.reviewCommitDesc')}</p>
                <CodeBlock
                  code={`ocr review --commit abc123\nocr review -c abc123`}
                  copied={copiedIndex === 'review-commit'}
                  onCopy={() => handleCopy('ocr review -c abc123', 'review-commit')}
                  copyLabel={t('docs.copy')}
                />
              </div>
            </div>

            <h3 className="text-lg font-semibold text-white mb-3">{t('docs.reviewAdvanced')}</h3>
            <div className="space-y-4 mb-8">
              <div className="feature-card rounded-xl p-4 glass">
                <h4 className="text-white font-semibold mb-2 flex items-center gap-2">
                  <i className="fa-solid fa-file-code text-brand-400 text-sm"></i>
                  {t('docs.reviewBackground')}
                </h4>
                <p className="text-slate-400 text-sm mb-2">{t('docs.reviewBackgroundDesc')}</p>
                <CodeBlock
                  code={`ocr review --background "requirement context"\nocr review -b "requirement context"`}
                  copied={copiedIndex === 'review-background'}
                  onCopy={() => handleCopy('ocr review --background "requirement context"', 'review-background')}
                  copyLabel={t('docs.copy')}
                />
              </div>
              <div className="feature-card rounded-xl p-4 glass">
                <h4 className="text-white font-semibold mb-2 flex items-center gap-2">
                  <i className="fa-solid fa-file-code text-brand-400 text-sm"></i>
                  {t('docs.reviewJson')}
                </h4>
                <p className="text-slate-400 text-sm mb-2">{t('docs.reviewJsonDesc')}</p>
                <CodeBlock
                  code={`ocr review --format json\nocr review -f json`}
                  copied={copiedIndex === 'review-json'}
                  onCopy={() => handleCopy('ocr review --format json', 'review-json')}
                  copyLabel={t('docs.copy')}
                />
              </div>
              <div className="feature-card rounded-xl p-4 glass">
                <h4 className="text-white font-semibold mb-2 flex items-center gap-2">
                  <i className="fa-solid fa-robot text-brand-400 text-sm"></i>
                  {t('docs.reviewAgent')}
                </h4>
                <p className="text-slate-400 text-sm mb-2">{t('docs.reviewAgentDesc')}</p>
                <CodeBlock
                  code="ocr review --audience agent"
                  copied={copiedIndex === 'review-agent'}
                  onCopy={() => handleCopy('ocr review --audience agent', 'review-agent')}
                  copyLabel={t('docs.copy')}
                />
              </div>
            </div>

            <h3 className="text-lg font-semibold text-white mb-3">{t('docs.reviewFlags')}</h3>
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-dark-600/30">
                    <th className="text-left py-2 px-3 text-slate-400 font-mono text-xs">{t('docs.reviewFlagCol1')}</th>
                    <th className="text-left py-2 px-3 text-slate-400 text-xs">{t('docs.reviewFlagCol2')}</th>
                    <th className="text-left py-2 px-3 text-slate-400 text-xs">{t('docs.reviewFlagCol3')}</th>
                  </tr>
                </thead>
                <tbody>
                  {[
                    ['-c, --commit', t('docs.reviewFlag1Desc'), ''],
                    ['--from', t('docs.reviewFlag2Desc'), ''],
                    ['--to', t('docs.reviewFlag3Desc'), ''],
                    ['-f, --format', t('docs.reviewFlag4Desc'), 'text'],
                    ['--repo', t('docs.reviewFlag5Desc'), t('docs.reviewFlag5Default')],
                    ['--rule', t('docs.reviewFlag6Desc'), t('docs.reviewFlag6Default')],
                    ['--concurrency', t('docs.reviewFlag7Desc'), '8'],
                    ['--timeout', t('docs.reviewFlag8Desc'), '10'],
                    ['--audience', t('docs.reviewFlag9Desc'), 'human'],
                    ['--max-tools', t('docs.reviewFlag10Desc'), t('docs.reviewFlag10Default')],
                    ['--max-git-procs', t('docs.reviewFlag11Desc'), t('docs.reviewFlag10Default')],
                  ].map(([flag, desc, def]) => (
                    <tr key={flag} className="border-b border-dark-800/30 hover:bg-dark-800/20 transition-colors">
                      <td className="py-2 px-3"><code className="text-brand-400 font-mono text-xs whitespace-nowrap">{flag}</code></td>
                      <td className="py-2 px-3 text-slate-300">{desc}</td>
                      <td className="py-2 px-3 text-slate-500 font-mono text-xs">{def || '—'}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <p className="text-slate-500 text-xs mt-3" dangerouslySetInnerHTML={{ __html: t('docs.reviewNote') }} />
          </DocSection>

          {/* Viewer */}
          <DocSection id="viewer" title={t('docs.viewerTitle')}>
            <p className="text-slate-300 leading-relaxed mb-6">
              {t('docs.viewerDesc')}
            </p>

            <CodeBlock code="ocr viewer" />
            <p className="text-slate-400 text-sm mt-4">
              {t('docs.viewerNote')}
            </p>
          </DocSection>

          {/* Environment variables */}
          <DocSection id="env" title={t('docs.envTitle')}>
            <p className="text-slate-300 leading-relaxed mb-4" dangerouslySetInnerHTML={{ __html: t('docs.envDesc') }} />
            <CodeBlock
              code={`export ANTHROPIC_BASE_URL=https://api.anthropic.com
export ANTHROPIC_AUTH_TOKEN=sk-ant-xxxxx
export ANTHROPIC_MODEL=claude-opus-4-6

${t('quickstart.commentEnvAuto')} ✨`}
            />
            <p className="text-slate-400 text-sm mt-4" dangerouslySetInnerHTML={{ __html: t('docs.envNote') }} />
          </DocSection>

          {/* Footer spacer */}
          <div className="h-32"></div>
        </main>
      </div>
    </div>
  );
};

export default DocsPage;
