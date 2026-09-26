"use client";

import { useEffect, useMemo, useState } from "react";
import { MapPin, Video, X } from "lucide-react";

import { ApiError } from "@/lib/api";
import {
  createEvent,
  updateEvent,
  type ApiEvent,
  type EventInput,
  type EventType,
  type EventVisibility,
} from "@/lib/events-api";
import {
  browserTimeZone,
  byteLength,
  isSafeHttpUrl,
  isValidTimeZone,
  timeZoneOptions,
  utcToZoned,
  zonedToUtcISO,
} from "@/lib/event-time";
import { useLanguage, type TranslationKey } from "@/lib/language-context";
import { VISIBILITY_META } from "@/components/events/EventCard";

// Backend limits (UTF-8 bytes, see validateEventFields in events.go).
const MAX_TITLE = 200;
const MAX_DESCRIPTION = 10000;
const MAX_LOCATION_NAME = 200;
const MAX_LOCATION_ADDRESS = 500;
const MAX_URL = 2048;

type Field = "title" | "description" | "start" | "end" | "timezone" | "locationName" | "locationAddress" | "onlineUrl";
type Errors = Partial<Record<Field, TranslationKey>>;

interface FormState {
  title: string;
  description: string;
  startDate: string;
  startTime: string;
  hasEnd: boolean;
  endDate: string;
  endTime: string;
  timezone: string;
  eventType: EventType;
  locationName: string;
  locationAddress: string;
  onlineUrl: string;
  visibility: EventVisibility;
}

function initialState(event?: ApiEvent): FormState {
  if (event) {
    const start = utcToZoned(event.startsAt, event.timezone);
    const end = event.endsAt ? utcToZoned(event.endsAt, event.timezone) : { date: "", time: "" };
    return {
      title: event.title,
      description: event.description ?? "",
      startDate: start.date,
      startTime: start.time,
      hasEnd: Boolean(event.endsAt),
      endDate: end.date,
      endTime: end.time,
      timezone: event.timezone,
      eventType: event.eventType,
      locationName: event.locationName ?? "",
      locationAddress: event.locationAddress ?? "",
      onlineUrl: event.onlineUrl ?? "",
      visibility: event.visibility,
    };
  }
  return {
    title: "",
    description: "",
    startDate: "",
    startTime: "",
    hasEnd: false,
    endDate: "",
    endTime: "",
    timezone: browserTimeZone(),
    eventType: "in_person",
    locationName: "",
    locationAddress: "",
    onlineUrl: "",
    visibility: "public",
  };
}

const orNull = (s: string) => (s.trim() ? s.trim() : null);

/** Mirrors the backend validation; returns the payload or field errors. */
function validate(f: FormState, isCreate: boolean): { input?: EventInput; errors: Errors } {
  const errors: Errors = {};
  const title = f.title.trim();
  if (!title) errors.title = "events.errTitleRequired";
  else if (byteLength(title) > MAX_TITLE) errors.title = "events.errTooLong";
  if (byteLength(f.description.trim()) > MAX_DESCRIPTION) errors.description = "events.errTooLong";
  if (!isValidTimeZone(f.timezone) || byteLength(f.timezone) > 100) errors.timezone = "events.errTimezone";

  const startsAt = errors.timezone ? null : zonedToUtcISO(f.startDate, f.startTime, f.timezone);
  if (!startsAt && !errors.timezone) errors.start = "events.errStartRequired";
  else if (startsAt && isCreate && new Date(startsAt).getTime() <= Date.now()) errors.start = "events.errStartPast";

  let endsAt: string | null = null;
  if (f.hasEnd && !errors.timezone) {
    endsAt = zonedToUtcISO(f.endDate, f.endTime, f.timezone);
    if (!endsAt) errors.end = "events.errEndIncomplete";
    else if (startsAt && new Date(endsAt).getTime() < new Date(startsAt).getTime()) errors.end = "events.errEndBeforeStart";
  }

  if (f.eventType === "in_person") {
    if (byteLength(f.locationName.trim()) > MAX_LOCATION_NAME) errors.locationName = "events.errTooLong";
    if (byteLength(f.locationAddress.trim()) > MAX_LOCATION_ADDRESS) errors.locationAddress = "events.errTooLong";
  } else {
    const url = f.onlineUrl.trim();
    if (url && byteLength(url) > MAX_URL) errors.onlineUrl = "events.errTooLong";
    else if (url && !isSafeHttpUrl(url)) errors.onlineUrl = "events.errUrl";
  }

  if (Object.keys(errors).length > 0 || !startsAt) return { errors };
  const online = f.eventType === "online";
  return {
    errors,
    input: {
      title,
      description: orNull(f.description),
      startsAt,
      endsAt,
      timezone: f.timezone,
      eventType: f.eventType,
      // The backend rejects fields that belong to the other event type.
      locationName: online ? null : orNull(f.locationName),
      locationAddress: online ? null : orNull(f.locationAddress),
      onlineUrl: online ? orNull(f.onlineUrl) : null,
      visibility: f.visibility,
    },
  };
}

