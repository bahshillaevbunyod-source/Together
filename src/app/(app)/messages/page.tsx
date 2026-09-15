"use client";

import { useCallback, useEffect, useState } from "react";
import Image from "next/image";
import Link from "next/link";

import {
  ApiError,
  getConversations,
  openConversation,
  type ApiConversation,
  type ApiMessage,
} from "@/lib/api";
import { formatTimeAgo } from "@/lib/format";
import { useAuth } from "@/lib/auth-context";
import { useRealtime } from "@/lib/realtime-context";
import { ConversationThread } from "@/components/messages/ConversationThread";

type Status = "loading" | "ready" | "error";

const FALLBACK_AVATAR =
  "data:image/svg+xml;utf8," +
  encodeURIComponent(
    '<svg xmlns="http://www.w3.org/2000/svg" width="48" height="48"><circle cx="24" cy="24" r="24" fill="#d4d4d8"/></svg>',
  );

export default function MessagesPage() {
  const [items, setItems] = useState<ApiConversation[]>([]);
  const [status, setStatus] = useState<Status>("loading");
  const [nextCursor, setNextCursor] = useState("");
  const [loadingMore, setLoadingMore] = useState(false);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const { user } = useAuth();
  const { subscribeMessageCreated } = useRealtime();

  // Zero the unread badge for a conversation once it has been read.
  const clearUnread = useCallback((id: string) => {
    setItems((prev) =>
      prev.map((c) => (c.id === id ? { ...c, unreadCount: 0 } : c)),
    );
  }, []);

  // After the viewer sends a message: update that conversation's preview +
  // activity time and move it to the top. Unread stays unchanged (own send).
  const applySent = useCallback((id: string, message: ApiMessage) => {
    setItems((prev) => {
      const idx = prev.findIndex((c) => c.id === id);
      if (idx === -1) return prev;
      const updated: ApiConversation = {
        ...prev[idx],
        lastMessage: {
          id: message.id,
          senderId: message.senderId,
          content: message.content,
          createdAt: message.createdAt,
        },
        updatedAt: message.createdAt,
      };
      const rest = prev.filter((_, i) => i !== idx);
      return [updated, ...rest]; // newest activity first
    });
  }, []);

  // Realtime: incoming messages update the conversation list (no full refetch).
  useEffect(() => {
    const unsubscribe = subscribeMessageCreated((event) => {
      setItems((prev) => {
        const idx = prev.findIndex((c) => c.id === event.conversationId);
        if (idx === -1) return prev; // conversation not in the current list
        const cur = prev[idx];
        if (cur.lastMessage?.id === event.id) return prev; // duplicate event
        const isOpen = selectedId === event.conversationId;
        const updated: ApiConversation = {
          ...cur,
          lastMessage: {
            id: event.id,
            senderId: event.senderId,
            content: event.content,
            createdAt: event.createdAt,
          },
          updatedAt: event.createdAt,
          // Keep unread at 0 for the open thread (it's being read); otherwise +1.
          unreadCount: isOpen ? cur.unreadCount : cur.unreadCount + 1,
        };
        const rest = prev.filter((_, i) => i !== idx);
        return [updated, ...rest];
      });
    });
    return unsubscribe;
  }, [subscribeMessageCreated, selectedId]);

  const load = useCallback((signal?: AbortSignal) => {
    setStatus("loading");
    getConversations({}, signal)
      .then((page) => {
        setItems(page.items);
        setNextCursor(page.nextCursor);
        setStatus("ready");
      })
      .catch((err) => {
        if (err instanceof DOMException && err.name === "AbortError") return;
        setStatus("error");
      });
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    load(controller.signal);
    return () => controller.abort();
  }, [load]);

  // Preselect a conversation passed via ?c=<id> (e.g. from a profile's Message
  // button). It opens once it appears in the loaded list.
  useEffect(() => {
    const c = new URLSearchParams(window.location.search).get("c");
    if (c) setSelectedId(c);
  }, []);

  const loadMore = () => {
    if (!nextCursor || loadingMore) return;
    setLoadingMore(true);
    getConversations({ cursor: nextCursor })
      .then((page) => {
        setItems((prev) => [...prev, ...page.items]);
        setNextCursor(page.nextCursor);
      })
      .catch(() => {
        // Keep what we have.
      })
      .finally(() => setLoadingMore(false));
  };

  const selected = items.find((c) => c.id === selectedId) ?? null;

  // "New conversation" composer state.
  const [newUsername, setNewUsername] = useState("");
  const [opening, setOpening] = useState(false);
  const [openError, setOpenError] = useState<string | null>(null);

  const openNew = async (e: React.FormEvent) => {
    e.preventDefault();
    const uname = newUsername.trim();
    if (!uname || opening) return;
    setOpening(true);
    setOpenError(null);
    try {
      const conv = await openConversation(uname);
      // Add only if not already present; move to top and open it either way.
      setItems((prev) => [conv, ...prev.filter((c) => c.id !== conv.id)]);
      setSelectedId(conv.id);
      setNewUsername("");
    } catch (err) {
      setOpenError(
        err instanceof ApiError ? err.message : "Couldn’t open conversation.",
      );
    } finally {
      setOpening(false);
    }
  };

  return (
    <div className="flex h-[calc(100vh-9rem)] overflow-hidden rounded-2xl border border-border bg-surface shadow-sm">
      {/* Conversation list */}
      <div className="flex w-full flex-col border-border sm:w-80 sm:border-r">
        <div className="border-b border-border px-4 py-3">
          <h1 className="text-base font-semibold text-foreground">Messages</h1>
          <form onSubmit={openNew} className="mt-3 flex items-center gap-2">
            <input
              type="text"
              value={newUsername}
              onChange={(e) => setNewUsername(e.target.value)}
              placeholder="Start a chat by @username"
              className="h-9 flex-1 rounded-full bg-background px-3 text-sm text-foreground placeholder:text-muted-soft focus:outline-none"
            />
            <button
              type="submit"
              disabled={newUsername.trim().length === 0 || opening}
              className="shrink-0 rounded-full bg-primary px-3 py-1.5 text-sm font-medium text-white transition-colors hover:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:bg-primary"
            >
              {opening ? "…" : "New"}
            </button>
          </form>
          {openError ? (
            <p className="mt-2 text-xs text-red-500" role="alert">
              {openError}
            </p>
          ) : null}
        </div>

        <div className="flex-1 overflow-y-auto">
          {status === "loading" ? (
            <p className="px-4 py-8 text-center text-sm text-muted">
              Loading conversations…
            </p>
          ) : null}

          {status === "error" ? (
            <div className="px-4 py-8 text-center">
              <p className="text-sm text-muted">
                Couldn’t load conversations.
              </p>
              <button
                type="button"
                onClick={() => load()}
                className="mt-2 text-sm text-primary hover:underline"
              >
                Try again
              </button>
            </div>
          ) : null}

          {status === "ready" && items.length === 0 ? (
            <p className="px-4 py-8 text-center text-sm text-muted-soft">
              No conversations yet.
            </p>
          ) : null}

          {items.length > 0 ? (
            <ul className="flex flex-col">
              {items.map((c) => (
                <li key={c.id} className="relative">
                  {/* Base click target: a real <button> covering the row,
                      opening the conversation. It sits BEHIND the content layer
                      so the avatar/name Links (siblings, not descendants) stay
                      out of the button — valid nesting and full keyboard access
                      for both the button and the links. */}
                  <button
                    type="button"
                    onClick={() => setSelectedId(c.id)}
                    aria-label={`Open conversation with ${c.otherUser.displayName}`}
                    className={`absolute inset-0 h-full w-full transition-colors hover:bg-background ${
                      selectedId === c.id ? "bg-background" : ""
                    }`}
                  />
                  {/* Content layer: transparent to pointer events so clicks fall
                      through to the button, except the Links (pointer-events
                      re-enabled) which navigate to the profile. */}
                  <div className="pointer-events-none relative flex items-center gap-3 px-4 py-3">
                    <Link
                      href={`/u/${encodeURIComponent(c.otherUser.username)}`}
                      aria-label={`View ${c.otherUser.displayName}'s profile`}
                      className="pointer-events-auto shrink-0 rounded-full"
                    >
                      <Image
                        src={c.otherUser.avatarUrl ?? FALLBACK_AVATAR}
                        alt={c.otherUser.displayName}
                        width={48}
                        height={48}
                        className="h-12 w-12 shrink-0 rounded-full object-cover"
                      />
                    </Link>
                    <span className="min-w-0 flex-1">
                      <span className="flex items-center justify-between gap-2">
                        <Link
                          href={`/u/${encodeURIComponent(c.otherUser.username)}`}
                          className="pointer-events-auto min-w-0 truncate font-semibold text-foreground hover:underline"
                        >
                          {c.otherUser.displayName}
                        </Link>
                        <span className="shrink-0 text-xs text-muted-soft">
                          {formatTimeAgo(c.updatedAt)}
                        </span>
                      </span>
                      <span className="mt-0.5 flex items-center justify-between gap-2">
                        <span className="truncate text-sm text-muted">
                          {c.lastMessage
                            ? c.lastMessage.content
                            : "No messages yet"}
                        </span>
                        {c.unreadCount > 0 ? (
                          <span className="flex h-5 min-w-5 shrink-0 items-center justify-center rounded-full bg-primary px-1.5 text-xs font-semibold text-white">
                            {c.unreadCount > 9 ? "9+" : c.unreadCount}
                          </span>
                        ) : null}
                      </span>
                    </span>
                  </div>
                </li>
              ))}
            </ul>
          ) : null}

          {nextCursor ? (
            <button
              type="button"
              onClick={loadMore}
              disabled={loadingMore}
              className="w-full border-t border-border px-4 py-2.5 text-center text-sm text-muted transition-colors hover:text-foreground disabled:opacity-50"
            >
              {loadingMore ? "Loading…" : "Load more"}
            </button>
          ) : null}
        </div>
      </div>

      {/* Selected conversation */}
      <div className="hidden flex-1 flex-col sm:flex">
        {selected ? (
          <>
            <div className="flex items-center gap-3 border-b border-border px-5 py-3">
              <Link
                href={`/u/${encodeURIComponent(selected.otherUser.username)}`}
                aria-label={`View ${selected.otherUser.displayName}'s profile`}
                className="flex items-center gap-3 rounded-lg transition-colors hover:opacity-80"
              >
                <Image
                  src={selected.otherUser.avatarUrl ?? FALLBACK_AVATAR}
                  alt={selected.otherUser.displayName}
                  width={40}
                  height={40}
                  className="h-10 w-10 rounded-full object-cover"
                />
                <div className="leading-tight">
                  <div className="font-semibold text-foreground hover:underline">
                    {selected.otherUser.displayName}
                  </div>
                  <div className="text-xs text-muted">
                    @{selected.otherUser.username}
                  </div>
                </div>
              </Link>
            </div>
            <ConversationThread
              key={selected.id}
              conversationId={selected.id}
              currentUserId={user?.id}
              onRead={clearUnread}
              onSent={applySent}
            />
          </>
        ) : (
          <div className="flex flex-1 items-center justify-center px-6 text-center">
            <p className="text-sm text-muted-soft">
              Select a conversation to open it.
            </p>
          </div>
        )}
      </div>
    </div>
  );
}
