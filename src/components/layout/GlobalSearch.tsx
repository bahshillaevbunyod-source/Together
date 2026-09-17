"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import Image from "next/image";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { Loader2, Search } from "lucide-react";

import { searchUsers, type SearchUserItem } from "@/lib/api";
import { useLanguage } from "@/lib/language-context";

type Status = "idle" | "loading" | "ready" | "error";

const MIN_QUERY = 2;
const DEBOUNCE_MS = 250;

const FALLBACK_AVATAR =
  "data:image/svg+xml;utf8," +
  encodeURIComponent(
    '<svg xmlns="http://www.w3.org/2000/svg" width="40" height="40"><circle cx="20" cy="20" r="20" fill="#d4d4d8"/></svg>',
  );

export function GlobalSearch() {
  const router = useRouter();
  const { t } = useLanguage();

  const [query, setQuery] = useState("");
  const [results, setResults] = useState<SearchUserItem[]>([]);
  const [status, setStatus] = useState<Status>("idle");
  const [open, setOpen] = useState(false);
  const [activeIndex, setActiveIndex] = useState(-1);

  const containerRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const abortRef = useRef<AbortController | null>(null);

  const trimmed = query.trim();
  const showDropdown = open && trimmed.length >= MIN_QUERY;

  const runSearch = useCallback((q: string) => {
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    setStatus("loading");
    searchUsers(q, {}, controller.signal)
      .then((items) => {
        setResults(items);
        setStatus("ready");
        setActiveIndex(items.length > 0 ? 0 : -1);
      })
      .catch((err) => {
        if (err instanceof DOMException && err.name === "AbortError") return;
        setStatus("error");
      });
  }, []);

  // Debounced query -> search. Short queries reset without hitting the network.
  useEffect(() => {
    if (trimmed.length < MIN_QUERY) {
      abortRef.current?.abort();
      setResults([]);
      setStatus("idle");
      setActiveIndex(-1);
      return;
    }
    const id = window.setTimeout(() => runSearch(trimmed), DEBOUNCE_MS);
    return () => window.clearTimeout(id);
  }, [trimmed, runSearch]);

  // Ctrl/Cmd+K focuses the search field from anywhere.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        inputRef.current?.focus();
        inputRef.current?.select();
        setOpen(true);
      }
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, []);

  // Close on outside click.
  useEffect(() => {
    if (!open) return;
    const onClick = (e: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener("mousedown", onClick);
    return () => document.removeEventListener("mousedown", onClick);
  }, [open]);

  const goToProfile = useCallback(
    (username: string) => {
      setOpen(false);
      setQuery("");
      setResults([]);
      setStatus("idle");
      setActiveIndex(-1);
      inputRef.current?.blur();
      router.push(`/u/${encodeURIComponent(username)}`);
    },
    [router],
  );

  const onKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Escape") {
      setOpen(false);
      inputRef.current?.blur();
      return;
    }
    if (e.key === "ArrowDown") {
      if (!showDropdown) {
        if (trimmed.length >= MIN_QUERY) setOpen(true);
        return;
      }
      if (results.length === 0) return;
      e.preventDefault();
      setActiveIndex((i) => (i + 1) % results.length);
      return;
    }
    if (e.key === "ArrowUp") {
      if (!showDropdown || results.length === 0) return;
      e.preventDefault();
      setActiveIndex((i) => (i - 1 + results.length) % results.length);
      return;
    }
    if (e.key === "Enter") {
      if (activeIndex >= 0 && activeIndex < results.length) {
        e.preventDefault();
        goToProfile(results[activeIndex].username);
      }
    }
  };

  return (
    <div className="flex min-w-0 flex-1 justify-center">
      <div ref={containerRef} className="relative w-full max-w-[560px]">
        <Search className="pointer-events-none absolute left-4 top-1/2 h-5 w-5 -translate-y-1/2 text-muted-soft" />
        <input
          ref={inputRef}
          type="text"
          value={query}
          onChange={(e) => {
            setQuery(e.target.value);
            setOpen(true);
          }}
          onFocus={() => setOpen(true)}
          onKeyDown={onKeyDown}
          role="combobox"
          aria-label={t("search.ariaLabel")}
          aria-expanded={showDropdown}
          aria-controls="global-search-listbox"
          aria-autocomplete="list"
          aria-activedescendant={
            showDropdown && activeIndex >= 0
              ? `global-search-option-${activeIndex}`
              : undefined
          }
          placeholder={t("search.placeholder")}
          className="h-11 w-full rounded-full border border-border bg-background pl-11 pr-16 text-sm text-foreground placeholder:text-muted-soft focus:border-primary focus:outline-none focus:ring-2 focus:ring-primary/20"
        />
        <span className="pointer-events-none absolute right-3 top-1/2 hidden -translate-y-1/2 rounded-md border border-border bg-surface px-2 py-0.5 text-xs text-muted-soft sm:block">
          {t("search.shortcut")}
        </span>

        {showDropdown ? (
          <div
            id="global-search-listbox"
            role="listbox"
            className="absolute left-0 right-0 top-full z-40 mt-2 overflow-hidden rounded-2xl border border-border bg-surface shadow-xl"
          >
            <div className="max-h-[min(70vh,420px)] overflow-y-auto py-1">
              {status === "loading" ? (
                <div className="flex items-center gap-2 px-4 py-6 text-sm text-muted">
                  <Loader2 className="h-4 w-4 animate-spin" />
                  {t("search.searching")}
                </div>
              ) : null}

              {status === "error" ? (
                <div className="px-4 py-6 text-center text-sm">
                  <p className="text-muted">{t("search.error")}</p>
                  <button
                    type="button"
                    onClick={() => runSearch(trimmed)}
                    className="mt-1 rounded text-primary hover:underline"
                  >
                    {t("search.tryAgain")}
                  </button>
                </div>
              ) : null}

              {status === "ready" && results.length === 0 ? (
                <div className="px-4 py-6 text-center text-sm text-muted-soft">
                {t("search.emptyPrefix")}{trimmed}{t("search.emptySuffix")}
                </div>
              ) : null}

              {results.length > 0 ? (
                <ul>
                  {results.map((u, i) => (
                    <li key={u.id}>
                      <Link
                        id={`global-search-option-${i}`}
                        role="option"
                        aria-selected={i === activeIndex}
                        href={`/u/${encodeURIComponent(u.username)}`}
                        onClick={() => goToProfile(u.username)}
                        onMouseEnter={() => setActiveIndex(i)}
                        className={`flex items-center gap-3 px-3 py-2.5 transition-colors ${
                          i === activeIndex ? "bg-background" : ""
                        }`}
                      >
                        <Image
                          src={u.avatarUrl ?? FALLBACK_AVATAR}
                          alt={u.displayName}
                          width={40}
                          height={40}
                          unoptimized={Boolean(u.avatarUrl)}
                          className="h-10 w-10 shrink-0 rounded-full object-cover"
                        />
                        <span className="min-w-0 flex-1">
                          <span className="block truncate text-sm font-semibold text-foreground">
                            {u.displayName}
                          </span>
                          <span className="block truncate text-xs text-muted">
                            @{u.username}
                          </span>
                        </span>
                      </Link>
                    </li>
                  ))}
                </ul>
              ) : null}
            </div>
          </div>
        ) : null}
      </div>
    </div>
  );
}
