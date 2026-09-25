# Together — Project Handoff

Canonical workspace: `C:\Users\user\source\Together` (do **not** use `C:\Together`).
Last synchronized with the repository: World V1 checkpoint `feat: complete world v1`.

> `PROJECT_AUDIT.md` is an older, backend-only audit written against a previous workspace (`C:\Together`, commits up to `eef878c`). It is kept for history; **this file is the current handoff document**. Engineering rules live in `AGENTS.md`.

---

## Product

Together is a global social platform: different people, one world. Users post, share stories, follow each other, message (with translation), join groups and channels, call each other, create and join events, and discover people around the world.

Product rules:

- **Every visible interactive control must either work for real or not appear interactive.** Target: `NON_FUNCTIONAL_VISIBLE_CONTROLS=0`.
- **Real data only.** No fake users, events, attendees, calls, online counts, markers or demo discovery data. A sparse real state is shown honestly (empty states), never padded.
- UI language (`platformLanguage`), identity language (`nativeLanguage`) and UGC translation target (`preferredLanguage`) are distinct concepts.

## Stack

| Layer | Technology |
|---|---|
| Frontend | Next.js 15 (App Router), TypeScript, Tailwind CSS 4, lucide-react |
| Backend | Go, standard-library `net/http`, `pgx/v5` / `pgxpool` |
| Database | **Supabase PostgreSQL only** (used purely as Postgres) |
| Object storage | Cloudflare R2 (S3-compatible; presigned URLs; private attachments) |
| Realtime | Existing WebSocket (`/api/v1/ws`, Gorilla WebSocket, per-user hub) |
| Auth | Server-side session cookie (`together_session`, HttpOnly; SHA-256 token hashes in DB) |
| Maps (World) | `maplibre-gl` ^5 with an OpenFreeMap vector style (no API key) — uncommitted |

Supabase is **not** used for Auth, Storage or Realtime.

Environment variable **names** (values are never documented or printed):
- Backend (`backend/.env`, loaded by the dev launcher): `DATABASE_URL`, `SUPABASE_DATABASE_URL`, `STORAGE_ACCESS_KEY_ID`, `STORAGE_SECRET_ACCESS_KEY`, `STORAGE_ENDPOINT`, `STORAGE_REGION`, `STORAGE_BUCKET`, `STORAGE_PRIVATE_BUCKET`, `STORAGE_PRIVATE_ACCESS_KEY_ID`, `STORAGE_PRIVATE_SECRET_ACCESS_KEY`, `MEDIA_PUBLIC_BASE_URL`, `TRANSLATION_PROVIDER`, `TRANSLATION_API_KEY`, `TRANSLATION_ENDPOINT`, plus `APP_ORIGIN` (defaults to `http://localhost:3000`).
- Frontend: `NEXT_PUBLIC_API_BASE_URL` (default `http://localhost:8080`; empty = same-origin proxy), `BACKEND_INTERNAL_URL` (Next `/api/*` rewrite target), `NEXT_PUBLIC_MAP_STYLE_URL` (optional World map style override).

## Architecture Rules

- Backend: domain packages under `backend/internal/<domain>` with repository interfaces and PostgreSQL implementations; handlers in `backend/internal/server`; routes registered centrally in `server.go`; production DI in `backend/cmd/api/main.go`.
- **Every new repository must be wired into `cmd/api/main.go`** (see the Events DI bug below).
- Parameterized SQL only; LIKE input is escaped (`escapeLike` + `ESCAPE '\'`).
- Preserve authentication, block, privacy, visibility and ownership rules server-side. Blocks apply in both directions.
- Deterministic keyset pagination (`encodeCursor(time, id)`), limit default 20 / max 50.
- State-changing routes: `requireAuth` + exact-Origin CSRF check.
- Frontend API calls go through `apiFetch` (`src/lib/api.ts`) or a feature module (`src/lib/events-api.ts`, `src/lib/world/world-api.ts`); no ad-hoc fetch in components.
- All visible strings go through i18n (`t(key)`); never hardcode a language.
- No `dangerouslySetInnerHTML` for user content.

## Safety / Git Rules

- Never print or commit `.env` values, credentials, tokens, cookies, session values or presigned URLs.
- **Do not mutate remote Supabase** from development tasks. Migrations are applied manually (`backend/migrations/README.md`).
- Inspect `git status` before staging; stage only the current step's files; keep feature and fix commits separate.
- No push unless explicitly requested. No force-push, rebase, reset, amend, revert or clean.
- Do not disable antivirus/security software; the Windows dev machine's AV occasionally locks Go test binaries (`unlinkat … being used by another process`) — rerun, it is not a test failure.
- Backend dev launcher: `backend/scripts/dev-backend.ps1` (`-NoBuild` to skip build). Verify `/health` and `/ready` = 200 and exactly one listener on `:8080` and `:3000` afterwards.
- Do not run `npm run build` while the dev server is active.

