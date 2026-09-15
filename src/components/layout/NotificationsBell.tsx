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

type Status = "idle" | "loading" | "ready" | "error";

// Neutral fallback avatar (a gray circle) used when an actor has no avatar.
const FALLBACK_AVATAR =
  "data:image/svg+xml;utf8," +
  encodeURIComponent(
    '<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"><circle cx="16" cy="16" r="16" fill="#d4d4d8"/></svg>',
  );

// Human-readable action text per notification type.
function actionText(type: string): string {
  switch (type) {
    case "follow":
      return "started following you";
    case "post_like":
      return "liked your post";
    case "post_comment":
      return "commented on your post";
    default:
      return "sent you a notification";
  }
}

export function NotificationsBell() {
  const [open, setOpen] = useState(false);
  const [items, setItems] = useState<ApiNotification[]>([]);
  const [status, setStatus] = useState<Status>("idle");
  const [nextCursor, setNextCursor] = useState("");
  const [loadingMore, setLoadingMore] = useState(false);
  const [unread, setUnread] = useState(0);

  const panelRef = useRef<HTMLDivElement>(null);

  // Initial unread count.
  useEffect(() => {
    const controller = new AbortController();
    getUnreadNotificationCount(controller.signal)
      .then((r) => setUnread(r.unreadCount))
      .catch((err) => {
        if (err instanceof DOMException && err.name === "AbortError") return;
        // Leave the badge hidden on failure.
      });
    return () => controller.abort();
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
    if (!nextCursor || loadingMore) return;
    setLoadingMore(true);
    getNotifications({ cursor: nextCursor })
      .then((page) => {
        setItems((prev) => [...prev, ...page.items]);
        setNextCursor(page.nextCursor);
      })
      .catch(() => {
        // Keep what we have.
      })
      .finally(() => setLoadingMore(false));
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
        aria-label="Notifications"
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
          className="absolute right-0 top-full z-30 mt-2 w-80 overflow-hidden rounded-xl border border-border bg-surface shadow-lg"
        >
          <div className="flex items-center justify-between border-b border-border px-4 py-3">
            <span className="text-sm font-semibold text-foreground">
              Notifications
            </span>
            <button
              type="button"
              onClick={markAll}
              disabled={unread === 0}
              className="text-xs text-primary transition-colors hover:underline disabled:cursor-not-allowed disabled:text-muted-soft disabled:no-underline"
            >
              Mark all as read
            </button>
          </div>

          <div className="max-h-96 overflow-y-auto">
            {status === "loading" ? (
              <p className="px-4 py-6 text-center text-sm text-muted">
                Loading…
              </p>
            ) : null}

            {status === "error" ? (
              <div className="px-4 py-6 text-center">
                <p className="text-sm text-muted">Couldn’t load notifications.</p>
                <button
                  type="button"
                  onClick={() => load()}
                  className="mt-2 text-sm text-primary hover:underline"
                >
                  Try again
                </button>
              </div>
            ) : null}

            {status === "ready" && items.length === 0 ? (
              <p className="px-4 py-6 text-center text-sm text-muted-soft">
                No notifications yet.
              </p>
            ) : null}

            {items.length > 0 ? (
              <ul className="flex flex-col">
                {items.map((n) => {
                  const actorName = n.actor?.displayName ?? "Someone";
                  // Follow notifications with a known actor link to that
                  // profile; other types keep the mark-read-only button.
                  const canNavigate =
                    n.type === "follow" && !!n.actor?.username;
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
                        className="mt-1 h-8 w-8 shrink-0 rounded-full object-cover"
                      />
                      <span className="min-w-0 flex-1">
                        <span className="text-sm text-foreground">
                          <span className="font-semibold">{actorName}</span>{" "}
                          {actionText(n.type)}
                        </span>
                        <span className="mt-0.5 block text-xs text-muted-soft">
                          {formatTimeAgo(n.createdAt)}
                        </span>
                      </span>
                      {n.readAt ? null : (
                        <span className="mt-2 h-2 w-2 shrink-0 rounded-full bg-primary" />
                      )}
                    </>
                  );

                  return (
                    <li key={n.id}>
                      {canNavigate ? (
                        <Link
                          href={`/u/${encodeURIComponent(n.actor!.username)}`}
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
                {loadingMore ? "Loading…" : "Load more"}
              </button>
            ) : null}
          </div>
        </div>
      ) : null}
    </div>
  );
}
