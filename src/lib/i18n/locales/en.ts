export const en = {
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
} as const;

export type TranslationKey = keyof typeof en;

export default en;
