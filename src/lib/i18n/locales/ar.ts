import type { TranslationKey } from "./en";

const ar = {
  "navigation.home": "الرئيسية",
  "navigation.discover": "اكتشف",
  "navigation.messages": "الرسائل",
  "navigation.calls": "المكالمات",
  "navigation.groups": "المجموعات",
  "navigation.explore": "استكشاف",
  "navigation.events": "الفعاليات",
  "navigation.bookmarks": "المحفوظات",
  "navigation.profile": "الملف الشخصي",
  "navigation.settings": "الإعدادات",
  "navigation.primary": "التنقل الرئيسي",
  "header.tagline": "أشخاص مختلفون. عالم واحد.",
  "search.ariaLabel": "البحث عن أشخاص",
  "search.placeholder": "البحث عن أشخاص…",
  "search.shortcut": "Ctrl K",
  "search.searching": "جارٍ البحث…",
  "search.error": "تعذّر تنفيذ البحث.",
  "search.tryAgain": "حاول مجددًا",
  "search.emptyPrefix": "لم يتم العثور على أشخاص لـ «",
  "search.emptySuffix": "».",
  "user.guest": "زائر",
  "user.logout": "تسجيل الخروج",
  "user.loggingOut": "جارٍ تسجيل الخروج…",
  "user.logoutError": "تعذّر تسجيل الخروج. حاول مجددًا.",
} satisfies Record<TranslationKey, string>;

export default ar;
