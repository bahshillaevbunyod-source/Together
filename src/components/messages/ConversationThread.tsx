"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import {
  getMessages,
  markConversationRead,
  type ApiMessage,
} from "@/lib/api";
import { formatTimeAgo } from "@/lib/format";

type Status = "loading" | "ready" | "error";

interface Props {
  conversationId: string;
  /** The signed-in user's id, to render sender-aware bubbles. */
  currentUserId?: string;
  /** Called after the conversation is marked read, to zero its unread badge. */
  onRead: (conversationId: string) => void;
}

export function ConversationThread({
  conversationId,
  currentUserId,
  onRead,
}: Props) {
  // Messages held oldest → newest so they read naturally top-to-bottom.
  const [messages, setMessages] = useState<ApiMessage[]>([]);
  const [status, setStatus] = useState<Status>("loading");
  const [olderCursor, setOlderCursor] = useState("");
  const [loadingOlder, setLoadingOlder] = useState(false);

  const load = useCallback(
    (signal?: AbortSignal) => {
      setStatus("loading");
      getMessages(conversationId, {}, signal)
        .then((page) => {
          // API returns newest first; reverse for natural chat order.
          setMessages(page.items.slice().reverse());
          setOlderCursor(page.nextCursor);
          setStatus("ready");
          // Mark read after opening; then clear the unread badge in the list.
          markConversationRead(conversationId)
            .then(() => onRead(conversationId))
            .catch(() => {
              // Non-fatal: the thread still renders.
            });
        })
        .catch((err) => {
          if (err instanceof DOMException && err.name === "AbortError") return;
          setStatus("error");
        });
    },
    [conversationId, onRead],
  );

  useEffect(() => {
    const controller = new AbortController();
    load(controller.signal);
    return () => controller.abort();
  }, [load]);

  const loadOlder = () => {
    if (!olderCursor || loadingOlder) return;
    setLoadingOlder(true);
    getMessages(conversationId, { cursor: olderCursor })
      .then((page) => {
        // Older page is also newest-first; reverse and prepend.
        setMessages((prev) => [...page.items.slice().reverse(), ...prev]);
        setOlderCursor(page.nextCursor);
      })
      .catch(() => {
        // Keep what we have.
      })
      .finally(() => setLoadingOlder(false));
  };

  const bottomRef = useRef<HTMLDivElement>(null);

  if (status === "loading") {
    return (
      <div className="flex flex-1 items-center justify-center px-6 text-center">
        <p className="text-sm text-muted">Loading messages…</p>
      </div>
    );
  }

  if (status === "error") {
    return (
      <div className="flex flex-1 flex-col items-center justify-center gap-2 px-6 text-center">
        <p className="text-sm text-muted">Couldn’t load messages.</p>
        <button
          type="button"
          onClick={() => load()}
          className="text-sm text-primary hover:underline"
        >
          Try again
        </button>
      </div>
    );
  }

  if (messages.length === 0) {
    return (
      <div className="flex flex-1 items-center justify-center px-6 text-center">
        <p className="text-sm text-muted-soft">No messages yet.</p>
      </div>
    );
  }

  return (
    <div className="flex flex-1 flex-col gap-2 overflow-y-auto px-5 py-4">
      {olderCursor ? (
        <button
          type="button"
          onClick={loadOlder}
          disabled={loadingOlder}
          className="mx-auto mb-2 rounded-full border border-border bg-surface px-4 py-1.5 text-xs text-muted transition-colors hover:text-foreground disabled:opacity-50"
        >
          {loadingOlder ? "Loading…" : "Load older messages"}
        </button>
      ) : null}

      {messages.map((m) => {
        const mine = currentUserId != null && m.senderId === currentUserId;
        return (
          <div
            key={m.id}
            className={`flex ${mine ? "justify-end" : "justify-start"}`}
          >
            <div
              className={`max-w-[75%] rounded-2xl px-3.5 py-2 ${
                mine
                  ? "bg-primary text-white"
                  : "bg-background text-foreground"
              }`}
            >
              <p className="whitespace-pre-wrap break-words text-sm">
                {m.content}
              </p>
              <p
                className={`mt-1 text-[10px] ${
                  mine ? "text-white/70" : "text-muted-soft"
                }`}
              >
                {formatTimeAgo(m.createdAt)}
              </p>
            </div>
          </div>
        );
      })}
      <div ref={bottomRef} />
    </div>
  );
}
