"use client";

import {
  createContext,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from "react";

import { ApiError, getMe, type CurrentUser } from "@/lib/api";

// Short backoff between /auth/me retries on transient failures (network / 5xx).
const RETRY_DELAYS_MS = [500, 1000, 2000, 4000];

/** Resolve after ms, or reject with AbortError if the signal aborts first. */
function delay(ms: number, signal?: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    const t = setTimeout(resolve, ms);
    signal?.addEventListener(
      "abort",
      () => {
        clearTimeout(t);
        reject(new DOMException("aborted", "AbortError"));
      },
      { once: true },
    );
  });
}

/** Auth lifecycle status. */
export type AuthStatus = "loading" | "authenticated" | "unauthenticated";

interface AuthState {
  status: AuthStatus;
  user: CurrentUser | null;
  /** Re-check the session (e.g. after login/logout elsewhere). */
  refresh: () => Promise<void>;
}

const AuthContext = createContext<AuthState | undefined>(undefined);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [status, setStatus] = useState<AuthStatus>("loading");
  const [user, setUser] = useState<CurrentUser | null>(null);

  async function load(signal?: AbortSignal) {
    // Retry transient failures (network / 5xx) with short backoff. A real 401
    // is the only thing that signs the user out; transient failures never
    // downgrade an authenticated session.
    for (let attempt = 0; ; attempt++) {
      try {
        const me = await getMe(signal);
        setUser(me);
        setStatus("authenticated");
        return;
      } catch (err) {
        if (err instanceof DOMException && err.name === "AbortError") return;

        // Genuine "signed out": clear and stop.
        if (err instanceof ApiError && err.status === 401) {
          setUser(null);
          setStatus("unauthenticated");
          return;
        }

        // Transient (network/status 0, 5xx, timeouts, other): do NOT mark the
        // user unauthenticated. Keep any existing authenticated state; on the
        // initial load the status simply stays "loading". Retry with backoff.
        if (attempt >= RETRY_DELAYS_MS.length) {
          // Give up retrying, but never force a logout on a transient error.
          return;
        }
        try {
          await delay(RETRY_DELAYS_MS[attempt], signal);
        } catch {
          return; // aborted during backoff
        }
      }
    }
  }

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  }, []);

  const refresh = () => load();

  return (
    <AuthContext.Provider value={{ status, user, refresh }}>
      {children}
    </AuthContext.Provider>
  );
}

/** Access the current auth state. Must be used within an AuthProvider. */
export function useAuth(): AuthState {
  const ctx = useContext(AuthContext);
  if (ctx === undefined) {
    throw new Error("useAuth must be used within an AuthProvider");
  }
  return ctx;
}
