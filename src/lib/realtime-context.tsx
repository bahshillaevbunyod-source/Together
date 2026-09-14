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

/** Payload of a `message.created` realtime event (matches the backend). */
export interface MessageCreatedEvent {
  conversationId: string;
  id: string;
  senderId: string;
  content: string;
  createdAt: string;
  translatedContent: string | null;
  sourceLanguage: string | null;
  targetLanguage: string | null;
}

type MessageCreatedHandler = (event: MessageCreatedEvent) => void;

interface RealtimeState {
  /** Subscribe to `message.created` events; returns an unsubscribe function. */
  subscribeMessageCreated: (handler: MessageCreatedHandler) => () => void;
}

const RealtimeContext = createContext<RealtimeState | undefined>(undefined);

const API_BASE_URL =
  process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080";

// http -> ws, https -> wss.
function socketURL(): string {
  return `${API_BASE_URL.replace(/^http/, "ws")}/api/v1/ws`;
}

const MAX_BACKOFF_MS = 10_000;

export function RealtimeProvider({ children }: { children: ReactNode }) {
  const { status } = useAuth();

  const handlersRef = useRef<Set<MessageCreatedHandler>>(new Set());
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
        if (envelope.type !== "message.created") return; // ignore unknown types
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
        handlersRef.current.forEach((h) => {
          try {
            h(event);
          } catch {
            // A misbehaving subscriber must not break delivery to others.
          }
        });
      };

      ws.onclose = () => {
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
        wsRef.current.close();
        wsRef.current = null;
      }
    };
  }, [status]);

  return (
    <RealtimeContext.Provider value={{ subscribeMessageCreated }}>
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
