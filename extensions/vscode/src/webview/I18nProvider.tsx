import { createContext } from 'preact';
import { useContext } from 'preact/hooks';
import { t, resolveLocale, SupportedLocale } from '../shared/i18n';

export const I18nContext = createContext<SupportedLocale>('en');

export function useT(): (key: string) => string {
  const locale = useContext(I18nContext);
  return (key: string) => t(locale, key);
}

export { resolveLocale, t };
export type { SupportedLocale };
