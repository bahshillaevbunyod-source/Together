"use client";

import { useEffect, useRef, useState } from "react";
import { ChevronDown, LogOut } from "lucide-react";

import { logout } from "@/lib/api";
import { useAuth } from "@/lib/auth-context";

export function HeaderUser() {
  const { user, refresh } = useAuth();

  const displayName = user?.displayName?.trim() || "Guest";
  const initial = displayName.charAt(0).toUpperCase() || "?";

  const [open, setOpen] = useState(false);
  const [loggingOut, setLoggingOut] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const menuRef = useRef<HTMLDivElement>(null);

  // Close the menu on outside click.
  useEffect(() => {
    if (!open) return;
    const onClick = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener("mousedown", onClick);
    return () => document.removeEventListener("mousedown", onClick);
  }, [open]);

  const onLogout = async () => {
    if (loggingOut) return; // prevent duplicate clicks
    setLoggingOut(true);
    setError(null);
    try {
      await logout();
      setOpen(false);
      // Re-check auth; the (app) gate redirects to /login when unauthenticated.
      await refresh();
    } catch {
      setError("Couldn’t log out. Try again.");
    } finally {
      setLoggingOut(false);
    }
  };

  return (
    <div className="relative" ref={menuRef}>
      <button
        type="button"
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => setOpen((prev) => !prev)}
        className="flex items-center gap-2 rounded-full border border-border bg-surface py-1 pl-1 pr-3 transition-colors hover:bg-background"
      >
        <span className="flex h-9 w-9 items-center justify-center rounded-full bg-gradient-to-br from-primary to-emerald-500 text-sm font-semibold text-white">
          {initial}
        </span>
        <span className="text-sm font-medium text-foreground">{displayName}</span>
        <ChevronDown className="h-4 w-4 text-muted-soft" />
      </button>

      {open ? (
        <div
          role="menu"
          className="absolute right-0 top-full z-30 mt-2 w-56 overflow-hidden rounded-xl border border-border bg-surface p-1 shadow-lg"
        >
          <div className="px-3 py-2">
            <div className="truncate text-sm font-medium text-foreground">
              {displayName}
            </div>
            {user?.username ? (
              <div className="truncate text-xs text-muted-soft">
                @{user.username}
              </div>
            ) : null}
          </div>

          {error ? (
            <p className="px-3 pb-1 text-xs text-red-500" role="alert">
              {error}
            </p>
          ) : null}

          <button
            type="button"
            role="menuitem"
            onClick={onLogout}
            disabled={loggingOut}
            className="flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-sm text-foreground transition-colors hover:bg-background disabled:cursor-not-allowed disabled:opacity-50"
          >
            <LogOut className="h-4 w-4 text-muted" />
            {loggingOut ? "Logging out…" : "Log out"}
          </button>
        </div>
      ) : null}
    </div>
  );
}
