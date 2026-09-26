"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import Image from "next/image";
import Link from "next/link";

import {
  getNotifications,
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

type Status = "loading" | "ready" | "error";

const FALLBACK_AVATAR =
  "data:image/svg+xml;utf8," +
  encodeURIComponent(
    '<svg xmlns="http://www.w3.org/2000/svg" width="48" height="48"><circle cx="24" cy="24" r="24" fill="#d4d4d8"/></svg>',
  );

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

export default function NotificationsPage() {
  const { t, locale } = useLanguage();
  const [items, setItems] = useState<ApiNotification[]>([]);
  const [status, setStatus] = useState<Status>("loading");
  const [nextCursor, setNextCursor] = useState("");
  const [loadingMore, setLoadingMore] = useState(false);
  const [mutating, setMutating] = useState(false);
  const loadingMoreRef = useRef(false);

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

  useEffect(() => {
    const controller = new AbortController();
    load(controller.signal);
    return () => controller.abort();
  }, [load]);

  const markRead = async (notification: ApiNotification) => {
    if (notification.readAt || mutating) return;
    const now = new Date().toISOString();
    setMutating(true);
    setItems((prev) =>
      prev.map((item) =>
        item.id === notification.id ? { ...item, readAt: now } : item,
      ),
    );
    try {
      await markNotificationRead(notification.id);
      notifyNotificationReadStateChanged();
    } catch {
      setItems((prev) =>
        prev.map((item) =>
          item.id === notification.id ? { ...item, readAt: null } : item,
        ),
      );
    } finally {
      setMutating(false);
    }
  };

  const markAll = async () => {
    if (mutating || !items.some((item) => !item.readAt)) return;
    const previousItems = items;
    const now = new Date().toISOString();
    setMutating(true);
    setItems((prev) =>
      prev.map((item) => (item.readAt ? item : { ...item, readAt: now })),
    );
    try {
      await markAllNotificationsRead();
      notifyNotificationReadStateChanged();
    } catch {
      setItems(previousItems);
    } finally {
      setMutating(false);
    }
  };

  const loadMore = () => {
    if (loadingMoreRef.current || !nextCursor) return;
    loadingMoreRef.current = true;
    setLoadingMore(true);
    getNotifications({ cursor: nextCursor })
      .then((page) => {
        setItems((prev) => [...prev, ...page.items]);
        setNextCursor(page.nextCursor);
      })
      .catch(() => {
        // Keep loaded rows visible and leave the control available to retry.
      })
      .finally(() => {
        loadingMoreRef.current = false;
        setLoadingMore(false);
      });
  };

  const hasUnread = items.some((item) => !item.readAt);

  return (
    <div className="mx-auto w-full max-w-2xl">
      <section className="overflow-hidden rounded-2xl border border-border bg-surface shadow-sm">
        <header className="flex items-center justify-between gap-3 border-b border-border px-4 py-3 sm:gap-4 sm:px-6 sm:py-4">
          <h1 className="min-w-0 truncate text-xl font-semibold text-foreground">
            {t("notifications.title")}
          </h1>
          <button
            type="button"
            onClick={() => void markAll()}
            disabled={!hasUnread || mutating}
            className="min-h-10 shrink-0 rounded-full px-2 text-end text-sm font-medium text-primary transition-colors hover:underline disabled:cursor-not-allowed disabled:text-muted-soft disabled:no-underline"
          >
            {t("notifications.markAllRead")}
          </button>
        </header>

        {status === "loading" ? (
          <p className="px-5 py-12 text-center text-sm text-muted">
            {t("feed.loadingMore")}
          </p>
        ) : null}

        {status === "error" ? (
          <div className="px-5 py-12 text-center">
            <p className="text-sm text-muted">{t("notifications.error")}</p>
            <button
              type="button"
              onClick={() => load()}
              className="mt-3 text-sm font-medium text-primary hover:underline"
            >
              {t("search.tryAgain")}
            </button>
          </div>
        ) : null}

        {status === "ready" && items.length === 0 ? (
          <p className="px-5 py-12 text-center text-sm text-muted-soft">
            {t("notifications.empty")}
          </p>
        ) : null}

        {items.length > 0 ? (
          <ul className="divide-y divide-border">
            {items.map((notification) => {
              const actorName =
                notification.actor?.displayName ?? t("notifications.someone");
              const destination = notificationDestination(notification);
              const rowClass = `flex w-full items-start gap-3 px-5 py-4 text-left transition-colors hover:bg-background sm:px-6 ${
                notification.readAt ? "" : "bg-background/60"
              }`;
              const row = (
                <>
                  <Image
                    src={notification.actor?.avatarUrl ?? FALLBACK_AVATAR}
                    alt={actorName}
                    width={48}
                    height={48}
                    unoptimized={Boolean(notification.actor?.avatarUrl)}
                    className="h-12 w-12 shrink-0 rounded-full object-cover"
                  />
                  <span className="min-w-0 flex-1">
                    <span className="block text-sm leading-5 text-foreground">
                      <span className="font-semibold">{actorName}</span>{" "}
                      {t(actionTextKey(notification.type))}
                    </span>
                    <span className="mt-1 block text-xs text-muted-soft">
                      {formatTimeAgo(notification.createdAt, locale)}
                    </span>
                  </span>
                  {notification.readAt ? null : (
                    <span className="mt-2 h-2.5 w-2.5 shrink-0 rounded-full bg-primary" />
                  )}
                </>
              );

              return (
                <li key={notification.id}>
                  {destination ? (
                    <Link
                      href={destination}
                      onClick={() => void markRead(notification)}
                      className={rowClass}
                    >
                      {row}
                    </Link>
                  ) : (
                    <button
                      type="button"
                      onClick={() => void markRead(notification)}
                      disabled={mutating}
                      className={rowClass}
                    >
                      {row}
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
            className="w-full border-t border-border px-5 py-3 text-sm text-muted transition-colors hover:text-foreground disabled:opacity-50"
          >
            {loadingMore ? t("feed.loadingMore") : t("feed.loadMore")}
          </button>
        ) : null}
      </section>
    </div>
  );
}
