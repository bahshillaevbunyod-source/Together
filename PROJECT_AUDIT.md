# Together — Project Technical Audit

Audit generated from:
C:\Together

Source of truth:
Current files in C:\Together

D:\Together-Agent-Test inspected or used:
NO

Generated: 2026-09-13 23:00 (+05:00); updated 2026-09-14 23:39 (+05:00) for Media V1, realtime inspection, and the PID-file dev launcher.

Every factual statement below was re-verified against the current source in `C:\Together`. No application code, database, migrations, credentials, or runtime configuration were modified to produce this document. No secret values are printed. Where something could not be verified it is marked **UNKNOWN**.

---

## 1. Project overview

**Purpose.** Together is a social network ("Different people. One world.") — a feed of posts (text + media) with likes/comments/bookmarks, a follow/block social graph, notifications, private 1‑to‑1 messaging with realtime delivery, and automatic translation of posts/comments/messages.

**Architecture (verified).**
- **Frontend:** Next.js **15.5.25** (App Router), React **19.1.1**, TypeScript **5.9.2**, Tailwind CSS **4.1.13** (`@tailwindcss/postcss`), `lucide-react` **0.544.0**. Client talks to the backend over HTTP with `credentials:"include"`.
- **Backend:** Go (module `together/backend`, `go 1.27`), stdlib `net/http` with the Go 1.22 method+wildcard mux. Layered `internal/` packages (repository + service pattern).
- **Database:** PostgreSQL (pgx/v5 + pgxpool). Plain SQL migrations applied manually.
- **Storage:** AWS SDK for Go v2 (`aws-sdk-go-v2`, `service/s3`, `credentials`) against an S3-compatible endpoint — **Cloudflare R2**, path-style, presigned PUT uploads.
- **Realtime:** `github.com/gorilla/websocket` hub — per-user connection tracking + push.
- **Translation:** provider-agnostic `translation.Service`; real Google Cloud Translation **v2 (REST)** provider via direct HTTPS (no Google SDK dependency) + offline stub.

Dependencies (`backend/go.mod`): `aws-sdk-go-v2` v1.47.0 (+ `credentials` v1.20.4, `service/s3` v1.113.0), `gorilla/websocket` v1.5.3, `jackc/pgx/v5` v5.11.0, `golang.org/x/crypto` v0.57.0. No Google translation SDK (provider is hand-rolled REST).

Separate dev origins: frontend `http://localhost:3000`, backend `http://localhost:8080`.

---

## 2. Frontend audit

**Versions:** Next 15.5.25 · React 19.1.1 · TypeScript 5.9.2 · Tailwind 4.1.13.

**Routes (`src/app/`).** Route groups: `(app)` (auth-gated, `AppShell` + `RealtimeProvider`) and `(auth)` (no shell).

| Path | File | Status |
|---|---|---|
| `/` | `(app)/page.tsx` — Stories + Composer + Feed (FeedProvider) | PARTIAL |
| `/messages` | `(app)/messages/page.tsx` — conversation list + thread | DONE |
| `/profile` | `(app)/profile/page.tsx` — own profile | DONE |
| `/profile/edit` | `(app)/profile/edit/page.tsx` — edit profile | DONE |
| `/settings` | `(app)/settings/page.tsx` — translation prefs | DONE |
| `/u/[username]` | `(app)/u/[username]/page.tsx` — public profile + follow + message | DONE |
| `/login` | `(auth)/login/page.tsx` — login + register | DONE |

No dedicated `/bookmarks`, `/notifications`, `/discover` pages exist (Sidebar shows those labels; only Home, Messages, Profile, Settings have hrefs — the rest are non-navigating buttons).

**Feature status (verified from current code):**

