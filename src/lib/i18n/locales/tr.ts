import type { TranslationKey } from "./en";

const tr = {
  "navigation.home": "Ana sayfa",
  "navigation.discover": "Keşfet",
  "navigation.messages": "Mesajlar",
  "navigation.calls": "Aramalar",
  "navigation.groups": "Gruplar",
  "navigation.explore": "Göz at",
  "navigation.events": "Etkinlikler",
  "navigation.bookmarks": "Yer imleri",
  "navigation.profile": "Profil",
  "navigation.settings": "Ayarlar",
  "navigation.primary": "Ana navigasyon",
  "header.tagline": "Farklı insanlar. Tek dünya.",
  "search.ariaLabel": "Kişileri ara",
  "search.placeholder": "Kişileri ara…",
  "search.shortcut": "Ctrl K",
  "search.searching": "Aranıyor…",
  "search.error": "Arama yapılamadı.",
  "search.tryAgain": "Tekrar dene",
  "search.emptyPrefix": "“",
  "search.emptySuffix": "” için kişi bulunamadı.",
  "user.guest": "Misafir",
  "user.logout": "Çıkış yap",
  "user.loggingOut": "Çıkış yapılıyor…",
  "user.logoutError": "Çıkış yapılamadı. Tekrar deneyin.",
} satisfies Record<TranslationKey, string>;

export default tr;
