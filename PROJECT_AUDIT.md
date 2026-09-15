# Together — Backend Project Audit

**Scope:** current backend source, migrations, repository state, and Go verification in `C:\Together` only. No `.env` values, database, migrations, or runtime processes were inspected or changed.

## 1. Current Git state

- Branch: `main`.
- Recent commits: `eef878c fix: validate discover cursor UUID`; `ab7774c fix: configure Next image quality`; `ffb676b feat: add discover posts experience`; `68f2c07 feat: add discover posts backend`; `2b4ab09 docs: add Codex engineering rules`.
- Committed backend state includes Discover Posts (`68f2c07`).
- At audit start, the only uncommitted files were the new topic migration pair. This audit additionally modifies this document:
  - `backend/migrations/000016_create_topics.up.sql` — untracked
  - `backend/migrations/000016_create_topics.down.sql` — untracked
  - `PROJECT_AUDIT.md` — modified by this audit

## 2. Backend architecture

- Go module: `together/backend`; HTTP API is `cmd/api` plus the standard-library `net/http` server in `internal/server`.
- `server.New` assembles repositories, cross-domain services, translation, and the realtime hub. Routes are centrally registered in `internal/server/server.go`.
- Domain packages use repository interfaces with PostgreSQL/pgx implementations: users, sessions, follows, blocks, posts, likes, comments, bookmarks, notifications, conversations, media, and storage.
- Cross-domain transactions are explicit services: `postservice` creates post + media; `followservice`, `likeservice`, and `commentservice` create their relation/content plus notification atomically.
- PostgreSQL access uses `pgx/v5` and `pgxpool`. Migrations are plain SQL files; no migration runner exists in the repository.
- Realtime uses Gorilla WebSocket and a concurrency-safe per-user hub. The implemented pushed event is `message.created` for the recipient.
- Media uses AWS SDK v2 against an S3-compatible endpoint (including Cloudflare R2-compatible configuration): presigned PUT, HeadObject validation, and DeleteObject.
- Translation is provider-agnostic. The current source supports `stub` and Google Translation v2 REST; provider selection and credentials are environment-driven.

## 3. Database

Committed migrations are `000001` through `000015`:

| Range | Schema purpose |
|---|---|
| 000001–000005 | users, credentials, sessions, follows, blocks |
| 000006–000012 | posts, likes, comments, media, nullable media-only content, media-key uniqueness, bookmarks |
| 000013–000015 | notifications; conversations/messages/read state; user translation preferences |

Important tables are `users`, `sessions`, `follows`, `blocks`, `posts`, `post_likes`, `post_comments`, `post_media`, `post_bookmarks`, `notifications`, `conversations`, `messages`, and `conversation_participants`.

`backend/migrations/README.md` says migrations are applied manually. No database was queried and no migration ledger is present in the repository, so the latest **applied** migration is **UNKNOWN**. The highest committed migration **available in source** is `000015`.

Uncommitted, not-applied-by-this-audit migration `000016_create_topics` adds:

- `topics`: UUID primary key, unique non-empty trimmed/lowercase slug, `char_length(slug) <= 100`, and `created_at`.
- `post_topics`: `(post_id, topic_id)` primary key; both foreign keys cascade on deletion.
- `(topic_id, post_id)` index for topic-to-post lookup.

`000016` is **UNCOMMITTED**. Its applied state is **UNKNOWN** and must not be assumed.

## 4. Auth, privacy, and security

- Authentication is server-side session based. Raw random session tokens are sent only in the `together_session` HttpOnly cookie; the database stores SHA-256 token hashes. Cookie lifetime is seven days, SameSite is Lax, and Secure is enabled in production.
- Registration/login use bcrypt. Register and login are IP rate-limited.
- `requireAuth` resolves the session and user before protected handlers. State-changing routes are protected by an exact-Origin CSRF check. Credentialed CORS is restricted to the configured app origin.
- Post ownership is enforced for update/delete. Post visibility is `public`, `followers`, or `private`; reads check visibility, follow state, and a block relationship in either direction. A blocked post is hidden as not found.
- Feed, bookmark, user search, user discovery, and post discovery queries exclude both directions of blocks where applicable. Post discovery also limits results to public posts by non-followed, non-self authors.
- SQL values are bound with PostgreSQL parameters. The few dynamic `UPDATE`/discover query constructions select only fixed, server-owned column names or query fragments; user values remain bound parameters.
- Configuration reads secrets from environment variables. Source comments and error paths avoid logging raw session tokens, DSNs, storage credentials, or translation keys.

## 5. Implemented backend features