const inputClass =
  "h-11 w-full rounded-xl border border-border bg-background px-3.5 text-sm text-foreground placeholder:text-muted-soft focus:border-primary focus:outline-none focus:ring-2 focus:ring-primary/20 aria-[invalid=true]:border-red-400";

function Section({ step, title, children }: { step: number; title: string; children: React.ReactNode }) {
  return (
    <section className="flex flex-col gap-3">
      <h3 className="flex items-center gap-2 text-sm font-semibold text-foreground">
        <span className="flex h-6 w-6 items-center justify-center rounded-full bg-primary-soft text-xs font-bold text-primary">
          {step}
        </span>
        {title}
      </h3>
      {children}
    </section>
  );
}

function Label({ htmlFor, text, optional }: { htmlFor: string; text: string; optional?: boolean }) {
  const { t } = useLanguage();
  return (
    <label htmlFor={htmlFor} className="mb-1 flex items-baseline gap-1.5 text-xs font-medium text-muted">
      {text}
      {optional ? <span className="font-normal text-muted-soft">· {t("events.optional")}</span> : null}
    </label>
  );
}

function FieldError({ id, error }: { id: string; error?: TranslationKey }) {
  const { t } = useLanguage();
  if (!error) return null;
  return (
    <p id={id} className="mt-1 text-xs text-red-500" role="alert">
      {t(error)}
    </p>
  );
}

/**
 * Guided create/edit form in a modal (bottom sheet on mobile). On success it
 * hands the saved event back; the caller decides where to go next.
 */