| Feature | Status | Notes |
|---|---|---|
| Auth gate | DONE | `(app)/layout.tsx` client gate: redirects to `/login` only on `unauthenticated`; branded loader while loading; wraps `AppShell` + `RealtimeProvider`. |
| AuthProvider | DONE | `src/lib/auth-context.tsx` — `getMe()`; 401→unauthenticated; network/5xx retried with backoff `[500,1000,2000,4000]ms`, no false sign-out. |
| API client | DONE | `src/lib/api.ts` — typed, `credentials:"include"`, `ApiError{status,message}`, 204/no-JSON/abort handling. Includes media-upload helpers: `requestMediaUploadUrl`, `uploadFileToPresignedUrl` (raw PUT, `credentials:"omit"`), `confirmMediaUpload`. |
| FeedProvider | DONE | `src/lib/feed-context.tsx` — posts, cursor, loadMore, prependPost. |
| Feed | DONE | `Feed.tsx` + `PostCard.tsx` render real feed. |
| Composer / createPost | DONE (text + 1–8 images) | Text and/or 1–8 images; Photo button opens the picker; `canPost` allows text OR image; `submitPost` uploads images then sends `storageKeys` in order. (Video/Voice/Poll/Feeling buttons remain inert.) |
| PostCard | DONE (text/actions) | Renders content, author, relative time, translation toggle. |
| Likes | DONE | Real like/unlike with authoritative state. |
| Comments | DONE | Load/create + per-comment translation toggle. |
| Bookmarks | PARTIAL | Save/unsave action works in PostCard; **no bookmarks list page**. |
| Notifications | DONE | `NotificationsBell.tsx` — real list, unread count, mark read / read-all. No standalone page. |
| Messages | DONE | List + open/create conversation + thread history + pagination + mark-read. |
| Message sending | DONE | `ConversationThread.tsx` — real `sendMessage`, appends server message, error handling. |
| WebSocket client | DONE | `src/lib/realtime-context.tsx` — single WS, backoff reconnect, `subscribeMessageCreated`; used by messages page + thread. |
| Translation UI | DONE | Per-item "Show original/translation" toggle + `SRC → TGT` hint in posts, comments, messages. |
| Media upload UI | DONE (images) | `Composer.tsx` — 1–8 image picker, per-file validation (JPEG/PNG/WebP ≤15 MiB), preview gallery, sequential presign→PUT→confirm upload with retry/reuse (no duplicate R2 objects), storageKeys sent to `createPost` in order. Video not included. |
| Media rendering | DONE (images) | `PostMedia.tsx` — single image at natural aspect clamped to [4:5, 16:9] (`object-cover`, no forced 4:3); 2–8 images in a premium native scroll-snap carousel (one stable frame from the first image, dots for 2–5 / thin progress for 6–8, no arrows, no counter, keyboard-navigable). Video not rendered. |
| Profile / settings | DONE | Own profile, edit profile (friendly errors), settings translation prefs (language + auto-translate). |
| Follow UI | DONE | Follow/unfollow on `/u/[username]`. |
| Block UI | NOT STARTED | No block functions in `api.ts`; backend endpoints exist but are unused by the frontend. |
| Logout | DONE | `HeaderUser.tsx` dropdown → `logout()`. |
| Stories | MOCK | `StoriesRow.tsx` from `src/data/stories.ts`. |
| Right sidebar | MOCK | `SuggestedPeople.tsx`, `WorldMapCard.tsx` static. |

**Remaining mock/local data:** `src/data/stories.ts` (Stories), `src/data/posts.ts` (legacy static posts — not used by the live feed path), right-sidebar suggested people / world map.

---

## 3. Backend audit

Packages in `backend/internal/`: `auth, block, bookmark, comment, commentservice, config, conversation, db, follow, followservice, like, likeservice, media, notification, post, postservice, ratelimit, realtime, server, session, storage, translation, user`.

