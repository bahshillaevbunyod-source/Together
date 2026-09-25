"use client";

import Image from "next/image";
import Link from "next/link";
import { Globe2, Lock, MapPin, Users, Video } from "lucide-react";

import type { ApiEvent, EventVisibility } from "@/lib/events-api";
import { formatInZone, sameDayInZone } from "@/lib/event-time";
import { useLanguage, type TranslationKey } from "@/lib/language-context";

export const FALLBACK_AVATAR =
  "data:image/svg+xml;utf8," +
  encodeURIComponent(
    '<svg xmlns="http://www.w3.org/2000/svg" width="44" height="44"><circle cx="22" cy="22" r="22" fill="#d4d4d8"/></svg>',
  );

export const VISIBILITY_META: Record<EventVisibility, { labelKey: TranslationKey; hintKey: TranslationKey; icon: typeof Globe2 }> = {
  public: { labelKey: "events.visibilityPublic", hintKey: "events.visibilityPublicHint", icon: Globe2 },
  followers: { labelKey: "events.visibilityFollowers", hintKey: "events.visibilityFollowersHint", icon: Users },
  private: { labelKey: "events.visibilityPrivate", hintKey: "events.visibilityPrivateHint", icon: Lock },
};

/** "Sat, Oct 12, 18:00 – 20:00 GMT+5" in the event's own timezone. */
export function formatEventWhen(e: Pick<ApiEvent, "startsAt" | "endsAt" | "timezone">, locale: string): string {
  const dateTime: Intl.DateTimeFormatOptions = { weekday: "short", month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" };
  const zoneName: Intl.DateTimeFormatOptions = { timeZoneName: "short" };
  if (!e.endsAt) return formatInZone(e.startsAt, locale, e.timezone, { ...dateTime, ...zoneName });
  const start = formatInZone(e.startsAt, locale, e.timezone, dateTime);
  const end = sameDayInZone(e.startsAt, e.endsAt, e.timezone)
    ? formatInZone(e.endsAt, locale, e.timezone, { hour: "2-digit", minute: "2-digit", ...zoneName })
    : formatInZone(e.endsAt, locale, e.timezone, { ...dateTime, ...zoneName });
  return `${start} – ${end}`;
}

/** Calendar tile: short month over the day number, in the event's timezone. */
export function EventDateTile({ startsAt, timezone, size = "md" }: { startsAt: string; timezone: string; size?: "md" | "lg" }) {
  const { locale } = useLanguage();
  const month = formatInZone(startsAt, locale, timezone, { month: "short" });
  const day = formatInZone(startsAt, locale, timezone, { day: "numeric" });
  const box = size === "lg" ? "h-16 w-16" : "h-14 w-14";
  return (
    <div
      aria-hidden
      className={`${box} flex shrink-0 flex-col items-center justify-center rounded-2xl bg-primary-soft text-primary ring-1 ring-primary/10`}
    >
      <span className="text-[11px] font-semibold uppercase leading-none tracking-wide">{month}</span>
      <span className={`${size === "lg" ? "text-2xl" : "text-xl"} mt-0.5 font-bold leading-none`}>{day}</span>
    </div>
  );
}

export function EventTypeLine({ event }: { event: Pick<ApiEvent, "eventType" | "locationName" | "locationAddress"> }) {
  const { t } = useLanguage();
  if (event.eventType === "online") {
    return (
      <span className="flex min-w-0 items-center gap-1.5">
        <Video className="h-3.5 w-3.5 shrink-0" aria-hidden />
        <span className="truncate">{t("events.online")}</span>
      </span>
    );
  }
  const place = event.locationName || event.locationAddress;
  return (
    <span className="flex min-w-0 items-center gap-1.5">
      <MapPin className="h-3.5 w-3.5 shrink-0" aria-hidden />
      <span className="truncate">{place || t("events.locationTba")}</span>
    </span>
  );
}

export function VisibilityChip({ visibility }: { visibility: EventVisibility }) {
  const { t } = useLanguage();
  const meta = VISIBILITY_META[visibility];
  const Icon = meta.icon;
  return (
    <span className="inline-flex items-center gap-1 rounded-full border border-border bg-background px-2 py-0.5 text-[11px] font-medium text-muted">
      <Icon className="h-3 w-3" aria-hidden />
      {t(meta.labelKey)}
    </span>
  );
}

/**
 * Event list card. List responses do not carry RSVP counts or the viewer's
 * RSVP, so the card shows only what the list genuinely knows.
 */
export function EventCard({ event, badge }: { event: ApiEvent; badge?: string }) {
  const { t, locale } = useLanguage();
  return (
    <Link
      href={`/events/${encodeURIComponent(event.id)}`}
      className="group flex gap-4 rounded-2xl border border-border bg-surface p-4 shadow-sm transition duration-200 hover:border-primary/30 hover:shadow-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/40 motion-safe:hover:-translate-y-0.5 motion-reduce:transition-none"
    >
      <EventDateTile startsAt={event.startsAt} timezone={event.timezone} />
      <div className="min-w-0 flex-1">
        <div className="flex items-start gap-2">
          <h3 className="line-clamp-2 min-w-0 flex-1 break-words text-[15px] font-semibold leading-snug text-foreground group-hover:text-primary">
            {event.title}
          </h3>
          {badge ? (
            <span className="shrink-0 rounded-full bg-primary px-2 py-0.5 text-[11px] font-semibold text-white">{badge}</span>
          ) : null}
        </div>
        <p className="mt-1 truncate text-xs font-medium text-primary">{formatEventWhen(event, locale)}</p>
        <div className="mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted">
          <EventTypeLine event={event} />
          {event.visibility !== "public" ? <VisibilityChip visibility={event.visibility} /> : null}
        </div>
        <div className="mt-2.5 flex items-center gap-2 text-xs text-muted">
          <Image
            src={event.creator.avatarUrl ?? FALLBACK_AVATAR}
            alt=""
            width={20}
            height={20}
            unoptimized={Boolean(event.creator.avatarUrl)}
            className="h-5 w-5 shrink-0 rounded-full object-cover"
          />
          <span className="truncate">{t("events.hostedBy", { name: event.creator.displayName })}</span>
        </div>
      </div>
    </Link>
  );
}
