import type { TranslationKey } from "./en";

const de = {
  "navigation.home": "Startseite",
  "navigation.discover": "Entdecken",
  "navigation.messages": "Nachrichten",
  "navigation.calls": "Anrufe",
  "navigation.groups": "Gruppen",
  "navigation.explore": "Erkunden",
  "navigation.events": "Veranstaltungen",
  "navigation.bookmarks": "Lesezeichen",
  "navigation.profile": "Profil",
  "navigation.settings": "Einstellungen",
  "navigation.primary": "Hauptnavigation",
  "header.tagline": "Verschiedene Menschen. Eine Welt.",
  "search.ariaLabel": "Personen suchen",
  "search.placeholder": "Personen suchen…",
  "search.shortcut": "Strg K",
  "search.searching": "Suche läuft…",
  "search.error": "Suche konnte nicht ausgeführt werden.",
  "search.tryAgain": "Erneut versuchen",
  "search.emptyPrefix": "Keine Personen für „",
  "search.emptySuffix": "“ gefunden.",
  "user.guest": "Gast",
  "user.logout": "Abmelden",
  "user.loggingOut": "Abmeldung…",
  "user.logoutError": "Abmelden nicht möglich. Erneut versuchen.",
} satisfies Record<TranslationKey, string>;

export default de;
