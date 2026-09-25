"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import Image from "next/image";
import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import {
  ArrowLeft,
  CalendarDays,
  Check,
  ExternalLink,
  MapPin,
  Pencil,
  Star,
  Trash2,
  Video,
} from "lucide-react";

import { ApiError } from "@/lib/api";
import {
  deleteEvent,
  getEvent,
  getEventAttendees,
  removeEventRsvp,
  setEventRsvp,
  type ApiEvent,
  type ApiEventAttendee,
  type RsvpStatus,
} from "@/lib/events-api";
import { browserTimeZone, formatInZone, isSafeHttpUrl } from "@/lib/event-time";
import { useLanguage } from "@/lib/language-context";
import { ConfirmDialog } from "@/components/community/ConfirmDialog";
import {
  EventDateTile,
  FALLBACK_AVATAR,
  VisibilityChip,
  formatEventWhen,
} from "@/components/events/EventCard";
import { EventForm } from "@/components/events/EventForm";

type Status = "loading" | "ready" | "notfound" | "error";

export default function EventDetailPage() {
  const params = useParams<{ eventId: string }>();
  const eventId = params.eventId;
  const router = useRouter();
  const { t, locale } = useLanguage();

  const [event, setEvent] = useState<ApiEvent | null>(null);
  const [status, setStatus] = useState<Status>("loading");
  const [rsvpPending, setRsvpPending] = useState(false);
  const [rsvpError, setRsvpError] = useState(false);
  const [editing, setEditing] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);
  // Bumped after RSVP changes so the attendee list reflects them.
  const [attendeesVersion, setAttendeesVersion] = useState(0);

  const load = useCallback(
    (signal?: AbortSignal) => {
      setStatus("loading");
      getEvent(eventId, signal)
        .then((e) => {
          setEvent(e);
          setStatus("ready");
        })
        .catch((err) => {
          if (err instanceof DOMException && err.name === "AbortError") return;
          // 404 covers missing, private, followers-only and blocked alike.
          setStatus(err instanceof ApiError && (err.status === 404 || err.status === 400) ? "notfound" : "error");
        });
    },
    [eventId],
  );

  useEffect(() => {
    const controller = new AbortController();
    load(controller.signal);
    return () => controller.abort();
  }, [load]);

  const changeRsvp = async (next: RsvpStatus | null) => {
    if (!event || rsvpPending) return;
    setRsvpPending(true);
    setRsvpError(false);
    try {
      const updated = next ? await setEventRsvp(event.id, next) : await removeEventRsvp(event.id);
      setEvent(updated);
      setAttendeesVersion((v) => v + 1);
    } catch (err) {
      if (err instanceof ApiError && err.status === 404) setStatus("notfound");
      else setRsvpError(true);
    } finally {
      setRsvpPending(false);
    }
  };

  const onDelete = async () => {
    if (!event || deleting) return;
    setDeleting(true);
    setDeleteError(null);
    try {
      await deleteEvent(event.id);
      router.push("/events");
    } catch {
      setDeleteError(t("events.deleteError"));
      setDeleting(false);
    }
  };

  const back = (
    <Link
      href="/events"
      className="inline-flex items-center gap-1.5 self-start rounded-full px-2 py-1 text-sm font-medium text-muted transition-colors hover:bg-surface hover:text-foreground"
    >
      <ArrowLeft className="h-4 w-4" aria-hidden />
      {t("events.back")}
    </Link>
  );

  if (status !== "ready" || !event) {
    return (
      <div className="mx-auto flex w-full max-w-2xl flex-col gap-4">
        {back}
        {status === "loading" ? (
          <div className="rounded-3xl border border-border bg-surface p-6" aria-busy="true" aria-label={t("events.loading")}>
            <div className="flex gap-4">
              <div className="h-16 w-16 rounded-2xl bg-background motion-safe:animate-pulse" />
              <div className="flex flex-1 flex-col gap-2 pt-1">
                <div className="h-5 w-3/4 rounded bg-background motion-safe:animate-pulse" />
                <div className="h-3 w-1/2 rounded bg-background motion-safe:animate-pulse" />
              </div>
            </div>
            <div className="mt-6 h-20 rounded-xl bg-background motion-safe:animate-pulse" />
          </div>
        ) : (
          <div className="flex flex-col items-center rounded-3xl border border-border bg-surface px-6 py-12 text-center shadow-sm">
            <span className="flex h-12 w-12 items-center justify-center rounded-2xl bg-primary-soft text-primary">
              <CalendarDays className="h-6 w-6" aria-hidden />
            </span>
            <p className="mt-3 max-w-xs text-sm text-muted">
              {status === "notfound" ? t("events.notFound") : t("events.loadError")}
            </p>
            {status === "error" ? (
              <button type="button" onClick={() => load()} className="mt-2 text-sm text-primary hover:underline">
                {t("search.tryAgain")}
              </button>
            ) : null}
          </div>
        )}
      </div>
    );
  }

  const now = Date.now();
  const startMs = new Date(event.startsAt).getTime();
  const endMs = event.endsAt ? new Date(event.endsAt).getTime() : null;
  const ended = endMs !== null ? endMs < now : startMs < now;
  const live = !ended && startMs <= now;
  const viewerZone = browserTimeZone();
  const showLocalTime = viewerZone !== event.timezone;
  const localTime = showLocalTime
    ? formatInZone(event.startsAt, locale, viewerZone, {
        weekday: "short",
        month: "short",
        day: "numeric",
        hour: "2-digit",
        minute: "2-digit",
        timeZoneName: "short",
      })
    : "";
  const safeUrl = event.onlineUrl && isSafeHttpUrl(event.onlineUrl) ? event.onlineUrl : null;
  const isOwner = event.permissions.canEdit || event.permissions.canDelete;

  const rsvpButton = (value: RsvpStatus, Icon: typeof Check, label: string) => {
    const active = event.viewerRsvp === value;
    return (
      <button
        type="button"
        aria-pressed={active}
        disabled={rsvpPending}
        onClick={() => changeRsvp(active ? null : value)}
        className={`inline-flex h-10 flex-1 items-center justify-center gap-1.5 rounded-full border px-4 text-sm font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-60 sm:flex-none ${
          active
            ? value === "going"
              ? "border-primary bg-primary text-white hover:bg-primary-hover"
              : "border-primary bg-primary-soft text-primary"
            : "border-border bg-surface text-foreground hover:border-primary/40 hover:bg-primary-soft/50"
        }`}
      >
        <Icon className="h-4 w-4" aria-hidden />
        {label}
      </button>
    );
  };

  return (
    <div className="mx-auto flex w-full max-w-2xl flex-col gap-4">
      {back}

      <article className="overflow-hidden rounded-3xl border border-border bg-surface shadow-sm">
        <div className="bg-gradient-to-br from-primary-soft/80 via-surface to-surface px-5 pb-5 pt-6 sm:px-6">
          <div className="flex items-start gap-4">
            <EventDateTile startsAt={event.startsAt} timezone={event.timezone} size="lg" />
            <div className="min-w-0 flex-1">
              <div className="flex flex-wrap items-center gap-1.5">
                <VisibilityChip visibility={event.visibility} />
                {ended ? (
                  <span className="rounded-full bg-background px-2 py-0.5 text-[11px] font-medium text-muted">{t("events.ended")}</span>
                ) : live ? (
                  <span className="inline-flex items-center gap-1 rounded-full bg-emerald-500/10 px-2 py-0.5 text-[11px] font-medium text-emerald-600">
                    <span className="h-1.5 w-1.5 rounded-full bg-emerald-500 motion-safe:animate-pulse" />
                    {t("events.happeningNow")}
                  </span>
                ) : null}
              </div>
              <h1 className="mt-1.5 break-words text-xl font-semibold leading-tight text-foreground sm:text-2xl">{event.title}</h1>
              <Link
                href={`/u/${encodeURIComponent(event.creator.username)}`}
                className="mt-2 inline-flex max-w-full items-center gap-2 rounded-full text-sm text-muted hover:text-foreground"
              >
                <Image
                  src={event.creator.avatarUrl ?? FALLBACK_AVATAR}
                  alt=""
                  width={24}
                  height={24}
                  unoptimized={Boolean(event.creator.avatarUrl)}
                  className="h-6 w-6 shrink-0 rounded-full object-cover"
                />
                <span className="truncate">{t("events.hostedBy", { name: event.creator.displayName })}</span>
              </Link>
            </div>
          </div>

          {isOwner ? (
            <div className="mt-4 flex flex-wrap gap-2">
              {event.permissions.canEdit ? (
                <button
                  type="button"
                  onClick={() => setEditing(true)}
                  className="inline-flex items-center gap-1.5 rounded-full border border-border bg-surface px-3.5 py-1.5 text-sm font-medium text-foreground transition-colors hover:bg-background"
                >
                  <Pencil className="h-3.5 w-3.5" aria-hidden />
                  {t("post.edit")}
                </button>
              ) : null}
              {event.permissions.canDelete ? (
                <button
                  type="button"
                  onClick={() => {
                    setDeleteError(null);
                    setConfirmDelete(true);
                  }}
                  className="inline-flex items-center gap-1.5 rounded-full border border-border bg-surface px-3.5 py-1.5 text-sm font-medium text-red-600 transition-colors hover:bg-red-50 dark:hover:bg-red-500/10"
                >
                  <Trash2 className="h-3.5 w-3.5" aria-hidden />
                  {t("post.delete")}
                </button>
              ) : null}
            </div>
          ) : null}
        </div>

        <dl className="flex flex-col gap-4 border-t border-border px-5 py-5 sm:px-6">
          <div className="flex gap-3">
            <dt className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-background text-muted">
              <CalendarDays className="h-4 w-4" aria-hidden />
              <span className="sr-only">{t("events.sectionWhen")}</span>
            </dt>
            <dd className="min-w-0 pt-0.5">
              <p className="text-sm font-medium text-foreground">{formatEventWhen(event, locale)}</p>
              <p className="mt-0.5 text-xs text-muted">{event.timezone.replace(/_/g, " ")}</p>
              {showLocalTime ? <p className="mt-0.5 text-xs text-muted">{t("events.yourTime", { time: localTime })}</p> : null}
            </dd>
          </div>

          <div className="flex gap-3">
            <dt className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-background text-muted">
              {event.eventType === "online" ? <Video className="h-4 w-4" aria-hidden /> : <MapPin className="h-4 w-4" aria-hidden />}
              <span className="sr-only">{t("events.sectionWhere")}</span>
            </dt>
            <dd className="min-w-0 pt-0.5">
              {event.eventType === "online" ? (
                <>
                  <p className="text-sm font-medium text-foreground">{t("events.online")}</p>
                  {safeUrl ? (
                    <a
                      href={safeUrl}
                      target="_blank"
                      rel="noopener noreferrer nofollow"
                      className="mt-1.5 inline-flex items-center gap-1.5 rounded-full bg-primary px-3.5 py-1.5 text-xs font-medium text-white transition-colors hover:bg-primary-hover"
                    >
                      <ExternalLink className="h-3.5 w-3.5" aria-hidden />
                      {t("events.openLink")}
                    </a>
                  ) : (
                    <p className="mt-0.5 text-xs text-muted">{t("events.linkMissing")}</p>
                  )}
                </>
              ) : event.locationName || event.locationAddress ? (
                <>
                  {event.locationName ? <p className="break-words text-sm font-medium text-foreground">{event.locationName}</p> : null}
                  {event.locationAddress ? <p className="mt-0.5 break-words text-xs text-muted">{event.locationAddress}</p> : null}
                </>
              ) : (
                <p className="text-sm text-muted">{t("events.locationTba")}</p>
              )}
            </dd>
          </div>
        </dl>

        <div className="border-t border-border px-5 py-4 sm:px-6">
          <p className="text-xs font-medium text-muted">
            {t("events.goingCount", { count: event.goingCount })} · {t("events.interestedCount", { count: event.interestedCount })}
          </p>
          <div className="mt-2.5 flex gap-2">
            {rsvpButton("going", Check, t("events.going"))}
            {rsvpButton("interested", Star, t("events.interested"))}
          </div>
          {event.viewerRsvp ? (
            <button
              type="button"
              disabled={rsvpPending}
              onClick={() => changeRsvp(null)}
              className="mt-2 text-xs font-medium text-muted hover:text-foreground disabled:opacity-50"
            >
              {t("events.rsvpRemove")}
            </button>
          ) : null}
          {rsvpError ? (
            <p className="mt-2 text-xs text-red-500" role="alert">
              {t("events.rsvpError")}
            </p>
          ) : null}
        </div>

        {event.description ? (
          <div className="border-t border-border px-5 py-5 sm:px-6">
            <h2 className="text-sm font-semibold text-foreground">{t("events.about")}</h2>
            <p className="mt-2 whitespace-pre-wrap break-words text-sm leading-relaxed text-foreground/90">{event.description}</p>
          </div>
        ) : null}
      </article>

      <Attendees eventId={event.id} version={attendeesVersion} goingCount={event.goingCount} interestedCount={event.interestedCount} />

      {editing ? (
        <EventForm
          event={event}
          onClose={() => setEditing(false)}
          onSaved={(saved) => {
            setEvent(saved);
            setEditing(false);
          }}
        />
      ) : null}

      {confirmDelete ? (
        <ConfirmDialog
          title={t("events.deleteTitle")}
          body={t("events.deleteBody")}
          confirmLabel={t("post.delete")}
          pendingLabel={t("events.deleting")}
          pending={deleting}
          error={deleteError}
          onConfirm={onDelete}
          onCancel={() => setConfirmDelete(false)}
        />
      ) : null}
    </div>
  );
}

