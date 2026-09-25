"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import Link from "next/link";
import { CalendarDays, Search, X } from "lucide-react";

import { getEvents, type ApiEvent, type EventListFilter } from "@/lib/events-api";
import { useLanguage, type TranslationKey } from "@/lib/language-context";
import { EventCard } from "@/components/events/EventCard";

export type EventScope = "all" | "in_person" | "online";

const SCOPES: { value: EventScope; labelKey: TranslationKey }[] = [
  { value: "all", labelKey: "world.scopeAll" },
  { value: "in_person", labelKey: "events.inPerson" },
  { value: "online", labelKey: "events.online" },
];

const PAGE = 20;
const MAX_QUERY = 100;

/**
 * Upcoming events the viewer is allowed to see (visibility and blocks are
 * enforced by GET /api/v1/events), searched by title/venue/address text.
 * Events have no coordinates, so they are listed — never pinned on the map.
 */
export function WorldEvents({
  query,
  onQueryChange,
  scope,
  onScopeChange,
}: {
  query: string;
  onQueryChange: (q: string) => void;
  scope: EventScope;
  onScopeChange: (s: EventScope) => void;
}) {
  const { t } = useLanguage();
  const [debounced, setDebounced] = useState(query.trim());
  const [items, setItems] = useState<ApiEvent[]>([]);
  const [status, setStatus] = useState<"loading" | "ready" | "error">("loading");
  const [nextCursor, setNextCursor] = useState("");
  const [loadingMore, setLoadingMore] = useState(false);
  const loadingMoreRef = useRef(false);
  const filterKeyRef = useRef("");

  useEffect(() => {
    const id = setTimeout(() => setDebounced(query.trim()), 300);
    return () => clearTimeout(id);
  }, [query]);

  const filter: EventListFilter = {
    q: debounced || undefined,
    type: scope === "all" ? undefined : scope,
    limit: PAGE,
  };
  const filterKey = `${filter.q ?? ""}|${filter.type ?? ""}`;

  const load = useCallback(
    (signal?: AbortSignal) => {
      const [q, type] = filterKey.split("|");
      filterKeyRef.current = filterKey;
      setStatus("loading");
      setItems([]);
      setNextCursor("");
      getEvents({ q: q || undefined, type: (type || undefined) as EventListFilter["type"], limit: PAGE }, signal)
        .then((page) => {
          setItems(page.items);
          setNextCursor(page.nextCursor);
          setStatus("ready");
        })
        .catch((err) => {
          if (err instanceof DOMException && err.name === "AbortError") return;
          setStatus("error");
        });
    },
    [filterKey],
  );

  useEffect(() => {
    const controller = new AbortController();
    load(controller.signal);
    return () => controller.abort();
  }, [load]);

  const loadMore = () => {
    if (loadingMoreRef.current || !nextCursor) return;
    const key = filterKeyRef.current;
    loadingMoreRef.current = true;
    setLoadingMore(true);
    getEvents({ ...filter, cursor: nextCursor })
      .then((page) => {
        if (filterKeyRef.current !== key) return;
        setItems((prev) => {
          const seen = new Set(prev.map((e) => e.id));
          return [...prev, ...page.items.filter((e) => !seen.has(e.id))];
        });
        setNextCursor(page.nextCursor);
      })
      .catch(() => {
        // Keep what we have; the button stays available to retry.
      })
      .finally(() => {
        loadingMoreRef.current = false;
        setLoadingMore(false);
      });
  };

  const hasFilter = Boolean(debounced) || scope !== "all";

  return (
    <div className="flex flex-col gap-3">
      <label className="relative block">
        <span className="sr-only">{t("world.searchEvents")}</span>
        <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-soft" aria-hidden />
        <input
          value={query}
          maxLength={MAX_QUERY}
          onChange={(e) => onQueryChange(e.target.value)}
          placeholder={t("world.searchEvents")}
          className="h-10 w-full rounded-full border border-border bg-background pl-9 pr-9 text-sm text-foreground placeholder:text-muted-soft focus:border-primary focus:outline-none focus:ring-2 focus:ring-primary/20"
        />
        {query ? (
          <button
            type="button"
            onClick={() => onQueryChange("")}
            aria-label={t("world.clearSearch")}
            className="absolute right-2 top-1/2 flex h-7 w-7 -translate-y-1/2 items-center justify-center rounded-full text-muted hover:bg-surface hover:text-foreground"
          >
            <X className="h-4 w-4" />
          </button>
        ) : null}
      </label>

      <div className="flex flex-wrap gap-2">
        {SCOPES.map(({ value, labelKey }) => {
          const active = scope === value;
          return (
            <button
              key={value}
              type="button"
              aria-pressed={active}
              onClick={() => onScopeChange(value)}
              className={`rounded-full border px-3.5 py-1 text-xs font-medium transition-colors ${
                active ? "border-primary bg-primary-soft text-primary" : "border-border text-muted hover:text-foreground"
              }`}
            >
              {t(labelKey)}
            </button>
          );
        })}
      </div>
      {scope === "online" ? <p className="px-1 text-xs text-muted">{t("world.onlineNote")}</p> : null}

      {status === "loading" ? (
        <ul className="flex flex-col gap-2" aria-busy="true" aria-label={t("events.loading")}>
          {[0, 1, 2].map((i) => (
            <li key={i} className="h-24 rounded-2xl bg-background motion-safe:animate-pulse" />
          ))}
        </ul>
      ) : null}

      {status === "error" ? (
        <div className="rounded-2xl bg-background px-4 py-8 text-center">
          <p className="text-sm text-muted">{t("events.loadError")}</p>
          <button type="button" onClick={() => load()} className="mt-2 text-sm text-primary hover:underline">
            {t("search.tryAgain")}
          </button>
        </div>
      ) : null}

      {status === "ready" && items.length === 0 ? (
        <div className="flex flex-col items-center rounded-2xl bg-background px-6 py-10 text-center">
          <CalendarDays className="h-6 w-6 text-muted-soft" aria-hidden />
          <p className="mt-2 max-w-xs text-sm text-muted">{hasFilter ? t("world.noEventsMatch") : t("events.emptyUpcoming")}</p>
          <Link
            href="/events"
            className="mt-4 rounded-full bg-primary px-4 py-1.5 text-sm font-medium text-white transition-colors hover:bg-primary-hover"
          >
            {t("navigation.events")}
          </Link>
        </div>
      ) : null}

      {items.length > 0 ? (
        <ul className="flex flex-col gap-2">
          {items.map((e) => (
            <li key={e.id}>
              <EventCard event={e} />
            </li>
          ))}
        </ul>
      ) : null}

      {status === "ready" && nextCursor ? (
        <button
          type="button"
          onClick={loadMore}
          disabled={loadingMore}
          className="self-center rounded-full border border-border px-5 py-2 text-sm font-medium text-muted transition-colors hover:text-foreground disabled:opacity-50"
        >
          {loadingMore ? t("feed.loadingMore") : t("feed.loadMore")}
        </button>
      ) : null}
    </div>
  );
}
