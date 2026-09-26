/**
 * Date formatting in Together's platform UI locale (never the browser's).
 *
 * Intl silently falls back to the runtime default locale for tags it does not
 * support, which would show e.g. a Russian date inside an English UI. So the
 * platform locale is used only when Intl actually supports it; otherwise the
 * canonical English fallback is used, matching the i18n dictionary fallback.
 */
function supportedDateLocale(locale: string): string {
  try {
    if (Intl.DateTimeFormat.supportedLocalesOf([locale]).length > 0) return locale;
  } catch {
    // Invalid tag: fall back to English.
  }
  return "en";
}

/** "Member since"-style month + year, e.g. "January 2025". */
export function formatMonthYear(iso: string, locale: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return new Intl.DateTimeFormat(supportedDateLocale(locale), {
    month: "long",
    year: "numeric",
  }).format(d);
}
