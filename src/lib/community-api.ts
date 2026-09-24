/**
 * Typed API boundary for Groups and Channels ("communities").
 *
 * PROPOSED CONTRACT — the backend does not expose these endpoints yet. Every
 * endpoint assumption for groups/channels lives in this file only, so the
 * backend integration touches one place. Nothing here simulates success: until
 * the endpoints exist, calls reject with the backend's real ApiError (404/405)
 * and the UI shows its unavailable / error states.
 *
 * Model: a group or channel IS a conversation (same id space). Messages,
 * read state, mute, edit/delete, attachments, voice and realtime events reuse
 * the existing /api/v1/conversations/{id}/... and /api/v1/messages/{id}
 * endpoints and the existing WebSocket. Only community metadata, membership
 * and roles are new.
 *
 * Authorization is decided by the backend. The `permissions` object returned
 * per community gates every management control in the UI; the frontend never
 * enables an action from a locally guessed role alone.
 */

import {
  ApiError,
  apiFetch,
  type ApiConversationLastMessage,
} from "@/lib/api";

export type CommunityKind = "group" | "channel";
export type CommunityRole = "owner" | "admin" | "member";

/** A user row inside a community (member list, pickers). */
export interface ApiCommunityUser {
  id: string;
  username: string;
  displayName: string;
  avatarUrl: string | null;
}

/** Viewer-specific capabilities, computed server-side. */
export interface ApiCommunityPermissions {
  /** Viewer may send messages (channel: owner/admin only). */
  canPost: boolean;
  /** Viewer may add members (groups). */
  canAddMembers: boolean;
  /** Viewer may remove members/subscribers below their own role. */
  canRemoveMembers: boolean;
  /** Viewer may promote members to admin / demote admins. */
  canManageAdmins: boolean;
  /** Viewer may see the full member/subscriber list. */
  canViewMembers: boolean;
  /** Viewer may leave (false e.g. for the sole owner). */
  canLeave: boolean;
}

/** A group or channel as returned by the list/detail endpoints. */
export interface ApiCommunity {
  /** Conversation id; also valid for the existing messages endpoints. */
  id: string;
  type: CommunityKind;
  name: string;
  description: string | null;
  avatarUrl: string | null;
  /** Viewer's role; null when the viewer is not a member (channel preview). */
  role: CommunityRole | null;
  /** Total members/subscribers, or null when the server does not disclose it. */
  memberCount: number | null;
  muted: boolean;
  unreadCount: number;
  lastMessage: ApiConversationLastMessage | null;
  createdAt: string;
  updatedAt: string;
  permissions: ApiCommunityPermissions;
}

export interface CommunityPage {
  items: ApiCommunity[];
  nextCursor: string;
}

export interface ApiCommunityMember {
  user: ApiCommunityUser;
  role: CommunityRole;
  joinedAt: string;
}

export interface CommunityMemberPage {
  items: ApiCommunityMember[];
  nextCursor: string;
}

export interface CreateCommunityInput {
  name: string;
  description?: string;
  /** Initial member user ids (groups only). */
  memberIds?: string[];
}

/** True when the endpoint does not exist on this backend yet. */
export function isCommunityApiUnavailable(err: unknown): boolean {
  return (
    err instanceof ApiError &&
    (err.status === 404 || err.status === 405 || err.status === 501)
  );
}

const BASE: Record<CommunityKind, string> = {
  group: "/api/v1/groups",
  channel: "/api/v1/channels",
};

function base(kind: CommunityKind, id?: string): string {
  return id ? `${BASE[kind]}/${encodeURIComponent(id)}` : BASE[kind];
}

function withQuery(path: string, params: Record<string, string | number | undefined>): string {
  const q = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== "") q.set(k, String(v));
  }
  const qs = q.toString();
  return qs ? `${path}?${qs}` : path;
}

/** Joined groups / subscribed channels, latest activity first (keyset cursor). */
export function listCommunities(
  kind: CommunityKind,
  params: { cursor?: string; limit?: number } = {},
  signal?: AbortSignal,
): Promise<CommunityPage> {
  return apiFetch<CommunityPage>(withQuery(base(kind), params), { signal });
}

/** One group/channel with the viewer's role and permissions. */
export function getCommunity(
  kind: CommunityKind,
  id: string,
  signal?: AbortSignal,
): Promise<ApiCommunity> {
  return apiFetch<ApiCommunity>(base(kind, id), { signal });
}

/** Create a group or channel; the creator becomes its owner. */
export function createCommunity(
  kind: CommunityKind,
  input: CreateCommunityInput,
): Promise<ApiCommunity> {
  return apiFetch<ApiCommunity>(base(kind), { method: "POST", body: input });
}

/** Search public channels the viewer can join. */
export function searchChannels(
  query: string,
  params: { limit?: number } = {},
  signal?: AbortSignal,
): Promise<ApiCommunity[]> {
  return apiFetch<{ items: ApiCommunity[] }>(
    withQuery(`${BASE.channel}/search`, { q: query, limit: params.limit }),
    { signal },
  ).then((r) => r.items ?? []);
}

/** Join a channel; returns the channel with the viewer's new role. */
export function joinChannel(id: string): Promise<ApiCommunity> {
  return apiFetch<ApiCommunity>(`${base("channel", id)}/join`, { method: "POST" });
}

/** Leave a group or channel (204). */
export function leaveCommunity(kind: CommunityKind, id: string): Promise<void> {
  return apiFetch<void>(`${base(kind, id)}/leave`, { method: "POST" });
}

/** Member/subscriber list, owner → admins → members, then by join time. */
export function listCommunityMembers(
  kind: CommunityKind,
  id: string,
  params: { cursor?: string; limit?: number } = {},
  signal?: AbortSignal,
): Promise<CommunityMemberPage> {
  return apiFetch<CommunityMemberPage>(
    withQuery(`${base(kind, id)}/members`, params),
    { signal },
  );
}

/** Add members to a group; returns the added member rows. */
export function addCommunityMembers(
  kind: CommunityKind,
  id: string,
  userIds: string[],
): Promise<{ items: ApiCommunityMember[] }> {
  return apiFetch<{ items: ApiCommunityMember[] }>(`${base(kind, id)}/members`, {
    method: "POST",
    body: { userIds },
  });
}

/** Remove a member/subscriber (204). */
export function removeCommunityMember(
  kind: CommunityKind,
  id: string,
  userId: string,
): Promise<void> {
  return apiFetch<void>(
    `${base(kind, id)}/members/${encodeURIComponent(userId)}`,
    { method: "DELETE" },
  );
}

/** Promote to admin or demote to member; returns the updated member row. */
export function setCommunityMemberRole(
  kind: CommunityKind,
  id: string,
  userId: string,
  role: Exclude<CommunityRole, "owner">,
): Promise<ApiCommunityMember> {
  return apiFetch<ApiCommunityMember>(
    `${base(kind, id)}/members/${encodeURIComponent(userId)}`,
    { method: "PATCH", body: { role } },
  );
}
