"use client";

import { useState } from "react";
import Image from "next/image";
import Link from "next/link";
import { MapPin } from "lucide-react";

import { ApiError, followUser } from "@/lib/api";
import { formatCount } from "@/lib/format";

export interface PersonCardUser {
  id: string;
  username: string;
  displayName: string;
  avatarUrl: string | null;
  countryCode?: string | null;
  city?: string | null;
  nativeLanguage?: string | null;
  followerCount?: number;
}

const FALLBACK_AVATAR =
  "data:image/svg+xml;utf8," +
  encodeURIComponent(
    '<svg xmlns="http://www.w3.org/2000/svg" width="56" height="56"><circle cx="28" cy="28" r="28" fill="#d4d4d8"/></svg>',
  );

function countryName(code?: string | null): string {
  if (!code) return "";
  try {
    const n = new Intl.DisplayNames(["en"], { type: "region" }).of(
      code.toUpperCase(),
    );
    if (n && n !== code.toUpperCase()) return n;
  } catch {
    // fall through
  }
  return code;
}

function languageName(code?: string | null): string {
  if (!code) return "";
  try {
    const n = new Intl.DisplayNames(["en"], { type: "language" }).of(
      code.toLowerCase(),
    );
    if (n && n.toLowerCase() !== code.toLowerCase()) return n;
  } catch {
    // fall through
  }
  return code;
}

/**
 * A premium person card used across Discover (For You) and search results.
 * onFollowed lets a "recommendations" surface drop the card once followed so it
 * doesn't leave a stale suggestion; omit it where a plain toggle is wanted.
 */
export function PersonCard({
  user,
  onFollowed,
  showFollowers = false,
}: {
  user: PersonCardUser;
  onFollowed?: (id: string) => void;
  /** Show a subtle follower-count line (used by the Popular section). */
  showFollowers?: boolean;
}) {
  const [following, setFollowing] = useState(false);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState(false);

  const location = [user.city, countryName(user.countryCode)]
    .filter((v) => v && v.trim().length > 0)
    .join(", ");
  const language = languageName(user.nativeLanguage);
  const href = `/u/${encodeURIComponent(user.username)}`;

  const onFollow = async () => {
    if (pending || following) return;
    setPending(true);
    setError(false);
    try {
      await followUser(user.username);
      setFollowing(true);
      onFollowed?.(user.id);
    } catch (err) {
      // Ignore "already following" races; surface anything else quietly.
      if (!(err instanceof ApiError && err.status === 409)) setError(true);
    } finally {
      setPending(false);
    }
  };

  return (
    <article className="group flex items-center gap-3.5 rounded-2xl border border-border bg-surface p-4 transition-all hover:border-primary/30 hover:shadow-sm">
      <Link href={href} aria-label={`View ${user.displayName}'s profile`} className="shrink-0">
        <Image
          src={user.avatarUrl ?? FALLBACK_AVATAR}
          alt={user.displayName}
          width={56}
          height={56}
          unoptimized={Boolean(user.avatarUrl)}
          className="h-14 w-14 rounded-full object-cover transition-transform group-hover:scale-[1.03]"
        />
      </Link>

      <div className="min-w-0 flex-1">
        <Link
          href={href}
          className="block truncate text-sm font-semibold text-foreground hover:underline"
        >
          {user.displayName}
        </Link>
        <div className="truncate text-xs text-muted">@{user.username}</div>
        {location || language ? (
          <div className="mt-1 flex items-center gap-1.5 truncate text-xs text-muted-soft">
            {location ? (
              <>
                <MapPin className="h-3.5 w-3.5 shrink-0" aria-hidden />
                <span className="truncate">{location}</span>
              </>
            ) : null}
            {location && language ? <span aria-hidden>·</span> : null}
            {language ? <span className="truncate">{language}</span> : null}
          </div>
        ) : null}
        {showFollowers &&
        typeof user.followerCount === "number" &&
        user.followerCount > 0 ? (
          <div className="mt-0.5 text-xs text-muted-soft">
            {formatCount(user.followerCount)}{" "}
            {user.followerCount === 1 ? "follower" : "followers"}
          </div>
        ) : null}
      </div>

      <button
        type="button"
        onClick={onFollow}
        disabled={pending || following}
        aria-pressed={following}
        className={`shrink-0 rounded-full px-4 py-1.5 text-sm font-medium transition-colors disabled:cursor-not-allowed ${
          following
            ? "border border-border text-muted"
            : "bg-primary text-white hover:bg-primary-hover disabled:opacity-60"
        }`}
      >
        {following ? "Following" : pending ? "…" : error ? "Retry" : "Follow"}
      </button>
    </article>
  );
}
