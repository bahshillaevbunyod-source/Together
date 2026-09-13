"use client";

import { ChevronDown } from "lucide-react";

import { useAuth } from "@/lib/auth-context";

export function HeaderUser() {
  const { user } = useAuth();

  const displayName = user?.displayName?.trim() || "Guest";
  const initial = displayName.charAt(0).toUpperCase() || "?";

  return (
    <button
      type="button"
      className="flex items-center gap-2 rounded-full border border-border bg-surface py-1 pl-1 pr-3 transition-colors hover:bg-background"
    >
      <span className="flex h-9 w-9 items-center justify-center rounded-full bg-gradient-to-br from-primary to-emerald-500 text-sm font-semibold text-white">
        {initial}
      </span>
      <span className="text-sm font-medium text-foreground">{displayName}</span>
      <ChevronDown className="h-4 w-4 text-muted-soft" />
    </button>
  );
}
