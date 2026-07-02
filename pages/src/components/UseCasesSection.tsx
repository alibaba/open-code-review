import React, { useCallback, useRef } from 'react';
import { useTranslation } from '../i18n';
import { useResponsive } from '../hooks/useResponsive';
import { useSectionTitleStyle } from '../hooks/useResponsiveStyle';
import iconCase1 from '../assets/icons/icon-usecase-developer.svg';
import iconCase2a from '../assets/icons/icon-usecase-platform-a.svg';
import iconCase2b from '../assets/icons/icon-usecase-platform-b.svg';
import iconCase2c from '../assets/icons/icon-usecase-platform-c.svg';
import iconCase3 from '../assets/icons/icon-usecase-researcher.svg';

const UseCasesSection: React.FC = () => {
  const { t } = useTranslation();
  const { isMobile, isTablet } = useResponsive();
  const titleStyle = useSectionTitleStyle();
  const cardRefs = useRef<(HTMLDivElement | null)[]>([]);

  const handleMouseMove = useCallback((e: React.MouseEvent<HTMLDivElement>, index: number) => {
    const card = cardRefs.current[index];
    if (!card) return;
    const rect = card.getBoundingClientRect();
    const cx = rect.left + rect.width / 2;
    const cy = rect.top + rect.height / 2;
    const angle = Math.atan2(e.clientY - cy, e.clientX - cx) * (180 / Math.PI) + 90;
    card.style.setProperty('--sweep-angle', `${angle}deg`);
  }, []);

  const handleMouseLeave = useCallback((index: number) => {
    const card = cardRefs.current[index];
    if (!card) return;
    card.style.setProperty('--sweep-angle', '0deg');
  }, []);

  const useCases = [
    { title: t('usecases.case1Title'), desc: t('usecases.case1Desc') },
    { title: t('usecases.case2Title'), desc: t('usecases.case2Desc') },
    { title: t('usecases.case3Title'), desc: t('usecases.case3Desc') },
  ];

  return (
    <section
      id="usecases"
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
          <span
            style={{
              color: '#2BDE5E',
              fontSize: 16,
              fontWeight: 500,
              lineHeight: '22px',
              letterSpacing: '0.48px',
            }}
          >
            {t('usecases.sectionLabel')}
          </span>
          <h2
            style={{
              color: '#FFFFFF',
              fontSize: titleStyle.fontSize,
              fontWeight: 500,
              textAlign: 'center',
              lineHeight: titleStyle.lineHeight,
              letterSpacing: '0.96px',
              margin: 0,
            }}
          >
            {t('usecases.title')}
          </h2>
        </div>

        {/* Cards */}
        <div style={{ display: 'flex', flexDirection: isMobile ? 'column' : 'row', gap: 16 }}>
          {useCases.map((item, i) => (
            <div
              key={i}
              ref={(el) => { cardRefs.current[i] = el; }}
              className="usecase-card"
              onMouseMove={(e) => handleMouseMove(e, i)}
              onMouseLeave={() => handleMouseLeave(i)}
              style={{
                flex: 1,
                display: 'flex',
                flexDirection: 'column',
                alignItems: 'center',
                padding: '48px 32px',
              }}
            >
                {/* Icon */}
                <div
                  style={{
                    width: 64,
                    height: 64,
                    marginBottom: 24,
                    position: 'relative',
                    zIndex: 2,
                  }}
                >
                  {i === 0 && (
                    <img src={iconCase1} alt="" style={{ position: 'absolute', left: 5.33, top: 2, width: 53, height: 58 }} />
                  )}
                  {i === 1 && (
                    <div style={{ position: 'absolute', left: 0, top: 0, width: 64, height: 64, transform: 'scale(1.1)', transformOrigin: 'center center' }}>
                      <img src={iconCase2c} alt="" style={{ position: 'absolute', left: -28, top: -5, width: 48, height: 45, zIndex: 2 }} />
                      <img src={iconCase2a} alt="" style={{ position: 'absolute', left: 4, top: 30, width: 48, height: 44, zIndex: 0 }} />
                      <img src={iconCase2b} alt="" style={{ position: 'absolute', left: 40, top: -8, width: 48, height: 53, zIndex: 1 }} />
                    </div>
                  )}
                  {i === 2 && (
                    <img src={iconCase3} alt="" style={{ position: 'absolute', left: 3.67, top: 3, width: 57, height: 60 }} />
                  )}
                </div>
                <div style={{ display: 'flex', flexDirection: 'column', gap: 8, position: 'relative', zIndex: 2 }}>
                  <p
                    style={{
                      color: '#FFFFFF',
                      fontSize: 18,
                      fontWeight: 500,
                      margin: 0,
                    }}
                  >
                    {item.title}
                  </p>
                  <p
                    style={{
                      color: 'rgba(255,255,255,0.5)',
                      fontSize: 14,
                      lineHeight: '20px',
                      margin: 0,
                    }}
                  >
                    {item.desc}
                  </p>
                </div>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
};

export default UseCasesSection;
