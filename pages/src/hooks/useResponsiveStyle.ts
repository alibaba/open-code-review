import { useResponsive } from './useResponsive';

/**
 * Returns responsive section title styles based on current breakpoint.
 *
 * | Breakpoint         | fontSize | lineHeight |
 * |--------------------|----------|------------|
 * | Mobile (<768px)    | 28px     | 34px       |
 * | Tablet (768-1024px)| 36px     | 42px       |
 * | Desktop (>1024px)  | 48px     | 52px       |
 */
export function useSectionTitleStyle() {
  const { isMobile, isTablet } = useResponsive();

  let fontSize: number;
  let lineHeight: string;

  if (isMobile) {
    fontSize = 28;
    lineHeight = '34px';
  } else if (isTablet) {
    fontSize = 36;
    lineHeight = '42px';
  } else {
    fontSize = 48;
    lineHeight = '52px';
  }

  return { fontSize, lineHeight };
}
