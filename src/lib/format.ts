const MINUTE = 60;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;

// Relative time in the platform UI locale. Intl falls back to the runtime
// (browser) locale for tags it does not support, so those use English instead.
function relativeFormatter(locale: string): Intl.RelativeTimeFormat {
  let tag = "en";
  try {
    if (Intl.RelativeTimeFormat.supportedLocalesOf([locale]).length > 0) tag = locale;
  } catch {
    // Invalid tag: keep English.
  }
  return new Intl.RelativeTimeFormat(tag, { numeric: "auto" });
}

/** Turn an ISO timestamp into a short relative label (e.g. "2 hours ago"). */
export function formatTimeAgo(iso: string, locale = "en"): string {
  const diff = Math.max(0, Math.round((Date.now() - new Date(iso).getTime()) / 1000));
  const rtf = relativeFormatter(locale);

  if (diff < MINUTE) return rtf.format(0, "second");
  if (diff < HOUR) return rtf.format(-Math.floor(diff / MINUTE), "minute");
  if (diff < DAY) return rtf.format(-Math.floor(diff / HOUR), "hour");
  return rtf.format(-Math.floor(diff / DAY), "day");
}

/** Compact number formatting (2400 -> "2.4K", 1200000 -> "1.2M"). */
export function formatCount(n: number): string {
  if (n < 1000) return String(n);
  if (n < 1_000_000) {
    const v = n / 1000;
    return `${Number.isInteger(v) ? v.toFixed(0) : v.toFixed(1)}K`;
  }
  const v = n / 1_000_000;
  return `${Number.isInteger(v) ? v.toFixed(0) : v.toFixed(1)}M`;
}