- Users and profiles: registration, login/logout, current-user lookup, profile update, public profile, follower/following lists, and user search.
- Posts: text/media creation, get, owner-only update/delete, authenticated following feed, and authenticated Discover Posts.
- Media: server-generated upload keys, MIME/size validation, presigned upload URLs, confirmation via storage metadata, ordered post attachment, and media response mapping.
- Social actions: follows/unfollows, blocks/unblocks, post likes, comments (create/list/update/delete), and bookmarks (save/remove/list).
- Notifications: follow, post-like, and post-comment notifications; list, unread count, mark-one, and mark-all-read.
- Messaging/realtime: idempotent private conversations, messages, read markers, conversation/message lists, and recipient `message.created` WebSocket delivery.
- Discovery/search: authenticated user search; user discovery modes `for_you`, `world`, and `popular`; authenticated Discover Posts.
- Translation: per-user language preferences and translation of posts, comments, and messages with graceful fallback to original content on provider failure.

## 6. Pagination

- Keyset pagination is used for feed, Discover Posts, bookmarks, comments, followers/following, notifications, conversations, messages, and user discovery.
- Feed-style cursors encode `created_at` plus UUID ID and are rejected with HTTP 400 when malformed or when the ID is not a UUID. Limits are parsed and bounded before repository calls.
- User discovery uses a base64url JSON cursor bound to its mode and, for world discovery, the country filter. Its ordering includes a final ID tie-breaker.
- Other cursor parsers validate their expected encoded structure; malformed cursors and invalid limits return HTTP 400 in the handlers.

## 7. Backend endpoints

| Feature | Main routes |
|---|---|
| Health | `GET /health`, `GET /ready` |
| Auth/profile | `POST /api/v1/auth/register`, `POST /api/v1/auth/login`, `POST /api/v1/auth/logout`, `GET /api/v1/auth/me`, `GET/PATCH /api/v1/profile` |
| Users/social graph | `GET /api/v1/users/search`, `GET /users/discover`, public profile/follower/following reads, follow and block mutations |
| Posts | `POST /api/v1/posts`, `GET/PATCH/DELETE /posts/{id}`, `GET /feed`, `GET /posts/discover` |
| Interactions | like/unlike, comments, bookmarks, and notifications under `/api/v1/posts/...` and `/api/v1/notifications/...` |
| Media | `POST /api/v1/media/upload-url`, `POST /api/v1/media/confirm` |
| Messaging/realtime | `/api/v1/conversations`, conversation messages/read routes, and `GET /api/v1/ws` |

Protected GET routes require a valid session where documented in route registration. Unsafe authenticated routes additionally require the configured Origin.

## 8. Tests and verification

- `go test ./...` — **PASS** on 2026-09-15.
- `go vet ./...` — **PASS** on 2026-09-15.
- Tests cover server handlers plus auth, bookmarks, services, configuration, conversations, media, notifications, realtime, storage, translation, user search, user discovery, and Discover Posts.
- Confirmed gaps: no migration runner or automated migration-against-PostgreSQL test is present; packages including `block`, `comment`, `db`, `follow`, `like`, `post`, `ratelimit`, and `session` have no package-local tests. This does not mean their behavior is entirely untested, because several are exercised through server tests.

## 9. Known real issues and technical debt

### Fixed since this audit started

`eef878c fix: validate discover cursor UUID` validates the decoded user-discovery cursor ID before repository/PostgreSQL access. Regression coverage proves that a valid encoded cursor with a non-UUID ID returns HTTP 400.

### Confirmed technical debt

- The migration README documents only manual application; there is no repository migration runner or automated schema-application verification.
- `backend/README.md` is stale: it describes a minimal skeleton and says several implemented subsystems are absent.
- Translation is applied per item on read; the source has no caching/batching layer.

### Future ideas, not current defects

- Topic parsing, topic search/feed endpoints, trend ranking, and frontend topic UI are not implemented. Their absence is expected at the current schema-foundation step.

## 10. STEP 62.7 Topics/Hashtags state

Exists now:

- Two untracked schema files: `backend/migrations/000016_create_topics.up.sql` and `.down.sql`.
- The up migration defines canonical topic storage and a post-topic join table; the down migration drops the join table before topics.

Does not exist now:

- No Go topic model, repository, parser, normalization writer, post-creation integration, API route, topic search/discovery query, response mapping, or frontend behavior.
- No applied-state proof for migration `000016`.

Status: migration `000016` is **UNCOMMITTED** and **NOT APPLIED BY THIS AUDIT**. Its actual database applied state remains **UNKNOWN**.

## 11. Safe next backend step

Verify how migration `000016` should be applied safely before running it. The repository uses manual SQL migrations, and this must establish the intended database/environment and an approved application procedure without assuming the migration has already run.
