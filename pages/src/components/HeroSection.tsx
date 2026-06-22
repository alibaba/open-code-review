import React, { useEffect, useState } from 'react';
import { useTranslation } from '../i18n';

const terminalLines = [
  { delay: 0, text: '$ ocr review --from main --to feature-auth', type: 'command' },
  { delay: 600, text: '[ocr] Reviewing 5 file(s) in /home/user/project', type: 'info' },
  { delay: 1200, text: '[ocr]   ▶ file_read "internal/auth/login.go"', type: 'tool' },
  { delay: 1600, text: '[ocr]   ✔ file_read (15ms)', type: 'tool-ok' },
  { delay: 2000, text: '[ocr]   ▶ code_search "password.*hash"', type: 'tool' },
  { delay: 2400, text: '[ocr]   ✔ code_search (8ms)', type: 'tool-ok' },
  { delay: 2800, text: '[ocr] Plan completed for internal/auth/login.go', type: 'info' },
  { delay: 3200, text: '', type: 'separator' },
  { delay: 3400, text: '─── internal/auth/login.go:42-55 ───', type: 'header' },
  { delay: 3600, text: 'Consider using bcrypt cost factor ≥ 12 for password hashing.', type: 'comment' },
  { delay: 4000, text: '', type: 'separator' },
  { delay: 4200, text: '[ocr] Summary: 5 file(s), 3 comment(s), ~8421 tokens, 12.5s', type: 'result' }
];

const TerminalLine: React.FC<{ line: typeof terminalLines[0]; visible: boolean }> = ({ line, visible }) => {
  const colorMap: Record<string, string> = {
    command: 'text-brand-400 font-semibold',
    info: 'text-slate-400',
    tool: 'text-cyan-400',
    'tool-ok': 'text-green-400',
    separator: 'text-slate-600',
    header: 'text-slate-500 font-mono',
    comment: 'text-slate-300 text-xs',
    result: 'text-brand-400 font-medium'
  };

  return (
    <div
      className={`font-mono text-sm transition-all duration-300 ${
        visible ? 'opacity-100 translate-y-0' : 'opacity-0 translate-y-2'
      } ${colorMap[line.type]}`}
    >
      {line.text}
    </div>
  );
};

