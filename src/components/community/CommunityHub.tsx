"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { BellOff, ChevronLeft, Info, Plus, Search, Users } from "lucide-react";

import type { ApiMessage } from "@/lib/api";
import {
  getCommunity,
  isCommunityApiUnavailable,
  joinChannel,
  listCommunities,
  searchChannels,
  type ApiCommunity,
  type CommunityKind,
} from "@/lib/community-api";
import { formatTimeAgo } from "@/lib/format";
import { useAuth } from "@/lib/auth-context";
import { useRealtime } from "@/lib/realtime-context";
import { useLanguage, type TranslationKey } from "@/lib/language-context";
import { ConversationThread } from "@/components/messages/ConversationThread";
import { CommunityAvatar } from "./CommunityAvatar";
import { CommunityDetails, RoleBadge } from "./CommunityDetails";
import { CreateCommunityDialog } from "./CreateCommunityDialog";
import { MessagingTabs } from "./MessagingTabs";

type Status = "loading" | "ready" | "error" | "unavailable";
type SearchStatus = "idle" | "loading" | "ready" | "error";

const COPY: Record<
  CommunityKind,
  {
    title: TranslationKey;
    loading: TranslationKey;
    loadError: TranslationKey;
    unavailable: TranslationKey;
    emptyTitle: TranslationKey;
    emptyBody: TranslationKey;
    create: TranslationKey;
    select: TranslationKey;
  }
> = {
  group: {
    title: "navigation.groups",
    loading: "groups.loading",
    loadError: "groups.loadError",
    unavailable: "groups.unavailable",
    emptyTitle: "groups.emptyTitle",
    emptyBody: "groups.emptyBody",
    create: "groups.create",
    select: "groups.select",
  },
  channel: {
    title: "navigation.channels",
    loading: "channels.loading",
    loadError: "channels.loadError",
    unavailable: "channels.unavailable",
    emptyTitle: "channels.emptyTitle",
    emptyBody: "channels.emptyBody",
    create: "channels.create",
    select: "channels.select",
  },
};

/**
 * Groups or Channels hub: list pane + conversation pane + details slide-over,
 * mirroring the direct-messages layout. Threads reuse ConversationThread (and
 * so the existing messages endpoints and the single realtime socket).
 */
