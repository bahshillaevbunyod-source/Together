"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import Image from "next/image";
import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { MoreHorizontal, Ban } from "lucide-react";

import {
  ApiError,
  blockUser,
  followUser,
  getUserProfile,
  openConversation,
  unblockUser,
  unfollowUser,
  type PublicUserProfile,
} from "@/lib/api";

type Status = "loading" | "ready" | "notfound" | "error";

const FALLBACK_AVATAR =
  "data:image/svg+xml;utf8," +
  encodeURIComponent(
    '<svg xmlns="http://www.w3.org/2000/svg" width="96" height="96"><circle cx="48" cy="48" r="48" fill="#d4d4d8"/></svg>',
  );

function memberSince(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return d.toLocaleDateString(undefined, { month: "long", year: "numeric" });
}

export default function PublicProfilePage() {
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
        await unfollowUser(profile.username);
        setProfile((p) =>
          p
            ? {
                ...p,
                isFollowing: false,
                followersCount: Math.max(0, p.followersCount - 1),
              }
            : p,
        );
      } else {
        await followUser(profile.username);
        setProfile((p) =>
          p
            ? { ...p, isFollowing: true, followersCount: p.followersCount + 1 }
            : p,
        );
      }
    } catch (err) {
      // Preserve current state; show a safe message (never raw backend text).
      if (err instanceof ApiError && err.status === 401) {
        setFollowError("Please sign in to follow people.");
      } else if (err instanceof ApiError && err.status === 403) {
        setFollowError("You can’t follow this user.");
      } else {
        setFollowError("Couldn’t update follow status. Try again.");
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
        setMessageError("Please sign in to send messages.");
      } else if (err instanceof ApiError && err.status === 403) {
        setMessageError("You can’t message this user.");
      } else if (err instanceof ApiError && err.status === 404) {
        setMessageError("This user is no longer available.");
      } else {
        setMessageError("Couldn’t open conversation. Try again.");
      }
      setMessagePending(false); // stay on page to show the error
    }
  };

  const friendlyRelError = (err: unknown, fallback: string): string => {
    if (err instanceof ApiError && err.status === 401) {
      return "Please sign in first.";
    }
    if (err instanceof ApiError && err.status === 403) {
      return "You can’t do that with this user.";
    }
    if (err instanceof ApiError && err.status === 404) {
      return "This user is no longer available.";
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
      setBlockError(friendlyRelError(err, "Couldn’t block this user. Try again."));
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
        friendlyRelError(err, "Couldn’t unblock this user. Try again."),
      );
    } finally {
      setBlockPending(false);
    }
  };

  if (status === "loading") {
    return (
      <p className="py-16 text-center text-sm text-muted">Loading profile…</p>
    );
  }

  if (status === "notfound") {
    return (
      <div className="mx-auto max-w-2xl rounded-2xl border border-border bg-surface p-8 text-center shadow-sm">
        <p className="text-sm text-muted">This user doesn’t exist.</p>
      </div>
    );
  }

  if (status === "error" || !profile) {
    return (
      <div className="mx-auto max-w-2xl rounded-2xl border border-border bg-surface p-8 text-center shadow-sm">
        <p className="text-sm text-muted">Couldn’t load this profile.</p>
        <button
          type="button"
          onClick={() => load()}
          className="mt-3 rounded-full bg-primary px-4 py-1.5 text-sm text-white transition-colors hover:bg-primary-hover"
        >
          Try again
        </button>
      </div>
    );
  }

  const location = [profile.city, profile.countryCode]
    .filter((v) => v && v.trim().length > 0)
    .join(", ");
  const joined = memberSince(profile.createdAt);

  return (
    <div className="mx-auto max-w-2xl">
      {profile.isSelf ? (
        <div className="mb-3 flex items-center justify-between rounded-xl border border-border bg-surface px-4 py-2 text-sm">
          <span className="text-muted">This is your profile.</span>
          <Link href="/profile" className="text-primary hover:underline">
            Go to your profile
          </Link>
        </div>
      ) : null}

      <section className="rounded-2xl border border-border bg-surface p-6 shadow-sm">
        <div className="flex items-start gap-4">
          <Image
            src={profile.avatarUrl ?? FALLBACK_AVATAR}
            alt={profile.displayName}
            width={96}
            height={96}
            className="h-24 w-24 shrink-0 rounded-full object-cover"
          />
          <div className="min-w-0 flex-1">
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0">
                <h1 className="truncate text-xl font-bold tracking-tight text-foreground">
                  {profile.displayName}
                </h1>
                <div className="text-sm text-muted">@{profile.username}</div>
              </div>
              {!profile.isSelf && !profile.isBlocked ? (
                <div className="flex shrink-0 items-center gap-2">
                  <button
                    type="button"
                    onClick={onToggleFollow}
                    disabled={followPending}
                    aria-pressed={profile.isFollowing}
                    className={`rounded-full px-5 py-1.5 text-sm font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-50 ${
                      profile.isFollowing
                        ? "border border-border text-foreground hover:bg-background"
                        : "bg-primary text-white hover:bg-primary-hover"
                    }`}
                  >
                    {profile.isFollowing ? "Following" : "Follow"}
                  </button>
                  <button
                    type="button"
                    onClick={onMessage}
                    disabled={messagePending}
                    className="rounded-full border border-border px-5 py-1.5 text-sm font-medium text-foreground transition-colors hover:bg-background disabled:cursor-not-allowed disabled:opacity-50"
                  >
                    {messagePending ? "…" : "Message"}
                  </button>
                  <div className="relative" ref={menuRef}>
                    <button
                      type="button"
                      aria-label="More options"
                      aria-haspopup="menu"
                      aria-expanded={menuOpen}
                      onClick={() => setMenuOpen((v) => !v)}
                      className={`flex h-9 w-9 items-center justify-center rounded-full border border-border transition-colors hover:bg-background ${
                        menuOpen ? "text-foreground" : "text-muted"
                      }`}
                    >
                      <MoreHorizontal className="h-5 w-5" />
                    </button>
                    {menuOpen ? (
                      <div
                        role="menu"
                        className="absolute right-0 top-full z-20 mt-2 w-40 overflow-hidden rounded-xl border border-border bg-surface p-1 shadow-lg"
                      >
                        <button
                          type="button"
                          role="menuitem"
                          onClick={() => {
                            setMenuOpen(false);
                            setBlockError(null);
                            setConfirmBlock(true);
                          }}
                          className="flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-sm text-red-600 transition-colors hover:bg-red-50"
                        >
                          <Ban className="h-4 w-4 shrink-0" />
                          Block user
                        </button>
                      </div>
                    ) : null}
                  </div>
                </div>
              ) : null}
            </div>

            <div className="mt-3 flex items-center gap-5 text-sm">
              <span className="text-foreground">
                <span className="font-semibold">{profile.followersCount}</span>{" "}
                <span className="text-muted">Followers</span>
              </span>
              <span className="text-foreground">
                <span className="font-semibold">{profile.followingCount}</span>{" "}
                <span className="text-muted">Following</span>
              </span>
            </div>

            {followError || messageError ? (
              <p className="mt-2 text-xs text-red-500" role="alert">
                {followError ?? messageError}
              </p>
            ) : null}
          </div>
        </div>

        {/* Blocked state: a clean panel with the Unblock action. */}
        {profile.isBlocked ? (
          <div className="mt-5 flex items-center gap-3 rounded-xl border border-border bg-background px-4 py-3">
            <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-red-100 text-red-600">
              <Ban className="h-5 w-5" />
            </span>
            <div className="min-w-0 flex-1">
              <p className="text-sm font-semibold text-foreground">
                You blocked @{profile.username}
              </p>
              <p className="text-xs text-muted">
                They can’t follow or message you. Unblock to restore normal
                interactions.
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
              className="shrink-0 rounded-full border border-border bg-surface px-5 py-1.5 text-sm font-medium text-foreground transition-colors hover:bg-background disabled:cursor-not-allowed disabled:opacity-50"
            >
              {blockPending ? "Unblocking…" : "Unblock"}
            </button>
          </div>
        ) : null}

        {profile.bio ? (
          <p className="mt-4 text-sm leading-relaxed text-foreground">
            {profile.bio}
          </p>
        ) : null}

        <dl className="mt-4 flex flex-wrap gap-x-8 gap-y-2 border-t border-border pt-4 text-sm">
          {location ? (
            <div>
              <dt className="text-muted-soft">Location</dt>
              <dd className="text-foreground">{location}</dd>
            </div>
          ) : null}
          <div>
            <dt className="text-muted-soft">Native language</dt>
            <dd className="text-foreground">{profile.nativeLanguage}</dd>
          </div>
          {joined ? (
            <div>
              <dt className="text-muted-soft">Member since</dt>
              <dd className="text-foreground">{joined}</dd>
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
          aria-label="Block user"
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
              Block @{profile.username}?
            </h2>
            <p className="mt-1.5 text-sm leading-relaxed text-muted">
              They won’t be able to follow or message you, and you’ll unfollow
              each other. You can unblock them any time.
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
                Cancel
              </button>
              <button
                type="button"
                onClick={onConfirmBlock}
                disabled={blockPending}
                className="rounded-full bg-red-600 px-5 py-1.5 text-sm font-medium text-white transition-colors hover:bg-red-700 disabled:cursor-not-allowed disabled:opacity-50"
              >
                {blockPending ? "Blocking…" : "Block"}
              </button>
            </div>
          </div>
        </div>
      ) : null}
    </div>
  );
}
