"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { Globe2, Users } from "lucide-react";

import { getWorldCountries, type WorldCountry } from "@/lib/world/world-api";
import { useLanguage, type TranslationKey } from "@/lib/language-context";
import { WorldMap } from "@/components/world/WorldMap";
import { WorldCountryList } from "@/components/world/WorldCountryList";
import { WorldPeople } from "@/components/world/WorldPeople";
import { WorldEvents, type EventScope } from "@/components/world/WorldEvents";

type Tab = "people" | "events";
type Status = "loading" | "ready" | "error";

const TABS: { value: Tab; labelKey: TranslationKey; icon: typeof Users }[] = [
  { value: "people", labelKey: "world.tabPeople", icon: Users },
  { value: "events", labelKey: "world.tabEvents", icon: Globe2 },
];

/**
 * World V1: one discovery hub. The map shows real, coarse, country-level
 * counts of people the viewer can discover; selecting a country lists those
 * people. Events are searched textually (they have no coordinates), with a
 * separate Online filter — no event is ever pinned on the map.
 */
export default function WorldPage() {
  const { t } = useLanguage();
  const [tab, setTab] = useState<Tab>("people");
  const [countries, setCountries] = useState<WorldCountry[]>([]);
  const [status, setStatus] = useState<Status>("loading");
  const [selected, setSelected] = useState<string | null>(null);
  const [eventQuery, setEventQuery] = useState("");
  const [eventScope, setEventScope] = useState<EventScope>("all");

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

  const select = useCallback((code: string | null) => {
    setSelected(code);
    setTab("people");
  }, []);

  const totalPeople = useMemo(() => countries.reduce((sum, c) => sum + c.people, 0), [countries]);

  return (
    <div className="-mx-4 -mt-4 flex flex-col sm:mx-0 sm:mt-0 sm:gap-4">
      <header className="hidden px-4 sm:block sm:px-0">
        <h1 className="text-xl font-semibold text-foreground">{t("navigation.world")}</h1>
        <p className="mt-0.5 text-sm text-muted">{t("world.pageDescription")}</p>
      </header>

      <div className="flex flex-col overflow-hidden sm:rounded-3xl sm:border sm:border-border sm:bg-surface sm:shadow-sm lg:grid lg:h-[calc(100vh-12rem)] lg:min-h-[560px] lg:grid-cols-[minmax(0,1fr)_380px]">
        {/* Map */}
        <div className="relative h-[42vh] min-h-[260px] lg:h-full">
          <WorldMap countries={countries} selected={selected} onSelect={select} />
          <div className="pointer-events-none absolute left-3 top-3 max-w-[70%]">
            <p className="pointer-events-auto rounded-full bg-surface/95 px-3 py-1.5 text-xs font-medium text-foreground shadow-sm">
              {status === "ready"
                ? t("world.mapSummary", { people: totalPeople, countries: countries.length })
                : t("world.mapLoading")}
            </p>
          </div>
        </div>

        {/* Discovery panel: a bottom sheet on mobile, a side panel on desktop */}
        <section className="relative z-10 -mt-5 flex min-h-[50vh] flex-col rounded-t-3xl border-t border-border bg-surface shadow-[0_-8px_24px_rgba(15,23,42,0.08)] sm:mt-0 sm:rounded-none sm:border-t-0 sm:shadow-none lg:min-h-0 lg:border-l">
          <div className="mx-auto mt-2 h-1 w-10 rounded-full bg-border sm:hidden" aria-hidden />
          <div className="px-4 pb-3 pt-3">
            <h1 className="mb-3 text-lg font-semibold text-foreground sm:hidden">{t("navigation.world")}</h1>
            <div role="tablist" aria-label={t("navigation.world")} className="flex rounded-full border border-border bg-background p-1">
              {TABS.map(({ value, labelKey, icon: Icon }) => {
                const active = tab === value;
                return (
                  <button
                    key={value}
                    type="button"
                    role="tab"
                    aria-selected={active}
                    onClick={() => setTab(value)}
                    className={`flex flex-1 items-center justify-center gap-1.5 rounded-full py-1.5 text-sm font-medium transition-colors ${
                      active ? "bg-primary text-white shadow-sm" : "text-muted hover:text-foreground"
                    }`}
                  >
                    <Icon className="h-4 w-4" aria-hidden />
                    {t(labelKey)}
                  </button>
                );
              })}
            </div>
          </div>

          <div className="min-h-0 flex-1 overflow-y-auto px-4 pb-4">
            {tab === "people" ? (
              selected ? (
                <WorldPeople
                  key={selected}
                  country={selected}
                  count={countries.find((c) => c.countryCode === selected)?.people ?? 0}
                  onBack={() => setSelected(null)}
                  onEvents={(query) => {
                    setEventQuery(query);
                    setEventScope("in_person");
                    setTab("events");
                  }}
                />
              ) : (
                <WorldCountryList countries={countries} status={status} onRetry={() => load()} onSelect={select} />
              )
            ) : (
              <WorldEvents query={eventQuery} onQueryChange={setEventQuery} scope={eventScope} onScopeChange={setEventScope} />
            )}
          </div>
        </section>
      </div>
    </div>
  );
}
