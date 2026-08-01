import React from 'react';
import { useTranslation } from '../i18n';
import { useResponsive } from '../hooks/useResponsive';
import { useSectionTitleStyle } from '../hooks/useResponsiveStyle';
import icon1 from '../assets/icons/icon-feature-architecture.svg';
import icon2 from '../assets/icons/icon-feature-positioning.svg';
import icon3 from '../assets/icons/icon-feature-multi-model.svg';
import icon4 from '../assets/icons/icon-feature-concurrent.svg';
import icon5 from '../assets/icons/icon-feature-compression.svg';
import icon6 from '../assets/icons/icon-feature-rules.svg';

const FeaturesSection: React.FC = () => {
  const { t } = useTranslation();
  const { isMobile, isTablet } = useResponsive();
  const titleStyle = useSectionTitleStyle();

  const features = [
    { icon: icon1, title: t('features.feat1Title'), desc: t('features.feat1Desc') },
    { icon: icon2, title: t('features.feat2Title'), desc: t('features.feat2Desc') },
    { icon: icon3, title: t('features.feat3Title'), desc: t('features.feat3Desc') },
    { icon: icon4, title: t('features.feat4Title'), desc: t('features.feat4Desc') },
    { icon: icon5, title: t('features.feat5Title'), desc: t('features.feat5Desc') },
    { icon: icon6, title: t('features.feat6Title'), desc: t('features.feat6Desc') },
  ];

  return (
    <section
      id="features"
      style={{
        width: '100%',
        display: 'flex',
        justifyContent: 'center',
        padding: isMobile ? '60px 20px' : isTablet ? '80px 40px' : '80px 0',
        overflow: 'hidden',
      }}
    >
      <div style={{ width: '100%', maxWidth: 1200, display: 'flex', flexDirection: 'column', gap: isMobile ? 32 : 48 }}>
        {/* Header */}
        <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 12 }}>
          <span style={{ color: '#2BDE5E', fontSize: 16, fontWeight: 500, lineHeight: '22px', letterSpacing: '0.48px' }}>
            {t('features.sectionBadge')}
          </span>
          <h2 style={{ color: '#FFFFFF', fontSize: titleStyle.fontSize, fontWeight: 500, textAlign: 'center', lineHeight: titleStyle.lineHeight, letterSpacing: '0.96px', margin: 0, maxWidth: 758 }}>
            {t('features.title')}
          </h2>
          <p style={{ color: 'rgba(255,255,255,0.5)', fontSize: 16, textAlign: 'center', lineHeight: '24px', margin: 0, maxWidth: 646 }}>
            {t('features.subtitle')}
          </p>
        </div>

        {/* Grid */}
        <div
          style={{
            display: 'grid',
            gridTemplateColumns: isMobile ? '1fr' : isTablet ? 'repeat(2, 1fr)' : 'repeat(3, 1fr)',
            border: '1px solid rgba(255,255,255,0.16)',
            borderRadius: 8,
          }}
        >
          {features.map((feat, i) => {
            const cols = isMobile ? 1 : isTablet ? 2 : 3;
            const isLastCol = (i % cols) === cols - 1;
            const isLastRow = i >= features.length - (features.length % cols || cols);
            return (
            <div
              key={i}
              style={{
                padding: isMobile ? '24px 20px' : '32px 28px',
                display: 'flex',
                flexDirection: 'column',
                gap: 16,
                borderRight: isLastCol ? 'none' : '1px solid rgba(255,255,255,0.16)',
                borderBottom: isLastRow ? 'none' : '1px solid rgba(255,255,255,0.16)',
                minHeight: isMobile ? undefined : 272,
              }}
            >
              <div
                style={{
                  width: 48,
                  height: 48,
                  display: 'flex',
                  justifyContent: 'center',
                  alignItems: 'center',
                  borderRadius: 8,
                  border: '1px solid rgba(255,255,255,0.16)',
                }}
              >
                <img src={feat.icon} alt="" style={{ width: 24, height: 24 }} />
              </div>
              <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                <p style={{ color: '#FFFFFF', fontSize: 16, fontWeight: 500, margin: 0, maxWidth: 352 }}>
                  {feat.title}
                </p>
                <p style={{ color: 'rgba(255,255,255,0.5)', fontSize: 14, lineHeight: '20px', margin: 0, maxWidth: 352 }}>
                  {feat.desc}
                </p>
              </div>
            </div>
            );
          })}
        </div>
      </div>
    </section>
  );
};

export default FeaturesSection;
