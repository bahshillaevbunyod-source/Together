const MINUTE = 60;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;

/** Turn an ISO timestamp into a short relative label (e.g. "2 hours ago"). */
export function formatTimeAgo(iso: string): string {
  const diff = Math.round((Date.now() - new Date(iso).getTime()) / 1000);

  if (diff < MINUTE) return "just now";
  if (diff < HOUR) {
    const m = Math.floor(diff / MINUTE);
    return `${m} ${m === 1 ? "minute" : "minutes"} ago`;
  }
  if (diff < DAY) {
    const h = Math.floor(diff / HOUR);
    return `${h} ${h === 1 ? "hour" : "hours"} ago`;
  }
  const d = Math.floor(diff / DAY);
  return `${d} ${d === 1 ? "day" : "days"} ago`;
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
