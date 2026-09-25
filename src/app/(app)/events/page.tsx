"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { CalendarDays, Plus } from "lucide-react";

import { getEvents, type ApiEvent, type EventListFilter } from "@/lib/events-api";
import { useLanguage, type TranslationKey } from "@/lib/language-context";
import { EventCard } from "@/components/events/EventCard";
import { EventForm } from "@/components/events/EventForm";

type Status = "loading" | "ready" | "error";
type Tab = "upcoming" | "created" | "going" | "interested";

const FILTERS: Record<Tab, EventListFilter> = {
  upcoming: {},
  created: { mine: "created" },
  going: { rsvp: "going" },
  interested: { rsvp: "interested" },
};

const EMPTY: Record<Tab, TranslationKey> = {
  upcoming: "events.emptyUpcoming",
  created: "events.emptyCreated",
  going: "events.emptyGoing",
  interested: "events.emptyInterested",
};

const MINE_TABS: { tab: Tab; labelKey: TranslationKey }[] = [
  { tab: "created", labelKey: "events.filterCreated" },
  { tab: "going", labelKey: "events.going" },
  { tab: "interested", labelKey: "events.interested" },
];

/**
 * Events hub. Every list is the backend's upcoming-only feed (starts_at >= now)
 * filtered by mine=created or rsvp=going|interested.
 */
