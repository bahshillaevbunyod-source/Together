"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import Image from "next/image";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { ArrowLeft, CalendarDays, Languages, MapPin, MessageCircle, Users } from "lucide-react";

import { ApiError, followUser, getDiscoverUsers, openConversation, type DiscoverUser } from "@/lib/api";
import { countryName, englishCountryName, flagEmoji, languageName } from "@/lib/world/world-api";
import { useLanguage, type TranslationKey } from "@/lib/language-context";
import { FALLBACK_AVATAR } from "@/components/events/EventCard";

type FollowState = "idle" | "pending" | "following" | "requested" | "error";
const PAGE = 20;

/**
 * Real people the viewer can discover in one country, via the existing
 * /users/discover?mode=world&country= endpoint (server-side self, block and
 * follow exclusions). Location stays coarse: the city/country the person
 * chose to put on their profile.
 */
export function WorldPeople({
  country,
  count,
  onBack,
  onEvents,
}: {
  country: string;
  count: number;
  onBack: () => void;
  onEvents: (query: string) => void;
}) {
  const { t, locale } = useLanguage();
  const [items, setItems] = useState<DiscoverUser[]>([]);
  const [status, setStatus] = useState<"loading" | "ready" | "error">("loading");
  const [nextCursor, setNextCursor] = useState("");
  const [loadingMore, setLoadingMore] = useState(false);
  const [follow, setFollow] = useState<Record<string, FollowState>>({});
  const [messagePending, setMessagePending] = useState<string | null>(null);
  const [messageError, setMessageError] = useState<{ id: string; key: TranslationKey } | null>(null);
  const router = useRouter();
  const loadingMoreRef = useRef(false);
  const name = countryName(country, locale);

  // Same idempotent open-conversation flow as the profile Message button.
  const doMessage = async (person: DiscoverUser) => {
    if (messagePending) return;
    setMessagePending(person.id);
    setMessageError(null);
    try {
      const conv = await openConversation(person.username);
      router.push(`/messages?c=${encodeURIComponent(conv.id)}`);
    } catch (err) {
      const status = err instanceof ApiError ? err.status : 0;
      const key: TranslationKey =
        status === 401
          ? "profile.messageSignIn"
          : status === 403
            ? "profile.messageForbidden"
            : status === 404
              ? "profile.userUnavailable"
              : "profile.messageError";
      setMessageError({ id: person.id, key });
      setMessagePending(null);
    }
  };

  const load = useCallback(
    (signal?: AbortSignal) => {
      setStatus("loading");
      getDiscoverUsers({ mode: "world", country, limit: PAGE }, signal)
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
    [country],
  );

  useEffect(() => {
    const controller = new AbortController();
    load(controller.signal);
    return () => controller.abort();
  }, [load]);

  const loadMore = () => {
    if (loadingMoreRef.current || !nextCursor) return;
    loadingMoreRef.current = true;
    setLoadingMore(true);
    getDiscoverUsers({ mode: "world", country, limit: PAGE, cursor: nextCursor })
      .then((page) => {
        setItems((prev) => {
          const seen = new Set(prev.map((p) => p.id));
          return [...prev, ...page.items.filter((p) => !seen.has(p.id))];
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

  const doFollow = async (person: DiscoverUser) => {
    const current = follow[person.id];
    if (current === "pending" || current === "following" || current === "requested") return;
    setFollow((s) => ({ ...s, [person.id]: "pending" }));
    try {
      const result = await followUser(person.username);
      setFollow((s) => ({ ...s, [person.id]: result.following ? "following" : result.requested ? "requested" : "idle" }));
    } catch (err) {
      // 409: the relationship already exists (changed elsewhere).
      setFollow((s) => ({ ...s, [person.id]: err instanceof ApiError && err.status === 409 ? "following" : "error" }));
    }
  };

  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center gap-2">
        <button
          type="button"
          onClick={onBack}
          aria-label={t("discover.allCountries")}
          className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full text-muted transition-colors hover:bg-background hover:text-foreground"
        >
          <ArrowLeft className="h-4 w-4 rtl:rotate-180" />
        </button>
        <span className="text-2xl leading-none" aria-hidden>
          {flagEmoji(country)}
        </span>
        <div className="min-w-0 flex-1">
          <h2 className="truncate text-base font-semibold text-foreground">{name}</h2>
          <p className="text-xs text-muted">{t("world.peopleCount", { count })}</p>
        </div>
      </div>

      <button
        type="button"
        onClick={() => onEvents(englishCountryName(country))}
        className="flex items-center gap-2 rounded-2xl border border-border px-3 py-2.5 text-left text-sm text-foreground transition-colors hover:border-primary/40 hover:bg-primary-soft/40"
      >
        <CalendarDays className="h-4 w-4 shrink-0 text-primary" aria-hidden />
        <span className="min-w-0 flex-1 truncate">{t("world.eventsMentioning", { country: name })}</span>
      </button>

      {status === "loading" ? (
        <ul className="flex flex-col gap-2" aria-busy="true" aria-label={t("world.loadingPeople")}>
          {[0, 1, 2].map((i) => (
            <li key={i} className="h-16 rounded-2xl bg-background motion-safe:animate-pulse" />
          ))}
        </ul>
      ) : null}

      {status === "error" ? (
        <div className="rounded-2xl bg-background px-4 py-8 text-center">
          <p className="text-sm text-muted">{t("discover.peopleError")}</p>
          <button type="button" onClick={() => load()} className="mt-2 text-sm text-primary hover:underline">
            {t("search.tryAgain")}
          </button>
        </div>
      ) : null}

      {status === "ready" && items.length === 0 ? (
        <div className="flex flex-col items-center rounded-2xl bg-background px-6 py-10 text-center">
          <Users className="h-6 w-6 text-muted-soft" aria-hidden />
          <p className="mt-2 text-sm text-muted">{t("discover.noPeopleInCountry", { country: name })}</p>
        </div>
      ) : null}

      {items.length > 0 ? (
        <ul className="flex flex-col gap-2">
          {items.map((person) => {
            const href = `/u/${encodeURIComponent(person.username)}`;
            const state = follow[person.id] ?? "idle";
            const place = [person.city, person.countryCode ? countryName(person.countryCode, locale) : null].filter(Boolean).join(", ");
            return (
              <li key={person.id} className="rounded-2xl border border-border p-3">
                <div className="flex items-center gap-3">
                <Link href={href} aria-label={t("discover.viewProfile", { name: person.displayName })} className="shrink-0">
                  <Image
                    src={person.avatarUrl ?? FALLBACK_AVATAR}
                    alt=""
                    width={44}
                    height={44}
                    unoptimized={Boolean(person.avatarUrl)}
                    className="h-11 w-11 rounded-full object-cover"
                  />
                </Link>
                <div className="min-w-0 flex-1">
                  <Link href={href} className="block truncate text-sm font-semibold text-foreground hover:underline">
                    {person.displayName}
                  </Link>
                  <p className="truncate text-xs text-muted">@{person.username}</p>
                  <p className="mt-0.5 flex flex-wrap items-center gap-x-2 text-[11px] text-muted">
                    {place ? (
                      <span className="inline-flex min-w-0 items-center gap-0.5">
                        <MapPin className="h-3 w-3 shrink-0" aria-hidden />
                        <span className="truncate">{place}</span>
                      </span>
                    ) : null}
                    {person.nativeLanguage ? (
                      <span className="inline-flex items-center gap-0.5">
                        <Languages className="h-3 w-3 shrink-0" aria-hidden />
                        {languageName(person.nativeLanguage, locale)}
                      </span>
                    ) : null}
                  </p>
                </div>
                {state === "following" || state === "requested" ? (
                  <span className="shrink-0 rounded-full bg-background px-3 py-1 text-xs font-semibold text-muted">
                    {state === "following" ? t("profile.followingState") : t("profile.requested")}
                  </span>
                ) : (
                  <button
                    type="button"
                    onClick={() => void doFollow(person)}
                    disabled={state === "pending"}
                    className="shrink-0 rounded-full bg-primary-soft px-3 py-1 text-xs font-semibold text-primary transition-colors hover:bg-primary/15 disabled:cursor-not-allowed disabled:opacity-60"
                  >
                    {state === "error" ? t("search.tryAgain") : t("profile.follow")}
                  </button>
                )}
                <button
                  type="button"
                  onClick={() => void doMessage(person)}
                  disabled={messagePending !== null}
                  aria-label={t("profile.message")}
                  title={t("profile.message")}
                  className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full border border-border text-muted transition-colors hover:border-primary/40 hover:text-primary disabled:cursor-not-allowed disabled:opacity-50"
                >
                  <MessageCircle className="h-4 w-4" aria-hidden />
                </button>
                </div>
                {messageError?.id === person.id ? (
                  <p className="mt-2 text-xs text-red-500" role="alert">
                    {t(messageError.key)}
                  </p>
                ) : null}
              </li>
            );
          })}
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
