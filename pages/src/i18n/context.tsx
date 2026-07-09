import React, { createContext, useContext, useState, useCallback } from 'react';
import { Language, TranslationKeys } from './types';
import { en } from './en';
import { zh } from './zh';
import { ja } from './ja';

const translations: Record<Language, TranslationKeys> = { en, zh, ja };

interface LanguageContextValue {
  language: Language;
  setLanguage: (lang: Language) => void;
  t: (key: string) => string;
}

const LanguageContext = createContext<LanguageContextValue | null>(null);

const STORAGE_KEY = 'ocr-lang';

const SUPPORTED_LANGUAGES: Language[] = ['en', 'zh', 'ja'];

function detectBrowserLanguage(): Language | null {
  try {
    for (const lang of navigator.languages ?? [navigator.language]) {
      const code = lang.toLowerCase().split('-')[0];
      if (SUPPORTED_LANGUAGES.includes(code as Language)) return code as Language;
    }
  } catch {}
  return null;
}

function getInitialLanguage(): Language {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored && SUPPORTED_LANGUAGES.includes(stored as Language)) return stored as Language;
  } catch {}
  return detectBrowserLanguage() ?? 'en';
}

export const LanguageProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [language, setLanguageState] = useState<Language>(getInitialLanguage);

  const setLanguage = useCallback((lang: Language) => {
    setLanguageState(lang);
    try { localStorage.setItem(STORAGE_KEY, lang); } catch {}
  }, []);

  const t = useCallback((key: string): string => {
    return translations[language][key] ?? key;
  }, [language]);

  return (
    <LanguageContext.Provider value={{ language, setLanguage, t }}>
      {children}
    </LanguageContext.Provider>
  );
};

export function useTranslation() {
  const ctx = useContext(LanguageContext);
  if (!ctx) throw new Error('useTranslation must be used within LanguageProvider');
  return ctx;
}
