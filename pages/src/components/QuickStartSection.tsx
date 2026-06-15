import React, { useState } from 'react';
import { useTranslation } from '../i18n';

const CodeBlock: React.FC<{
  block: { label?: string; code: string };
  index: string;
  copiedIndex: string | null;
  onCopy: (code: string, key: string) => void;
  copyLabel: string;
}> = ({ block, index, copiedIndex, onCopy, copyLabel }) => (
  <div>
    {block.label && (
      <p className="text-slate-500 text-xs font-mono mb-2 ml-1">{block.label}</p>
    )}
    <div className="relative group/code">
      <div className="code-block rounded-xl p-4 overflow-x-auto group-hover/code:border-brand-500/30 transition-colors duration-300">
        <pre className="font-mono text-xs text-brand-400 whitespace-pre">{block.code}</pre>
      </div>
      <button
        onClick={() => onCopy(block.code, index)}
        className="absolute top-2 right-3 text-slate-600 hover:text-brand-400 transition-colors text-xs flex items-center gap-1 opacity-60 group-hover/code:opacity-100"
      >
        <i className={`fa-solid ${copiedIndex === index ? 'fa-check text-brand-400' : 'fa-copy'}`}></i>
        {copiedIndex === index ? '' : copyLabel}
      </button>
    </div>
  </div>
);

const QuickStartSection: React.FC = () => {
  const [copiedIndex, setCopiedIndex] = useState<string | null>(null);
  const { t } = useTranslation();

  const steps = [
    {
      number: '01',
      title: t('quickstart.step1Title'),
      icon: 'fa-solid fa-cloud-arrow-down',
      description: t('quickstart.step1Desc'),
      codeBlocks: [
        { label: t('quickstart.step1Label1'), code: `npm i -g @alibaba-group/open-code-review` },
        { label: t('quickstart.step1Label2'), code: `ocr version` }
      ]
    },
    {
      number: '02',
      title: t('quickstart.step2Title'),
      icon: 'fa-solid fa-sliders',
      description: t('quickstart.step2Desc'),
      codeBlocks: [
        {
          label: t('quickstart.step2Label1'),
          code: `ocr config provider`
        },
        {
          label: t('quickstart.step2Label2'),
          code: `ocr config set llm.url https://api.anthropic.com \\\n    && ocr config set llm.auth_token {{your-api-key}} \\\n    && ocr config set llm.model claude-opus-4-6 \\\n    && ocr config set llm.use_anthropic true`
        },
        { label: t('quickstart.step2Label3'), code: `ocr llm test` }
      ]
    },
    {
      number: '03',
      title: t('quickstart.step3Title'),
      icon: 'fa-solid fa-magnifying-glass-code',
      description: t('quickstart.step3Desc'),
      codeBlocks: [
        {
          code: `${t('quickstart.commentReview')}\nocr review\n\n${t('quickstart.commentBranch')}\nocr review --from main --to feature-auth\n\n${t('quickstart.commentCommit')}\nocr review --commit abc123`
        }
      ]
    }
  ];

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

  return (
    <section id="quickstart" className="py-24 relative noise-overlay">
      {/* Ambient glow */}
      <div className="absolute bottom-0 left-1/4 w-[600px] h-[400px] rounded-full bg-brand-500/[0.02] blur-[100px] pointer-events-none"></div>

      <div className="relative z-10">
        <div className="section-divider mb-24"></div>
        <div className="max-w-4xl mx-auto px-6">
          <div className="text-center mb-16">
            <p className="text-slate-500 text-sm font-mono uppercase tracking-widest mb-3">{t('quickstart.sectionLabel')}</p>
            <h2 className="text-4xl font-bold text-white mb-4">
              {t('quickstart.title')}
            </h2>
            <p className="text-slate-400 max-w-xl mx-auto">
              {t('quickstart.subtitle')}
            </p>
          </div>

          <div className="space-y-8">
            {steps.map((step, stepIndex) => (
              <div key={step.number} className="feature-card rounded-2xl p-6 relative glass">
                {/* Connector line between steps */}
                {stepIndex < steps.length - 1 && (
                  <div className="absolute left-8 -bottom-8 w-px h-8 bg-gradient-to-b from-brand-500/20 to-transparent hidden lg:block"></div>
                )}

                {/* Step header */}
                <div className="flex items-start gap-4 mb-6">
                  <div className="flex-shrink-0 w-10 h-10 rounded-xl bg-brand-500/10 border border-brand-500/20 flex items-center justify-center">
                    <span className="font-mono text-brand-400 text-sm font-bold">{step.number}</span>
                  </div>
                  <div className="flex-1">
                    <div className="flex items-center gap-2 mb-1">
                      <i className={`${step.icon} text-slate-500 text-sm`}></i>
                      <h3 className="text-white font-semibold text-lg">{step.title}</h3>
                    </div>
                    <p className="text-slate-400 text-sm">{step.description}</p>
                  </div>
                </div>

                {/* Code blocks */}
                <div className="space-y-3">
                  {step.codeBlocks.map((block, blockIdx) => (
                    <CodeBlock
                      key={blockIdx}
                      block={block}
                      index={`${stepIndex}-${blockIdx}`}
                      copiedIndex={copiedIndex}
                      onCopy={handleCopy}
                      copyLabel={t('quickstart.copy')}
                    />
                  ))}
                </div>
              </div>
            ))}
          </div>

          {/* Zero-config callout */}
          <div className="mt-8 stat-card rounded-2xl p-6 border-brand-500/15 glass">
            <div className="absolute inset-0 bg-gradient-to-r from-brand-500/[0.03] via-transparent to-transparent rounded-2xl pointer-events-none"></div>
            <h3 className="text-white font-semibold mb-3 flex items-center gap-2 relative z-10">
              <i className="fa-solid fa-magic text-brand-400"></i>
              {t('quickstart.zeroCfgTitle')}
            </h3>
            <p className="text-slate-400 text-sm mb-3 relative z-10">
              {t('quickstart.zeroCfgDesc')}
            </p>
            <div className="code-block rounded-xl p-4 overflow-x-auto relative z-10">
              <pre className="font-mono text-xs text-brand-400 whitespace-pre">{`export ANTHROPIC_BASE_URL=https://api.anthropic.com
export ANTHROPIC_AUTH_TOKEN=sk-ant-xxxxx
export ANTHROPIC_MODEL=claude-opus-4-6

${t('quickstart.commentEnvAuto')} ✨`}</pre>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
};

export default QuickStartSection;
