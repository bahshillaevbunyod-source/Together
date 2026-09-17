import type { TranslationKey } from "./en";

const d_is = {
  "navigation.home": "Heim",
  "navigation.discover": "Uppgötva",
  "navigation.messages": "Skilaboð",
  "navigation.calls": "Símtöl",
  "navigation.groups": "Hópar",
  "navigation.explore": "Kanna",
  "navigation.events": "Viðburðir",
  "navigation.bookmarks": "Bókamerki",
  "navigation.profile": "Prófíll",
  "navigation.settings": "Stillingar",
  "navigation.primary": "Aðalflakk",
  "header.tagline": "Ólíkt fólk. Einn heimur.",
  "search.ariaLabel": "Leita að fólki",
  "search.placeholder": "Leita að fólki…",
  "search.shortcut": "Ctrl K",
  "search.searching": "Leita…",
  "search.error": "Ekki tókst að leita.",
  "search.tryAgain": "Reyna aftur",
  "search.emptyPrefix": "Ekkert fólk fannst fyrir „",
  "search.emptySuffix": "“.",
  "user.guest": "Gestur",
  "user.logout": "Skrá út",
  "user.loggingOut": "Skrái út…",
  "user.logoutError": "Ekki tókst að skrá út. Reyndu aftur.",
} satisfies Record<TranslationKey, string>;

export default d_is;
