"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import { Send } from "lucide-react";

import {
  getMessages,
  markConversationRead,
  sendMessage,
  type ApiMessage,
} from "@/lib/api";
import { formatTimeAgo } from "@/lib/format";
import { useRealtime } from "@/lib/realtime-context";

type Status = "loading" | "ready" | "error";

interface Props {
  conversationId: string;
  /** The signed-in user's id, to render sender-aware bubbles. */
  currentUserId?: string;
  /** Called after the conversation is marked read, to zero its unread badge. */
  onRead: (conversationId: string) => void;
  /** Called after a message is successfully sent, to update the list. */
  onSent?: (conversationId: string, message: ApiMessage) => void;
}

export function ConversationThread({
  conversationId,
  currentUserId,
  onRead,
  onSent,
}: Props) {
  // Messages held oldest → newest so they read naturally top-to-bottom.
  const [messages, setMessages] = useState<ApiMessage[]>([]);
  const [status, setStatus] = useState<Status>("loading");
  const [olderCursor, setOlderCursor] = useState("");
  const [loadingOlder, setLoadingOlder] = useState(false);

  // Composer state.
  const [text, setText] = useState("");
  const [sending, setSending] = useState(false);
  const [sendError, setSendError] = useState<string | null>(null);

  const { subscribeMessageCreated } = useRealtime();
  const bottomRef = useRef<HTMLDivElement>(null);
  // Tracks message ids currently in the thread, to ignore duplicates.
  const seenIdsRef = useRef<Set<string>>(new Set());

  const scrollToBottom = useCallback(() => {
    requestAnimationFrame(() =>
      bottomRef.current?.scrollIntoView({ behavior: "smooth" }),
    );
  }, []);

  const load = useCallback(
    (signal?: AbortSignal) => {
      setStatus("loading");
      getMessages(conversationId, {}, signal)
        .then((page) => {
          // API returns newest first; reverse for natural chat order.
          setMessages(page.items.slice().reverse());
          seenIdsRef.current = new Set(page.items.map((m) => m.id));
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
        page.items.forEach((m) => seenIdsRef.current.add(m.id));
        setOlderCursor(page.nextCursor);
      })
      .catch(() => {
        // Keep what we have.
      })
      .finally(() => setLoadingOlder(false));
  };

  // Realtime: append incoming messages for THIS conversation only.
  useEffect(() => {
    const unsubscribe = subscribeMessageCreated((event) => {
      if (event.conversationId !== conversationId) return;
      if (seenIdsRef.current.has(event.id)) return; // ignore duplicates
      seenIdsRef.current.add(event.id);

      const incoming: ApiMessage = {
        id: event.id,
        senderId: event.senderId,
        content: event.content,
        createdAt: event.createdAt,
        translatedContent: event.translatedContent,
        sourceLanguage: event.sourceLanguage,
        targetLanguage: event.targetLanguage,
      };
      setMessages((prev) => [...prev, incoming]); // preserve oldest→newest
      scrollToBottom();

      // The thread is open, so mark it read; clears the list's unread badge.
      markConversationRead(conversationId)
        .then(() => onRead(conversationId))
        .catch(() => {
          // Non-fatal.
        });
    });
    return unsubscribe;
  }, [subscribeMessageCreated, conversationId, onRead, scrollToBottom]);

  const canSend = text.trim().length > 0 && !sending;

  const onSend = async (e: React.FormEvent) => {
    e.preventDefault();
    const content = text.trim();
    if (!content || sending) return; // trim + empty guard + duplicate-submit guard

    setSending(true);
    setSendError(null);
    try {
      const created = await sendMessage(conversationId, content);
      // Append only the real server response (no optimistic fake message).
      seenIdsRef.current.add(created.id);
      setMessages((prev) => [...prev, created]);
      setText(""); // clear only on success
      onSent?.(conversationId, created);
      scrollToBottom();
    } catch {
      setSendError("Couldn’t send message. Try again."); // keep input on failure
    } finally {
      setSending(false);
    }
  };

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

  return (
    <div className="flex flex-1 flex-col overflow-hidden">
      {/* Messages (history loading/pagination/read logic unchanged) */}
      <div className="flex flex-1 flex-col gap-2 overflow-y-auto px-5 py-4">
        {messages.length === 0 ? (
          <div className="flex flex-1 items-center justify-center px-1 text-center">
            <p className="text-sm text-muted-soft">No messages yet.</p>
          </div>
        ) : (
          <>
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

            {messages.map((m) => (
              <MessageBubble
                key={m.id}
                message={m}
                mine={currentUserId != null && m.senderId === currentUserId}
              />
            ))}
          </>
        )}
        <div ref={bottomRef} />
      </div>

      {/* Composer */}
      <div className="border-t border-border px-4 py-3">
        {sendError ? (
          <p className="mb-2 text-xs text-red-500" role="alert">
            {sendError}
          </p>
        ) : null}
        <form onSubmit={onSend} className="flex items-center gap-2">
          <input
            type="text"
            value={text}
            onChange={(e) => setText(e.target.value)}
            placeholder="Write a message…"
            className="h-10 flex-1 rounded-full bg-background px-4 text-sm text-foreground placeholder:text-muted-soft focus:outline-none"
          />
          <button
            type="submit"
            disabled={!canSend}
            aria-label="Send message"
            className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-primary text-white transition-colors hover:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:bg-primary"
          >
            <Send className="h-4 w-4" />
          </button>
        </form>
      </div>
    </div>
  );
}

// MessageBubble renders one message. When a translation is present it shows the
// translated text by default with a per-message "Show original" toggle. It never
// translates in the browser — it only displays what the backend provided.
function MessageBubble({
  message,
  mine,
}: {
  message: ApiMessage;
  mine: boolean;
}) {
  const [showOriginal, setShowOriginal] = useState(false);

  const translated = message.translatedContent;
  const hasTranslation =
    translated != null &&
    translated.trim().length > 0 &&
    translated !== message.content;

  const primary =
    hasTranslation && !showOriginal ? (translated as string) : message.content;

  const langHint =
    hasTranslation && message.sourceLanguage && message.targetLanguage
      ? `${message.sourceLanguage.toUpperCase()} → ${message.targetLanguage.toUpperCase()}`
      : null;

  return (
    <div className={`flex ${mine ? "justify-end" : "justify-start"}`}>
      <div
        className={`max-w-[75%] rounded-2xl px-3.5 py-2 ${
          mine ? "bg-primary text-white" : "bg-background text-foreground"
        }`}
      >
        <p className="whitespace-pre-wrap break-words text-sm">{primary}</p>

        <div
          className={`mt-1 flex items-center gap-2 text-[10px] ${
            mine ? "text-white/70" : "text-muted-soft"
          }`}
        >
          <span>{formatTimeAgo(message.createdAt)}</span>
          {langHint && !showOriginal ? <span>· {langHint}</span> : null}
          {hasTranslation ? (
            <button
              type="button"
              onClick={() => setShowOriginal((v) => !v)}
              className={`underline transition-opacity hover:opacity-80 ${
                mine ? "text-white/80" : "text-primary"
              }`}
            >
              {showOriginal ? "Show translation" : "Show original"}
            </button>
          ) : null}
        </div>
      </div>
    </div>
  );
}
