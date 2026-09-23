"use client";

import { useCallback, useEffect, useState } from "react";
import Image from "next/image";
import Link from "next/link";
import { Users, X } from "lucide-react";

import {
  ApiError,
  followUser,
  getDiscoverUsers,
  type DiscoverUser,
} from "@/lib/api";
import { useAuth } from "@/lib/auth-context";
import { useLanguage } from "@/lib/language-context";

const SUGGESTION_LIMIT = 5;

const FALLBACK_AVATAR =
  "data:image/svg+xml;utf8," +
  encodeURIComponent(
    '<svg xmlns="http://www.w3.org/2000/svg" width="48" height="48"><circle cx="24" cy="24" r="24" fill="#d4d4d8"/></svg>',
  );

type Status = "loading" | "ready" | "error";

function uniqueSuggestions(items: DiscoverUser[], currentUserID?: string): DiscoverUser[] {
  const seen = new Set<string>();
  return items.filter((item) => {
    if (item.id === currentUserID || seen.has(item.id)) return false;
    seen.add(item.id);
    return true;
  });
}

export function SuggestedPeople() {
  const { t } = useLanguage();
  const { user } = useAuth();
  const [status, setStatus] = useState<Status>("loading");
  const [people, setPeople] = useState<DiscoverUser[]>([]);
  const [pendingID, setPendingID] = useState<string | null>(null);
  const [requestedIDs, setRequestedIDs] = useState<Set<string>>(new Set());
  const [actionErrorID, setActionErrorID] = useState<string | null>(null);

  const load = useCallback(
    (signal?: AbortSignal) => {
      setStatus("loading");
      getDiscoverUsers({ mode: "for_you", limit: SUGGESTION_LIMIT }, signal)
        .then((page) => {
          setPeople(uniqueSuggestions(page.items, user?.id));
          setRequestedIDs(new Set());
          setActionErrorID(null);
          setStatus("ready");
        })
        .catch((err) => {
          if (err instanceof DOMException && err.name === "AbortError") return;
          setStatus("error");
        });
    },
    [user?.id],
  );

  useEffect(() => {
    const controller = new AbortController();
    load(controller.signal);
    return () => controller.abort();
  }, [load]);

  const dismiss = (id: string) => {
    setPeople((current) => current.filter((person) => person.id !== id));
    setRequestedIDs((current) => {
      const next = new Set(current);
      next.delete(id);
      return next;
    });
    if (actionErrorID === id) setActionErrorID(null);
  };

  const follow = async (person: DiscoverUser) => {
    if (pendingID === person.id || requestedIDs.has(person.id)) return;

    setPendingID(person.id);
    setActionErrorID(null);
    try {
      const result = await followUser(person.username);
      if (result.following) {
        dismiss(person.id);
      } else if (result.requested) {
        setRequestedIDs((current) => new Set(current).add(person.id));
      }
    } catch (err) {
      // A 409 can only be a stale candidate after another client changed the
      // relationship. Removing it prevents a duplicate follow affordance.
      if (err instanceof ApiError && err.status === 409) {
        dismiss(person.id);
      } else {
        setActionErrorID(person.id);
      }
    } finally {
      setPendingID(null);
    }
  };

  return (
    <section
      aria-busy={status === "loading"}
      className="mr-1 min-h-[292px] rounded-2xl border border-border bg-surface p-5 shadow-sm"
    >
      <div className="flex items-center justify-between gap-2">
        <h2 className="text-sm font-semibold text-foreground">
          {t("sidebar.suggestedForYou")}
        </h2>
        <Link
          href="/discover"
          className="rounded text-xs font-medium text-primary transition-colors hover:text-primary-hover"
        >
          {t("sidebar.seeAll")}
        </Link>
      </div>

      {status === "loading" ? (
        <ul className="mt-4 flex flex-col gap-3" aria-label={t("sidebar.suggestedForYou")}>
          {Array.from({ length: SUGGESTION_LIMIT }, (_, index) => (
            <li key={index} className="flex items-center gap-3" aria-hidden="true">
              <div className="h-12 w-12 shrink-0 animate-pulse rounded-full bg-background" />
              <div className="min-w-0 flex-1 space-y-2">
                <div className="h-3 w-28 animate-pulse rounded bg-background" />
                <div className="h-2.5 w-20 animate-pulse rounded bg-background" />
              </div>
              <div className="h-7 w-16 animate-pulse rounded-full bg-background" />
            </li>
          ))}
        </ul>
      ) : null}

      {status === "error" ? (
        <div className="flex min-h-48 flex-col items-center justify-center text-center">
          <p className="text-sm text-muted">{t("discover.suggestionsError")}</p>
          <button
            type="button"
            onClick={() => load()}
            className="mt-3 rounded-full bg-primary px-4 py-1.5 text-sm font-medium text-white transition-colors hover:bg-primary-hover"
          >
            {t("search.tryAgain")}
          </button>
        </div>
      ) : null}

      {status === "ready" && people.length === 0 ? (
        <div className="flex min-h-48 flex-col items-center justify-center text-center">
          <span className="flex h-10 w-10 items-center justify-center rounded-full bg-primary-soft text-primary">
            <Users className="h-5 w-5" aria-hidden />
          </span>
          <p className="mt-3 text-sm font-medium text-foreground">
            {t("discover.allCaughtUp")}
          </p>
          <p className="mt-1 text-xs text-muted">{t("discover.noNewPeople")}</p>
        </div>
      ) : null}

      {status === "ready" && people.length > 0 ? (
        <ul className="mt-4 flex flex-col gap-3">
          {people.map((person) => {
            const href = `/u/${encodeURIComponent(person.username)}`;
            const requested = requestedIDs.has(person.id);
            const pending = pendingID === person.id;
            const actionFailed = actionErrorID === person.id;

            return (
              <li key={person.id} className="flex items-center gap-3">
                <Link
                  href={href}
                  aria-label={t("discover.viewProfile", { name: person.displayName })}
                  className="shrink-0"
                >
                  <Image
                    src={person.avatarUrl ?? FALLBACK_AVATAR}
                    alt={person.displayName}
                    width={48}
                    height={48}
                    unoptimized={Boolean(person.avatarUrl)}
                    className="h-12 w-12 rounded-full object-cover"
                  />
                </Link>
                <div className="min-w-0 flex-1">
                  <Link
                    href={href}
                    className="block truncate text-sm font-semibold text-foreground hover:underline"
                  >
                    {person.displayName}
                  </Link>
                  <div className="truncate text-xs text-muted">@{person.username}</div>
                </div>
                <div className="flex shrink-0 items-center gap-1.5">
                  <button
                    type="button"
                    onClick={() => void follow(person)}
                    disabled={pending || requested}
                    aria-pressed={requested}
                    className="rounded-full bg-primary-soft px-3 py-1 text-xs font-semibold text-primary transition-colors hover:bg-primary/15 disabled:cursor-not-allowed disabled:opacity-60"
                  >
                    {pending
                      ? "…"
                      : requested
                        ? t("profile.requested")
                        : actionFailed
                          ? t("search.tryAgain")
                          : t("sidebar.follow")}
                  </button>
                  <button
                    type="button"
                    onClick={() => dismiss(person.id)}
                    disabled={pending}
                    aria-label={t("sidebar.dismiss", { name: person.displayName })}
                    className="flex h-6 w-6 items-center justify-center rounded-full text-muted-soft transition-colors hover:bg-background hover:text-foreground disabled:cursor-not-allowed disabled:opacity-50"
                  >
                    <X className="h-4 w-4" />
                  </button>
                </div>
              </li>
            );
          })}
        </ul>
      ) : null}
    </section>
  );
}