## Completed V1 Blocks

Status legend: **Checkpointed** = committed feature checkpoint. "Real smoke" is noted only where a real browser/device test is known to have passed.

| Block | Status | Git evidence |
|---|---|---|
| Global i18n, 192 locales (platform language, onboarding, switcher) | Checkpointed | `160544f` … `555e52f`, `0930a12`, `ae6d1ec` |
| Source-language pipeline (resolver, Google v3 detector, ADC) | Checkpointed | `35d14f9`, `a9ddb85`, `d445b5a`, `aa1bcc5` |
| Private accounts / follow requests / blocks | Checkpointed | `b531386`, `8272600`, `867b76f`, `709a3b0`, `bb1d6f2` |
| Suggested People, Share/repost, Notifications page | Checkpointed | `4a44a22` |
| Settings V1 | Checkpointed | `c382f0e` |
| Create Post / Story UX | Checkpointed | `1cb9efc` (stories viewer `f91f269`) |
| Messages core (search / edit / delete / mute) | Checkpointed | `2304e15` |
| Private attachments via R2 | Checkpointed | `e4ba531` |
| Voice messages | Checkpointed | `d374773` |
| Groups V1 + Channels V1 | Checkpointed | `590813fabc537bb48d4d3429fc0cf927106f8f20` "feat: complete groups and channels v1" |
| Calls V1 | Checkpointed, **real two-device smoke passed** | `bd3a44c` "feat: complete calls v1"; later fix `c66a916` "fix: remove duplicate community detail key" |
| Events V1 | Checkpointed, **real browser smoke passed** | `1c7604228908eb5931dcd0ddc3e9e608216b47ea` "feat: complete events v1" |
| World / Explore V1 | Checkpointed, **real authenticated browser smoke passed** | `feat: complete world v1` |

i18n: 192 dictionaries in `src/lib/i18n/locales/*.ts` (`en.ts` is the source); validate with `node scripts/validate-i18n.mjs`. With the uncommitted World keys the catalog is 486 keys per dictionary (463 at the Events checkpoint).

## Calls V1

Checkpoint `bd3a44c` (plus fix `c66a916`).

Implemented: `/calls` page and Sidebar Calls; voice and video calls; incoming call UI; accept / reject / cancel / end; busy, timeout and disconnect cleanup; one active call at a time; signaling over the existing WebSocket (offer / answer / ICE); real microphone and camera; real remote audio/video.

**Real smoke passed on two physical devices:** Account A calls Account B → B receives the incoming call → B accepts → WebRTC connects → two-way audio and two-way video work.

**Production TURN: DEFERRED.** ICE servers are provided to the client by the backend call configuration; no production TURN service is set up, so do not describe TURN as done. No call history is persisted.

## Events V1

Checkpoint `1c7604228908eb5931dcd0ddc3e9e608216b47ea` — "feat: complete events v1". **Real browser smoke passed.**

Backend (`backend/internal/event`, `backend/internal/server/events.go`), migration `000031_create_events` with tables `events` and `event_rsvps`.

Supports: create, get, list (upcoming only), My Events filters (`mine=created`, `rsvp=going|interested`), update, delete, RSVP Going / Interested / remove, attendee list, cursor pagination, visibility `public` / `followers` / `private` (private = **creator-only** in V1), block enforcement, IANA timezone validation, online URL validation (http/https with host), request size limits, `canEdit` / `canDelete` permissions.

Frontend: `/events` hub (Upcoming + My Events), guided create/edit form, `/events/[eventId]` detail with RSVP, attendees, edit, delete confirmation; 192-locale strings.

Important past bug: `event.PostgresRepository` initially was **not wired into production DI** in `backend/cmd/api/main.go`, causing real `GET /api/v1/events` → 500 and `POST /api/v1/events` → 500 despite green unit tests. Fixed (now wired, `main.go` passes `events` to `server.New`); Postgres repository regression tests exist in `backend/internal/event/repository_postgres_test.go` (run when `TOGETHER_TEST_DATABASE_URL` points to a local database).

