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

const messages = {
  en: {
    "navigation.home": "Home",
    "navigation.discover": "Discover",
    "navigation.messages": "Messages",
    "navigation.calls": "Calls",
    "navigation.groups": "Groups",
    "navigation.explore": "Explore",
    "navigation.events": "Events",
    "navigation.bookmarks": "Bookmarks",
    "navigation.profile": "Profile",
    "navigation.settings": "Settings",
    "navigation.primary": "Primary navigation",
  },
  ru: {
    "navigation.home": "Главная",
    "navigation.discover": "Интересное",
    "navigation.messages": "Сообщения",
    "navigation.calls": "Звонки",
    "navigation.groups": "Группы",
    "navigation.explore": "Обзор",
    "navigation.events": "События",
    "navigation.bookmarks": "Закладки",
    "navigation.profile": "Профиль",
    "navigation.settings": "Настройки",
    "navigation.primary": "Основная навигация",
  },
} as const;

export type TranslationKey = keyof (typeof messages)["en"];

interface LanguageContextValue {
  locale: string;
  t: (key: TranslationKey) => string;
  setLanguage: (language: string | null) => void;
}

const LanguageContext = createContext<LanguageContextValue | undefined>(undefined);

function normalizeLocale(preferredLanguage: string | null): string {
  const normalized = preferredLanguage?.trim().toLowerCase();
  return normalized || "en";
}

export function LanguageProvider({
  preferredLanguage,
  children,
}: {
  preferredLanguage: string | null;
  children: ReactNode;
}) {
  const [locale, setLocale] = useState(() => normalizeLocale(preferredLanguage));

  useEffect(() => {
    const nextLocale = normalizeLocale(preferredLanguage);
    setLocale((currentLocale) =>
      currentLocale === nextLocale ? currentLocale : nextLocale,
    );
  }, [preferredLanguage]);

  const setLanguage = useCallback((language: string | null) => {
    setLocale(normalizeLocale(language));
  }, []);

  const value = useMemo<LanguageContextValue>(() => {
    const dictionary = messages[locale as keyof typeof messages] ?? messages.en;

    return {
      locale,
      t: (key) => dictionary[key] ?? messages.en[key],
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
