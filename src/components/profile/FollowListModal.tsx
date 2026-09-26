"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import Image from "next/image";
import Link from "next/link";
import { X } from "lucide-react";

import {
  getFollowers,
  getFollowing,
  type FollowListItem,
} from "@/lib/api";
import { useLanguage, type TranslationKey } from "@/lib/language-context";

type Mode = "followers" | "following";
type Status = "loading" | "ready" | "error";

const FALLBACK_AVATAR =
  "data:image/svg+xml;utf8," +
  encodeURIComponent(
    '<svg xmlns="http://www.w3.org/2000/svg" width="40" height="40"><circle cx="20" cy="20" r="20" fill="#d4d4d8"/></svg>',
  );

// Translation keys per mode. Keyed on the stable `mode` id (not any display
// label), so logic never depends on translated/English copy.
const TITLE_KEY: Record<Mode, TranslationKey> = {
  followers: "profile.followers",
  following: "profile.following",
};

const EMPTY_KEY: Record<Mode, TranslationKey> = {
  followers: "profile.noFollowers",
  following: "profile.noFollowing",
};

const ERROR_KEY: Record<Mode, TranslationKey> = {
  followers: "profile.followListErrorFollowers",
  following: "profile.followListErrorFollowing",
};

/**
 * A dialog listing a user's followers or following. Reused by both the own and
 * public profile pages. Each row links to that user's profile and closes the
 * dialog on navigation. Backed by the public list endpoints (no auth needed).
 */
export function FollowListModal({
  username,
  mode,
  onClose,
}: {
  username: string;
  mode: Mode;
  onClose: () => void;
}) {
  const { t } = useLanguage();
  const [items, setItems] = useState<FollowListItem[]>([]);
  const [status, setStatus] = useState<Status>("loading");
  const [nextCursor, setNextCursor] = useState("");
  const [loadingMore, setLoadingMore] = useState(false);
  const loadingMoreRef = useRef(false);

  const fetcher = mode === "followers" ? getFollowers : getFollowing;

  const load = useCallback(
    (signal?: AbortSignal) => {
      setStatus("loading");
      fetcher(username, {}, signal)
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
    [fetcher, username],
  );

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
    fetcher(username, { cursor })
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

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4"
      role="dialog"
      aria-modal="true"
      aria-label={t(TITLE_KEY[mode])}
      onClick={onClose}
    >
      <div
        className="flex max-h-[80dvh] w-full max-w-md flex-col overflow-hidden rounded-2xl border border-border bg-surface shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between border-b border-border px-4 py-3">
          <h2 className="text-sm font-semibold text-foreground">
            {t(TITLE_KEY[mode])}
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
            <p className="px-4 py-10 text-center text-sm text-muted">{t("feed.loadingMore")}</p>
          ) : null}

          {status === "error" ? (
            <div className="px-4 py-10 text-center">
              <p className="text-sm text-muted">{t(ERROR_KEY[mode])}</p>
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
              {t(EMPTY_KEY[mode])}
            </p>
          ) : null}

          {items.length > 0 ? (
            <ul className="flex flex-col">
              {items.map((u) => (
                <li key={u.id}>
                  <Link
                    href={`/u/${encodeURIComponent(u.username)}`}
                    onClick={onClose}
                    className="flex items-center gap-3 px-4 py-3 transition-colors hover:bg-background focus:outline-none focus-visible:bg-background focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary"
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
