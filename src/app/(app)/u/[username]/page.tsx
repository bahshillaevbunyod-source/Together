"use client";

import { useCallback, useEffect, useState } from "react";
import Image from "next/image";
import Link from "next/link";
import { useParams, useRouter } from "next/navigation";

import {
  ApiError,
  followUser,
  getUserProfile,
  openConversation,
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
              {!profile.isSelf ? (
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
    </div>
  );
}
