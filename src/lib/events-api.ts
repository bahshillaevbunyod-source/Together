/**
 * Events V1 API client. Shapes mirror backend/internal/server/events.go.
 */

import { apiFetch } from "@/lib/api";

export type EventType = "in_person" | "online";
export type EventVisibility = "public" | "followers" | "private";
export type RsvpStatus = "going" | "interested";

export interface ApiEventCreator {
  id: string;
  username: string;
  displayName: string;
  avatarUrl: string | null;
}

export interface ApiEvent {
  id: string;
  creatorUserId: string;
  title: string;
  description: string | null;
  /** RFC3339 instant. */
  startsAt: string;
  endsAt: string | null;
  /** IANA timezone the event is organised in. */
  timezone: string;
  eventType: EventType;
  locationName: string | null;
  locationAddress: string | null;
  onlineUrl: string | null;
  visibility: EventVisibility;
  createdAt: string;
  updatedAt: string;
  creator: ApiEventCreator;
  /**
   * Viewer RSVP, counts and permissions are only populated by the single-event
   * endpoints (detail, create, update, RSVP). List items carry null/0/false.
   */
  viewerRsvp: RsvpStatus | null;
  goingCount: number;
  interestedCount: number;
  permissions: { canEdit: boolean; canDelete: boolean };
}

export interface ApiEventAttendee {
  id: string;
  username: string;
  displayName: string;
  avatarUrl: string | null;
  status: RsvpStatus;
  joinedAt: string;
}

export interface Page<T> {
  items: T[];
  nextCursor: string;
}

/** Create/replace payload. Optional text fields are null when empty. */
export interface EventInput {
  title: string;
  description: string | null;
  startsAt: string;
  endsAt: string | null;
  timezone: string;
  eventType: EventType;
  locationName: string | null;
  locationAddress: string | null;
  onlineUrl: string | null;
  visibility: EventVisibility;
}

/** List filters supported by GET /api/v1/events (upcoming events only). */
export interface EventListFilter {
  mine?: "created";
  rsvp?: RsvpStatus;
  /** Case-insensitive match on title, venue and address (max 100 chars). */
  q?: string;
  type?: EventType;
  cursor?: string;
  limit?: number;
}

const base = "/api/v1/events";
const eventPath = (id: string) => `${base}/${encodeURIComponent(id)}`;

export function getEvents(filter: EventListFilter = {}, signal?: AbortSignal): Promise<Page<ApiEvent>> {
  const q = new URLSearchParams();
  if (filter.mine) q.set("mine", filter.mine);
  if (filter.rsvp) q.set("rsvp", filter.rsvp);
  if (filter.q) q.set("q", filter.q);
  if (filter.type) q.set("type", filter.type);
  if (filter.cursor) q.set("cursor", filter.cursor);
  if (filter.limit) q.set("limit", String(filter.limit));
  const qs = q.toString();
  return apiFetch<Page<ApiEvent>>(qs ? `${base}?${qs}` : base, { signal });
}

export function getEvent(id: string, signal?: AbortSignal): Promise<ApiEvent> {
  return apiFetch<ApiEvent>(eventPath(id), { signal });
}

export function createEvent(input: EventInput): Promise<ApiEvent> {
  return apiFetch<ApiEvent>(base, { method: "POST", body: input });
}

export function updateEvent(id: string, input: Partial<EventInput>): Promise<ApiEvent> {
  return apiFetch<ApiEvent>(eventPath(id), { method: "PATCH", body: input });
}

export async function deleteEvent(id: string): Promise<void> {
  await apiFetch<unknown>(eventPath(id), { method: "DELETE" });
}

export function setEventRsvp(id: string, status: RsvpStatus): Promise<ApiEvent> {
  return apiFetch<ApiEvent>(`${eventPath(id)}/rsvp`, { method: "PUT", body: { status } });
}

export function removeEventRsvp(id: string): Promise<ApiEvent> {
  return apiFetch<ApiEvent>(`${eventPath(id)}/rsvp`, { method: "DELETE" });
}

export function getEventAttendees(
  id: string,
  opts: { status: RsvpStatus; cursor?: string; limit?: number },
  signal?: AbortSignal,
): Promise<Page<ApiEventAttendee>> {
  const q = new URLSearchParams({ status: opts.status });
  if (opts.cursor) q.set("cursor", opts.cursor);
  if (opts.limit) q.set("limit", String(opts.limit));
  return apiFetch<Page<ApiEventAttendee>>(`${eventPath(id)}/attendees?${q}`, { signal });
}