- **auth** (`internal/auth`, `server/auth.go`): bcrypt hashing/policy; register (also creates a session → auto-login), login, logout, `/auth/me`; session token generated then **SHA-256 hashed** before storage; `together_session` cookie.
- **password hashing:** bcrypt (`golang.org/x/crypto/bcrypt`) in `internal/auth`.
- **sessions** (`session`, `server`): opaque random token issued to client; only its SHA-256 hash persisted (`sessions.token_hash`); 7‑day TTL.
- **users/profile** (`user`, `server/profile.go`, `server/users.go`): `User` incl. `PreferredLanguage`, `AutoTranslateEnabled`, `NativeLanguage`; profile GET/PATCH; public profile by username with follower/following counts + `isFollowing`/`isSelf`.
- **follows/blocks** (`follow`, `followservice`, `block`): follow graph with follower/following lists + counts; block graph hides both directions. `followservice` writes follow + notification atomically.
- **posts** (`post`, `postservice`): create with media (transactional `CreatePostWithMedia`), read (visibility public/followers/private + block filtering), edit, delete, feed.
- **likes** (`like`, `likeservice`): like/unlike; `likeservice` creates like + `post_like` notification atomically.
- **comments** (`comment`, `commentservice`): create/list/edit/delete; `commentservice` creates comment + `post_comment` notification atomically.
- **bookmarks** (`bookmark`): save/unsave/list.
- **notifications** (`notification`): list, unread-count, mark one read, mark all read; types `follow`, `post_like`, `post_comment`.
- **conversations/messages** (`conversation`): canonical-pair conversations, idempotent `OpenPrivateConversation`, `CreateMessage` (returns message + recipient id), keyset-paginated `ListMessages`, `ListConversations` (unread via LATERAL), `MarkConversationRead`.
- **realtime** (`realtime.Hub`, `server/websocket.go`): per-user client sets, non-blocking send, slow-client drop.
- **translation** (`translation`): `Service` interface, `StubService`, `googleService` (v2 REST); applied to posts/comments/messages per viewer/recipient prefs; failures swallowed (original preserved).
- **media** (`media`, `server/media*.go`): presign, confirm, validation, post attachment; `post_media` persistence.
- **storage** (`storage.S3Repository`): presigned PUT, HeadObject, DeleteObject against R2.
- **rate limiting** (`ratelimit`): in-memory limiter, applied to register + login.
- **CSRF/CORS** (`server/middleware.go`, `server.go`): Origin-pinned CSRF on unsafe methods; credentialed CORS for `AppOrigin` only.
- **validation:** `MaxBytesReader` + `DisallowUnknownFields` on JSON bodies; content-type checks; length/visibility/media limits.

Architecture is repository interfaces + Postgres implementations, with cross-package atomic use-cases in `*service` packages; the HTTP layer is the `server` package assembling handlers via `server.New(...)`.

---

## 4. Database + migrations

**Migration count: 15** (numbered `000001`–`000015`, each with `.up.sql`/`.down.sql`) plus `backend/migrations/README.md`.

| # | File | Creates / changes |
|---|---|---|
| 000001 | create_users | `users` |
| 000002 | add_user_credentials | password/credential columns on `users` |
| 000003 | create_sessions | `sessions` (token_hash, expires_at) |
| 000004 | create_follows | `follows` |
| 000005 | create_blocks | `blocks` |
| 000006 | create_posts | `posts` (visibility) |
| 000007 | create_post_likes | `post_likes` |
| 000008 | create_post_comments | `post_comments` |
| 000009 | create_post_media | `post_media` |
| 000010 | allow_media_only_posts | relax post content-required constraint |
| 000011 | unique_post_media_storage_key | `post_media_storage_key_unique` |
| 000012 | create_post_bookmarks | `post_bookmarks` |
| 000013 | create_notifications | `notifications` |
| 000014 | create_conversations | `conversations`, `messages`, `conversation_participants` |
| 000015 | add_user_language_prefs | `users.preferred_language`, `users.auto_translate_enabled` |

**Key structures/constraints (verified):**
- **post_media:** `type IN ('image','video')`, `size_bytes > 0`, `sort_order >= 0`, nullable positive `width`/`height`, nonneg `duration_ms`; `UNIQUE(post_id, sort_order)`; `UNIQUE(storage_key)` (000011); index `(post_id, sort_order)`; FK `post_id → posts ON DELETE CASCADE`.
- **conversations:** canonical ordered pair `CHECK(user_low < user_high)`, `UNIQUE(user_low, user_high)`, FKs to `users ON DELETE CASCADE`; indexes on both member columns.
- **messages:** FK `conversation_id`, `sender_id`; `CHECK(length(btrim(content)) > 0)`; keyset index `(conversation_id, created_at DESC, id DESC)`.
- **conversation_participants (read-state):** PK `(conversation_id, user_id)`, `last_read_message_id` (FK, ON DELETE SET NULL), `last_read_at`; index on `user_id`.
- **translation preferences:** `users.preferred_language text` (nullable → fall back to native), `users.auto_translate_enabled boolean NOT NULL DEFAULT false`.

---

## 5. Current API inventory

Rebuilt from `server.go registerRoutes`. Auth = requires session; CSRF = Origin-checked (unsafe methods). "FE" = used by current frontend.