function Attendees({
  eventId,
  version,
  goingCount,
  interestedCount,
}: {
  eventId: string;
  version: number;
  goingCount: number;
  interestedCount: number;
}) {
  const { t } = useLanguage();
  const [tab, setTab] = useState<RsvpStatus>("going");
  const [items, setItems] = useState<ApiEventAttendee[]>([]);
  const [status, setStatus] = useState<"loading" | "ready" | "error">("loading");
  const [nextCursor, setNextCursor] = useState("");
  const [loadingMore, setLoadingMore] = useState(false);
  const loadingMoreRef = useRef(false);
  const tabRef = useRef(tab);
  useEffect(() => {
    tabRef.current = tab;
  }, [tab]);

  const load = useCallback(
    (which: RsvpStatus, signal?: AbortSignal) => {
      setStatus("loading");
      getEventAttendees(eventId, { status: which }, signal)
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
    [eventId],
  );

  useEffect(() => {
    const controller = new AbortController();
    load(tab, controller.signal);
    return () => controller.abort();
  }, [load, tab, version]);

  const loadMore = () => {
    if (loadingMoreRef.current || !nextCursor) return;
    const which = tab;
    loadingMoreRef.current = true;
    setLoadingMore(true);
    getEventAttendees(eventId, { status: which, cursor: nextCursor })
      .then((page) => {
        if (tabRef.current !== which) return;
        setItems((prev) => {
          const seen = new Set(prev.map((a) => a.id));
          return [...prev, ...page.items.filter((a) => !seen.has(a.id))];
        });
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

  const tabs: { value: RsvpStatus; label: string }[] = [
    { value: "going", label: `${t("events.going")} · ${goingCount}` },
    { value: "interested", label: `${t("events.interested")} · ${interestedCount}` },
  ];

  return (
    <section className="overflow-hidden rounded-3xl border border-border bg-surface shadow-sm">
      <div className="flex items-center justify-between gap-3 px-5 pt-4 sm:px-6">
        <h2 className="text-sm font-semibold text-foreground">{t("events.attendees")}</h2>
      </div>
      <div role="tablist" aria-label={t("events.attendees")} className="mt-3 flex gap-2 px-5 sm:px-6">
        {tabs.map(({ value, label }) => {
          const active = tab === value;
          return (
            <button
              key={value}
              type="button"
              role="tab"
              aria-selected={active}
              onClick={() => setTab(value)}
              className={`rounded-full border px-3.5 py-1 text-xs font-medium transition-colors ${
                active ? "border-primary bg-primary-soft text-primary" : "border-border text-muted hover:text-foreground"
              }`}
            >
              {label}
            </button>
          );
        })}
      </div>

      <div className="mt-2">
        {status === "loading" ? (
          <p className="px-5 py-8 text-center text-sm text-muted sm:px-6">{t("events.loading")}</p>
        ) : null}
        {status === "error" ? (
          <div className="px-5 py-8 text-center sm:px-6">
            <p className="text-sm text-muted">{t("events.attendeesError")}</p>
            <button type="button" onClick={() => load(tab)} className="mt-2 text-sm text-primary hover:underline">
              {t("search.tryAgain")}
            </button>
          </div>
        ) : null}
        {status === "ready" && items.length === 0 ? (
          <p className="px-5 py-8 text-center text-sm text-muted sm:px-6">
            {tab === "going" ? t("events.attendeesEmptyGoing") : t("events.attendeesEmptyInterested")}
          </p>
        ) : null}
        {status === "ready" && items.length > 0 ? (
          <ul className="divide-y divide-border">
            {items.map((a) => (
              <li key={a.id}>
                <Link
                  href={`/u/${encodeURIComponent(a.username)}`}
                  className="flex items-center gap-3 px-5 py-2.5 transition-colors hover:bg-background sm:px-6"
                >
                  <Image
                    src={a.avatarUrl ?? FALLBACK_AVATAR}
                    alt=""
                    width={36}
                    height={36}
                    unoptimized={Boolean(a.avatarUrl)}
                    className="h-9 w-9 shrink-0 rounded-full object-cover"
                  />
                  <span className="min-w-0">
                    <span className="block truncate text-sm font-medium text-foreground">{a.displayName}</span>
                    <span className="block truncate text-xs text-muted">@{a.username}</span>
                  </span>
                </Link>
              </li>
            ))}
          </ul>
        ) : null}
        {status === "ready" && nextCursor ? (
          <button
            type="button"
            onClick={loadMore}
            disabled={loadingMore}
            className="w-full border-t border-border px-4 py-2.5 text-center text-sm text-muted transition-colors hover:text-foreground disabled:opacity-50"
          >
            {loadingMore ? t("feed.loadingMore") : t("feed.loadMore")}
          </button>
        ) : null}
      </div>
    </section>
  );
}
