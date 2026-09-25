/**
 * World V1 API layer. People discovery reuses /users/discover (world mode)
 * and events reuse /events; this module adds only the country aggregate.
 */

import { apiFetch } from "@/lib/api";

/** Discoverable people per ISO country code (same rules as /users/discover). */
export interface WorldCountry {
  countryCode: string;
  people: number;
}

export function getWorldCountries(signal?: AbortSignal): Promise<WorldCountry[]> {
  return apiFetch<{ items: WorldCountry[] }>("/api/v1/world/countries", { signal }).then((r) => r.items ?? []);
}

/** Localized country name, falling back to the code. */
export function countryName(code: string, locale: string): string {
  for (const loc of [locale, "en"]) {
    try {
      const name = new Intl.DisplayNames([loc], { type: "region" }).of(code);
      if (name && name !== code) return name;
    } catch {
      // Unknown locale tag: try the next one.
    }
  }
  return code;
}

/** English country name (event locations are free text, most often Latin). */
export function englishCountryName(code: string): string {
  return countryName(code, "en");
}

/** Localized language name for a BCP-47 code, falling back to the code. */
export function languageName(code: string, locale: string): string {
  for (const loc of [locale, "en"]) {
    try {
      const name = new Intl.DisplayNames([loc], { type: "language" }).of(code);
      if (name && name !== code) return name;
    } catch {
      // Unknown tag: try the next one.
    }
  }
  return code;
}

/** Regional-indicator flag emoji for a two-letter code. */
export function flagEmoji(code: string): string {
  if (!/^[A-Z]{2}$/.test(code)) return "";
  return String.fromCodePoint(...[...code].map((c) => 0x1f1e6 + c.charCodeAt(0) - 65));
}
