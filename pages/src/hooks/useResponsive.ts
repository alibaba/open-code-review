import { useState, useEffect } from 'react';

export interface Breakpoints {
  isMobile: boolean;   // < 768px
  isTablet: boolean;   // 768px ~ 1024px
  isDesktop: boolean;  // > 1024px
}

export function useResponsive(): Breakpoints {
  const [bp, setBp] = useState<Breakpoints>(() => getBreakpoints());

  useEffect(() => {
    const handleResize = () => setBp(getBreakpoints());
    window.addEventListener('resize', handleResize);
    return () => window.removeEventListener('resize', handleResize);
  }, []);

  return bp;
}

function getBreakpoints(): Breakpoints {
  const w = window.innerWidth;
  return {
    isMobile: w < 768,
    isTablet: w >= 768 && w <= 1024,
    isDesktop: w > 1024,
  };
}
