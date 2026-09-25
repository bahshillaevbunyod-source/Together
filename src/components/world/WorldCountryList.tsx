"use client";

import { useMemo, useState } from "react";
import { ChevronRight, Globe2, Search, X } from "lucide-react";

import { countryName, englishCountryName, flagEmoji, type WorldCountry } from "@/lib/world/world-api";
import { useLanguage } from "@/lib/language-context";

/** Real countries where the viewer has people to discover, searchable by name. */
export function WorldCountryList({
  countries,
  status,
  onRetry,
  onSelect,
}: {
  countries: WorldCountry[];
  status: "loading" | "ready" | "error";
  onRetry: () => void;
  onSelect: (code: string) => void;
}) {
  const { t, locale } = useLanguage();
  const [query, setQuery] = useState("");

  const named = useMemo(
    () => countries.map((c) => ({ ...c, name: countryName(c.countryCode, locale), en: englishCountryName(c.countryCode) })),
    [countries, locale],
  );
  const q = query.trim().toLocaleLowerCase();
  const visible = q
    ? named.filter((c) => c.name.toLocaleLowerCase().includes(q) || c.en.toLowerCase().includes(q) || c.countryCode.toLowerCase() === q)
    : named;

  return (
    <div className="flex flex-col gap-3">
      <label className="relative block">
        <span className="sr-only">{t("world.searchCountries")}</span>
        <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-soft" aria-hidden />
        <input
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder={t("world.searchCountries")}
          className="h-10 w-full rounded-full border border-border bg-background pl-9 pr-9 text-sm text-foreground placeholder:text-muted-soft focus:border-primary focus:outline-none focus:ring-2 focus:ring-primary/20"
        />
        {query ? (
          <button
            type="button"
            onClick={() => setQuery("")}
            aria-label={t("world.clearSearch")}
            className="absolute right-2 top-1/2 flex h-7 w-7 -translate-y-1/2 items-center justify-center rounded-full text-muted hover:bg-surface hover:text-foreground"
          >
            <X className="h-4 w-4" />
          </button>
        ) : null}
      </label>

      {status === "loading" ? (
        <ul className="flex flex-col gap-2" aria-busy="true" aria-label={t("world.loadingCountries")}>
          {[0, 1, 2, 3].map((i) => (
            <li key={i} className="h-12 rounded-2xl bg-background motion-safe:animate-pulse" />
          ))}
        </ul>
      ) : null}

      {status === "error" ? (
        <div className="rounded-2xl bg-background px-4 py-8 text-center">
          <p className="text-sm text-muted">{t("world.countriesError")}</p>
          <button type="button" onClick={onRetry} className="mt-2 text-sm text-primary hover:underline">
            {t("search.tryAgain")}
          </button>
        </div>
      ) : null}

      {status === "ready" && countries.length === 0 ? (
        <div className="flex flex-col items-center rounded-2xl bg-background px-6 py-10 text-center">
          <span className="flex h-12 w-12 items-center justify-center rounded-2xl bg-primary-soft text-primary">
            <Globe2 className="h-6 w-6" aria-hidden />
          </span>
          <p className="mt-3 text-sm font-medium text-foreground">{t("world.emptyTitle")}</p>
          <p className="mt-1 max-w-xs text-xs text-muted">{t("world.emptyBody")}</p>
        </div>
      ) : null}

      {status === "ready" && countries.length > 0 && visible.length === 0 ? (
        <p className="rounded-2xl bg-background px-4 py-8 text-center text-sm text-muted">{t("world.noCountryMatch", { query: query.trim() })}</p>
      ) : null}

      {status === "ready" && visible.length > 0 ? (
        <>
          <p className="px-1 text-xs font-medium uppercase tracking-wide text-muted-soft">{t("world.countriesHeading")}</p>
          <ul className="flex flex-col gap-1">
            {visible.map((c) => (
              <li key={c.countryCode}>
                <button
                  type="button"
                  onClick={() => onSelect(c.countryCode)}
                  className="flex w-full items-center gap-3 rounded-2xl px-3 py-2.5 text-left transition-colors hover:bg-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/40"
                >
                  <span className="text-xl leading-none" aria-hidden>
                    {flagEmoji(c.countryCode)}
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-sm font-medium text-foreground">{c.name}</span>
                    <span className="block text-xs text-muted">{t("world.peopleCount", { count: c.people })}</span>
                  </span>
                  <ChevronRight className="h-4 w-4 shrink-0 text-muted-soft rtl:rotate-180" aria-hidden />
                </button>
              </li>
            ))}
          </ul>
        </>
      ) : null}
    </div>
  );
}
