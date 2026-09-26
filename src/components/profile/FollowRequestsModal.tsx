"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import Image from "next/image";
import Link from "next/link";
import { X } from "lucide-react";

import {
  acceptFollowRequest,
  declineFollowRequest,
  getFollowRequests,
  type FollowRequestItem,
} from "@/lib/api";
import { useLanguage } from "@/lib/language-context";

type Status = "loading" | "ready" | "error";

const FALLBACK_AVATAR =
  "data:image/svg+xml;utf8," +
  encodeURIComponent(
    '<svg xmlns="http://www.w3.org/2000/svg" width="40" height="40"><circle cx="20" cy="20" r="20" fill="#d4d4d8"/></svg>',
  );

/**
 * A dialog listing the authenticated (private) account's incoming follow
 * requests, with Accept / Decline actions. Accepting turns a requester into a
 * real follower (reported to the parent so the follower count can increment);
 * declining just removes the request. Mirrors FollowListModal's design.
 */
export function FollowRequestsModal({
  onClose,
  onAccepted,
}: {
  onClose: () => void;
  /** Called after a request is accepted, so the parent can bump its follower count. */
  onAccepted?: () => void;
}) {
  const { t } = useLanguage();
  const [items, setItems] = useState<FollowRequestItem[]>([]);
  const [status, setStatus] = useState<Status>("loading");
  const [nextCursor, setNextCursor] = useState("");
  const [loadingMore, setLoadingMore] = useState(false);
  const loadingMoreRef = useRef(false);
  // Usernames with an Accept/Decline request in flight, to disable their row.
  const [pending, setPending] = useState<Record<string, boolean>>({});

  const load = useCallback((signal?: AbortSignal) => {
    setStatus("loading");
    getFollowRequests({}, signal)
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

  // Close on Escape.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [onClose]);

  const loadMore = () => {
    if (loadingMoreRef.current) return;
    const cursor = nextCursor;
    if (!cursor) return;
    loadingMoreRef.current = true;
    setLoadingMore(true);
    getFollowRequests({ cursor })
      .then((page) => {
        setItems((prev) => [...prev, ...page.items]);
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

  const setRowPending = (username: string, value: boolean) =>
    setPending((prev) => {
      const next = { ...prev };
      if (value) next[username] = true;
      else delete next[username];
      return next;
    });

  const onAccept = async (username: string) => {
    if (pending[username]) return;
    setRowPending(username, true);
    try {
      await acceptFollowRequest(username);
      // Remove the accepted request from the inbox; it is now a real follower.
      setItems((prev) => prev.filter((r) => r.username !== username));
      onAccepted?.();
    } catch {
      // Leave the row in place so the user can retry.
    } finally {
      setRowPending(username, false);
    }
  };

  const onDecline = async (username: string) => {
    if (pending[username]) return;
    setRowPending(username, true);
    try {
      await declineFollowRequest(username);
      // Remove the declined request; no follower count change.
      setItems((prev) => prev.filter((r) => r.username !== username));
    } catch {
      // Leave the row in place so the user can retry.
    } finally {
      setRowPending(username, false);
    }
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4"
      role="dialog"
      aria-modal="true"
      aria-label={t("profile.followRequests")}
      onClick={onClose}
    >
      <div
        className="flex max-h-[80dvh] w-full max-w-md flex-col overflow-hidden rounded-2xl border border-border bg-surface shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between border-b border-border px-4 py-3">
          <h2 className="text-sm font-semibold text-foreground">
            {t("profile.followRequests")}
          </h2>
          <button
            type="button"
            aria-label={t("profile.close")}
            onClick={onClose}
            className="flex h-8 w-8 items-center justify-center rounded-full text-muted transition-colors hover:bg-background focus:outline-none focus-visible:ring-2 focus-visible:ring-primary"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        <div className="min-h-0 flex-1 overflow-y-auto">
          {status === "loading" ? (
            <p className="px-4 py-10 text-center text-sm text-muted">
              {t("feed.loadingMore")}
            </p>
          ) : null}

          {status === "error" ? (
            <div className="px-4 py-10 text-center">
              <p className="text-sm text-muted">
                {t("profile.followRequestsError")}
              </p>
              <button
                type="button"
                onClick={() => load()}
                className="mt-2 rounded text-sm text-primary hover:underline focus:outline-none focus-visible:ring-2 focus-visible:ring-primary"
              >
                {t("search.tryAgain")}
              </button>
            </div>
          ) : null}

          {status === "ready" && items.length === 0 ? (
            <p className="px-4 py-10 text-center text-sm text-muted-soft">
              {t("profile.noFollowRequests")}
            </p>
          ) : null}

          {items.length > 0 ? (
            <ul className="flex flex-col">
              {items.map((u) => (
                <li
                  key={u.id}
                  className="flex items-center gap-3 px-4 py-3"
                >
                  <Link
                    href={`/u/${encodeURIComponent(u.username)}`}
                    onClick={onClose}
                    className="flex min-w-0 flex-1 items-center gap-3 transition-colors hover:opacity-90 focus:outline-none"
                  >
                    <Image
                      src={u.avatarUrl ?? FALLBACK_AVATAR}
                      alt={u.displayName}
                      width={40}
                      height={40}
                      unoptimized={Boolean(u.avatarUrl)}
                      className="h-10 w-10 shrink-0 rounded-full object-cover"
                    />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-sm font-semibold text-foreground">
                        {u.displayName}
                      </span>
                      <span className="block truncate text-xs text-muted">
                        @{u.username}
                      </span>
                    </span>
                  </Link>
                  <div className="flex shrink-0 items-center gap-1.5">
                    <button
                      type="button"
                      onClick={() => onAccept(u.username)}
                      disabled={Boolean(pending[u.username])}
                      className="rounded-full bg-primary px-3 py-1 text-xs font-medium text-white transition-colors hover:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-50"
                    >
                      {t("profile.acceptRequest")}
                    </button>
                    <button
                      type="button"
                      onClick={() => onDecline(u.username)}
                      disabled={Boolean(pending[u.username])}
                      className="rounded-full border border-border px-3 py-1 text-xs font-medium text-foreground transition-colors hover:bg-background disabled:cursor-not-allowed disabled:opacity-50"
                    >
                      {t("profile.declineRequest")}
                    </button>
                  </div>
                </li>
              ))}
            </ul>
          ) : null}

          {nextCursor ? (
            <button
              type="button"
              onClick={loadMore}
              disabled={loadingMore}
              className="w-full border-t border-border px-4 py-2.5 text-center text-sm text-muted transition-colors hover:text-foreground focus:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary disabled:opacity-50"
            >
              {loadingMore ? t("feed.loadingMore") : t("feed.loadMore")}
            </button>
          ) : null}
        </div>
      </div>
    </div>
  );
}
