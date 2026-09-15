/**
 * Small typed API client for the Together backend.
 *
 * All requests send cookies (`credentials: "include"`) so the HttpOnly session
 * cookie is carried, and share one JSON/error handling path.
 */

/** Base URL of the Go backend. Override with NEXT_PUBLIC_API_BASE_URL. */
const API_BASE_URL =
  process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080";

/** The authenticated user, as returned by GET /api/v1/auth/me. */
export interface CurrentUser {
  id: string;
  email: string | null;
  username: string;
  displayName: string;
  nativeLanguage: string;
  createdAt: string;
}

/** Error carrying the HTTP status and the backend's `{error}` message. */
export class ApiError extends Error {
  readonly status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

interface RequestOptions {
  method?: string;
  /** JSON-serializable body; sets Content-Type automatically. */
  body?: unknown;
  signal?: AbortSignal;
}

/**
 * Perform a request against the API and parse a JSON response. Non-2xx
 * responses reject with an ApiError carrying the status and server message.
 */
export async function apiFetch<T>(
  path: string,
  options: RequestOptions = {},
): Promise<T> {
  const { method = "GET", body, signal } = options;

  const headers: Record<string, string> = {};
  let payload: BodyInit | undefined;
  if (body !== undefined) {
    headers["Content-Type"] = "application/json";
    payload = JSON.stringify(body);
  }

  let res: Response;
  try {
    res = await fetch(`${API_BASE_URL}${path}`, {
      method,
      headers,
      body: payload,
      credentials: "include",
      signal,
    });
  } catch (err) {
    if (err instanceof DOMException && err.name === "AbortError") throw err;
    throw new ApiError(0, "network error");
  }

  if (res.status === 204) {
    return undefined as T;
  }

  const isJSON = res.headers
    .get("content-type")
    ?.includes("application/json");
  const data = isJSON ? await res.json().catch(() => null) : null;

  if (!res.ok) {
    const message =
      (data && typeof data === "object" && "error" in data
        ? String((data as { error: unknown }).error)
        : null) ?? `request failed (${res.status})`;
    throw new ApiError(res.status, message);
  }

  return data as T;
}

/** Fetch the currently authenticated user. Rejects with 401 when signed out. */
export function getMe(signal?: AbortSignal): Promise<CurrentUser> {
  return apiFetch<CurrentUser>("/api/v1/auth/me", { signal });
}

/** Log in with email + password. Sets the session cookie; returns the user. */
export function login(email: string, password: string): Promise<CurrentUser> {
  return apiFetch<CurrentUser>("/api/v1/auth/login", {
    method: "POST",
    body: { email, password },
  });
}

/** Log out: clears the session server-side and the cookie (200). */
export function logout(): Promise<void> {
  return apiFetch<void>("/api/v1/auth/logout", { method: "POST" });
}

/* ----------------------------- Profile ---------------------------------- */

/** The authenticated user's own profile (GET /api/v1/profile). */
export interface ProfileResponse {
  id: string;
  email: string | null;
  username: string;
  displayName: string;
  bio: string | null;
  countryCode: string | null;
  city: string | null;
  nativeLanguage: string;
  avatarUrl: string | null;
  createdAt: string;
  preferredLanguage: string | null;
  autoTranslateEnabled: boolean;
}

/** A user's public profile (GET /api/v1/users/{username}). */
export interface PublicUserProfile {
  id: string;
  username: string;
  displayName: string;
  avatarUrl: string | null;
  bio: string | null;
  countryCode: string | null;
  city: string | null;
  nativeLanguage: string;
  createdAt: string;
  followersCount: number;
  followingCount: number;
  isFollowing: boolean;
  isSelf: boolean;
}

/** A partial profile update. Nullable fields accept null to clear them. */
export interface UpdateProfileInput {
  displayName?: string;
  bio?: string | null;
  countryCode?: string | null;
  city?: string | null;
  nativeLanguage?: string;
  avatarUrl?: string | null;
  preferredLanguage?: string | null;
  autoTranslateEnabled?: boolean;
}

/** Fetch the authenticated user's own profile. */
export function getProfile(signal?: AbortSignal): Promise<ProfileResponse> {
  return apiFetch<ProfileResponse>("/api/v1/profile", { signal });
}

/** Apply a partial profile update and return the updated profile. */
export function updateProfile(
  input: UpdateProfileInput,
): Promise<ProfileResponse> {
  return apiFetch<ProfileResponse>("/api/v1/profile", {
    method: "PATCH",
    body: input,
  });
}

/** Follow a user by username. */
export function followUser(username: string): Promise<{ following: boolean }> {
  return apiFetch<{ following: boolean }>(
    `/api/v1/users/${encodeURIComponent(username)}/follow`,
    { method: "POST" },
  );
}

/** Unfollow a user by username. */
export function unfollowUser(username: string): Promise<{ following: boolean }> {
  return apiFetch<{ following: boolean }>(
    `/api/v1/users/${encodeURIComponent(username)}/follow`,
    { method: "DELETE" },
  );
}

/** Fetch a user's public profile by username. */
export function getUserProfile(
  username: string,
  signal?: AbortSignal,
): Promise<PublicUserProfile> {
  return apiFetch<PublicUserProfile>(
    `/api/v1/users/${encodeURIComponent(username)}`,
    { signal },
  );
}

/** Register a new account. Sets the session cookie; returns the user. */
export function register(input: {
  email: string;
  username: string;
  displayName: string;
  nativeLanguage: string;
  password: string;
}): Promise<CurrentUser> {
  return apiFetch<CurrentUser>("/api/v1/auth/register", {
    method: "POST",
    body: input,
  });
}

/* ------------------------- Conversations -------------------------------- */

/** The other participant in a private conversation. */
export interface ApiConversationOtherUser {
  id: string;
  username: string;
  displayName: string;
  avatarUrl: string | null;
}

/** The latest message in a conversation (null when there are none yet). */
export interface ApiConversationLastMessage {
  id: string;
  senderId: string;
  content: string;
  createdAt: string;
}

/** A conversation as returned by GET /api/v1/conversations. */
export interface ApiConversation {
  id: string;
  otherUser: ApiConversationOtherUser;
  lastMessage: ApiConversationLastMessage | null;
  unreadCount: number;
  updatedAt: string;
}

/** One page of conversations. `nextCursor` is "" when there is no more. */
export interface ConversationPage {
  items: ApiConversation[];
  nextCursor: string;
}

/** Open (or return the existing) private conversation with a user by username. */
export function openConversation(username: string): Promise<ApiConversation> {
  return apiFetch<ApiConversation>("/api/v1/conversations", {
    method: "POST",
    body: { username },
  });
}

/** Fetch a page of the current user's conversations (latest activity first). */
export function getConversations(
  params: { cursor?: string; limit?: number } = {},
  signal?: AbortSignal,
): Promise<ConversationPage> {
  const query = new URLSearchParams();
  if (params.cursor) query.set("cursor", params.cursor);
  if (params.limit) query.set("limit", String(params.limit));
  const qs = query.toString();
  return apiFetch<ConversationPage>(
    `/api/v1/conversations${qs ? `?${qs}` : ""}`,
    { signal },
  );
}

/* --------------------------- Messages ----------------------------------- */

/** A message within a conversation. */
export interface ApiMessage {
  id: string;
  senderId: string;
  content: string;
  createdAt: string;
  translatedContent: string | null;
  sourceLanguage: string | null;
  targetLanguage: string | null;
}

/** One page of messages (newest first). `nextCursor` fetches older messages. */
export interface MessagePage {
  items: ApiMessage[];
  nextCursor: string;
}

/** Fetch a page of a conversation's messages (newest first). */
export function getMessages(
  conversationId: string,
  params: { cursor?: string; limit?: number } = {},
  signal?: AbortSignal,
): Promise<MessagePage> {
  const query = new URLSearchParams();
  if (params.cursor) query.set("cursor", params.cursor);
  if (params.limit) query.set("limit", String(params.limit));
  const qs = query.toString();
  return apiFetch<MessagePage>(
    `/api/v1/conversations/${conversationId}/messages${qs ? `?${qs}` : ""}`,
    { signal },
  );
}

/** Mark a conversation as read up to its latest message (204 No Content). */
export function markConversationRead(conversationId: string): Promise<void> {
  return apiFetch<void>(`/api/v1/conversations/${conversationId}/read`, {
    method: "POST",
  });
}

/** Send a message in a conversation and return the stored message. */
export function sendMessage(
  conversationId: string,
  content: string,
): Promise<ApiMessage> {
  return apiFetch<ApiMessage>(
    `/api/v1/conversations/${conversationId}/messages`,
    { method: "POST", body: { content } },
  );
}

/* -------------------------- Notifications -------------------------------- */

/** Actor (the user who triggered a notification); null if the actor was removed. */
export interface ApiNotificationActor {
  id: string;
  username: string;
  displayName: string;
  avatarUrl: string | null;
}

/** A notification as returned by GET /api/v1/notifications. */
export interface ApiNotification {
  id: string;
  type: string;
  actor: ApiNotificationActor | null;
  postId: string | null;
  commentId: string | null;
  readAt: string | null;
  createdAt: string;
}

/** One page of notifications. `nextCursor` is "" when there is no more. */
export interface NotificationPage {
  items: ApiNotification[];
  nextCursor: string;
}

/** Fetch a page of the current user's notifications (newest first). */
export function getNotifications(
  params: { cursor?: string; limit?: number } = {},
  signal?: AbortSignal,
): Promise<NotificationPage> {
  const query = new URLSearchParams();
  if (params.cursor) query.set("cursor", params.cursor);
  if (params.limit) query.set("limit", String(params.limit));
  const qs = query.toString();
  return apiFetch<NotificationPage>(
    `/api/v1/notifications${qs ? `?${qs}` : ""}`,
    { signal },
  );
}

/** Fetch the current user's unread notification count. */
export function getUnreadNotificationCount(
  signal?: AbortSignal,
): Promise<{ unreadCount: number }> {
  return apiFetch<{ unreadCount: number }>(
    "/api/v1/notifications/unread-count",
    { signal },
  );
}

/** Mark a single notification as read (204 No Content). */
export function markNotificationRead(id: string): Promise<void> {
  return apiFetch<void>(`/api/v1/notifications/${id}/read`, { method: "POST" });
}

/** Mark all notifications as read (204 No Content). */
export function markAllNotificationsRead(): Promise<void> {
  return apiFetch<void>("/api/v1/notifications/read-all", { method: "POST" });
}

/* ----------------------------- Feed ------------------------------------- */

/** A media attachment on a post, as returned by the backend. */
export interface ApiMedia {
  id: string;
  type: string;
  url: string;
  mimeType: string;
  width: number | null;
  height: number | null;
  durationMs: number | null;
  sortOrder: number;
}

/** Public-safe author fields on a post. */
export interface ApiPostAuthor {
  id: string;
  username: string;
  displayName: string;
  avatarUrl: string | null;
}

/** A post as returned by GET /api/v1/feed and GET /api/v1/posts/{id}. */
export interface ApiPost {
  id: string;
  author: ApiPostAuthor;
  content: string | null;
  visibility: string;
  createdAt: string;
  updatedAt: string;
  likesCount: number;
  likedByMe: boolean;
  commentsCount: number;
  savedByMe: boolean;
  media: ApiMedia[];
  translatedContent: string | null;
  sourceLanguage: string | null;
  targetLanguage: string | null;
}

/** One page of feed results. `nextCursor` is "" when there is no more. */
export interface FeedPage {
  items: ApiPost[];
  nextCursor: string;
}

/* ---------------------------- Comments ---------------------------------- */

/** Public-safe author fields on a comment. */
export interface ApiCommentAuthor {
  id: string;
  username: string;
  displayName: string;
  avatarUrl: string | null;
}

/** A comment as returned by the comments endpoints. */
export interface ApiComment {
  id: string;
  author: ApiCommentAuthor;
  content: string;
  createdAt: string;
  updatedAt: string;
  translatedContent: string | null;
  sourceLanguage: string | null;
  targetLanguage: string | null;
}

/** One page of comments. `nextCursor` is "" when there is no more. */
export interface CommentPage {
  items: ApiComment[];
  nextCursor: string;
}

/** Fetch a page of a post's comments (newest first). */
export function getComments(
  postId: string,
  params: { cursor?: string; limit?: number } = {},
  signal?: AbortSignal,
): Promise<CommentPage> {
  const query = new URLSearchParams();
  if (params.cursor) query.set("cursor", params.cursor);
  if (params.limit) query.set("limit", String(params.limit));
  const qs = query.toString();
  return apiFetch<CommentPage>(
    `/api/v1/posts/${postId}/comments${qs ? `?${qs}` : ""}`,
    { signal },
  );
}

/** Create a comment on a post and return the stored comment. */
export function createComment(
  postId: string,
  content: string,
): Promise<ApiComment> {
  return apiFetch<ApiComment>(`/api/v1/posts/${postId}/comments`, {
    method: "POST",
    body: { content },
  });
}

/** Result of a like/unlike request. */
export interface LikeState {
  liked: boolean;
  likesCount: number;
}

/** Like a post. Returns the authoritative like state. */
export function likePost(id: string): Promise<LikeState> {
  return apiFetch<LikeState>(`/api/v1/posts/${id}/like`, { method: "POST" });
}

/** Remove a like. Returns the authoritative like state. */
export function unlikePost(id: string): Promise<LikeState> {
  return apiFetch<LikeState>(`/api/v1/posts/${id}/like`, { method: "DELETE" });
}

/** Bookmark a post (204 No Content). */
export function savePost(id: string): Promise<void> {
  return apiFetch<void>(`/api/v1/posts/${id}/bookmark`, { method: "POST" });
}

/** Remove a bookmark (204 No Content). */
export function unsavePost(id: string): Promise<void> {
  return apiFetch<void>(`/api/v1/posts/${id}/bookmark`, { method: "DELETE" });
}

/** Create a new post and return the stored post. */
export function createPost(input: {
  content?: string;
  visibility?: string;
  storageKeys?: string[];
}): Promise<ApiPost> {
  return apiFetch<ApiPost>("/api/v1/posts", { method: "POST", body: input });
}

/**
 * Update an own post's editable fields (content and/or visibility) and return
 * the updated post. Existing media is preserved server-side (not editable here).
 */
export function updatePost(
  id: string,
  input: { content?: string; visibility?: string },
): Promise<ApiPost> {
  return apiFetch<ApiPost>(`/api/v1/posts/${id}`, {
    method: "PATCH",
    body: input,
  });
}

/** Delete an own post (204 No Content). */
export function deletePost(id: string): Promise<void> {
  return apiFetch<void>(`/api/v1/posts/${id}`, { method: "DELETE" });
}

/** Fetch a page of the authenticated user's feed. */
export function getFeed(
  params: { cursor?: string; limit?: number } = {},
  signal?: AbortSignal,
): Promise<FeedPage> {
  const query = new URLSearchParams();
  if (params.cursor) query.set("cursor", params.cursor);
  if (params.limit) query.set("limit", String(params.limit));
  const qs = query.toString();
  return apiFetch<FeedPage>(`/api/v1/feed${qs ? `?${qs}` : ""}`, { signal });
}

/* ------------------------------ Media ----------------------------------- */

/** Ask the backend for a presigned upload URL for one image. */
export interface MediaUploadUrlInput {
  type: "image";
  mimeType: string;
  sizeBytes: number;
  /**
   * Storage namespace: "post" (default) -> post media, "avatar" -> profile
   * photo. Omitting it keeps the existing post-upload behavior.
   */
  purpose?: "post" | "avatar";
}

/** Response from POST /api/v1/media/upload-url. */
export interface MediaUploadUrlResponse {
  uploadUrl: string;
  storageKey: string;
  publicUrl: string;
  expiresAt: string;
}

/** Response from POST /api/v1/media/confirm. */
export interface MediaConfirmResponse {
  storageKey: string;
  type: string;
  mimeType: string;
  sizeBytes: number;
  publicUrl: string;
}

/**
 * Request a presigned upload URL for one image. The server derives the object
 * key and validates the type/mime/size; the client only states its intent.
 */
export function requestMediaUploadUrl(
  input: MediaUploadUrlInput,
): Promise<MediaUploadUrlResponse> {
  return apiFetch<MediaUploadUrlResponse>("/api/v1/media/upload-url", {
    method: "POST",
    body: input,
  });
}

/**
 * Confirm an uploaded object. The server re-derives type/mime/size from storage
 * (never trusting the client) and returns the confirmed metadata.
 */
export function confirmMediaUpload(
  storageKey: string,
): Promise<MediaConfirmResponse> {
  return apiFetch<MediaConfirmResponse>("/api/v1/media/confirm", {
    method: "POST",
    body: { storageKey },
  });
}

/**
 * Upload the raw file bytes directly to object storage via a presigned URL.
 *
 * This request does NOT go to the Together backend: it is a cross-origin PUT to
 * the storage provider, so it sends no cookies and no Authorization header — the
 * presigned URL carries its own authorization. Passing the raw File as the body
 * lets the browser set Content-Length automatically (it may not be set manually).
 *
 * On failure it throws a generic error that never contains the presigned URL,
 * its query parameters, or the response URL.
 */
export async function uploadFileToPresignedUrl(
  uploadUrl: string,
  file: File,
): Promise<void> {
  let res: Response;
  try {
    res = await fetch(uploadUrl, {
      method: "PUT",
      body: file,
      headers: { "Content-Type": file.type },
      credentials: "omit",
    });
  } catch {
    // Never surface the presigned URL or underlying detail.
    throw new Error("media upload failed");
  }

  if (!res.ok) {
    throw new Error("media upload failed");
  }
}
