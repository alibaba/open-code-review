import React, { useRef, useEffect, useState } from 'react';
import { useTranslation } from '../i18n';
import { useResponsive } from '../hooks/useResponsive';

// 从字符串中解析数字和前后缀
function parseStatValue(value: string): { prefix: string; number: number; suffix: string } {
  // 匹配 "> 30%" 等格式
  const match = value.match(/^([^\d]*?)(\d+)(.*)$/);
  if (match) {
    return { prefix: match[1], number: parseInt(match[2], 10), suffix: match[3] };
  }
  return { prefix: '', number: 0, suffix: value };
}

function useCountUp(target: number, duration: number = 1200, isActive: boolean) {
  const [current, setCurrent] = useState(0);
  const rafRef = useRef<number>(0);
  const startTimeRef = useRef<number>(0);

  useEffect(() => {
    if (!isActive) return;
    startTimeRef.current = performance.now();

    const animate = (now: number) => {
      const elapsed = now - startTimeRef.current;
      const progress = Math.min(elapsed / duration, 1);
      // easeOutExpo
      const eased = progress === 1 ? 1 : 1 - Math.pow(2, -10 * progress);
      setCurrent(Math.round(eased * target));
      if (progress < 1) {
        rafRef.current = requestAnimationFrame(animate);
      }
    };

    rafRef.current = requestAnimationFrame(animate);
    return () => cancelAnimationFrame(rafRef.current);
  }, [isActive, target, duration]);

  return current;
}

const CountUpValue: React.FC<{ value: string; isVisible: boolean }> = ({ value, isVisible }) => {
  const { prefix, number, suffix } = parseStatValue(value);
  const count = useCountUp(number, 2000, isVisible);

  if (number === 0) {
    // 无法解析数字，直接显示原文
    return <>{value}</>;
  }

  return <>{prefix}{isVisible ? count : 0}{suffix}</>;
};

const HighlightsSection: React.FC = () => {
  const { t } = useTranslation();
  const { isMobile, isTablet } = useResponsive();
  const sectionRef = useRef<HTMLDivElement>(null);
  const [isVisible, setIsVisible] = useState(false);

  useEffect(() => {
    const observer = new IntersectionObserver(
      ([entry]) => {
        if (entry.isIntersecting) {
          setIsVisible(true);
          observer.unobserve(entry.target);
        }
      },
      { threshold: 0.3 }
    );
    if (sectionRef.current) {
      observer.observe(sectionRef.current);
    }
    return () => observer.disconnect();
  }, []);

  const stats = [
    { value: t('highlights.stat1Value'), label: t('highlights.stat1Label'), caption: t('highlights.stat1Caption') },
    { value: t('highlights.stat2Value'), label: t('highlights.stat2Label'), caption: t('highlights.stat2Caption') },
    { value: t('highlights.stat3Value'), label: t('highlights.stat3Label'), caption: t('highlights.stat3Caption') },
    { value: t('highlights.stat4Value'), label: t('highlights.stat4Label'), caption: t('highlights.stat4Caption') },
    { value: t('highlights.stat5Value'), label: t('highlights.stat5Label'), caption: t('highlights.stat5Caption') },
  ];

  return (
    <section
      id="highlights"
      ref={sectionRef}
      style={{
        width: '100%',
        display: 'flex',
        justifyContent: 'center',
        padding: isMobile ? '60px 20px' : isTablet ? '80px 40px' : '80px 120px',
      }}
    >
      <div
        style={{
          width: '100%',
          maxWidth: 1200,
          display: 'flex',
          justifyContent: isMobile ? 'center' : 'space-between',
          alignItems: 'flex-start',
          flexWrap: 'wrap',
          gap: isMobile ? 32 : isTablet ? 24 : 0,
        }}
      >
        {stats.map((stat, i) => (
          <div
            key={i}
            style={{
              width: isMobile ? '45%' : 164,
              display: 'flex',
              flexDirection: 'column',
              alignItems: 'center',
            }}
          >
            <span
              style={{
                color: '#FFFFFF',
                fontSize: 40,
                fontWeight: 600,
                lineHeight: '48px',
              }}
            >
              <CountUpValue value={stat.value} isVisible={isVisible} />
            </span>
            <div
              style={{
                display: 'flex',
                flexDirection: 'column',
                alignItems: 'center',
                marginTop: 8,
              }}
            >
              <p
                style={{
                  color: 'rgba(255,255,255,0.8)',
                  fontSize: 12,
                  fontWeight: 500,
                  letterSpacing: '0.5px',
                  textTransform: 'uppercase',
                  margin: 0,
                }}
              >
                {stat.label}
              </p>
              <p
                style={{
                  color: 'rgba(255,255,255,0.4)',
                  fontSize: 12,
                  textAlign: 'center',
                  lineHeight: '16px',
                  marginTop: 4,
                  whiteSpace: 'nowrap',
                }}
              >
                {stat.caption}
              </p>
            </div>
          </div>
        ))}
      </div>
    </section>
  );
};

export default HighlightsSection;
