"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import Image from "next/image";
import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { MoreHorizontal, Ban } from "lucide-react";

import {
  ApiError,
  blockUser,
  cancelFollowRequest,
  followUser,
  getUserProfile,
  openConversation,
  unblockUser,
  unfollowUser,
  type PublicUserProfile,
} from "@/lib/api";
import { FollowListModal } from "@/components/profile/FollowListModal";
import { useLanguage } from "@/lib/language-context";
import { formatMonthYear } from "@/lib/locale-format";
import { countryName, languageName } from "@/lib/world/world-api";

type Status = "loading" | "ready" | "notfound" | "error";

const FALLBACK_AVATAR =
  "data:image/svg+xml;utf8," +
  encodeURIComponent(
    '<svg xmlns="http://www.w3.org/2000/svg" width="96" height="96"><circle cx="48" cy="48" r="48" fill="#d4d4d8"/></svg>',
  );

export default function PublicProfilePage() {
  const { t, locale } = useLanguage();
  const params = useParams<{ username: string }>();
  const router = useRouter();
  const username =
    typeof params.username === "string" ? params.username : "";

  const [profile, setProfile] = useState<PublicUserProfile | null>(null);
  const [status, setStatus] = useState<Status>("loading");
  const [followPending, setFollowPending] = useState(false);
  const [followError, setFollowError] = useState<string | null>(null);
  const [messagePending, setMessagePending] = useState(false);
  const [messageError, setMessageError] = useState<string | null>(null);
  const [menuOpen, setMenuOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);
  const [confirmBlock, setConfirmBlock] = useState(false);
  const [blockPending, setBlockPending] = useState(false);
  const [blockError, setBlockError] = useState<string | null>(null);
  const [followModal, setFollowModal] = useState<
    "followers" | "following" | null
  >(null);

  // Close the options menu on outside click.
  useEffect(() => {
    if (!menuOpen) return;
    const onClick = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        setMenuOpen(false);
      }
    };
    document.addEventListener("mousedown", onClick);
    return () => document.removeEventListener("mousedown", onClick);
  }, [menuOpen]);

  // Close the block confirmation on Escape (unless a request is in flight).
  useEffect(() => {
    if (!confirmBlock) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape" && !blockPending) setConfirmBlock(false);
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [confirmBlock, blockPending]);

  const load = useCallback(
    (signal?: AbortSignal) => {
      if (!username) {
        setStatus("notfound");
        return;
      }
      setStatus("loading");
      getUserProfile(username, signal)
        .then((p) => {
          setProfile(p);
          setStatus("ready");
        })
        .catch((err) => {
          if (err instanceof DOMException && err.name === "AbortError") return;
          if (err instanceof ApiError && err.status === 404) {
            setStatus("notfound");
            return;
          }
          setStatus("error");
        });
    },
    [username],
  );

  useEffect(() => {
    const controller = new AbortController();
    load(controller.signal);
    return () => controller.abort();
  }, [load]);

  // Silently refresh the profile with authoritative server state (counts,
  // isFollowing, isBlocked) after a mutation — no loading flicker, since the
  // optimistic UI stays until the fresh data replaces it. A failed reconcile
  // leaves the optimistic state in place (the mutation itself already
  // succeeded, and a full refresh will correct it).
  const reconcileProfile = useCallback(async (uname: string) => {
    try {
      const fresh = await getUserProfile(uname);
      setProfile(fresh);
    } catch {
      // Keep optimistic state; the next full load/refresh will correct it.
    }
  }, []);

  const onToggleFollow = async () => {
    if (!profile || followPending) return; // prevent duplicate clicks
    setFollowPending(true);
    setFollowError(null);
    try {
      if (profile.isFollowing) {
        // Accepted follower: unfollow (follower count decreases).
        await unfollowUser(profile.username);
        setProfile((p) =>
          p
            ? {
                ...p,
                isFollowing: false,
                followRequested: false,
                followersCount: Math.max(0, p.followersCount - 1),
              }
            : p,
        );
      } else if (profile.followRequested) {
        // Pending request to a private account: cancel it. A pending request
        // never counted toward followers, so the count does not change.
        await cancelFollowRequest(profile.username);
        setProfile((p) => (p ? { ...p, followRequested: false } : p));
      } else {
        // Follow: public accounts become followers immediately (count +1);
        // private accounts return a pending request (no count change).
        const res = await followUser(profile.username);
        setProfile((p) => {
          if (!p) return p;
          if (res.following) {
            return {
              ...p,
              isFollowing: true,
              followRequested: false,
              followersCount: p.followersCount + 1,
            };
          }
          return { ...p, isFollowing: false, followRequested: res.requested };
        });
      }
    } catch (err) {
      // Preserve current state; show a safe message (never raw backend text).
      if (err instanceof ApiError && err.status === 401) {
        setFollowError(t("profile.followSignIn"));
      } else if (err instanceof ApiError && err.status === 403) {
        setFollowError(t("profile.followForbidden"));
      } else {
        setFollowError(t("profile.followError"));
      }
    } finally {
      setFollowPending(false);
    }
  };

  const onMessage = async () => {
    if (!profile || messagePending) return; // prevent duplicate clicks
    setMessagePending(true);
    setMessageError(null);
    try {
      // Reuses the idempotent open-conversation flow; no duplicate logic.
      const conv = await openConversation(profile.username);
      router.push(`/messages?c=${encodeURIComponent(conv.id)}`);
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setMessageError(t("profile.messageSignIn"));
      } else if (err instanceof ApiError && err.status === 403) {
        setMessageError(t("profile.messageForbidden"));
      } else if (err instanceof ApiError && err.status === 404) {
        setMessageError(t("profile.userUnavailable"));
      } else {
        setMessageError(t("profile.messageError"));
      }
      setMessagePending(false); // stay on page to show the error
    }
  };

  const friendlyRelError = (err: unknown, fallback: string): string => {
    if (err instanceof ApiError && err.status === 401) {
      return t("profile.relSignIn");
    }
    if (err instanceof ApiError && err.status === 403) {
      return t("profile.relForbidden");
    }
    if (err instanceof ApiError && err.status === 404) {
      return t("profile.userUnavailable");
    }
    return fallback;
  };

  const onConfirmBlock = async () => {
    if (!profile || blockPending) return;
    setBlockPending(true);
    setBlockError(null);
    try {
      await blockUser(profile.username);
      // Immediate responsive UI: blocking removes follow edges both ways.
      setProfile((p) => (p ? { ...p, isBlocked: true, isFollowing: false } : p));
      setConfirmBlock(false);
      // Reconcile with authoritative server state (counts, isFollowing,
      // isBlocked) instead of guessing.
      await reconcileProfile(profile.username);
    } catch (err) {
      setBlockError(friendlyRelError(err, t("profile.blockError")));
    } finally {
      setBlockPending(false);
    }
  };

  const onUnblock = async () => {
    if (!profile || blockPending) return;
    setBlockPending(true);
    setBlockError(null);
    try {
      await unblockUser(profile.username);
      // Immediate responsive UI; unblocking does not restore prior follow edges.
      setProfile((p) => (p ? { ...p, isBlocked: false } : p));
      // Reconcile with authoritative server state.
      await reconcileProfile(profile.username);
    } catch (err) {
      setBlockError(
        friendlyRelError(err, t("profile.unblockError")),
      );
    } finally {
      setBlockPending(false);
    }
  };

  if (status === "loading") {
    return (
      <p className="py-16 text-center text-sm text-muted">{t("profile.loading")}</p>
    );
  }

  if (status === "notfound") {
    return (
      <div className="mx-auto max-w-2xl rounded-2xl border border-border bg-surface p-8 text-center shadow-sm">
        <p className="text-sm text-muted">{t("profile.notFound")}</p>
      </div>
    );
  }

  if (status === "error" || !profile) {
    return (
      <div className="mx-auto max-w-2xl rounded-2xl border border-border bg-surface p-8 text-center shadow-sm">
        <p className="text-sm text-muted">{t("profile.publicLoadError")}</p>
        <button
          type="button"
          onClick={() => load()}
          className="mt-3 rounded-full bg-primary px-4 py-1.5 text-sm text-white transition-colors hover:bg-primary-hover"
        >
          {t("search.tryAgain")}
        </button>
      </div>
    );
  }

  const location = [
    profile.city,
    profile.countryCode ? countryName(profile.countryCode.toUpperCase(), locale) : null,
  ]
    .filter((v) => v && v.trim().length > 0)
    .join(", ");
  const joined = formatMonthYear(profile.createdAt, locale);

  return (
    <div className="mx-auto max-w-2xl">
      {profile.isSelf ? (
        <div className="mb-3 flex flex-wrap items-center justify-between gap-x-3 gap-y-1 rounded-xl border border-border bg-surface px-4 py-2 text-sm">
          <span className="text-muted">{t("profile.selfBanner")}</span>
          <Link href="/profile" className="text-primary hover:underline">
            {t("profile.goToYourProfile")}
          </Link>
        </div>
      ) : null}

      <section className="@container rounded-2xl border border-border bg-surface p-4 shadow-sm sm:p-6">
        {/* One responsive grid (each control rendered once): narrow cards
            stack identity, actions, stats; once the card itself is wide enough
            (container query, so the desktop right rail is accounted for) the
            actions sit right of the name and the stats under it. */}
        <div className="grid grid-cols-[auto_minmax(0,1fr)] items-center gap-x-4 gap-y-3 @xl:grid-cols-[auto_minmax(0,1fr)_auto] @xl:items-start @xl:gap-y-0">
          <Image
            src={profile.avatarUrl ?? FALLBACK_AVATAR}
            alt={profile.displayName}
            width={96}
            height={96}
            unoptimized={Boolean(profile.avatarUrl)}
            className="h-20 w-20 shrink-0 rounded-full object-cover sm:h-24 sm:w-24 @xl:row-span-2"
          />
          <div className="min-w-0">
            <h1 className="line-clamp-2 break-words text-xl font-bold leading-tight tracking-tight text-foreground">
              {profile.displayName}
            </h1>
            <div className="truncate text-sm text-muted">@{profile.username}</div>
          </div>

          {!profile.isSelf && !profile.isBlocked ? (
            <div className="col-span-2 grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] gap-2 @xl:col-span-1 @xl:col-start-3 @xl:row-start-1 @xl:flex @xl:items-center">
              <button
                type="button"
                onClick={onToggleFollow}
                disabled={followPending}
                aria-pressed={profile.isFollowing || profile.followRequested}
                className={`h-11 min-w-0 truncate rounded-full px-4 text-sm font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-50 sm:h-9 sm:px-5 ${
                  profile.isFollowing || profile.followRequested
                    ? "border border-border text-foreground hover:bg-background"
                    : "bg-primary text-white hover:bg-primary-hover"
                }`}
              >
                {profile.isFollowing
                  ? t("profile.followingState")
                  : profile.followRequested
                    ? t("profile.requested")
                    : t("profile.follow")}
              </button>
              <button
                type="button"
                onClick={onMessage}
                disabled={messagePending}
                className="h-11 min-w-0 truncate rounded-full border border-border px-4 text-sm font-medium text-foreground transition-colors hover:bg-background disabled:cursor-not-allowed disabled:opacity-50 sm:h-9 sm:px-5"
              >
                {messagePending ? "…" : t("profile.message")}
              </button>
              <div className="relative" ref={menuRef}>
                <button
                  type="button"
                  aria-label={t("profile.moreOptions")}
                  aria-haspopup="menu"
                  aria-expanded={menuOpen}
                  onClick={() => setMenuOpen((v) => !v)}
                  className={`flex h-11 w-11 items-center justify-center rounded-full border border-border transition-colors hover:bg-background sm:h-9 sm:w-9 ${
                    menuOpen ? "text-foreground" : "text-muted"
                  }`}
                >
                  <MoreHorizontal className="h-5 w-5" />
                </button>
                {menuOpen ? (
                  <div
                    role="menu"
                    className="absolute end-0 top-full z-20 mt-2 w-44 overflow-hidden rounded-xl border border-border bg-surface p-1 shadow-lg"
                  >
                    <button
                      type="button"
                      role="menuitem"
                      onClick={() => {
                        setMenuOpen(false);
                        setBlockError(null);
                        setConfirmBlock(true);
                      }}
                      className="flex min-h-11 w-full items-center gap-2.5 rounded-lg px-3 py-2 text-start text-sm text-red-600 transition-colors hover:bg-red-50"
                    >
                      <Ban className="h-4 w-4 shrink-0" />
                      {t("profile.blockUser")}
                    </button>
                  </div>
                ) : null}
              </div>
            </div>
          ) : null}

          {/* Stats: tappable blocks on phones, inline text from sm. */}
          <div className="col-span-2 grid grid-cols-2 gap-2 text-sm sm:flex sm:items-center sm:gap-5 @xl:col-start-2 @xl:mt-3">
            <button
              type="button"
              onClick={() => setFollowModal("followers")}
              className="flex min-h-14 min-w-0 flex-col items-center justify-center rounded-xl bg-background px-2 py-2 text-foreground transition-colors hover:bg-primary-soft/60 sm:min-h-0 sm:flex-row sm:gap-1 sm:bg-transparent sm:p-0 sm:hover:bg-transparent sm:hover:text-primary"
            >
              <span className="text-base font-semibold leading-tight sm:text-sm">{profile.followersCount}</span>
              <span className="max-w-full truncate text-xs text-muted sm:text-sm">{t("profile.followers")}</span>
            </button>
            <button
              type="button"
              onClick={() => setFollowModal("following")}
              className="flex min-h-14 min-w-0 flex-col items-center justify-center rounded-xl bg-background px-2 py-2 text-foreground transition-colors hover:bg-primary-soft/60 sm:min-h-0 sm:flex-row sm:gap-1 sm:bg-transparent sm:p-0 sm:hover:bg-transparent sm:hover:text-primary"
            >
              <span className="text-base font-semibold leading-tight sm:text-sm">{profile.followingCount}</span>
              <span className="max-w-full truncate text-xs text-muted sm:text-sm">{t("profile.following")}</span>
            </button>
          </div>

          {followError || messageError ? (
            <p className="col-span-2 break-words text-xs text-red-500 @xl:col-start-2 @xl:mt-2" role="alert">
              {followError ?? messageError}
            </p>
          ) : null}
        </div>

        {/* Blocked state: a clean panel with the Unblock action. */}
        {profile.isBlocked ? (
          <div className="mt-5 flex flex-wrap items-center gap-3 rounded-xl border border-border bg-background px-4 py-3 sm:flex-nowrap">
            <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-red-100 text-red-600">
              <Ban className="h-5 w-5" />
            </span>
            <div className="min-w-0 flex-1">
              <p className="break-words text-sm font-semibold text-foreground">
                {t("profile.blockedHeading", { name: profile.username })}
              </p>
              <p className="text-xs text-muted">
                {t("profile.blockedBody")}
              </p>
              {blockError ? (
                <p className="mt-1 text-xs text-red-500" role="alert">
                  {blockError}
                </p>
              ) : null}
            </div>
            <button
              type="button"
              onClick={onUnblock}
              disabled={blockPending}
              className="h-11 w-full shrink-0 rounded-full border border-border bg-surface px-5 text-sm font-medium text-foreground sm:h-9 sm:w-auto transition-colors hover:bg-background disabled:cursor-not-allowed disabled:opacity-50"
            >
              {blockPending ? t("profile.unblocking") : t("profile.unblock")}
            </button>
          </div>
        ) : null}

        {profile.bio ? (
          <p className="mt-4 whitespace-pre-line break-words text-sm leading-relaxed text-foreground">
            {profile.bio}
          </p>
        ) : null}

        <dl className="mt-4 flex flex-col gap-2 border-t border-border pt-4 text-sm sm:flex-row sm:flex-wrap sm:gap-x-8">
          {location ? (
            <div className="min-w-0">
              <dt className="text-muted-soft">{t("profile.location")}</dt>
              <dd className="break-words text-foreground">{location}</dd>
            </div>
          ) : null}
          <div>
            <dt className="text-muted-soft">{t("profile.nativeLanguage")}</dt>
            <dd className="break-words text-foreground">{languageName(profile.nativeLanguage, locale)}</dd>
          </div>
          {joined ? (
            <div className="min-w-0">
              <dt className="text-muted-soft">{t("profile.memberSince")}</dt>
              <dd className="break-words text-foreground">{joined}</dd>
            </div>
          ) : null}
        </dl>
      </section>

      {/* Block confirmation */}
      {confirmBlock ? (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4"
          role="dialog"
          aria-modal="true"
          aria-label={t("profile.blockUser")}
          onClick={() => {
            if (!blockPending) setConfirmBlock(false);
          }}
        >
          <div
            className="w-full max-w-sm rounded-2xl border border-border bg-surface p-6 shadow-xl"
            onClick={(e) => e.stopPropagation()}
          >
            <span className="flex h-12 w-12 items-center justify-center rounded-full bg-red-100 text-red-600">
              <Ban className="h-6 w-6" />
            </span>
            <h2 className="mt-4 text-lg font-semibold text-foreground">
              {t("profile.blockConfirmTitle", { name: profile.username })}
            </h2>
            <p className="mt-1.5 text-sm leading-relaxed text-muted">
              {t("profile.blockConfirmBody")}
            </p>
            {blockError ? (
              <p className="mt-2 text-xs text-red-500" role="alert">
                {blockError}
              </p>
            ) : null}
            <div className="mt-4 flex items-center justify-end gap-2">
              <button
                type="button"
                onClick={() => setConfirmBlock(false)}
                disabled={blockPending}
                className="rounded-full px-4 py-1.5 text-sm font-medium text-muted transition-colors hover:bg-background disabled:opacity-50"
              >
                {t("post.cancel")}
              </button>
              <button
                type="button"
                onClick={onConfirmBlock}
                disabled={blockPending}
                className="rounded-full bg-red-600 px-5 py-1.5 text-sm font-medium text-white transition-colors hover:bg-red-700 disabled:cursor-not-allowed disabled:opacity-50"
              >
                {blockPending ? t("profile.blocking") : t("profile.block")}
              </button>
            </div>
          </div>
        </div>
      ) : null}

      {followModal ? (
        <FollowListModal
          username={profile.username}
          mode={followModal}
          onClose={() => setFollowModal(null)}
        />
      ) : null}
    </div>
  );
}
