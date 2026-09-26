"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import Image from "next/image";
import Link from "next/link";
import { Bell } from "lucide-react";

import {
  getNotifications,
  getUnreadNotificationCount,
  markAllNotificationsRead,
  markNotificationRead,
  type ApiNotification,
} from "@/lib/api";
import { formatTimeAgo } from "@/lib/format";
import { useLanguage, type TranslationKey } from "@/lib/language-context";
import {
  notificationDestination,
  notifyNotificationReadStateChanged,
} from "@/lib/notification-destination";

type Status = "idle" | "loading" | "ready" | "error";

// Neutral fallback avatar (a gray circle) used when an actor has no avatar.
const FALLBACK_AVATAR =
  "data:image/svg+xml;utf8," +
  encodeURIComponent(
    '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"><circle cx="16" cy="16" r="16" fill="#d4d4d8"/></svg>',
  );

// Translation key for the static action fragment per notification type. The
// actor name is rendered separately (bold) and never translated; this maps the
// stable event type id to its localized predicate.
function actionTextKey(type: string): TranslationKey {
  switch (type) {
    case "follow":
      return "notifications.action.follow";
    case "post_like":
      return "notifications.action.postLike";
    case "post_comment":
      return "notifications.action.postComment";
    case "follow_request":
      return "notifications.action.followRequest";
    default:
      return "notifications.action.default";
  }
}