export function EventForm({
  event,
  onSaved,
  onClose,
}: {
  event?: ApiEvent;
  onSaved: (event: ApiEvent) => void;
  onClose: () => void;
}) {
  const { t } = useLanguage();
  const isCreate = !event;
  const [form, setForm] = useState<FormState>(() => initialState(event));
  const [errors, setErrors] = useState<Errors>({});
  const [submitError, setSubmitError] = useState<TranslationKey | null>(null);
  const [saving, setSaving] = useState(false);
  const zones = useMemo(() => timeZoneOptions([form.timezone]), [form.timezone]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape" && !saving) onClose();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [onClose, saving]);

  const set = <K extends keyof FormState>(key: K, value: FormState[K]) => {
    setForm((prev) => ({ ...prev, [key]: value }));
  };

  const onSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (saving) return;
    const { input, errors: next } = validate(form, isCreate);
    setErrors(next);
    setSubmitError(null);
    if (!input) return;
    setSaving(true);
    try {
      const saved = event ? await updateEvent(event.id, input) : await createEvent(input);
      onSaved(saved);
    } catch (err) {
      setSubmitError(err instanceof ApiError && err.status === 400 ? "events.errInvalid" : "events.errSave");
      setSaving(false);
    }
  };

  const err = (f: Field) => (errors[f] ? { "aria-invalid": true as const, "aria-describedby": `ev-${f}-err` } : {});
  const today = useMemo(() => utcToZoned(new Date().toISOString(), form.timezone).date, [form.timezone]);

  return (
    <div
      className="fixed inset-0 z-50 flex items-end justify-center bg-black/40 sm:items-center sm:p-4"
      onClick={() => {
        if (!saving) onClose();
      }}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="event-form-title"
        className="flex max-h-[92dvh] w-full flex-col overflow-hidden rounded-t-3xl border border-border bg-surface shadow-xl sm:max-w-lg sm:rounded-3xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between border-b border-border px-5 py-4">
          <h2 id="event-form-title" className="text-base font-semibold text-foreground">
            {isCreate ? t("events.create") : t("events.formEditTitle")}
          </h2>
          <button
            type="button"
            onClick={onClose}
            disabled={saving}
            aria-label={t("profile.close")}
            className="flex h-9 w-9 items-center justify-center rounded-full text-muted transition-colors hover:bg-background hover:text-foreground disabled:opacity-50"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        <form onSubmit={onSubmit} noValidate className="flex min-h-0 flex-1 flex-col">
          <div className="flex flex-col gap-6 overflow-y-auto px-5 py-5">
            <Section step={1} title={t("events.sectionBasics")}>
              <div>
                <Label htmlFor="ev-title" text={t("events.titleLabel")} />
                <input
                  id="ev-title"
                  className={inputClass}
                  value={form.title}
                  onChange={(e) => set("title", e.target.value)}
                  placeholder={t("events.titlePlaceholder")}
                  maxLength={MAX_TITLE}
                  autoFocus
                  {...err("title")}
                />
                <FieldError id="ev-title-err" error={errors.title} />
              </div>
              <div>
                <Label htmlFor="ev-description" text={t("events.descriptionLabel")} optional />
                <textarea
                  id="ev-description"
                  className={`${inputClass} h-28 resize-y py-2.5`}
                  value={form.description}
                  onChange={(e) => set("description", e.target.value)}
                  placeholder={t("events.descriptionPlaceholder")}
                  maxLength={MAX_DESCRIPTION}
                  {...err("description")}
                />
                <FieldError id="ev-description-err" error={errors.description} />
              </div>
            </Section>

            <Section step={2} title={t("events.sectionWhen")}>
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <Label htmlFor="ev-start-date" text={t("events.startDate")} />
                  <input
                    id="ev-start-date"
                    type="date"
                    className={inputClass}
                    value={form.startDate}
                    min={isCreate ? today : undefined}
                    onChange={(e) => set("startDate", e.target.value)}
                    {...err("start")}
                  />
                </div>
                <div>
                  <Label htmlFor="ev-start-time" text={t("events.startTime")} />
                  <input
                    id="ev-start-time"
                    type="time"
                    className={inputClass}
                    value={form.startTime}
                    onChange={(e) => set("startTime", e.target.value)}
                    {...err("start")}
                  />
                </div>
              </div>
              <FieldError id="ev-start-err" error={errors.start} />

              {form.hasEnd ? (
                <>
                  <div className="grid grid-cols-2 gap-3">
                    <div>
                      <Label htmlFor="ev-end-date" text={t("events.endDate")} />
                      <input
                        id="ev-end-date"
                        type="date"
                        className={inputClass}
                        value={form.endDate}
                        min={form.startDate || undefined}
                        onChange={(e) => set("endDate", e.target.value)}
                        {...err("end")}
                      />
                    </div>
                    <div>
                      <Label htmlFor="ev-end-time" text={t("events.endTime")} />
                      <input
                        id="ev-end-time"
                        type="time"
                        className={inputClass}
                        value={form.endTime}
                        onChange={(e) => set("endTime", e.target.value)}
                        {...err("end")}
                      />
                    </div>
                  </div>
                  <FieldError id="ev-end-err" error={errors.end} />
                  <button
                    type="button"
                    onClick={() => {
                      setForm((p) => ({ ...p, hasEnd: false, endDate: "", endTime: "" }));
                      setErrors((p) => ({ ...p, end: undefined }));
                    }}
                    className="-my-1 min-h-9 self-start text-xs font-medium text-muted hover:text-foreground"
                  >
                    {t("events.removeEnd")}
                  </button>
                </>
              ) : (
                <button
                  type="button"
                  onClick={() => setForm((p) => ({ ...p, hasEnd: true, endDate: p.endDate || p.startDate }))}
                  className="-my-1 min-h-9 self-start text-xs font-medium text-primary hover:underline"
                >
                  + {t("events.addEnd")}
                </button>
              )}

              <div>
                <Label htmlFor="ev-timezone" text={t("events.timezone")} />
                <select
                  id="ev-timezone"
                  className={inputClass}
                  value={form.timezone}
                  onChange={(e) => set("timezone", e.target.value)}
                  {...err("timezone")}
                >
                  {zones.map((z) => (
                    <option key={z} value={z}>
                      {z.replace(/_/g, " ")}
                    </option>
                  ))}
                </select>
                <FieldError id="ev-timezone-err" error={errors.timezone} />
              </div>
            </Section>

            <Section step={3} title={t("events.sectionWhere")}>
              <div role="radiogroup" aria-label={t("events.sectionWhere")} className="grid grid-cols-2 gap-2">
                {(
                  [
                    ["in_person", MapPin, "events.inPerson"],
                    ["online", Video, "events.online"],
                  ] as const
                ).map(([value, Icon, labelKey]) => {
                  const active = form.eventType === value;
                  return (
                    <button
                      key={value}
                      type="button"
                      role="radio"
                      aria-checked={active}
                      onClick={() => set("eventType", value)}
                      className={`flex h-11 items-center justify-center gap-2 rounded-xl border text-sm font-medium transition-colors ${
                        active
                          ? "border-primary bg-primary-soft text-primary"
                          : "border-border text-muted hover:border-primary/40 hover:text-foreground"
                      }`}
                    >
                      <Icon className="h-4 w-4" aria-hidden />
                      {t(labelKey)}
                    </button>
                  );
                })}
              </div>

              {form.eventType === "in_person" ? (
                <>
                  <div>
                    <Label htmlFor="ev-location-name" text={t("events.locationName")} optional />
                    <input
                      id="ev-location-name"
                      className={inputClass}
                      value={form.locationName}
                      onChange={(e) => set("locationName", e.target.value)}
                      placeholder={t("events.locationNamePlaceholder")}
                      maxLength={MAX_LOCATION_NAME}
                      {...err("locationName")}
                    />
                    <FieldError id="ev-locationName-err" error={errors.locationName} />
                  </div>
                  <div>
                    <Label htmlFor="ev-location-address" text={t("events.locationAddress")} optional />
                    <input
                      id="ev-location-address"
                      className={inputClass}
                      value={form.locationAddress}
                      onChange={(e) => set("locationAddress", e.target.value)}
                      placeholder={t("events.locationAddressPlaceholder")}
                      maxLength={MAX_LOCATION_ADDRESS}
                      {...err("locationAddress")}
                    />
                    <FieldError id="ev-locationAddress-err" error={errors.locationAddress} />
                  </div>
                </>
              ) : (
                <div>
                  <Label htmlFor="ev-online-url" text={t("events.onlineUrl")} optional />
                  <input
                    id="ev-online-url"
                    type="url"
                    inputMode="url"
                    className={inputClass}
                    value={form.onlineUrl}
                    onChange={(e) => set("onlineUrl", e.target.value)}
                    placeholder="https://"
                    maxLength={MAX_URL}
                    {...err("onlineUrl")}
                  />
                  <FieldError id="ev-onlineUrl-err" error={errors.onlineUrl} />
                </div>
              )}
            </Section>

            <Section step={4} title={t("events.sectionVisibility")}>
              <div role="radiogroup" aria-label={t("events.sectionVisibility")} className="flex flex-col gap-2">
                {(["public", "followers", "private"] as const).map((value) => {
                  const meta = VISIBILITY_META[value];
                  const Icon = meta.icon;
                  const active = form.visibility === value;
                  return (
                    <button
                      key={value}
                      type="button"
                      role="radio"
                      aria-checked={active}
                      onClick={() => set("visibility", value)}
                      className={`flex items-start gap-3 rounded-xl border px-3.5 py-3 text-left transition-colors ${
                        active ? "border-primary bg-primary-soft" : "border-border hover:border-primary/40"
                      }`}
                    >
                      <Icon className={`mt-0.5 h-4 w-4 shrink-0 ${active ? "text-primary" : "text-muted"}`} aria-hidden />
                      <span className="min-w-0">
                        <span className={`block text-sm font-medium ${active ? "text-primary" : "text-foreground"}`}>
                          {t(meta.labelKey)}
                        </span>
                        <span className="block text-xs text-muted">{t(meta.hintKey)}</span>
                      </span>
                    </button>
                  );
                })}
              </div>
            </Section>
          </div>

          <div className="flex items-center justify-end gap-2 border-t border-border px-5 py-3.5 pb-[max(0.875rem,env(safe-area-inset-bottom))]">
            {submitError ? (
              <p className="mr-auto text-xs text-red-500" role="alert">
                {t(submitError)}
              </p>
            ) : null}
            <button
              type="button"
              onClick={onClose}
              disabled={saving}
              className="rounded-full px-4 py-2 text-sm font-medium text-muted transition-colors hover:bg-background disabled:opacity-50"
            >
              {t("post.cancel")}
            </button>
            <button
              type="submit"
              disabled={saving}
              className="rounded-full bg-primary px-5 py-2 text-sm font-medium text-white transition-colors hover:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-50"
            >
              {saving ? t("events.saving") : isCreate ? t("events.create") : t("post.save")}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