Real smoke covered: `/events` opens; create; detail shows date, time, timezone, place, description; Going; Interested; remove RSVP; edit; second account sees the event and can RSVP; attendee appears; counts update; non-creator gets no Edit/Delete; creator can delete; deleted event disappears.

Deferred / intentional:
- Event notifications — **DEFERRED**.
- Event cover images — **intentionally not implemented** (not a bug).
- List responses do not include RSVP counts / viewer RSVP (only detail does), so cards show no counts.
- Remote Supabase: migration `000031` was **not** applied to remote as part of the Events work.

## World / Explore V1

**Status: `WORLD_V1_STATUS=COMPLETE`. Checkpointed; real authenticated browser smoke passed.**

Design decision: there is **one** discovery section, **World / Мир**. "Explore" is the concept; the World map is its interface. There is no separate Explore page and no separate decorative "World Map" product. Intended flow: World → country → real people / events → profile / event → Follow / RSVP → Message → Call (all via existing flows).

### Implemented in code (automated validation passed; NOT real-browser verified unless stated)

Frontend:
- `src/app/(app)/world/page.tsx` — World hub: map + People / Events tabs + discovery panel (side panel on desktop, bottom-sheet style on mobile). The right rail is hidden on `/world` to give the map width.
- `src/components/world/WorldMap.tsx` — MapLibre loaded browser-only via dynamic `import("maplibre-gl")`; default style `https://tiles.openfreemap.org/styles/positron` (override `NEXT_PUBLIC_MAP_STYLE_URL`); GeoJSON source of **country-level aggregate** circles at country centroids, clustered with summed counts; cluster click zooms, country click selects; `ResizeObserver` → `map.resize()`; attribution control non-compact; loading / error overlays; reduced-motion respected.
- `src/components/world/WorldCountryList.tsx` — searchable (localized + English names) list of countries with real counts; loading / empty / error / no-match states.
- `src/components/world/WorldPeople.tsx` — people in the selected country via existing `GET /api/v1/users/discover?mode=world&country=`; avatar, name, @username, city/country, native language; Follow → Following / Requested (existing follow API); Message reusing the existing `openConversation` flow (→ `/messages?c=…`); profile links; load more.
- `src/components/world/WorldEvents.tsx` — upcoming visible events via `GET /api/v1/events` with search (`q`) and type filter chips All / In person / Online; reuses `EventCard` → `/events/{id}` (RSVP stays on the detail page); load more.
- `src/lib/world/world-api.ts` (country counts client, Intl country/language names, flag emoji), `src/lib/world/country-centroids.ts` (approximate ISO-3166 country centres, used **only** for marker placement).
- Sidebar: single **World** item (`/world`, active on subroutes, also in the mobile bottom bar); the old dead "Explore" item was removed; the "Meet the World" card button now links to `/world`.
- `src/components/right-sidebar/WorldMapCard.tsx`: the previous fake "12,436 online" count and the static image with fake people pins were removed; the card now shows real totals, top countries and an "Explore Map" link to `/world`.
- 23 new i18n keys (`navigation.world`, `world.*`) across all 192 locales.

Backend:
- `GET /api/v1/world/countries` (`backend/internal/server/world.go`, auth required) → `{items:[{countryCode, people}]}` computed by `user.Repository.DiscoverCountries`, which aggregates the **same candidate set as `/users/discover`** (excludes self, blocks in both directions, already-followed and already-requested users; users without a country are not counted).
- `GET /api/v1/events` gained optional `q` (≤100 chars, case-insensitive match on title / location name / address, LIKE-escaped, parameterized) and `type=in_person|online` (invalid → 400). Default Events V1 listing behavior and visibility/block rules are unchanged.
- Tests: `server/world_test.go` (auth, exclusions/order, empty array, repo error), `server/events_test.go` (filter validation), `event/escape_test.go`, extended `event/repository_postgres_test.go` (filters, DB-gated), fake `DiscoverCountries` in `server/auth_test.go`.

Privacy model: people are only shown at coarse, self-declared granularity (profile `country_code` + free-text `city`). **No exact user coordinates exist or are exposed**; map markers are per-country aggregates. Event addresses are shown only as the creator published them and only to viewers allowed by event visibility.

### Real browser verification (by the user)

- `/world` page loads — **PASS**.
- MapLibre/OpenFreeMap renders real geography, borders, labels, markers and attribution — **PASS**.
- Real country aggregates and people discovery — **PASS**.
- Follow/requested state, Message flow and real Events flow — **PASS**.
- World V1 uses existing Follow, Message and Event flows; no fake discovery data or map markers are used.

### Known limitations (by design in V1)