export default function EventsPage() {
  const { t } = useLanguage();
  const router = useRouter();
  const [tab, setTab] = useState<Tab>("upcoming");
  const [items, setItems] = useState<ApiEvent[]>([]);
  const [status, setStatus] = useState<Status>("loading");
  const [nextCursor, setNextCursor] = useState("");
  const [loadingMore, setLoadingMore] = useState(false);
  const [moreError, setMoreError] = useState(false);
  const [creating, setCreating] = useState(false);
  const loadingMoreRef = useRef(false);
  const tabRef = useRef(tab);
  useEffect(() => {
    tabRef.current = tab;
  }, [tab]);

  const load = useCallback((which: Tab, signal?: AbortSignal) => {
    setStatus("loading");
    setItems([]);
    setNextCursor("");
    setMoreError(false);
    getEvents(FILTERS[which], signal)
      .then((page) => {
        setItems(page.items);
        setNextCursor(page.nextCursor);
        setStatus("ready");
      })
      .catch((err) => {
        if (err instanceof DOMException && err.name === "AbortError") return;
        setStatus("error");
      });
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    load(tab, controller.signal);
    return () => controller.abort();
  }, [load, tab]);

  const loadMore = () => {
    if (loadingMoreRef.current || !nextCursor) return;
    const which = tab;
    loadingMoreRef.current = true;
    setLoadingMore(true);
    setMoreError(false);
    getEvents({ ...FILTERS[which], cursor: nextCursor })
      .then((page) => {
        if (tabRef.current !== which) return; // tab changed mid-flight
        setItems((prev) => {
          const seen = new Set(prev.map((e) => e.id));
          return [...prev, ...page.items.filter((e) => !seen.has(e.id))];
        });
        setNextCursor(page.nextCursor);
      })
      .catch(() => {
        if (tabRef.current === which) setMoreError(true);
      })
      .finally(() => {
        loadingMoreRef.current = false;
        setLoadingMore(false);
      });
  };

  const inMine = tab !== "upcoming";
  const badge = tab === "going" ? t("events.going") : tab === "interested" ? t("events.interested") : undefined;
  const tabBtn = (active: boolean) =>
    `relative flex-1 rounded-full py-1.5 text-sm font-medium transition-colors ${
      active ? "bg-primary text-white shadow-sm" : "text-muted hover:text-foreground"
    }`;

  return (
    <div className="mx-auto flex w-full max-w-2xl flex-col gap-4">
      <header className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <h1 className="text-xl font-semibold text-foreground">{t("navigation.events")}</h1>
          <p className="mt-0.5 text-sm text-muted">{t("events.pageDescription")}</p>
        </div>
        <button
          type="button"
          onClick={() => setCreating(true)}
          className="inline-flex shrink-0 items-center gap-1.5 rounded-full bg-primary px-4 py-2 text-sm font-medium text-white shadow-sm transition-colors hover:bg-primary-hover"
        >
          <Plus className="h-4 w-4" aria-hidden />
          <span>{t("events.create")}</span>
        </button>
      </header>

      <div role="tablist" aria-label={t("navigation.events")} className="flex rounded-full border border-border bg-surface p-1 sm:max-w-sm">
        <button type="button" role="tab" aria-selected={!inMine} onClick={() => setTab("upcoming")} className={tabBtn(!inMine)}>
          {t("events.tabUpcoming")}
        </button>
        <button type="button" role="tab" aria-selected={inMine} onClick={() => !inMine && setTab("created")} className={tabBtn(inMine)}>
          {t("events.tabMine")}
        </button>
      </div>

      {inMine ? (
        <div className="flex flex-wrap gap-2">
          {MINE_TABS.map(({ tab: value, labelKey }) => {
            const active = tab === value;
            return (
              <button
                key={value}
                type="button"
                aria-pressed={active}
                onClick={() => setTab(value)}
                className={`rounded-full border px-3.5 py-1 text-xs font-medium transition-colors ${
                  active
                    ? "border-primary bg-primary-soft text-primary"
                    : "border-border bg-surface text-muted hover:text-foreground"
                }`}
              >
                {t(labelKey)}
              </button>
            );
          })}
        </div>
      ) : null}

      {status === "loading" ? (
        <div className="flex flex-col gap-3" aria-busy="true" aria-label={t("events.loading")}>
          {[0, 1, 2].map((i) => (
            <div key={i} className="flex gap-4 rounded-2xl border border-border bg-surface p-4">
              <div className="h-14 w-14 shrink-0 rounded-2xl bg-background motion-safe:animate-pulse" />
              <div className="flex flex-1 flex-col gap-2 pt-1">
                <div className="h-4 w-2/3 rounded bg-background motion-safe:animate-pulse" />
                <div className="h-3 w-1/2 rounded bg-background motion-safe:animate-pulse" />
                <div className="h-3 w-1/3 rounded bg-background motion-safe:animate-pulse" />
              </div>
            </div>
          ))}
        </div>
      ) : null}

      {status === "error" ? (
        <div className="rounded-2xl border border-border bg-surface px-4 py-10 text-center shadow-sm">
          <p className="text-sm text-muted">{t("events.loadError")}</p>
          <button type="button" onClick={() => load(tab)} className="mt-2 text-sm text-primary hover:underline">
            {t("search.tryAgain")}
          </button>
        </div>
      ) : null}

      {status === "ready" && items.length === 0 ? (
        <div className="flex flex-col items-center rounded-2xl border border-border bg-surface px-6 py-12 text-center shadow-sm">
          <span className="flex h-12 w-12 items-center justify-center rounded-2xl bg-primary-soft text-primary">
            <CalendarDays className="h-6 w-6" aria-hidden />
          </span>
          <p className="mt-3 max-w-xs text-sm text-muted">{t(EMPTY[tab])}</p>
          {tab === "upcoming" || tab === "created" ? (
            <button
              type="button"
              onClick={() => setCreating(true)}
              className="mt-4 rounded-full bg-primary px-4 py-1.5 text-sm font-medium text-white transition-colors hover:bg-primary-hover"
            >
              {t("events.create")}
            </button>
          ) : (
            <button
              type="button"
              onClick={() => setTab("upcoming")}
              className="mt-4 rounded-full border border-border px-4 py-1.5 text-sm font-medium text-foreground transition-colors hover:bg-background"
            >
              {t("events.browseUpcoming")}
            </button>
          )}
        </div>
      ) : null}

      {items.length > 0 ? (
        <ul className="flex flex-col gap-3">
          {items.map((e) => (
            <li key={e.id}>
              <EventCard event={e} badge={badge} />
            </li>
          ))}
        </ul>
      ) : null}

      {status === "ready" && nextCursor ? (
        <div className="flex flex-col items-center gap-1">
          <button
            type="button"
            onClick={loadMore}
            disabled={loadingMore}
            className="rounded-full border border-border bg-surface px-5 py-2 text-sm font-medium text-muted transition-colors hover:text-foreground disabled:opacity-50"
          >
            {loadingMore ? t("feed.loadingMore") : t("feed.loadMore")}
          </button>
          {moreError ? (
            <p className="text-xs text-red-500" role="alert">
              {t("events.loadError")}
            </p>
          ) : null}
        </div>
      ) : null}

      {creating ? (
        <EventForm
          onClose={() => setCreating(false)}
          onSaved={(saved) => {
            setCreating(false);
            router.push(`/events/${encodeURIComponent(saved.id)}`);
          }}
        />
      ) : null}
    </div>
  );
}
