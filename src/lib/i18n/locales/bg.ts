import type { TranslationKey } from "./en";

const d_bg = {
  "navigation.home": "Начало",
  "navigation.discover": "Откриване",
  "navigation.messages": "Съобщения",
  "navigation.calls": "Обаждания",
  "navigation.groups": "Групи",
  "navigation.explore": "Разглеждане",
  "navigation.events": "Събития",
  "navigation.bookmarks": "Отметки",
  "navigation.profile": "Профил",
  "navigation.settings": "Настройки",
  "navigation.primary": "Основна навигация",
  "header.tagline": "Различни хора. Един свят.",
  "search.ariaLabel": "Търсене на хора",
  "search.placeholder": "Търсене на хора…",
  "search.shortcut": "Ctrl K",
  "search.searching": "Търсене…",
  "search.error": "Търсенето не бе успешно.",
  "search.tryAgain": "Опитайте отново",
  "search.emptyPrefix": "Няма намерени хора за „",
  "search.emptySuffix": "“.",
  "user.guest": "Гост",
  "user.logout": "Изход",
  "user.loggingOut": "Излизане…",
  "user.logoutError": "Излизането не бе успешно. Опитайте отново.",
} satisfies Record<TranslationKey, string>;

export default d_bg;
