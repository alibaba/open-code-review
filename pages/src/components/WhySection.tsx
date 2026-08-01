import React from 'react';
import { useTranslation } from '../i18n';

const WhySection: React.FC = () => {
  const { t } = useTranslation();

  const useCases = [
    {
      icon: 'fa-user-astronaut',
      title: t('why.case1Title'),
      description: t('why.case1Desc'),
      color: 'text-brand-400',
      bgColor: 'bg-brand-500/8'
    },
    {
      icon: 'fa-building',
      title: t('why.case2Title'),
      description: t('why.case2Desc'),
      color: 'text-cyan-400',
      bgColor: 'bg-cyan-500/8'
    },
    {
      icon: 'fa-brain',
      title: t('why.case3Title'),
      description: t('why.case3Desc'),
      color: 'text-purple-400',
      bgColor: 'bg-purple-500/8'
    }
  ];

  return (
    <section id="why" className="py-24 relative noise-overlay">
      {/* Ambient glow */}
      <div className="absolute top-1/4 left-1/4 w-[400px] h-[400px] rounded-full bg-red-500/[0.015] blur-[100px] pointer-events-none"></div>
      <div className="absolute bottom-1/4 right-1/4 w-[400px] h-[400px] rounded-full bg-brand-500/[0.02] blur-[100px] pointer-events-none"></div>

      <div className="relative z-10">
        <div className="section-divider mb-24"></div>
        <div className="max-w-7xl mx-auto px-6">
          <div className="text-center mb-16">
            <p className="text-slate-500 text-sm font-mono uppercase tracking-widest mb-3">{t('why.sectionLabel')}</p>
            <h2 className="text-4xl font-bold text-white mb-4">{t('why.title')}</h2>
          </div>

          {/* Use cases */}
          <div>
            <div className="grid md:grid-cols-3 gap-6">
              {useCases.map((useCase) => (
                <div key={useCase.title} className={`feature-card rounded-2xl p-6 ${useCase.bgColor} glass`}>
                  <div className={`text-3xl mb-4 ${useCase.color}`}>
                    <i className={`fa-solid ${useCase.icon}`}></i>
                  </div>
                  <h4 className="text-white font-semibold text-lg mb-2">{useCase.title}</h4>
                  <p className="text-slate-400 text-sm leading-relaxed">{useCase.description}</p>
                </div>
              ))}
            </div>
          </div>
        </div>
      </div>
    </section>
  );
};

export default WhySection;
