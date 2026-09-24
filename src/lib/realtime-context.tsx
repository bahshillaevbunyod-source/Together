"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  type ReactNode,
} from "react";

import { useAuth } from "@/lib/auth-context";
import type { ApiMessageAttachment, ApiMessageSender } from "@/lib/api";

/** Payload of a `message.created` realtime event (matches the backend). */
export interface MessageCreatedEvent {
  conversationId: string;
  id: string;
  senderId: string;
  content: string;
  createdAt: string;
	updatedAt?: string;
  translatedContent: string | null;
  sourceLanguage: string | null;
  targetLanguage: string | null;
  attachment: ApiMessageAttachment | null;
  /** Sender identity for group/channel messages (optional for direct). */
  sender?: ApiMessageSender | null;
}

/** A raw call.* frame. Payload validation happens in the call controller. */
export interface CallSignalEvent { type: string; data: Record<string, unknown>; }

export interface MessageDeletedEvent { conversationId: string; id: string; deletedAt: string; }

type MessageCreatedHandler = (event: MessageCreatedEvent) => void;
type MessageDeletedHandler = (event: MessageDeletedEvent) => void;
type CallSignalHandler = (event: CallSignalEvent) => void;

interface RealtimeState {
  /** Subscribe to `message.created` events; returns an unsubscribe function. */
  subscribeMessageCreated: (handler: MessageCreatedHandler) => () => void;
	subscribeMessageUpdated: (handler: MessageCreatedHandler) => () => void;
	subscribeMessageDeleted: (handler: MessageDeletedHandler) => () => void;
  /** Subscribe to call.* signaling frames on the same socket. */
  subscribeCallSignal: (handler: CallSignalHandler) => () => void;
  /** Send a {type, data} frame on the existing socket; false when not open. */
  sendRealtime: (type: string, data: unknown) => boolean;
}

const RealtimeContext = createContext<RealtimeState | undefined>(undefined);

const API_BASE_URL =
  process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080";

// http -> ws, https -> wss. An empty base means "same origin" (the Next server
// proxies /api, e.g. behind an HTTPS dev tunnel): use the page's own host and
// wss:// on https pages.
function socketURL(): string {
  if (!API_BASE_URL) {
    const scheme = window.location.protocol === "https:" ? "wss" : "ws";
    return `${scheme}://${window.location.host}/api/v1/ws`;
  }
  return `${API_BASE_URL.replace(/^http/, "ws")}/api/v1/ws`;
}

const MAX_BACKOFF_MS = 10_000;

