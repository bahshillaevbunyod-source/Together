"use client";

import { useCallback, useEffect, useState } from "react";
import Image from "next/image";
import Link from "next/link";

import {
  getProfile,
  getUserProfile,
  type ProfileResponse,
} from "@/lib/api";

type Status = "loading" | "ready" | "error";

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

export default function ProfilePage() {
  const [profile, setProfile] = useState<ProfileResponse | null>(null);
  const [followers, setFollowers] = useState<number | null>(null);
  const [following, setFollowing] = useState<number | null>(null);
  const [status, setStatus] = useState<Status>("loading");

  const load = useCallback((signal?: AbortSignal) => {
    setStatus("loading");
    getProfile(signal)
      .then((p) => {
        setProfile(p);
        setStatus("ready");
        // Counts come from the public endpoint; failure here is non-fatal.
        getUserProfile(p.username, signal)
          .then((pub) => {
            setFollowers(pub.followersCount);
            setFollowing(pub.followingCount);
          })
          .catch(() => {
            setFollowers(null);
            setFollowing(null);
          });
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

  if (status === "loading") {
    return (
      <p className="py-16 text-center text-sm text-muted">Loading profile…</p>
    );
  }

  if (status === "error" || !profile) {
    return (
      <div className="mx-auto max-w-2xl rounded-2xl border border-border bg-surface p-8 text-center shadow-sm">
        <p className="text-sm text-muted">Couldn’t load your profile.</p>
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
              <Link
                href="/profile/edit"
                className="shrink-0 rounded-full border border-border px-4 py-1.5 text-sm text-muted transition-colors hover:bg-background hover:text-foreground"
              >
                Edit profile
              </Link>
            </div>

            {/* Follower / following counts */}
            <div className="mt-3 flex items-center gap-5 text-sm">
              <span className="text-foreground">
                <span className="font-semibold">
                  {followers ?? "—"}
                </span>{" "}
                <span className="text-muted">Followers</span>
              </span>
              <span className="text-foreground">
                <span className="font-semibold">
                  {following ?? "—"}
                </span>{" "}
                <span className="text-muted">Following</span>
              </span>
            </div>
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
