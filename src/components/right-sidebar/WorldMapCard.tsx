"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { ArrowRight, Globe2 } from "lucide-react";

import { countryName, flagEmoji, getWorldCountries, type WorldCountry } from "@/lib/world/world-api";
import { useLanguage } from "@/lib/language-context";

const TOP = 4;

/**
 * Right-rail entry point into World. Shows only real, coarse data: how many
 * people the viewer can discover and the countries with the most of them.
 */
export function WorldMapCard() {
  const { t, locale } = useLanguage();
  const [countries, setCountries] = useState<WorldCountry[]>([]);
  const [status, setStatus] = useState<"loading" | "ready" | "error">("loading");

  const load = useCallback((signal?: AbortSignal) => {
    setStatus("loading");
    getWorldCountries(signal)
      .then((items) => {
        setCountries(items);
        setStatus("ready");
      })
      .catch((err) => {
        if (err instanceof DOMException && err.name === "AbortError") return;
        setStatus("error");
      });
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    load(controller.signal);
    return () => controller.abort();
  }, [load]);

  const total = countries.reduce((sum, c) => sum + c.people, 0);

  return (
    <section className="mr-1 mt-2 rounded-2xl border border-border bg-surface p-5 shadow-sm">
      <div className="flex items-center gap-2">
        <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-xl bg-primary-soft text-primary">
          <Globe2 className="h-4 w-4" aria-hidden />
        </span>
        <div className="min-w-0">
          <h2 className="text-sm font-semibold text-foreground">{t("discover.exploreWorld")}</h2>
          {status === "ready" && countries.length > 0 ? (
            <p className="text-xs text-muted">{t("world.mapSummary", { people: total, countries: countries.length })}</p>
          ) : null}
        </div>
      </div>

      {status === "loading" ? (
        <ul className="mt-4 flex flex-col gap-2" aria-busy="true">
          {[0, 1, 2].map((i) => (
            <li key={i} className="h-8 rounded-lg bg-background motion-safe:animate-pulse" />
          ))}
        </ul>
      ) : null}

      {status === "error" ? (
        <div className="mt-4 text-center">
          <p className="text-xs text-muted">{t("world.countriesError")}</p>
          <button type="button" onClick={() => load()} className="mt-1 text-xs text-primary hover:underline">
            {t("search.tryAgain")}
          </button>
        </div>
      ) : null}

      {status === "ready" && countries.length === 0 ? <p className="mt-4 text-xs text-muted">{t("world.emptyBody")}</p> : null}

      {status === "ready" && countries.length > 0 ? (
        <ul className="mt-4 flex flex-col gap-1.5">
          {countries.slice(0, TOP).map((c) => (
            <li key={c.countryCode} className="flex items-center gap-2 text-sm">
              <span aria-hidden>{flagEmoji(c.countryCode)}</span>
              <span className="min-w-0 flex-1 truncate text-foreground">{countryName(c.countryCode, locale)}</span>
              <span className="shrink-0 text-xs text-muted">{t("world.peopleCount", { count: c.people })}</span>
            </li>
          ))}
        </ul>
      ) : null}

      <Link
        href="/world"
        className="mt-4 inline-flex items-center gap-1.5 rounded-full border border-border bg-surface px-3.5 py-2 text-xs font-medium text-foreground shadow-sm transition-colors hover:bg-background"
      >
        {t("world.exploreMap")}
        <ArrowRight className="h-3.5 w-3.5 rtl:rotate-180" />
      </Link>
    </section>
  );
}
