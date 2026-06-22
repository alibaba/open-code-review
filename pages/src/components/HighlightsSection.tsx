import React, { useEffect, useRef, useState } from 'react';
import { useTranslation } from '../i18n';

const HighlightsSection: React.FC = () => {
  const { t } = useTranslation();
  const sectionRef = useRef<HTMLDivElement>(null);
  const [visible, setVisible] = useState(false);

  useEffect(() => {
    const observer = new IntersectionObserver(
      ([entry]) => {
        if (entry.isIntersecting) {
          setVisible(true);
          observer.disconnect();
        }
      },
      { threshold: 0.3 }
    );
    if (sectionRef.current) observer.observe(sectionRef.current);
    return () => observer.disconnect();
  }, []);

  const stats = [
    {
      label: t('highlights.stat1Label'),
      value: t('highlights.stat1Value'),
      caption: t('highlights.stat1Caption'),
      delay: 0,
    },
    {
      label: t('highlights.stat2Label'),
      value: t('highlights.stat2Value'),
      caption: t('highlights.stat2Caption'),
      delay: 100,
    },
    {
      label: t('highlights.stat3Label'),
      value: t('highlights.stat3Value'),
      caption: t('highlights.stat3Caption'),
      delay: 200,
    },
    {
      label: t('highlights.stat4Label'),
      value: t('highlights.stat4Value'),
      caption: t('highlights.stat4Caption'),
      delay: 300,
    },
  ];

  return (
    <section ref={sectionRef} className="relative py-16 overflow-hidden">
      <div className="absolute inset-0 bg-gradient-to-b from-dark-900 via-dark-800/50 to-dark-900 pointer-events-none"></div>

      <div className="relative z-10 max-w-7xl mx-auto px-6">
        <div className="highlight-card rounded-2xl p-1">
          <div className="rounded-2xl bg-dark-900/80 backdrop-blur-md p-10 md:p-14">
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-10 md:gap-8 lg:gap-0 lg:divide-x lg:divide-dark-500/40">
              {stats.map((stat, i) => (
                <article
                  key={i}
                  className={`flex flex-col items-center lg:items-start lg:px-10 xl:px-14 first:lg:pl-0 last:lg:pr-0 transition-all duration-700 ${
                    visible
                      ? 'opacity-100 translate-y-0'
                      : 'opacity-0 translate-y-6'
                  }`}
                  style={{ transitionDelay: `${stat.delay}ms` }}
                >
                  <header className="flex items-center gap-3 mb-4">
                    <span className="block w-8 h-px bg-gradient-to-r from-brand-400 to-transparent"></span>
                    <span className="text-xs font-medium uppercase tracking-widest text-slate-500 whitespace-nowrap">
                      {stat.label}
                    </span>
                  </header>
                  <p className="text-4xl md:text-5xl font-bold bg-gradient-to-r from-brand-400 to-cyan-400 bg-clip-text text-transparent leading-tight">
                    {stat.value}
                  </p>
                  <p className="mt-2 text-sm text-slate-400">{stat.caption}</p>
                </article>
              ))}
            </div>
          </div>
        </div>
      </div>
    </section>
  );
};

export default HighlightsSection;
