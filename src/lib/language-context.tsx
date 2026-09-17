"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import {
  getDirection,
  locales,
  resolveLocale,
  type TranslationKey,
} from "@/lib/i18n";

export type { TranslationKey };

interface LanguageContextValue {
  locale: string;
  t: (key: TranslationKey) => string;
  setLanguage: (language: string | null) => void;
}

const LanguageContext = createContext<LanguageContextValue | undefined>(undefined);

export function LanguageProvider({
  platformLanguage,
  children,
}: {
  platformLanguage: string | null;
  children: ReactNode;
}) {
  const [locale, setLocale] = useState(() => resolveLocale(platformLanguage));

  useEffect(() => {
    const nextLocale = resolveLocale(platformLanguage);
    setLocale((currentLocale) =>
      currentLocale === nextLocale ? currentLocale : nextLocale,
    );
  }, [platformLanguage]);

  useEffect(() => {
    document.documentElement.dir = getDirection(locale);
  }, [locale]);

  const setLanguage = useCallback((language: string | null) => {
    setLocale(resolveLocale(language));
  }, []);

  const value = useMemo<LanguageContextValue>(() => {
    const dictionary = locales[locale];

    return {
      locale,
      t: (key) => dictionary[key] ?? locales.en[key],
      setLanguage,
    };
  }, [locale, setLanguage]);

  return <LanguageContext.Provider value={value}>{children}</LanguageContext.Provider>;
}

export function useLanguage(): LanguageContextValue {
  const context = useContext(LanguageContext);
  if (context === undefined) {
    throw new Error("useLanguage must be used within a LanguageProvider");
  }
  return context;
}