| Method | Path | Auth | CSRF | Purpose | FE |
|---|---|---|---|---|---|
| GET | /health | no | no | liveness | (ops) |
| GET | /ready | no | no | readiness (DB ping) | (ops) |
| POST | /api/v1/auth/register | no (rate-limited) | no | register + auto-login | yes |
| POST | /api/v1/auth/login | no (rate-limited) | no | login | yes |
| POST | /api/v1/auth/logout | yes | yes | logout | yes |
| GET | /api/v1/auth/me | yes | no | current user | yes |
| GET | /api/v1/protected/ping | yes | no | protected ping | no |
| GET | /api/v1/profile | yes | no | own profile | yes |
| PATCH | /api/v1/profile | yes | yes | update profile / prefs | yes |
| GET | /api/v1/users/{username} | no | no | public profile | yes |
| GET | /api/v1/users/{username}/followers | no | no | followers list | no |
| GET | /api/v1/users/{username}/following | no | no | following list | no |
| POST | /api/v1/users/{username}/follow | yes | yes | follow | yes |
| DELETE | /api/v1/users/{username}/follow | yes | yes | unfollow | yes |
| POST | /api/v1/users/{username}/block | yes | yes | block | no |
| DELETE | /api/v1/users/{username}/block | yes | yes | unblock | no |
| POST | /api/v1/posts | yes | yes | create post (+storageKeys) | yes (text + images) |
| GET | /api/v1/posts/{id} | optional | no | get post | no |
| PATCH | /api/v1/posts/{id} | yes | yes | edit post | no |
| DELETE | /api/v1/posts/{id} | yes | yes | delete post | no |
| GET | /api/v1/feed | yes | no | feed | yes |
| GET | /api/v1/bookmarks | yes | no | list bookmarks | no |
| GET | /api/v1/ws | (session in handler) | Origin-checked | WebSocket | yes |
| GET | /api/v1/conversations | yes | no | list conversations | yes |
| POST | /api/v1/conversations | yes | yes | open/create conversation | yes |
| GET | /api/v1/conversations/{id}/messages | yes | no | list messages | yes |
| POST | /api/v1/conversations/{id}/messages | yes | yes | send message | yes |
| POST | /api/v1/conversations/{id}/read | yes | yes | mark read | yes |
| GET | /api/v1/notifications | yes | no | list notifications | yes |
| GET | /api/v1/notifications/unread-count | yes | no | unread count | yes |
| POST | /api/v1/notifications/{id}/read | yes | yes | mark one read | yes |
| POST | /api/v1/notifications/read-all | yes | yes | mark all read | yes |
| POST | /api/v1/posts/{id}/like | yes | yes | like | yes |
| DELETE | /api/v1/posts/{id}/like | yes | yes | unlike | yes |
| POST | /api/v1/posts/{id}/comments | yes | yes | create comment | yes |
| GET | /api/v1/posts/{id}/comments | optional | no | list comments | yes |
| POST | /api/v1/posts/{id}/bookmark | yes | yes | save | yes |
| DELETE | /api/v1/posts/{id}/bookmark | yes | yes | unsave | yes |
| POST | /api/v1/media/upload-url | yes | yes | presign upload | yes |
| POST | /api/v1/media/confirm | yes | yes | confirm upload | yes |
| PATCH | /api/v1/comments/{id} | yes | yes | edit comment | no |
| DELETE | /api/v1/comments/{id} | yes | yes | delete comment | no |

---

## 6. Authentication

- **Registration** (`POST /auth/register`): validates fields incl. required `nativeLanguage`; bcrypt-hashes password; creates user **and** a session (auto-login) + cookie; rate-limited.
- **Login** (`POST /auth/login`): verifies bcrypt; creates session + cookie; generic error `invalid email or password`; rate-limited.
- **Logout** (`POST /auth/logout`): deletes server session (if cookie present) and clears cookie; CSRF-protected.
- **/me** (`GET /auth/me`): returns current user; 401 when signed out.
- **Session creation/persistence:** opaque random token to client; **SHA-256 hash** stored (`sessions.token_hash`); TTL **7 days**.
- **Cookie config** (`together_session`): `Path=/`, `Expires`+`MaxAge`=7d, `HttpOnly=true`, `Secure=IsProduction()` (off in dev), `SameSite=Lax`.
- **Frontend auth retry:** transient network/5xx retried with backoff, only 401 signs out (`auth-context.tsx`).
- **Route protection:** server via `requireAuth`; client via `(app)/layout.tsx` gate.
- **Logout frontend:** DONE (HeaderUser dropdown).

**Gaps:** No refresh/rotation of session tokens; no CSRF token scheme (relies on strict Origin check — acceptable for the SameSite=Lax cookie model); `Secure` only in production (correct for local HTTP dev).

---

## 7. Frontend ↔ backend integration