export function NotificationsBell() {
  const { t, locale } = useLanguage();
  const [open, setOpen] = useState(false);
  const [items, setItems] = useState<ApiNotification[]>([]);
  const [status, setStatus] = useState<Status>("idle");
  const [nextCursor, setNextCursor] = useState("");
  const [loadingMore, setLoadingMore] = useState(false);
  const [unread, setUnread] = useState(0);

  const panelRef = useRef<HTMLDivElement>(null);
  const loadingMoreRef = useRef(false);

  // Initial unread count.
  useEffect(() => {
    const controller = new AbortController();
    const refreshUnread = () => getUnreadNotificationCount(controller.signal)
      .then((r) => setUnread(r.unreadCount))
      .catch((err) => {
        if (err instanceof DOMException && err.name === "AbortError") return;
        // Leave the badge hidden on failure.
      });
    void refreshUnread();
    window.addEventListener("together:notification-read-state-changed", refreshUnread);
    return () => {
      controller.abort();
      window.removeEventListener("together:notification-read-state-changed", refreshUnread);
    };
  }, []);

  const load = useCallback((signal?: AbortSignal) => {
    setStatus("loading");
    getNotifications({}, signal)
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

  // Load the list whenever the panel opens. Keyed on `open` only — never on
  // `status` — so load()'s own setStatus("loading") cannot re-trigger this
  // effect and abort (via cleanup) the request it just started. The cleanup
  // still aborts a genuinely in-flight request when the panel closes or the
  // component unmounts.
  useEffect(() => {
    if (!open) return;
    const controller = new AbortController();
    load(controller.signal);
    return () => controller.abort();
  }, [open, load]);

  // Close on outside click.
  useEffect(() => {
    if (!open) return;
    const onClick = (e: MouseEvent) => {
      if (panelRef.current && !panelRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener("mousedown", onClick);
    return () => document.removeEventListener("mousedown", onClick);
  }, [open]);

  const loadMore = () => {
    if (loadingMoreRef.current) return;
    const cursor = nextCursor;
    if (!cursor) return;
    loadingMoreRef.current = true;
    setLoadingMore(true);
    getNotifications({ cursor })
      .then((page) => {
        setItems((prev) => [...prev, ...page.items]);
        setNextCursor(page.nextCursor);
      })
      .catch(() => {
        // Keep what we have.
      })
      .finally(() => {
        loadingMoreRef.current = false;
        setLoadingMore(false);
      });
  };

  const markRead = async (n: ApiNotification) => {
    if (n.readAt) return;
    const now = new Date().toISOString();
    setItems((prev) =>
      prev.map((it) => (it.id === n.id ? { ...it, readAt: now } : it)),
    );
    setUnread((c) => Math.max(0, c - 1));
    try {
      await markNotificationRead(n.id);
      notifyNotificationReadStateChanged();
    } catch {
      // Roll back on failure.
      setItems((prev) =>
        prev.map((it) => (it.id === n.id ? { ...it, readAt: null } : it)),
      );
      setUnread((c) => c + 1);
    }
  };

  const markAll = async () => {
    const prevItems = items;
    const prevUnread = unread;
    const now = new Date().toISOString();
    setItems((prev) =>
      prev.map((it) => (it.readAt ? it : { ...it, readAt: now })),
    );
    setUnread(0);
    try {
      await markAllNotificationsRead();
      notifyNotificationReadStateChanged();
    } catch {
      setItems(prevItems);
      setUnread(prevUnread);
    }
  };

  const badge = unread > 9 ? "9+" : String(unread);

  return (
    <div className="relative" ref={panelRef}>
      <button
        type="button"
        aria-label={t("notifications.title")}
        aria-expanded={open}
        aria-haspopup="menu"
        onClick={() => setOpen((prev) => !prev)}
        className="relative flex h-10 w-10 items-center justify-center rounded-full border border-border bg-surface text-muted transition-colors hover:bg-background"
      >
        <Bell className="h-5 w-5" />
        {unread > 0 ? (
          <span className="absolute -right-1 -top-1 flex h-5 min-w-5 items-center justify-center rounded-full bg-red-500 px-1 text-[10px] font-semibold text-white ring-2 ring-surface">
            {badge}
          </span>
        ) : null}
      </button>

      {open ? (
        <div
          role="menu"
          className="fixed inset-x-3 top-[calc(4.5rem+env(safe-area-inset-top))] z-40 overflow-hidden rounded-xl border border-border bg-surface shadow-lg sm:absolute sm:inset-x-auto sm:right-0 sm:top-full sm:mt-2 sm:w-80"
        >
          <div className="flex items-center justify-between border-b border-border px-4 py-3">
            <span className="text-sm font-semibold text-foreground">
              {t("notifications.title")}
            </span>
            <button
              type="button"
              onClick={markAll}
              disabled={unread === 0}
              className="-my-2 min-h-9 px-1 text-xs text-primary transition-colors hover:underline disabled:cursor-not-allowed disabled:text-muted-soft disabled:no-underline"
            >
              {t("notifications.markAllRead")}
            </button>
          </div>

          <div className="max-h-[min(24rem,calc(100dvh-12rem))] overflow-y-auto">
            {status === "loading" ? (
              <p className="px-4 py-6 text-center text-sm text-muted">
                {t("feed.loadingMore")}
              </p>
            ) : null}

            {status === "error" ? (
              <div className="px-4 py-6 text-center">
                <p className="text-sm text-muted">{t("notifications.error")}</p>
                <button
                  type="button"
                  onClick={() => load()}
                  className="mt-2 text-sm text-primary hover:underline"
                >
                  {t("search.tryAgain")}
                </button>
              </div>
            ) : null}

            {status === "ready" && items.length === 0 ? (
              <p className="px-4 py-6 text-center text-sm text-muted-soft">
                {t("notifications.empty")}
              </p>
            ) : null}

            {items.length > 0 ? (
              <ul className="flex flex-col">
                {items.map((n) => {
                  const actorName = n.actor?.displayName ?? t("notifications.someone");
                  const destination = notificationDestination(n);
                  const rowClass = `flex w-full items-start gap-3 px-4 py-3 text-left transition-colors hover:bg-background ${
                    n.readAt ? "" : "bg-background/60"
                  }`;
                  const inner = (
                    <>
                      <Image
                        src={n.actor?.avatarUrl ?? FALLBACK_AVATAR}
                        alt={actorName}
                        width={32}
                        height={32}
                        unoptimized={Boolean(n.actor?.avatarUrl)}
                        className="mt-1 h-8 w-8 shrink-0 rounded-full object-cover"
                      />
                      <span className="min-w-0 flex-1">
                        <span className="text-sm text-foreground">
                          <span className="font-semibold">{actorName}</span>{" "}
                          {t(actionTextKey(n.type))}
                        </span>
                        <span className="mt-0.5 block text-xs text-muted-soft">
                          {formatTimeAgo(n.createdAt, locale)}
                        </span>
                      </span>
                      {n.readAt ? null : (
                        <span className="mt-2 h-2 w-2 shrink-0 rounded-full bg-primary" />
                      )}
                    </>
                  );

                  return (
                    <li key={n.id}>
                      {destination ? (
                        <Link
                          href={destination}
                          onClick={() => {
                            markRead(n);
                            setOpen(false);
                          }}
                          className={rowClass}
                        >
                          {inner}
                        </Link>
                      ) : (
                        <button
                          type="button"
                          role="menuitem"
                          onClick={() => markRead(n)}
                          className={rowClass}
                        >
                          {inner}
                        </button>
                      )}
                    </li>
                  );
                })}
              </ul>
            ) : null}

            {nextCursor ? (
              <button
                type="button"
                onClick={loadMore}
                disabled={loadingMore}
                className="w-full border-t border-border px-4 py-2.5 text-center text-sm text-muted transition-colors hover:text-foreground disabled:opacity-50"
              >
                {loadingMore ? t("feed.loadingMore") : t("feed.loadMore")}
              </button>
            ) : null}
          </div>

          <Link
            href="/notifications"
            onClick={() => setOpen(false)}
            className="block border-t border-border px-4 py-3 text-center text-sm font-medium text-primary transition-colors hover:bg-background hover:underline"
          >
            {t("notifications.viewAll")}
          </Link>
        </div>
      ) : null}
    </div>
  );
}
