"use client";

import { useCallback, useEffect, useState } from "react";
import Image from "next/image";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { CalendarDays, Globe, MapPin } from "lucide-react";

import {
  getProfile,
  getUserProfile,
  type ProfileResponse,
} from "@/lib/api";
import { LANGUAGES } from "@/lib/languages";
import { useLanguage } from "@/lib/language-context";
import { FollowListModal } from "@/components/profile/FollowListModal";
import { FollowRequestsModal } from "@/components/profile/FollowRequestsModal";

type Status = "loading" | "ready" | "error";

const FALLBACK_AVATAR =
  "data:image/svg+xml;utf8," +
  encodeURIComponent(
    '<svg xmlns="http://www.w3.org/2000/svg" width="112" height="112"><circle cx="56" cy="56" r="56" fill="#d4d4d8"/></svg>',
  );

function memberSince(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return d.toLocaleDateString("en-US", { month: "long", year: "numeric" });
}

// Human-readable language name from a code (e.g. "en" -> "English"), rendered in
// the resolved platform UI locale so the label matches the rest of the UI. The
// stored nativeLanguage code is never changed.
function languageName(code: string, locale: string): string {
  if (!code) return "";
  try {
    const name = new Intl.DisplayNames([locale], { type: "language" }).of(
      code.toLowerCase(),
    );
    if (name && name.toLowerCase() !== code.toLowerCase()) return name;
  } catch {
    // fall through to the static list / raw code
  }
  const found = LANGUAGES.find((l) => l.code === code.toLowerCase());
  return found ? found.label : code;
}

// Human-readable country name from an ISO code (e.g. "UZ" -> "Uzbekistan").
function countryName(code: string): string {
  if (!code) return "";
  try {
    const name = new Intl.DisplayNames(["en"], { type: "region" }).of(
      code.toUpperCase(),
    );
    if (name && name !== code.toUpperCase()) return name;
  } catch {
    // fall through to the raw code
  }
  return code;
}

export default function ProfilePage() {
  const { t, locale } = useLanguage();
  const searchParams = useSearchParams();
  const [profile, setProfile] = useState<ProfileResponse | null>(null);
  const [followers, setFollowers] = useState<number | null>(null);
  const [following, setFollowing] = useState<number | null>(null);
  const [status, setStatus] = useState<Status>("loading");
  const [followModal, setFollowModal] = useState<
    "followers" | "following" | null
  >(null);
  const [requestsOpen, setRequestsOpen] = useState(false);

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

  useEffect(() => {
    if (profile?.isPrivate && searchParams.get("followRequests") === "1") {
      setRequestsOpen(true);
    }
  }, [profile?.isPrivate, searchParams]);

  if (status === "loading") {
    return (
      <p className="py-16 text-center text-sm text-muted">{t("profile.loading")}</p>
    );
  }

  if (status === "error" || !profile) {
    return (
      <div className="mx-auto max-w-2xl rounded-2xl border border-border bg-surface p-8 text-center shadow-sm">
        <p className="text-sm text-muted">{t("profile.loadError")}</p>
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

  const location = [profile.city, countryName(profile.countryCode ?? "")]
    .filter((v) => v && v.trim().length > 0)
    .join(", ");
  const joined = memberSince(profile.createdAt);
  const language = languageName(profile.nativeLanguage, locale);

  return (
    <div className="mx-auto max-w-2xl">
      <section className="rounded-2xl border border-border bg-surface p-5 shadow-sm sm:p-6">
        {/* Identity */}
        <div className="flex items-start gap-4 sm:gap-5">
          <Image
            src={profile.avatarUrl ?? FALLBACK_AVATAR}
            alt={profile.displayName}
            width={112}
            height={112}
            unoptimized={Boolean(profile.avatarUrl)}
            className="h-24 w-24 shrink-0 rounded-full object-cover sm:h-28 sm:w-28"
          />
          <div className="min-w-0 flex-1">
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0">
                <h1 className="truncate text-2xl font-bold leading-tight tracking-tight text-foreground">
                  {profile.displayName}
                </h1>
                <div className="truncate text-sm text-muted">
                  @{profile.username}
                </div>
              </div>
              <Link
                href="/profile/edit"
                className="shrink-0 rounded-full border border-border px-4 py-1.5 text-sm font-medium text-foreground transition-colors hover:bg-background"
              >
                {t("profile.edit")}
              </Link>
            </div>

            {/* Compact stats */}
            <div className="mt-3 flex items-center gap-6 text-sm">
              <button
                type="button"
                onClick={() => setFollowModal("followers")}
                className="transition-colors hover:text-primary"
              >
                <span className="font-semibold text-foreground">
                  {followers ?? "—"}
                </span>{" "}
                <span className="text-muted">{t("profile.followers")}</span>
              </button>
              <button
                type="button"
                onClick={() => setFollowModal("following")}
                className="transition-colors hover:text-primary"
              >
                <span className="font-semibold text-foreground">
                  {following ?? "—"}
                </span>{" "}
                <span className="text-muted">{t("profile.following")}</span>
              </button>
            </div>

            {/* Follow requests inbox entry — only for private accounts. */}
            {profile.isPrivate ? (
              <button
                type="button"
                onClick={() => setRequestsOpen(true)}
                className="mt-2 text-sm font-medium text-primary transition-colors hover:text-primary-hover"
              >
                {t("profile.followRequests")}
              </button>
            ) : null}
          </div>
        </div>

        {/* Bio, directly under identity */}
        {profile.bio ? (
          <p className="mt-4 whitespace-pre-line text-sm leading-relaxed text-foreground">
            {profile.bio}
          </p>
        ) : null}

        {/* Secondary info: subtle, compact, single wrapping line */}
        <div className="mt-4 flex flex-wrap items-center gap-x-5 gap-y-2 border-t border-border pt-4 text-sm text-muted">
          {location ? (
            <span className="flex items-center gap-1.5">
              <MapPin className="h-4 w-4 text-muted-soft" aria-hidden />
              {location}
            </span>
          ) : null}
          {language ? (
            <span className="flex items-center gap-1.5">
              <Globe className="h-4 w-4 text-muted-soft" aria-hidden />
              {language}
            </span>
          ) : null}
          {joined ? (
            <span className="flex items-center gap-1.5">
              <CalendarDays className="h-4 w-4 text-muted-soft" aria-hidden />
              {t("profile.joined", { date: joined })}
            </span>
          ) : null}
        </div>
      </section>

      {followModal ? (
        <FollowListModal
          username={profile.username}
          mode={followModal}
          onClose={() => setFollowModal(null)}
        />
      ) : null}

      {requestsOpen ? (
        <FollowRequestsModal
          onClose={() => setRequestsOpen(false)}
          onAccepted={() =>
            setFollowers((n) => (typeof n === "number" ? n + 1 : n))
          }
        />
      ) : null}
    </div>
  );
}