- **FULLY INTEGRATED:** Auth (register/login/logout/me + gate), Feed, Post creation (text + 1–8 images), Image upload (presign→PUT→confirm→attach), Media rendering (single natural-ratio image + 2–8 carousel), Likes, Comments (+translation), Bookmarks (save/unsave action), Notifications (bell), Conversations list, Message history, Message sending, Realtime message delivery, Translation display (posts/comments/messages), Profile (view/edit), Settings (translation prefs), Follow/unfollow, Public profile.
- **PARTIALLY INTEGRATED:** Bookmarks (no list page).
- **BACKEND ONLY:** Post edit/delete, Comment edit/delete, Block/unblock, Followers/following lists, Get single post, Bookmarks list endpoint. (Video upload/rendering is out of scope for Media V1.)
- **MOCK / LOCAL:** Stories, Right sidebar (suggested people, world map).

---

## 8. Realtime

- **Hub** (`internal/realtime/hub.go`): per-user set of clients; non-blocking send; drops slow clients; register/unregister lifecycle.
- **WebSocket endpoint** `GET /api/v1/ws` (`server/websocket.go`): authenticates the session first; **strict Origin match** against `AppOrigin` (mirrors CSRF) before upgrade; upgrader also checks origin.
- **Events implemented:** **one** — `message.created`, delivered **to the recipient only**, payload = message fields (incl. `translatedContent`/`sourceLanguage`/`targetLanguage`) + `conversationId`.
- **Recipient behavior:** sender does not receive their own event (they get the message from the POST response); recipient's open thread appends + marks read; conversation list updates unread.
- **Frontend client** (`realtime-context.tsx`): single shared WS (one `RealtimeProvider` in the authenticated `(app)` layout — not remounted on navigation), converts `http(s)`→`ws(s)`, exponential backoff reconnect capped at 10s, resets backoff on healthy connection, intentional teardown on unmount/logout; exposes `subscribeMessageCreated`.
- **Stability (inspected):** HEALTHY — no lifecycle bug. Verified live: healthy backend = 0 new sockets / 0 errors over 15s; navigation Home↔Messages↔Profile = 0 reconnects; a backend restart = exactly one clean reconnect with realtime delivery still working. The occasional `WebSocket connection failed` / `ERR_CONNECTION_*` console lines are browser-emitted during genuine backend unavailability (restarts) or hard reload — not an application defect (no `console.*` noise in the client).
- **Limitations:** no typing indicators, no read-receipt events, no realtime notification events (notifications are polled via the bell). Only messaging is realtime.

---

## 9. Translation

- **Service interface** (`translation/translation.go`): `Translate(ctx, Request) (*Result, error)`; `Request.Validate` trims, length-limits (≤5000 runes), lowercases codes, maps `auto`→"" (auto-detect), validates BCP-47-ish codes; `ValidLanguage` shared for stored prefs.
- **Google provider** (`translation/provider.go`): Google Cloud Translation **v2 REST**; API key sent only via `X-Goog-Api-Key` header (never URL/logs); 10s timeout; opaque `ErrProviderRequestFailed`; auto-detect resolves `detectedSourceLanguage`.
- **Stub provider** (`translation/stub.go`): offline; echoes input (used when provider is `stub`/unset).
- **Config/selection** (`config.go`): `TRANSLATION_PROVIDER` (default `stub`), `TRANSLATION_API_KEY`, `TRANSLATION_ENDPOINT`; `validateTranslation` requires the key when provider=`google`; never echoes secrets.
- **Applied in:** posts (`server/posts_read.go`), comments (`server/comments.go`), messages (`server/conversations.go`) — per viewer/recipient prefs.
- **preferredLanguage / autoTranslateEnabled:** stored on `users` (migration 000015); editable via settings; used to decide target language + whether to translate.
- **Realtime events:** `message.created` carries translation fields for the recipient.
- **Failure fallback:** translation errors are swallowed; original content preserved.
- **Batching / caching:** NONE (each item translated independently on read; no cache layer).
- **Frontend integration:** DONE — per-item toggle + `SRC → TGT` hint; `hasTranslation` requires non-empty translated text ≠ original.
- **Secret config presence (names only):** `TRANSLATION_API_KEY` — **PRESENT** (value never shown).
- **STEP 51.6 launcher:** `backend/scripts/dev-backend.ps1` loads **all** valid `KEY=VALUE` entries from `backend/.env` into the process (not only `DATABASE_URL`) — verified this session (launcher reported provider `google`, key present).
- **Live translation:** **LIVE VERIFIED** — supported by current state; the running backend uses provider `google` with the key loaded via the launcher.