const HeroSection: React.FC = () => {
  const [visibleLines, setVisibleLines] = useState<number[]>([]);
  const { t } = useTranslation();

  useEffect(() => {
    const timers: ReturnType<typeof setTimeout>[] = [];
    terminalLines.forEach((line, index) => {
      const timer = setTimeout(() => {
        setVisibleLines((prev) => [...prev, index]);
      }, line.delay + 500);
      timers.push(timer);
    });
    return () => { timers.forEach((timer) => clearTimeout(timer)); };
  }, []);

  const pills = [
    { icon: 'fa-check-circle', color: 'text-brand-400', label: t('hero.pill1') },
    { icon: 'fa-shield-halved', color: 'text-cyan-400', label: t('hero.pill2') },
    { icon: 'fa-bolt', color: 'text-yellow-400', label: t('hero.pill3') }
  ];

  return (
    <section className="relative min-h-screen flex items-center justify-center overflow-hidden grid-bg noise-overlay spotlight">
      {/* Layered glow orbs */}
      <div className="absolute top-1/4 left-1/2 -translate-x-1/2 w-[800px] h-[800px] rounded-full bg-brand-500/[0.04] blur-[120px] pointer-events-none"></div>
      <div className="absolute bottom-1/4 right-1/4 w-[500px] h-[500px] rounded-full bg-cyan-500/[0.03] blur-[100px] pointer-events-none"></div>
      <div className="absolute top-1/3 left-1/4 w-[300px] h-[300px] rounded-full bg-purple-500/[0.02] blur-[80px] pointer-events-none"></div>

      {/* Decorative horizontal lines */}
      <div className="absolute top-0 left-0 right-0 h-px bg-gradient-to-r from-transparent via-brand-500/20 to-transparent"></div>

      <div className="relative z-10 max-w-7xl mx-auto px-6 pt-24 pb-16 grid lg:grid-cols-2 gap-16 items-center">
        {/* Left content */}
        <div className="space-y-8">
          <div className="space-y-4">
            <h1 className="text-5xl lg:text-5xl font-bold leading-tight tracking-tight">
              <span className="text-white">{t('hero.title')}</span>
              <br />
              <span className="bg-gradient-to-r from-brand-400 via-green-300 to-cyan-400 bg-clip-text text-transparent text-glow">{t('hero.titleHighlight')}</span>
            </h1>
            <p className="text-slate-400 text-lg leading-relaxed max-w-xl">
              {t('hero.description')}
            </p>
          </div>

          {/* Feature pills */}
          <div className="flex flex-wrap gap-6 text-sm">
            {pills.map((item) => (
              <div key={item.label} className="flex items-center gap-2 text-slate-300 group">
                <i className={`fa-solid ${item.icon} ${item.color} group-hover:scale-110 transition-transform`}></i>
                <span className="group-hover:text-white transition-colors">{item.label}</span>
              </div>
            ))}
          </div>

          {/* CTA buttons */}
          <div className="flex flex-wrap gap-4">
            <button
              onClick={() => document.getElementById('quickstart')?.scrollIntoView({ behavior: 'smooth' })}
              className="btn-primary text-dark-900 font-semibold px-6 py-3 rounded-xl flex items-center gap-2 shadow-lg"
            >
              <i className="fa-solid fa-rocket"></i>
              {t('hero.cta1')}
            </button>
            <button
              onClick={() => document.getElementById('benchmark')?.scrollIntoView({ behavior: 'smooth' })}
              className="btn-secondary text-brand-400 font-medium px-6 py-3 rounded-xl flex items-center gap-2"
            >
              <i className="fa-solid fa-chart-bar"></i>
              {t('hero.cta2')}
            </button>
          </div>

          {/* Social proof */}
          <div className="flex items-center gap-6 pt-2">
            <div className="flex items-center gap-2 text-slate-500 text-sm">
              <i className="fa-solid fa-users text-slate-600"></i>
              <span>{t('hero.users')}</span>
            </div>
            <div className="w-px h-4 bg-dark-500"></div>
            <div className="flex items-center gap-2 text-slate-500 text-sm">
              <i className="fa-brands fa-github text-slate-600"></i>
              <span>{t('hero.openSource')}</span>
            </div>
          </div>
        </div>

        {/* Right terminal */}
        <div className="relative">
          {/* Outer glow frame */}
          <div className="absolute -inset-1 bg-gradient-to-r from-brand-500/10 via-cyan-500/10 to-purple-500/10 rounded-3xl blur-xl opacity-50 pointer-events-none"></div>

          <div className="code-block rounded-2xl p-6 space-y-3 relative overflow-hidden glass-strong">
            {/* Scan line effect */}
            <div className="absolute inset-0 bg-gradient-to-b from-transparent via-brand-500/[0.02] to-transparent h-1/4 animate-scan-line pointer-events-none"></div>

            <div className="flex items-center gap-2 mb-4">
              <div className="w-3 h-3 rounded-full bg-red-500/70"></div>
              <div className="w-3 h-3 rounded-full bg-yellow-500/70"></div>
              <div className="w-3 h-3 rounded-full bg-brand-500/70"></div>
              <span className="ml-2 text-slate-500 text-xs font-mono">{t('hero.terminal')}</span>
            </div>

            {terminalLines.map((line, index) => (
              <TerminalLine
                key={index}
                line={line}
                visible={visibleLines.includes(index)}
              />
            ))}

            {visibleLines.length < terminalLines.length && (
              <div className="font-mono text-sm text-brand-400 terminal-cursor"></div>
            )}
          </div>

          {/* F1 badge */}
          <div className="absolute -top-4 -right-4 floating-badge">
            <div className="rank-badge px-3 py-2 rounded-xl text-xs font-mono text-brand-400 backdrop-blur-md">
              <div className="text-lg font-bold">F1: {t('highlights.stat4Value')}</div>
              <div className="text-slate-500">{t('hero.badgeLabel')}</div>
            </div>
          </div>

          {/* Decorative accent line */}
          <div className="absolute -bottom-1 left-8 right-8 h-px bg-gradient-to-r from-brand-500/40 via-cyan-500/20 to-transparent"></div>
        </div>
      </div>

      {/* Scroll indicator */}
      <div className="absolute bottom-8 left-1/2 -translate-x-1/2 flex flex-col items-center gap-2 animate-bounce">
        <span className="text-slate-600 text-[10px] uppercase tracking-widest">Scroll</span>
        <i className="fa-solid fa-chevron-down text-slate-600 text-sm"></i>
      </div>
    </section>
  );
};

export default HeroSection;
