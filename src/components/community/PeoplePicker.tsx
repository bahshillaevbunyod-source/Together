"use client";

import { useEffect, useState } from "react";
import Image from "next/image";
import { Check, Search, X } from "lucide-react";

import { searchUsers, type SearchUserItem } from "@/lib/api";
import { useLanguage } from "@/lib/language-context";

const FALLBACK_AVATAR =
  "data:image/svg+xml;utf8," +
  encodeURIComponent(
    '<svg xmlns="http://www.w3.org/2000/svg" width="36" height="36"><circle cx="18" cy="18" r="18" fill="#d4d4d8"/></svg>',
  );

type SearchStatus = "idle" | "loading" | "ready" | "error";

/**
 * Multi-select people picker backed by the real people-search endpoint. Used
 * when creating a group and when adding members. `excludeIds` hides people who
 * are already members (and the viewer).
 */
export function PeoplePicker({
  selected,
  onChange,
  excludeIds,
  disabled = false,
}: {
  selected: SearchUserItem[];
  onChange: (next: SearchUserItem[]) => void;
  excludeIds?: ReadonlySet<string>;
  disabled?: boolean;
}) {
  const { t } = useLanguage();
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<SearchUserItem[]>([]);
  const [status, setStatus] = useState<SearchStatus>("idle");
  const [retry, setRetry] = useState(0);

  useEffect(() => {
    const q = query.trim();
    if (q.length < 2) {
      setResults([]);
      setStatus("idle");
      return;
    }
    const controller = new AbortController();
    setStatus("loading");
    const timer = setTimeout(() => {
      searchUsers(q, { limit: 12 }, controller.signal)
        .then((items) => {
          setResults(items);
          setStatus("ready");
        })
        .catch((err) => {
          if (err instanceof DOMException && err.name === "AbortError") return;
          setStatus("error");
        });
    }, 250);
    return () => {
      clearTimeout(timer);
      controller.abort();
    };
  }, [query, retry]);

  const selectedIds = new Set(selected.map((u) => u.id));
  const visible = results.filter((u) => !excludeIds?.has(u.id));

  const toggle = (user: SearchUserItem) => {
    if (disabled) return;
    onChange(
      selectedIds.has(user.id)
        ? selected.filter((u) => u.id !== user.id)
        : [...selected, user],
    );
  };

  return (
    <div className="flex min-h-0 flex-col">
      {selected.length > 0 ? (
        <div className="mb-2 flex flex-wrap gap-1.5">
          {selected.map((u) => (
            <span
              key={u.id}
              className="flex max-w-full items-center gap-1 rounded-full bg-primary-soft py-0.5 pl-2.5 pr-1 text-xs font-medium text-primary"
            >
              <span className="truncate">{u.displayName}</span>
              <button
                type="button"
                onClick={() => toggle(u)}
                disabled={disabled}
                aria-label={t("community.removeSelected", { name: u.displayName })}
                className="flex h-5 w-5 shrink-0 items-center justify-center rounded-full hover:bg-primary/10 disabled:opacity-50"
              >
                <X className="h-3 w-3" />
              </button>
            </span>
          ))}
        </div>
      ) : null}

      <label className="relative block">
        <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-soft" />
        <input
          type="search"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          disabled={disabled}
          placeholder={t("discover.searchPlaceholder")}
          aria-label={t("search.ariaLabel")}
          className="h-10 w-full rounded-full bg-background pl-9 pr-4 text-sm text-foreground placeholder:text-muted-soft disabled:opacity-50"
        />
      </label>

      <div className="mt-2 min-h-[8rem] flex-1 overflow-y-auto">
        {status === "loading" ? (
          <p className="py-6 text-center text-sm text-muted">{t("search.searching")}</p>
        ) : null}
        {status === "error" ? (
          <div className="py-6 text-center">
            <p className="text-sm text-muted">{t("search.error")}</p>
            <button
              type="button"
              onClick={() => setRetry((n) => n + 1)}
              className="mt-1 text-sm text-primary hover:underline"
            >
              {t("search.tryAgain")}
            </button>
          </div>
        ) : null}
        {status === "ready" && visible.length === 0 ? (
          <p className="py-6 text-center text-sm text-muted-soft">
            {t("community.noPeopleFound")}
          </p>
        ) : null}
        {status === "ready" && visible.length > 0 ? (
          <ul className="flex flex-col">
            {visible.map((u) => {
              const isSelected = selectedIds.has(u.id);
              return (
                <li key={u.id}>
                  <button
                    type="button"
                    onClick={() => toggle(u)}
                    disabled={disabled}
                    aria-pressed={isSelected}
                    className="flex w-full items-center gap-3 rounded-xl px-2 py-2 text-left transition-colors hover:bg-background focus-visible:bg-background disabled:opacity-50"
                  >
                    <Image
                      src={u.avatarUrl ?? FALLBACK_AVATAR}
                      alt=""
                      width={36}
                      height={36}
                      unoptimized={Boolean(u.avatarUrl)}
                      className="h-9 w-9 shrink-0 rounded-full object-cover"
                    />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-sm font-medium text-foreground">
                        {u.displayName}
                      </span>
                      <span className="block truncate text-xs text-muted">@{u.username}</span>
                    </span>
                    <span
                      aria-hidden
                      className={`flex h-5 w-5 shrink-0 items-center justify-center rounded-full border transition-colors ${
                        isSelected
                          ? "border-primary bg-primary text-white"
                          : "border-border bg-surface"
                      }`}
                    >
                      {isSelected ? <Check className="h-3 w-3" /> : null}
                    </span>
                  </button>
                </li>
              );
            })}
          </ul>
        ) : null}
      </div>
    </div>
  );
}