export function RealtimeProvider({ children }: { children: ReactNode }) {
  const { status } = useAuth();

  const handlersRef = useRef<Set<MessageCreatedHandler>>(new Set());
	const updatedHandlersRef = useRef<Set<MessageCreatedHandler>>(new Set());
	const deletedHandlersRef = useRef<Set<MessageDeletedHandler>>(new Set());
  const callHandlersRef = useRef<Set<CallSignalHandler>>(new Set());
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const attemptRef = useRef(0);
  const intentionalCloseRef = useRef(false);

  const subscribeMessageCreated = useCallback(
    (handler: MessageCreatedHandler) => {
      handlersRef.current.add(handler);
      return () => {
        handlersRef.current.delete(handler);
      };
    },
    [],
  );
	const subscribeMessageUpdated = useCallback((handler: MessageCreatedHandler) => { updatedHandlersRef.current.add(handler); return () => updatedHandlersRef.current.delete(handler); }, []);
	const subscribeMessageDeleted = useCallback((handler: MessageDeletedHandler) => { deletedHandlersRef.current.add(handler); return () => deletedHandlersRef.current.delete(handler); }, []);
  const subscribeCallSignal = useCallback((handler: CallSignalHandler) => {
    callHandlersRef.current.add(handler);
    return () => {
      callHandlersRef.current.delete(handler);
    };
  }, []);
  const sendRealtime = useCallback((type: string, data: unknown) => {
    const ws = wsRef.current;
    if (!ws || ws.readyState !== WebSocket.OPEN) return false;
    try {
      ws.send(JSON.stringify({ type, data }));
      return true;
    } catch {
      return false;
    }
  }, []);

  useEffect(() => {
    // Only maintain a socket while authenticated.
    if (status !== "authenticated") return;

    intentionalCloseRef.current = false;

    const scheduleReconnect = () => {
      if (intentionalCloseRef.current) return;
      const delay = Math.min(MAX_BACKOFF_MS, 1000 * 2 ** attemptRef.current);
      attemptRef.current += 1;
      reconnectRef.current = setTimeout(connect, delay);
    };

    const connect = () => {
      // Prevent duplicate connections.
      const existing = wsRef.current;
      if (
        existing &&
        (existing.readyState === WebSocket.OPEN ||
          existing.readyState === WebSocket.CONNECTING)
      ) {
        return;
      }

      let ws: WebSocket;
      try {
        // The browser attaches the session cookie automatically (same-site).
        ws = new WebSocket(socketURL());
      } catch {
        scheduleReconnect();
        return;
      }
      wsRef.current = ws;

      ws.onopen = () => {
        attemptRef.current = 0; // reset backoff on a healthy connection
      };

      ws.onmessage = (ev) => {
        if (typeof ev.data !== "string") return;
        let msg: unknown;
        try {
          msg = JSON.parse(ev.data);
        } catch {
          return; // ignore malformed JSON
        }
        if (!msg || typeof msg !== "object") return;
        const envelope = msg as { type?: unknown; data?: unknown };
        if (typeof envelope.type === "string" && envelope.type.startsWith("call.")) {
          const data = envelope.data;
          if (!data || typeof data !== "object") return;
          const event: CallSignalEvent = { type: envelope.type, data: data as Record<string, unknown> };
          callHandlersRef.current.forEach((h) => {
            try {
              h(event);
            } catch {
              // A misbehaving subscriber must not break delivery to others.
            }
          });
          return;
        }
		if (envelope.type === "message.deleted") {
			const data = envelope.data as Record<string, unknown> | null;
			if (data && typeof data.conversationId === "string" && typeof data.id === "string" && typeof data.deletedAt === "string") deletedHandlersRef.current.forEach((h) => { try { h(data as unknown as MessageDeletedEvent); } catch {} });
			return;
		}
		if (envelope.type !== "message.created" && envelope.type !== "message.updated") return;
        const d = envelope.data;
        if (!d || typeof d !== "object") return;
        const data = d as Record<string, unknown>;
        if (
          typeof data.conversationId !== "string" ||
          typeof data.id !== "string" ||
          typeof data.senderId !== "string"
        ) {
          return; // ignore malformed payloads
        }
        const event = data as unknown as MessageCreatedEvent;
		const handlers = envelope.type === "message.updated" ? updatedHandlersRef.current : handlersRef.current;
		handlers.forEach((h) => {
          try {
            h(event);
          } catch {
            // A misbehaving subscriber must not break delivery to others.
          }
        });
      };

      ws.onclose = () => {
        // A superseded socket (e.g. the first one of a Strict Mode double
        // mount, closing late) must not clear the live socket's ref or start a
        // second reconnect — that left an orphaned duplicate socket and a
        // window where sendRealtime (call signaling) silently had no socket.
        if (wsRef.current !== ws) return;
        wsRef.current = null;
        if (!intentionalCloseRef.current) scheduleReconnect();
      };

      ws.onerror = () => {
        // The close handler will run next and drive reconnection.
      };
    };

    connect();

    return () => {
      // Intentional teardown (logout/unmount): stop reconnecting and close.
      intentionalCloseRef.current = true;
      if (reconnectRef.current) {
        clearTimeout(reconnectRef.current);
        reconnectRef.current = null;
      }
      attemptRef.current = 0;
      if (wsRef.current) {
        const closing = wsRef.current;
        wsRef.current = null;
        closing.onmessage = null; // no late deliveries from a retired socket
        closing.close();
      }
    };
  }, [status]);

  return (
		<RealtimeContext.Provider value={{ subscribeMessageCreated, subscribeMessageUpdated, subscribeMessageDeleted, subscribeCallSignal, sendRealtime }}>
      {children}
    </RealtimeContext.Provider>
  );
}

/** Access the realtime subscription API. Must be used within a RealtimeProvider. */
export function useRealtime(): RealtimeState {
  const ctx = useContext(RealtimeContext);
  if (ctx === undefined) {
    throw new Error("useRealtime must be used within a RealtimeProvider");
  }
  return ctx;
}