---

## 10. Media / Cloudflare R2

- **storage.S3Repository** (`internal/storage/s3.go`): AWS SDK v2 S3 client; `Region` from config; static credentials provider; `BaseEndpoint` = configured endpoint; `UsePathStyle=true` (required for R2). Credentials held by the SDK, never logged.
- **CreateUploadURL:** presigned **PUT**, pinned to `ContentType` + `ContentLength`; **expiry 10 minutes** (`uploadURLExpiry`).
- **HeadObject:** returns `ContentType` + `SizeBytes`; maps missing object to `ErrObjectNotFound`.
- **DeleteObject:** deletes by key.
- **Upload handlers** (`server/media_upload.go`): `handleCreateUploadURL` validates type/mime/size, derives a **safe extension from the mime** (never the client), generates a server-side key `users/{userID}/uploads/{uuidv4}.{ext}`, returns `{uploadUrl, storageKey, publicUrl, expiresAt}`. `handleConfirmUpload` re-derives type/mime/size from **HeadObject** (storage truth) and returns confirmed metadata + `publicUrl`.
- **Validation/ownership** (`server/media_validate.go`): enforces `users/{userID}/uploads/` prefix, rejects `..`, backslash, nested/extra `/`; trusts only HeadObject for type/mime/size.
- **Generated storage keys:** server-generated from authenticated user id + UUID + safe ext.
- **Public URL generation** (`server/media.go`): `mediaURL` joins `MEDIA_PUBLIC_BASE_URL` + storage key; `storage_key` never exposed in responses.
- **Post attachment** (`server/posts.go`): `POST /posts` accepts `storageKeys[]`; `resolveMediaForPost` enforces **≤8 items** (product media limit — `maxPostMedia = 8`), rejects duplicates, re-validates each via HeadObject, assigns `sort_order` by array index; persisted atomically via `CreatePostWithMedia`; `post_media_storage_key_unique` → 409 on reuse.
- **Allowed types / limits (current code):** images `image/jpeg`, `image/png`, `image/webp` ≤ **15 MiB**; videos `video/mp4`, `video/webm` ≤ **200 MiB** (video accepted by the API but not produced/rendered by the frontend in Media V1); **max 8 media per post**; metadata bodies ≤ 1 MiB.
- **Frontend upload support (Media V1):** `Composer.tsx` selects 1–8 images, validates each (JPEG/PNG/WebP ≤15 MiB), then uploads **sequentially** via `requestMediaUploadUrl` → `uploadFileToPresignedUrl` (raw PUT to R2) → `confirmMediaUpload`, remembering each `storageKey`/confirmed flag so a retry after a failed `createPost` reuses completed work (no duplicate R2 objects); `storageKeys` are passed to `createPost` in selection order. Never deletes R2 objects on failure.
- **Frontend media rendering (Media V1):** `PostMedia.tsx` — single image at natural aspect clamped to [4:5, 16:9] with `object-cover` (no forced 4:3, no letterbox); 2–8 images in a native scroll-snap carousel with one stable frame (first image's clamped ratio), dots (2–5) / thin progress (6–8), no arrows, no numeric counter, keyboard-navigable, `sizes` tuned to the real ~640px column, `quality=85`. `next.config.ts` allows `localhost` + any `https` host (covers the R2 public domain). Video not rendered.

**Env presence (names only, values never printed):**
- STORAGE_ENDPOINT — **PRESENT**
- STORAGE_REGION — **PRESENT**
- STORAGE_BUCKET — **PRESENT**
- STORAGE_ACCESS_KEY_ID — **PRESENT**
- STORAGE_SECRET_ACCESS_KEY — **PRESENT**
- MEDIA_PUBLIC_BASE_URL — **PRESENT**

**Cloudflare R2 status:**
- CODE IMPLEMENTED: **YES**
- CONFIGURED: **YES**
- LIVE VERIFIED: **YES** (live smoke test passed: presign PUT, real PUT, HeadObject with Content-Type + size match, public GET 200 with body match, DeleteObject, cleanup confirmed object-not-found; additionally confirmed end-to-end via the browser — 1/2/4/8-image posts upload and render from real `pub-*.r2.dev` URLs and survive refresh)
- FRONTEND INTEGRATED: **YES** (images, 1–8 per post; video excluded from Media V1)

---

## 11. Security

Confirmed present:
- **bcrypt** password hashing (`internal/auth`).
- **Session token hashing:** raw token to client, only SHA-256 hash stored.
- **Cookies:** HttpOnly, SameSite=Lax, Secure in production, Path=/, 7d.
- **CSRF:** strict Origin match on unsafe methods (`csrfProtect`).
- **CORS:** credentialed, restricted to `AppOrigin` only; OPTIONS preflight handled.
- **WebSocket Origin validation:** strict match before upgrade + session auth.
- **Authorization:** author-only edit/delete; post visibility (public/followers/private) + block filtering; media ownership by storage-key prefix.
- **Input limits / strict decoding:** `MaxBytesReader` + `DisallowUnknownFields`; content-type checks; content length/visibility/media-count limits.
- **Media/MIME validation:** allow-listed MIME → safe extension; size caps; storage truth via HeadObject; storage-key ownership + traversal rejection.
- **Rate limiting:** register + login.
- **Secrets handling:** secrets read from env only; provider key sent via header only; errors never echo keys/URLs; launcher prints key **names** only.

No confirmed vulnerabilities were found in current source. (Observations, not vulnerabilities: no CSRF token beyond Origin; translation has no cache/rate-limit of its own; `internal/config/config.go` retains a stale comment "Storage … no SDK/client yet" though the SDK is wired.)

---

## 12. Local development / runtime

- **Frontend:** `npm run dev` (Next dev) on port **3000**; started detached in local workflow (log in `%TEMP%\together-dev.log`).
- **Backend:** `backend/scripts/dev-backend.ps1` — **PID-file launcher**: builds to `backend\.dev-bin\together-api.exe` (stable project-local path, git-ignored via `/.dev-bin/`), **loads all valid `KEY=VALUE` from `backend/.env`** into the process env (splits on first `=`, strips quotes, ignores blanks/comments/malformed keys), stops **only** the PID it recorded in `backend\.dev-bin\together-api.pid` (and only after confirming that PID still points at our exe), `Wait-Process` until it exits, then starts the backend detached and records the new PID. Logs to `%TEMP%\together-api.log` (stdout) + `.log.err` (stderr). Prints key **names** + provider + key-present boolean only — never values. ASCII-only (PS 5.1 safe).
- **No network/port scanning in the launcher (AV-friendly):** earlier launcher variants that added in-script `Invoke-WebRequest`/`TcpClient`/port-owner enumeration were deterministically quarantined by Avast's behavior shield (kill + hidden-spawn + network = dropper heuristic). The current PID-file design removes all in-script network activity; `/health`, `/ready`, and single-listener verification are done by the caller. **5 consecutive restarts passed** (distinct PID each time, exactly one listener, /health & /ready 200, no bind race, script/exe not quarantined).
- **.env loading behavior:** full `.env` (DATABASE_URL, STORAGE_*, TRANSLATION_*) — **verified this session**.
- **Ports:** backend 8080, frontend 3000.
- **Logs:** in `%TEMP%` as above.
- **PostgreSQL:** local Postgres; `DATABASE_URL` from `.env` (dev fallback DSN exists in config for dev/test only).
- **/health:** 200 (verified). **/ready:** 200 (verified, includes DB ping).

---

## 13. Tests

- **Go tests:** present across `server` (auth, posts, comments, likes, bookmarks, messaging/conversations, notifications, follow/block, media upload/confirm/validate, ratelimit, csrf, profile, users, realtime, server/ready), plus `internal` packages (`auth`, `bookmark`, `commentservice`, `config`, `conversation`, `followservice`, `likeservice`, `media`, `notification`, `postservice`, `realtime`, `storage`, `translation`). Packages without tests: `block`, `comment`, `db`, `follow`, `like`, `post`, `ratelimit`(pkg), `session`, `user`.
- **Frontend tests:** none (no unit/component/e2e test setup).
- **Integration/e2e:** none automated (live R2/translation verified manually via STEP 51.7 / 51.5).

**Validation run this session (real results):**
- `go build ./...` → **PASS** (exit 0)
- `go vet ./...` → **PASS** (exit 0)
- `go test ./...` → **PASS** (exit 0; all packages ok/cached, no failures)
- `npx tsc --noEmit` → **PASS** (exit 0)

`next build` was intentionally **not** run (dev server active).

**Missing test categories:** frontend tests, HTTP-level integration/e2e, live-provider contract tests (R2/Google) behind a flag.

---

## 14. Technical debt (current, confirmed)

1. **Video not supported** — Media V1 is images only; `map-post.ts` still filters out video and `PostMedia` renders no `<video>` (the backend accepts video MIME/size but the frontend neither uploads nor renders it).
2. **No orphan cleanup for abandoned uploads** — by design, R2 objects are never deleted on `createPost` failure (a post-commit failure is ambiguous), so an uploaded-but-never-posted image can linger. A storage lifecycle/expiry rule is the intended future remedy (out of scope).
3. **Bookmarks list page missing** — save/unsave works and `GET /bookmarks` exists, but no `/bookmarks` route.
4. **Block UI missing** — backend block/unblock endpoints exist; no frontend functions/UI.
5. **Post/comment edit & delete UI missing** — backend endpoints exist; no frontend affordances.
6. **Stories & right sidebar are mock** — static data, no backend.
7. **Translation has no caching/batching** — each post/comment/message translated per read; potential cost/latency at scale.
8. **No realtime for notifications/typing/read receipts** — only `message.created` is pushed; notifications are polled.
9. **Dev launcher does not self-verify readiness** — to stay AV-friendly the launcher does no in-script `/health` polling; readiness is verified by the caller.
10. **Stale comment** in `internal/config/config.go` ("no SDK/client yet") contradicts the now-wired S3 SDK (doc-only).

Resolved since the previous audit (removed): media upload UI (1–8 images), multi-image rendering / premium carousel, natural/clamped single-image ratios, Composer `canPost`/`submitPost` mismatch, backend media max reduced 10→8, realtime lifecycle inspection (healthy), and the AV-triggered restart race (PID-file launcher).

---

## 15. Current project status table

| Area | Status |
|---|---|
| Auth | DONE |
| Session persistence | DONE |
| Feed | DONE |
| Post creation | DONE (text + 1–8 images) |
| Media backend | DONE (max 8 media/post) |
| Cloudflare R2 | DONE (code + configured + LIVE VERIFIED) |
| Media upload frontend | DONE (images 1–8; video excluded) |
| Media rendering | DONE (single natural/clamped ratio + 2–8 carousel; video excluded) |
| Post edit/delete | PARTIAL (backend DONE, frontend NOT STARTED) |
| Likes | DONE |
| Comments | DONE |
| Bookmarks | PARTIAL (action DONE, list page NOT STARTED) |
| Notifications | DONE |
| Follows/blocks | PARTIAL (follow DONE; block backend-only) |
| Conversations | DONE |
| Message sending | DONE |
| WebSocket backend | DONE |
| WebSocket frontend | DONE (inspected: healthy, no lifecycle bug) |
| Translation backend | DONE |
| Translation live provider | DONE (LIVE VERIFIED) |
| Translation frontend | DONE |
| Profile/settings | DONE |
| Stories | MOCK |
| RightSidebar | MOCK |
| Security | DONE |
| Local runtime | DONE |

---

## 16. Recommended next steps

1. **Add a Bookmarks page** using the existing `GET /bookmarks`.
2. **Add post/comment edit & delete UI** on the existing endpoints.
3. **Add block/unblock UI** (and wire followers/following lists) using existing endpoints.
4. **Consider translation caching** to cut repeat provider calls on feed reads.
5. **Backfill tests** for `block`, `follow`, `like`, `post`, `session`, `user` packages and add minimal frontend tests.
6. **(Future) Video media** — extend upload + `PostMedia`/`map-post.ts` to carry and render `<video>` (out of scope for Media V1).
7. **(Future) Orphaned-upload cleanup** — a storage lifecycle/expiry rule for uploaded-but-never-posted R2 objects.

---

## 17. Audit completeness check

- frontend inspected — YES
- backend inspected — YES
- migrations inspected — YES (15 migrations)
- API routes inspected — YES (from `registerRoutes`)
- auth inspected — YES
- realtime inspected — YES
- translation inspected — YES
- media inspected — YES
- Cloudflare R2 current status documented — YES (LIVE VERIFIED)
- security inspected — YES
- runtime inspected — YES
- tests inspected — YES (build/vet/test/tsc all PASS)

**UNKNOWN items:** none material to this audit. (Exact `.env` values are intentionally not inspected; only presence was checked. Live provider behavior under production load is not covered by automated tests.)
