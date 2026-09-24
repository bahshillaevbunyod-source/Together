"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import Image from "next/image";
import Link from "next/link";
import { Phone } from "lucide-react";

import { getConversations, type ApiConversation } from "@/lib/api";
import { useLanguage } from "@/lib/language-context";
import { CallButtons } from "@/components/calls/CallButtons";
import { MessagingTabs } from "@/components/community/MessagingTabs";

type Status = "loading" | "ready" | "error";

const FALLBACK_AVATAR =
  "data:image/svg+xml;utf8," +
  encodeURIComponent(
    '<svg xmlns="http://www.w3.org/2000/svg" width="44" height="44"><circle cx="22" cy="22" r="22" fill="#d4d4d8"/></svg>',
  );

/**
 * Calls V1 hub: the people the viewer can call, taken from their real direct
 * conversations (GET /api/v1/conversations returns direct conversations only).
 * Each row's voice/video actions are the same CallButtons → startCall flow as
 * the DM header; the global CallOverlay owns the call itself. There is no call
 * history yet (the backend does not persist calls), so none is shown.
 */
export default function CallsPage() {
  const { t } = useLanguage();
  const [items, setItems] = useState<ApiConversation[]>([]);
  const [status, setStatus] = useState<Status>("loading");
  const [nextCursor, setNextCursor] = useState("");
  const [loadingMore, setLoadingMore] = useState(false);
  const loadingMoreRef = useRef(false);

  const load = useCallback((signal?: AbortSignal) => {
    setStatus("loading");
    getConversations({}, signal)
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
    load(controller.signal);
    return () => controller.abort();
  }, [load]);

  const loadMore = () => {
    if (loadingMoreRef.current || !nextCursor) return;
    loadingMoreRef.current = true;
    setLoadingMore(true);
    getConversations({ cursor: nextCursor })
      .then((page) => {
        setItems((prev) => {
          const seen = new Set(prev.map((c) => c.id));
          return [...prev, ...page.items.filter((c) => !seen.has(c.id))];
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

  // Only rows with a real other participant can be called.
  const callable = items.filter((c) => c.otherUser && c.otherUser.id);

  return (
    <div className="mx-auto flex w-full max-w-2xl flex-col gap-4">
      <div className="sm:max-w-sm">
        <MessagingTabs />
      </div>
      <header>
        <h1 className="text-xl font-semibold text-foreground">{t("navigation.calls")}</h1>
        <p className="mt-0.5 text-sm text-muted">{t("calls.pageDescription")}</p>
      </header>

      <section className="overflow-hidden rounded-2xl border border-border bg-surface shadow-sm">
        {status === "loading" ? (
          <p className="px-4 py-10 text-center text-sm text-muted">{t("messages.loadingConversations")}</p>
        ) : null}

        {status === "error" ? (
          <div className="px-4 py-10 text-center">
            <p className="text-sm text-muted">{t("messages.loadError")}</p>
            <button type="button" onClick={() => load()} className="mt-2 text-sm text-primary hover:underline">
              {t("search.tryAgain")}
            </button>
          </div>
        ) : null}

        {status === "ready" && callable.length === 0 ? (
          <div className="flex flex-col items-center px-6 py-12 text-center">
            <span className="flex h-12 w-12 items-center justify-center rounded-2xl bg-primary-soft text-primary">
              <Phone className="h-6 w-6" aria-hidden />
            </span>
            <p className="mt-3 text-sm text-muted">{t("calls.emptyPeople")}</p>
            <Link
              href="/messages"
              className="mt-4 rounded-full bg-primary px-4 py-1.5 text-sm font-medium text-white transition-colors hover:bg-primary-hover"
            >
              {t("navigation.messages")}
            </Link>
          </div>
        ) : null}

        {callable.length > 0 ? (
          <ul className="divide-y divide-border">
            {callable.map((c) => (
              <li key={c.id} className="flex items-center gap-3 px-4 py-3">
                <Link
                  href={`/u/${encodeURIComponent(c.otherUser.username)}`}
                  aria-label={t("messages.viewProfileAria", { name: c.otherUser.displayName })}
                  className="flex min-w-0 flex-1 items-center gap-3 rounded-lg hover:opacity-90"
                >
                  <Image
                    src={c.otherUser.avatarUrl ?? FALLBACK_AVATAR}
                    alt=""
                    width={44}
                    height={44}
                    unoptimized={Boolean(c.otherUser.avatarUrl)}
                    className="h-11 w-11 shrink-0 rounded-full object-cover"
                  />
                  <span className="min-w-0">
                    <span className="block truncate text-sm font-semibold text-foreground">
                      {c.otherUser.displayName}
                    </span>
                    <span className="block truncate text-xs text-muted">@{c.otherUser.username}</span>
                  </span>
                </Link>
                <CallButtons conversationId={c.id} peer={c.otherUser} />
              </li>
            ))}
          </ul>
        ) : null}

        {status === "ready" && nextCursor ? (
          <button
            type="button"
            onClick={loadMore}
            disabled={loadingMore}
            className="w-full border-t border-border px-4 py-2.5 text-center text-sm text-muted transition-colors hover:text-foreground disabled:opacity-50"
          >
            {loadingMore ? t("feed.loadingMore") : t("feed.loadMore")}
          </button>
        ) : null}
      </section>
    </div>
  );
}