1. Country counts are **discoverable people for the viewer**, not all accounts globally (followed/requested users are excluded, matching the people list).
2. Events have **no normalized country field and no coordinates**. "Events mentioning {country}" is an honest text match of the English country name against title / location name / address. Events are never pinned on the map; online events are a separate filter.
3. People without a country on their profile do not appear in map/country discovery.
4. The map depends on an external provider (OpenFreeMap tiles/style/glyphs).
5. The mobile bottom bar now has 8 items and may be cramped at 320px (not yet checked in the Mobile pass).

## Fixed Historical Map Bug

The World map previously rendered as a blank rectangle because MapLibre CSS `position: relative` overrode Tailwind's layered `absolute inset-0`, collapsing the map container height.

The fix uses full-width/full-height sizing inside the already-sized parent. MapLibre cleanup remains in place (`map.remove()` and `ResizeObserver` teardown), with attribution enabled and visible. The bug is fixed and covered by the real browser smoke; it is not a current blocker.

## Mobile / Responsive Status

**PLANNED / NOT COMPLETED.** No dedicated responsive pass has been done yet. Individual features have mobile-aware layouts, but nothing is certified mobile-ready.

Planned audit viewports: **320, 375, 390, 430 px**. Areas requiring real mobile QA: Login, Register, onboarding, Home, Stories, Posts, Profile, Messages, Calls, Video Calls (call overlay portrait/landscape), Groups, Channels, Events, World (map mobile UX), Settings, mobile navigation, modals, forms, keyboard overlap, safe-area insets, horizontal overflow, button/touch-target sizes, dropdown positioning.

## Database / Migrations

- Plain SQL files in `backend/migrations`, applied **manually** (no migration runner, no ledger in the repo).
- Source contains `000001` … `000031` (latest: `000031_create_events` — `events`, `event_rsvps`; `000030_create_communities` — groups/channels).
- World V1 adds **no migration** (it aggregates existing `users.country_code` and filters existing `events` columns).
- Applied state of the remote Supabase database is **not tracked here**; migration `000031` was applied locally during Events development and **not** to remote as part of that work. Do not change remote Supabase without an explicit instruction.

## Testing Philosophy

**CODE PASS ≠ REAL PRODUCT PASS.**

Automated checks (`go test ./...`, `go vet ./...`, `npx tsc --noEmit`, `npx eslint .`, `node scripts/validate-i18n.mjs`, HTTP 200 route checks) are required but **do not replace real browser/device smoke tests**. Major features require a real smoke before their checkpoint is closed. Evidence:

- Events had a real production DI bug (500s) despite earlier green automated validation.
- World currently has a real blank-map bug despite passing typecheck, lint, Go tests and `/world` = 200.

Report status precisely: *implemented* / *automated validation passed* / *real smoke passed* / *pending real smoke* / *deferred* / *known issue*.

## Current Git State

- Branch: `main`. HEAD: `1c7604228908eb5931dcd0ddc3e9e608216b47ea` — "feat: complete events v1" (Events checkpoint, untouched).
- Recent checkpoints: `1c76042` Events V1 → `c66a916` community detail key fix → `bd3a44c` Calls V1 → `590813f` Groups + Channels V1.
- **Worktree is dirty with uncommitted World / Explore V1 work** (nothing staged):
  - Modified backend: `internal/event/{event.go,repository_postgres.go,repository_postgres_test.go}`, `internal/server/{auth_test.go,events.go,events_test.go,server.go}`, `internal/user/{repository.go,repository_postgres.go}`.
  - New backend: `internal/event/escape_test.go`, `internal/server/{world.go,world_test.go}`.
  - Modified frontend: `src/components/layout/{Sidebar.tsx,RightSidebar.tsx}`, `src/components/right-sidebar/WorldMapCard.tsx`, `src/lib/events-api.ts`, all 192 `src/lib/i18n/locales/*.ts`.
  - New frontend: `src/app/(app)/world/`, `src/components/world/`, `src/lib/world/`.
  - Packages: `package.json`, `package-lock.json` (`maplibre-gl`).
  - Docs: `PROJECT.md` (this file, new).
- **World V1 checkpoint:** `feat: complete world v1`; real authenticated browser smoke passed.

## Immediate Next Step

World V1 is checkpointed. The next block is the Mobile / Responsive Pass.

## Roadmap

1. World V1 checkpoint complete.
2. Mobile / Responsive Pass (320 / 375 / 390 / 430).
3. Full product QA.
4. Release polish (incl. deferred items as decided: production TURN, event notifications).
5. Demo.
