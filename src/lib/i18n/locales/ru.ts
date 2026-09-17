import type { TranslationKey } from "./en";

const ru = {
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
} satisfies Record<TranslationKey, string>;

export default ru;
