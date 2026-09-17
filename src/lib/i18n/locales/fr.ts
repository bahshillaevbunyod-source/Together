import type { TranslationKey } from "./en";

const fr = {
  "navigation.home": "Accueil",
  "navigation.discover": "Découvrir",
  "navigation.messages": "Messages",
  "navigation.calls": "Appels",
  "navigation.groups": "Groupes",
  "navigation.explore": "Explorer",
  "navigation.events": "Événements",
  "navigation.bookmarks": "Signets",
  "navigation.profile": "Profil",
  "navigation.settings": "Paramètres",
  "navigation.primary": "Navigation principale",
  "header.tagline": "Des personnes différentes. Un seul monde.",
  "search.ariaLabel": "Rechercher des personnes",
  "search.placeholder": "Rechercher des personnes…",
  "search.shortcut": "Ctrl K",
  "search.searching": "Recherche en cours…",
  "search.error": "Impossible d’effectuer la recherche.",
  "search.tryAgain": "Réessayer",
  "search.emptyPrefix": "Aucune personne trouvée pour «",
  "search.emptySuffix": "».",
  "user.guest": "Invité",
  "user.logout": "Se déconnecter",
  "user.loggingOut": "Déconnexion…",
  "user.logoutError": "Impossible de se déconnecter. Réessayez.",
} satisfies Record<TranslationKey, string>;

export default fr;