export function CommunityHub({ kind }: { kind: CommunityKind }) {
  const { t } = useLanguage();
  const { user } = useAuth();
  const { subscribeMessageCreated } = useRealtime();
  const router = useRouter();
  const copy = COPY[kind];

  const [items, setItems] = useState<ApiCommunity[]>([]);
  const [status, setStatus] = useState<Status>("loading");
  const [nextCursor, setNextCursor] = useState("");
  const [loadingMore, setLoadingMore] = useState(false);
  const loadingMoreRef = useRef(false);

  // The open community. Kept as an object (not only an id) so a channel opened
  // from search or a link — not yet in the joined list — can still render.
  const [selected, setSelected] = useState<ApiCommunity | null>(null);
  const [detailsOpen, setDetailsOpen] = useState(false);
  const [creating, setCreating] = useState(false);

  const load = useCallback(
    (signal?: AbortSignal) => {
      setStatus("loading");
      listCommunities(kind, {}, signal)
        .then((page) => {
          setItems(page.items);
          setNextCursor(page.nextCursor);
          setStatus("ready");
        })
        .catch((err) => {
          if (err instanceof DOMException && err.name === "AbortError") return;
          setStatus(isCommunityApiUnavailable(err) ? "unavailable" : "error");
        });
    },
    [kind],
  );

  useEffect(() => {
    const controller = new AbortController();
    load(controller.signal);
    return () => controller.abort();
  }, [load]);

  // Deep link: /groups?id=<id> or /channels?id=<id> opens that community (a
  // channel preview for non-members). Resolved from the server, never guessed.
  useEffect(() => {
    const id = new URLSearchParams(window.location.search).get("id");
    if (!id) return;
    const controller = new AbortController();
    getCommunity(kind, id, controller.signal)
      .then((c) => setSelected(c))
      .catch(() => {
        // Unavailable or not permitted: stay on the list.
      });
    return () => controller.abort();
  }, [kind]);

  const loadMore = () => {
    if (loadingMoreRef.current || !nextCursor) return;
    loadingMoreRef.current = true;
    setLoadingMore(true);
    listCommunities(kind, { cursor: nextCursor })
      .then((page) => {
        setItems((prev) => {
          const seen = new Set(prev.map((c) => c.id));
          return [...prev, ...page.items.filter((c) => !seen.has(c.id))];
        });
        setNextCursor(page.nextCursor);
      })
      .catch(() => {
        // Keep what we have.
      })
      .finally(() => {
        loadingMoreRef.current = false;
        setLoadingMore(false);
      });
  };

  const selectedId = selected?.id ?? null;

  // One place to apply a changed community to both the list and the selection.
  const applyCommunity = useCallback((next: ApiCommunity) => {
    setItems((prev) => prev.map((c) => (c.id === next.id ? next : c)));
    setSelected((cur) => (cur && cur.id === next.id ? next : cur));
  }, []);

  const clearUnread = useCallback((id: string) => {
    setItems((prev) => prev.map((c) => (c.id === id ? { ...c, unreadCount: 0 } : c)));
  }, []);

  const applySent = useCallback((id: string, message: ApiMessage) => {
    setItems((prev) => {
      const idx = prev.findIndex((c) => c.id === id);
      if (idx === -1) return prev;
      const updated: ApiCommunity = {
        ...prev[idx],
        lastMessage: {
          id: message.id,
          senderId: message.senderId,
          content: message.content,
          createdAt: message.createdAt,
        },
        updatedAt: message.createdAt,
      };
      return [updated, ...prev.filter((_, i) => i !== idx)];
    });
  }, []);

  // Realtime: the existing socket's message.created events update previews and
  // unread counts for communities in this list (same logic as direct messages).
  useEffect(() => {
    return subscribeMessageCreated((event) => {
      setItems((prev) => {
        const idx = prev.findIndex((c) => c.id === event.conversationId);
        if (idx === -1) return prev;
        const cur = prev[idx];
        if (cur.lastMessage?.id === event.id) return prev;
        const isOpen = selectedId === event.conversationId;
        const updated: ApiCommunity = {
          ...cur,
          lastMessage: {
            id: event.id,
            senderId: event.senderId,
            content: event.content,
            createdAt: event.createdAt,
          },
          updatedAt: event.createdAt,
          unreadCount: isOpen ? cur.unreadCount : cur.unreadCount + 1,
        };
        return [updated, ...prev.filter((_, i) => i !== idx)];
      });
    });
  }, [subscribeMessageCreated, selectedId]);

  const open = (c: ApiCommunity) => {
    // Prefer the list's copy (freshest unread/mute state).
    setSelected(items.find((x) => x.id === c.id) ?? c);
    setDetailsOpen(false);
  };

  const onJoined = (c: ApiCommunity) => {
    setItems((prev) => [c, ...prev.filter((x) => x.id !== c.id)]);
    setResults((prev) => prev.map((x) => (x.id === c.id ? c : x)));
    setSelected(c);
  };

  const onLeft = (id: string) => {
    setItems((prev) => prev.filter((c) => c.id !== id));
    setSelected(null);
    setDetailsOpen(false);
  };

  // Channel discovery (channels only).
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<ApiCommunity[]>([]);
  const [searchStatus, setSearchStatus] = useState<SearchStatus>("idle");
  const [searchRetry, setSearchRetry] = useState(0);
  const searching = kind === "channel" && query.trim().length >= 2;

  useEffect(() => {
    if (kind !== "channel") return;
    const q = query.trim();
    if (q.length < 2) {
      setSearchStatus("idle");
      setResults([]);
      return;
    }
    const controller = new AbortController();
    setSearchStatus("loading");
    const timer = setTimeout(() => {
      searchChannels(q, { limit: 20 }, controller.signal)
        .then((found) => {
          setResults(found);
          setSearchStatus("ready");
        })
        .catch((err) => {
          if (err instanceof DOMException && err.name === "AbortError") return;
          setSearchStatus("error");
        });
    }, 250);
    return () => {
      clearTimeout(timer);
      controller.abort();
    };
  }, [kind, query, searchRetry]);

  return (
    <div className="flex h-[calc(100dvh_-_10rem_-_env(safe-area-inset-top)_-_env(safe-area-inset-bottom))] min-h-[22rem] overflow-hidden rounded-2xl border border-border bg-surface shadow-sm sm:h-[calc(100dvh_-_10.5rem_-_env(safe-area-inset-top)_-_env(safe-area-inset-bottom))] lg:h-[calc(100dvh-9rem)]">
      {/* List pane */}
      <div className={`${selected ? "hidden" : "flex"} w-full flex-col border-border sm:flex sm:w-72 sm:shrink-0 sm:border-r xl:w-64 2xl:w-80`}>
        <div className="border-b border-border px-4 py-3">
          <MessagingTabs />
          <div className="mt-3 flex items-center justify-between gap-2">
            <h1 className="min-w-0 truncate text-base font-semibold text-foreground">{t(copy.title)}</h1>
            <button
              type="button"
              onClick={() => setCreating(true)}
              disabled={status !== "ready"}
              className="flex shrink-0 items-center gap-1 rounded-full bg-primary px-3 py-1.5 text-sm font-medium text-white transition-colors hover:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:bg-primary"
            >
              <Plus className="h-4 w-4" />
              {t(copy.create)}
            </button>
          </div>
          {kind === "channel" && status === "ready" ? (
            <label className="relative mt-3 block">
              <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-soft" />
              <input
                type="search"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder={t("channels.searchPlaceholder")}
                aria-label={t("channels.searchPlaceholder")}
                className="h-9 w-full rounded-full bg-background pl-9 pr-3 text-sm text-foreground placeholder:text-muted-soft"
              />
            </label>
          ) : null}
        </div>

        <div className="flex-1 overflow-y-auto">
          {searching ? (
            <>
              {searchStatus === "loading" ? (
                <p className="px-4 py-8 text-center text-sm text-muted">{t("search.searching")}</p>
              ) : null}
              {searchStatus === "error" ? (
                <div className="px-4 py-8 text-center">
                  <p className="text-sm text-muted">{t("channels.searchError")}</p>
                  <button type="button" onClick={() => setSearchRetry((n) => n + 1)} className="mt-2 text-sm text-primary hover:underline">
                    {t("search.tryAgain")}
                  </button>
                </div>
              ) : null}
              {searchStatus === "ready" && results.length === 0 ? (
                <p className="px-4 py-8 text-center text-sm text-muted-soft">{t("channels.searchEmpty")}</p>
              ) : null}
              {searchStatus === "ready" && results.length > 0 ? (
                <ul className="flex flex-col">
                  {results.map((c) => (
                    <CommunityRow key={c.id} community={c} active={selectedId === c.id} onOpen={() => open(c)} showMeta />
                  ))}
                </ul>
              ) : null}
            </>
          ) : (
            <>
              {status === "loading" ? (
                <p className="px-4 py-8 text-center text-sm text-muted">{t(copy.loading)}</p>
              ) : null}
              {status === "unavailable" || status === "error" ? (
                <div className="px-6 py-10 text-center">
                  <p className="text-sm text-muted">{t(status === "unavailable" ? copy.unavailable : copy.loadError)}</p>
                  <button type="button" onClick={() => load()} className="mt-2 text-sm text-primary hover:underline">
                    {t("search.tryAgain")}
                  </button>
                </div>
              ) : null}
              {status === "ready" && items.length === 0 ? (
                <div className="flex flex-col items-center px-6 py-12 text-center">
                  <span className="flex h-12 w-12 items-center justify-center rounded-2xl bg-primary-soft text-primary">
                    <Users className="h-6 w-6" aria-hidden />
                  </span>
                  <p className="mt-3 text-sm font-semibold text-foreground">{t(copy.emptyTitle)}</p>
                  <p className="mt-1 max-w-[16rem] text-sm text-muted">{t(copy.emptyBody)}</p>
                </div>
              ) : null}
              {items.length > 0 ? (
                <ul className="flex flex-col">
                  {items.map((c) => (
                    <CommunityRow key={c.id} community={c} active={selectedId === c.id} onOpen={() => open(c)} />
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
            </>
          )}
        </div>
      </div>

      {/* Conversation pane */}
      <div className={`${selected ? "flex" : "hidden"} relative min-w-0 flex-1 flex-col sm:flex`}>
        {selected ? (
          <>
            <div className="flex items-center gap-3 border-b border-border px-3 py-3 sm:px-5">
              <button
                type="button"
                onClick={() => {
                  setSelected(null);
                  setDetailsOpen(false);
                }}
                aria-label={t("messages.backAria")}
                className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full text-muted transition-colors hover:bg-background sm:hidden"
              >
                <ChevronLeft className="h-5 w-5" />
              </button>
              {selected.role ? (
                <>
                  <button
                    type="button"
                    onClick={() => setDetailsOpen(true)}
                    className="flex min-w-0 flex-1 items-center gap-3 rounded-lg text-left transition-opacity hover:opacity-80"
                  >
                    <HeaderIdentity community={selected} />
                  </button>
                  <button
                    type="button"
                    onClick={() => setDetailsOpen((v) => !v)}
                    aria-label={t("community.info")}
                    aria-expanded={detailsOpen}
                    className={`flex h-9 w-9 shrink-0 items-center justify-center rounded-full transition-colors hover:bg-background ${
                      detailsOpen ? "text-primary" : "text-muted"
                    }`}
                  >
                    <Info className="h-5 w-5" />
                  </button>
                </>
              ) : (
                // Non-member preview: identity only; details open after joining.
                <div className="flex min-w-0 flex-1 items-center gap-3">
                  <HeaderIdentity community={selected} />
                </div>
              )}
            </div>

            {selected.role ? (
              <ConversationThread
                key={selected.id}
                conversationId={selected.id}
                currentUserId={user?.id}
                onRead={clearUnread}
                onSent={applySent}
                // Strict: a missing/unknown flag must never fall back to the
                // thread's DM default (true) and expose a posting composer.
                canPost={selected.permissions?.canPost === true}
                readOnlyNotice={kind === "channel" ? t("channels.readOnly") : undefined}
                showSenders={kind === "group"}
              />
            ) : (
              <ChannelPreview key={selected.id} channel={selected} onJoined={onJoined} />
            )}

            {detailsOpen && selected.role ? (
              <CommunityDetails
                // Sibling of the thread (keyed by the community id), so it needs
                // its own key; still per-community so member state resets.
                key={`details:${selected.id}`}
                community={selected}
                currentUserId={user?.id}
                onClose={() => setDetailsOpen(false)}
                onChange={applyCommunity}
                onLeft={onLeft}
              />
            ) : null}
          </>
        ) : (
          <div className="flex flex-1 items-center justify-center px-6 text-center">
            <p className="text-sm text-muted-soft">{t(copy.select)}</p>
          </div>
        )}
      </div>

      {creating ? (
        <CreateCommunityDialog
          initialKind={kind}
          currentUserId={user?.id}
          onClose={() => setCreating(false)}
          onCreated={(c) => {
            setCreating(false);
            if (c.type === kind) {
              setItems((prev) => [c, ...prev.filter((x) => x.id !== c.id)]);
              setSelected(c);
            } else {
              // Created the other kind: go to its hub, opened.
              router.push(`/${c.type === "group" ? "groups" : "channels"}?id=${encodeURIComponent(c.id)}`);
            }
          }}
        />
      ) : null}
    </div>
  );
}

function HeaderIdentity({ community: c }: { community: ApiCommunity }) {
  const { t } = useLanguage();
  return (
    <>
      <CommunityAvatar kind={c.type} name={c.name} avatarUrl={c.avatarUrl} size="sm" />
      <span className="min-w-0 leading-tight">
        <span className="flex items-center gap-1.5">
          <span className="truncate font-semibold text-foreground">{c.name}</span>
          {c.muted ? <BellOff className="h-3.5 w-3.5 shrink-0 text-muted-soft" aria-label={t("community.muted")} /> : null}
        </span>
        <span className="flex items-center gap-1 text-xs text-muted">
          {c.memberCount != null ? (
            <>
              <Users className="h-3 w-3" aria-hidden />
              <span className="tabular-nums">{c.memberCount}</span>
            </>
          ) : (
            <span>{t(c.type === "channel" ? "community.kindChannel" : "community.kindGroup")}</span>
          )}
        </span>
      </span>
    </>
  );
}

function CommunityRow({
  community: c,
  active,
  onOpen,
  showMeta = false,
}: {
  community: ApiCommunity;
  active: boolean;
  onOpen: () => void;
  /** Search results: show member count / membership instead of last message. */
  showMeta?: boolean;
}) {
  const { t, locale } = useLanguage();
  return (
    <li>
      <button
        type="button"
        onClick={onOpen}
        aria-label={t("community.openAria", { name: c.name })}
        className={`flex w-full items-center gap-3 px-4 py-3 text-left transition-colors hover:bg-background ${active ? "bg-background" : ""}`}
      >
        <CommunityAvatar kind={c.type} name={c.name} avatarUrl={c.avatarUrl} />
        <span className="min-w-0 flex-1">
          <span className="flex items-center justify-between gap-2">
            <span className="flex min-w-0 items-center gap-1.5">
              <span className="truncate font-semibold text-foreground">{c.name}</span>
              {c.muted ? <BellOff className="h-3.5 w-3.5 shrink-0 text-muted-soft" aria-label={t("community.muted")} /> : null}
            </span>
            {!showMeta ? <span className="shrink-0 text-xs text-muted-soft">{formatTimeAgo(c.updatedAt, locale)}</span> : null}
          </span>
          <span className="mt-0.5 flex items-center justify-between gap-2">
            {showMeta ? (
              <span className="flex min-w-0 items-center gap-1 text-sm text-muted">
                {c.memberCount != null ? (
                  <>
                    <Users className="h-3.5 w-3.5 shrink-0" aria-hidden />
                    <span className="tabular-nums">{c.memberCount}</span>
                  </>
                ) : null}
                {c.description ? <span className="truncate">{c.memberCount != null ? " · " : ""}{c.description}</span> : null}
              </span>
            ) : (
              <span className="truncate text-sm text-muted">
                {c.lastMessage ? c.lastMessage.content : t("messages.lastMessageNone")}
              </span>
            )}
            {showMeta && c.role ? <RoleBadge role={c.role} /> : null}
            {!showMeta && c.unreadCount > 0 ? (
              <span className={`flex h-5 min-w-5 shrink-0 items-center justify-center rounded-full px-1.5 text-xs font-semibold text-white ${c.muted ? "bg-muted-soft" : "bg-primary"}`}>
                {c.unreadCount > 9 ? "9+" : c.unreadCount}
              </span>
            ) : null}
          </span>
        </span>
      </button>
    </li>
  );
}

/** A channel the viewer has not joined: identity + Join. No thread access. */
function ChannelPreview({
  channel,
  onJoined,
}: {
  channel: ApiCommunity;
  onJoined: (c: ApiCommunity) => void;
}) {
  const { t } = useLanguage();
  const [joining, setJoining] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const joiningRef = useRef(false);

  const join = async () => {
    if (joiningRef.current) return;
    joiningRef.current = true;
    setJoining(true);
    setError(null);
    try {
      onJoined(await joinChannel(channel.id));
    } catch {
      setError(t("channels.joinError"));
    } finally {
      joiningRef.current = false;
      setJoining(false);
    }
  };

  return (
    <div className="flex flex-1 flex-col items-center justify-center overflow-y-auto px-6 py-10 text-center">
      <CommunityAvatar kind="channel" name={channel.name} avatarUrl={channel.avatarUrl} size="lg" />
      <h2 className="mt-3 max-w-md break-words text-lg font-semibold text-foreground">{channel.name}</h2>
      {channel.memberCount != null ? (
        <p className="mt-1 flex items-center gap-1 text-xs text-muted">
          <Users className="h-3.5 w-3.5" aria-hidden />
          <span className="tabular-nums">{channel.memberCount}</span>
        </p>
      ) : null}
      {channel.description ? (
        <p className="mt-3 max-w-md whitespace-pre-wrap break-words text-sm text-foreground/90">{channel.description}</p>
      ) : null}
      <p className="mt-4 text-sm text-muted">{t("channels.notMember")}</p>
      {error ? (
        <p className="mt-2 text-xs text-red-500" role="alert">
          {error}
        </p>
      ) : null}
      <button
        type="button"
        onClick={() => void join()}
        disabled={joining}
        className="mt-4 rounded-full bg-primary px-6 py-2 text-sm font-medium text-white transition-colors hover:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-50"
      >
        {joining ? t("channels.joining") : t("channels.join")}
      </button>
    </div>
  );
}
