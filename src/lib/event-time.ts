/**
 * Timezone helpers for Events. Events store an absolute instant (startsAt)
 * plus the IANA timezone they are organised in; the form edits wall-clock
 * date/time in that timezone.
 */

/** The browser's IANA timezone, or UTC when unavailable. */
export function browserTimeZone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
  } catch {
    return "UTC";
  }
}

/** Every IANA timezone the browser knows, always including UTC and `extra`. */
export function timeZoneOptions(extra: string[] = []): string[] {
  let zones: string[] = [];
  try {
    const fn = (Intl as unknown as { supportedValuesOf?: (k: string) => string[] }).supportedValuesOf;
    zones = fn ? fn("timeZone") : [];
  } catch {
    zones = [];
  }
  const set = new Set([...zones, "UTC", browserTimeZone(), ...extra.filter(Boolean)]);
  return [...set].sort();
}

/** True when the browser recognises `tz` as a timezone. */
export function isValidTimeZone(tz: string): boolean {
  if (!tz) return false;
  try {
    new Intl.DateTimeFormat("en-US", { timeZone: tz });
    return true;
  } catch {
    return false;
  }
}

interface WallParts {
  year: number;
  month: number;
  day: number;
  hour: number;
  minute: number;
}

function wallParts(ms: number, tz: string): WallParts {
  const parts = new Intl.DateTimeFormat("en-US", {
    timeZone: tz,
    hourCycle: "h23",
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).formatToParts(new Date(ms));
  const get = (type: string) => Number(parts.find((p) => p.type === type)?.value ?? "0");
  return { year: get("year"), month: get("month"), day: get("day"), hour: get("hour") % 24, minute: get("minute") };
}

/** Offset of `tz` from UTC at instant `ms`, in milliseconds. */
function offsetAt(ms: number, tz: string): number {
  const p = wallParts(ms, tz);
  const asUtc = Date.UTC(p.year, p.month - 1, p.day, p.hour, p.minute);
  return asUtc - Math.floor(ms / 60000) * 60000;
}

/**
 * Convert a wall-clock date ("YYYY-MM-DD") and time ("HH:MM") in `tz` into an
 * RFC3339 UTC string. Returns null for malformed input.
 */
export function zonedToUtcISO(date: string, time: string, tz: string): string | null {
  const dm = /^(\d{4})-(\d{2})-(\d{2})$/.exec(date);
  const tm = /^(\d{2}):(\d{2})$/.exec(time);
  if (!dm || !tm || !isValidTimeZone(tz)) return null;
  const guess = Date.UTC(+dm[1], +dm[2] - 1, +dm[3], +tm[1], +tm[2]);
  if (Number.isNaN(guess)) return null;
  let ms = guess - offsetAt(guess, tz);
  // Re-check once so instants near a DST transition land on the right offset.
  const second = guess - offsetAt(ms, tz);
  if (second !== ms) ms = second;
  return new Date(ms).toISOString().replace(/\.\d{3}Z$/, "Z");
}

/** Split an instant into wall-clock "YYYY-MM-DD" / "HH:MM" in `tz`. */
export function utcToZoned(iso: string, tz: string): { date: string; time: string } {
  const p = wallParts(new Date(iso).getTime(), isValidTimeZone(tz) ? tz : "UTC");
  const pad = (n: number) => String(n).padStart(2, "0");
  return { date: `${p.year}-${pad(p.month)}-${pad(p.day)}`, time: `${pad(p.hour)}:${pad(p.minute)}` };
}

/** Intl formatting that tolerates locale codes the runtime does not know. */
export function formatInZone(iso: string, locale: string, tz: string, opts: Intl.DateTimeFormatOptions): string {
  const zone = isValidTimeZone(tz) ? tz : "UTC";
  const date = new Date(iso);
  try {
    return new Intl.DateTimeFormat(locale, { ...opts, timeZone: zone }).format(date);
  } catch {
    return new Intl.DateTimeFormat("en", { ...opts, timeZone: zone }).format(date);
  }
}

/** Same calendar day in `tz`? */
export function sameDayInZone(a: string, b: string, tz: string): boolean {
  return utcToZoned(a, tz).date === utcToZoned(b, tz).date;
}

/** UTF-8 byte length (the backend limits lengths in bytes). */
export function byteLength(s: string): number {
  return new TextEncoder().encode(s).length;
}

/** http(s) URL with a host, the only kind the backend accepts or we link to. */
export function isSafeHttpUrl(raw: string): boolean {
  try {
    const u = new URL(raw);
    return (u.protocol === "http:" || u.protocol === "https:") && Boolean(u.host);
  } catch {
    return false;
  }
}
