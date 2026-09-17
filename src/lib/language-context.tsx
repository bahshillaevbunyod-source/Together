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
    "header.tagline": "Different people. One world.",
    "search.ariaLabel": "Search people",
    "search.placeholder": "Search people…",
    "search.shortcut": "Ctrl K",
    "search.searching": "Searching…",
    "search.error": "Couldn’t run search.",
    "search.tryAgain": "Try again",
    "search.emptyPrefix": "No people found for “",
    "search.emptySuffix": "”.",
    "user.guest": "Guest",
    "user.logout": "Log out",
    "user.loggingOut": "Logging out…",
    "user.logoutError": "Couldn’t log out. Try again.",
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
    "header.tagline": "Разные люди. Один мир.",
    "search.ariaLabel": "Поиск людей",
    "search.placeholder": "Поиск людей…",
    "search.shortcut": "Ctrl K",
    "search.searching": "Поиск…",
    "search.error": "Не удалось выполнить поиск.",
    "search.tryAgain": "Повторить",
    "search.emptyPrefix": "По запросу «",
    "search.emptySuffix": "» ничего не найдено.",
    "user.guest": "Гость",
    "user.logout": "Выйти",
    "user.loggingOut": "Выход…",
    "user.logoutError": "Не удалось выйти. Попробуйте ещё раз.",
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
