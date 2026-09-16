"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { Globe, Loader2, Search, TrendingUp, Users } from "lucide-react";
import Link from "next/link";

import {
  getDiscoverPosts,
  getDiscoverUsers,
  getTrendingTopics,
  searchUsers,
  type DiscoverUser,
  type SearchUserItem,
  type TrendingTopic,
} from "@/lib/api";
import { PersonCard } from "@/components/discover/PersonCard";
import { Feed } from "@/components/feed/Feed";
import { FeedProvider } from "@/lib/feed-context";
import { canonicalTopicSlug } from "@/lib/topic";

type Status = "loading" | "ready" | "error";
type SearchStatus = "idle" | "loading" | "ready" | "error";

const FOR_YOU_LIMIT = 12;
const SEARCH_LIMIT = 20;
const TOPICS_LIMIT = 12;
const TOPIC_SEARCH_LIMIT = 50;

function isAbort(err: unknown): boolean {
  return err instanceof DOMException && err.name === "AbortError";
}

/** Keep only ids not seen before, recording new ones — prevents cross-page dupes. */
function dedupe(items: DiscoverUser[], seen: Set<string>): DiscoverUser[] {
  const out: DiscoverUser[] = [];
  for (const it of items) {
    if (seen.has(it.id)) continue;
    seen.add(it.id);
    out.push(it);
  }
  return out;
}

export default function DiscoverPage() {
  // For You feed.
  const [people, setPeople] = useState<DiscoverUser[]>([]);
  const [status, setStatus] = useState<Status>("loading");
  const [nextCursor, setNextCursor] = useState("");
  const [loadingMore, setLoadingMore] = useState(false);
  const loadingMoreRef = useRef(false);
  const seenRef = useRef<Set<string>>(new Set());

  // In-page search.
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<SearchUserItem[]>([]);
  const [searchStatus, setSearchStatus] = useState<SearchStatus>("idle");
  const searchAbort = useRef<AbortController | null>(null);
  const [topicResults, setTopicResults] = useState<TrendingTopic[]>([]);
  const [topicSearchStatus, setTopicSearchStatus] =
    useState<SearchStatus>("idle");
  const topicSearchAbort = useRef<AbortController | null>(null);

  const trimmed = query.trim();
  const searching = trimmed.length >= 2;

  const loadForYou = useCallback((signal?: AbortSignal) => {
    setStatus("loading");
    seenRef.current = new Set();
    getDiscoverUsers({ mode: "for_you", limit: FOR_YOU_LIMIT }, signal)
      .then((page) => {
        setPeople(dedupe(page.items, seenRef.current));
        setNextCursor(page.nextCursor);
        setStatus("ready");
      })
      .catch((err) => {
        if (isAbort(err)) return;
        setStatus("error");
      });
  }, []);

  useEffect(() => {
    const c = new AbortController();
    loadForYou(c.signal);
    return () => c.abort();
  }, [loadForYou]);

  const loadMore = () => {
    if (loadingMoreRef.current) return;
    const cursor = nextCursor;
    if (!cursor) return;
    loadingMoreRef.current = true;
    setLoadingMore(true);
    getDiscoverUsers({ mode: "for_you", limit: FOR_YOU_LIMIT, cursor })
      .then((page) => {
        setPeople((prev) => [...prev, ...dedupe(page.items, seenRef.current)]);
        setNextCursor(page.nextCursor);
      })
      .catch(() => {
        // Keep what we have; the button stays available to retry.
      })
      .finally(() => {
        loadingMoreRef.current = false;
        setLoadingMore(false);
      });
  };

  // Debounced, abortable search.
  useEffect(() => {
    if (trimmed.length < 2) {
      searchAbort.current?.abort();
      setResults([]);
      setSearchStatus("idle");
      return;
    }
    const id = window.setTimeout(() => {
      searchAbort.current?.abort();
      const c = new AbortController();
      searchAbort.current = c;
      setSearchStatus("loading");
      searchUsers(trimmed, { limit: SEARCH_LIMIT }, c.signal)
        .then((items) => {
          setResults(items);
          setSearchStatus("ready");
        })
        .catch((err) => {
          if (isAbort(err)) return;
          setSearchStatus("error");
        });
    }, 250);
    return () => window.clearTimeout(id);
  }, [trimmed]);

  // The current API exposes the viewer-visible topic list; filter its canonical
  // slugs locally while search is active without changing people search.
  useEffect(() => {
    if (trimmed.length < 2) {
      topicSearchAbort.current?.abort();
      setTopicResults([]);
      setTopicSearchStatus("idle");
      return;
    }

    const id = window.setTimeout(() => {
      const rawQuery = trimmed.startsWith("#") ? trimmed.slice(1) : trimmed;
      const normalizedQuery = canonicalTopicSlug(rawQuery);
      if (!normalizedQuery) {
        setTopicResults([]);
        setTopicSearchStatus("ready");
        return;
      }

      topicSearchAbort.current?.abort();
      const c = new AbortController();
      topicSearchAbort.current = c;
      setTopicSearchStatus("loading");
      getTrendingTopics({ limit: TOPIC_SEARCH_LIMIT }, c.signal)
        .then((response) => {
          setTopicResults(
            response.items.filter((item) => item.slug.includes(normalizedQuery)),
          );
          setTopicSearchStatus("ready");
        })
        .catch((err) => {
          if (isAbort(err)) return;
          setTopicSearchStatus("error");
        });
    }, 250);

    return () => {
      window.clearTimeout(id);
      topicSearchAbort.current?.abort();
    };
  }, [trimmed]);

  const removeFollowed = (id: string) =>
    setPeople((prev) => prev.filter((p) => p.id !== id));

  return (
    <div className="mx-auto w-full max-w-3xl">
      <header className="mb-5">
        <h1 className="text-2xl font-bold tracking-tight text-foreground">
          Discover
        </h1>
        <p className="mt-1 text-sm text-muted">
          Meet people from around the world.
        </p>
      </header>

      {/* Large in-page search */}
      <div className="relative">
        <Search className="pointer-events-none absolute left-4 top-1/2 h-5 w-5 -translate-y-1/2 text-muted-soft" />
        <input
          type="search"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Search people by name or @username"
          aria-label="Search people"
          className="h-12 w-full rounded-2xl border border-border bg-surface pl-12 pr-4 text-sm text-foreground shadow-sm placeholder:text-muted-soft focus:border-primary focus:outline-none focus:ring-2 focus:ring-primary/20"
        />
      </div>

      <div className="mt-6">
        {searching ? (
          <SearchSection
            query={trimmed}
            status={searchStatus}
            results={results}
            topicStatus={topicSearchStatus}
            topics={topicResults}
          />
        ) : (
          <>
            <ForYouSection
              people={people}
              status={status}
              nextCursor={nextCursor}
              loadingMore={loadingMore}
              onReload={() => loadForYou()}
              onLoadMore={loadMore}
              onFollowed={removeFollowed}
            />
            <div className="mt-10">
              <WorldSection />
            </div>
            <div className="mt-10">
              <PopularSection />
            </div>
            <div className="mt-10">
              <TrendingTopicsSection />
            </div>
            <div className="mt-10">
              <section>
                <SectionHeading>Discover posts</SectionHeading>
                <FeedProvider fetchPage={getDiscoverPosts}>
                  <Feed
                    loadingMessage="Loading posts…"
                    errorMessage="Couldn’t load discover posts."
                    emptyMessage="No new posts to discover right now."
                  />
                </FeedProvider>
              </section>
            </div>
          </>
        )}
      </div>
    </div>
  );
}

function SectionHeading({ children }: { children: React.ReactNode }) {
  return (
    <h2 className="mb-3 text-sm font-semibold text-foreground">{children}</h2>
  );
}

function CardGrid({ children }: { children: React.ReactNode }) {
  return <div className="grid gap-3 sm:grid-cols-2">{children}</div>;
}

function CardSkeletons() {
  return (
    <CardGrid>
      {[0, 1, 2, 3].map((i) => (
        <div
          key={i}
          className="flex items-center gap-3.5 rounded-2xl border border-border bg-surface p-4"
        >
          <div className="h-14 w-14 shrink-0 animate-pulse rounded-full bg-background" />
          <div className="min-w-0 flex-1 space-y-2">
            <div className="h-3 w-28 animate-pulse rounded bg-background" />
            <div className="h-2.5 w-20 animate-pulse rounded bg-background" />
          </div>
        </div>
      ))}
    </CardGrid>
  );
}

function ForYouSection({
  people,
  status,
  nextCursor,
  loadingMore,
  onReload,
  onLoadMore,
  onFollowed,
}: {
  people: DiscoverUser[];
  status: Status;
  nextCursor: string;
  loadingMore: boolean;
  onReload: () => void;
  onLoadMore: () => void;
  onFollowed: (id: string) => void;
}) {
  return (
    <section>
      <SectionHeading>For you</SectionHeading>

      {status === "loading" ? <CardSkeletons /> : null}

      {status === "error" ? (
        <div className="rounded-2xl border border-border bg-surface p-8 text-center">
          <p className="text-sm text-muted">Couldn’t load suggestions.</p>
          <button
            type="button"
            onClick={onReload}
            className="mt-3 rounded-full bg-primary px-4 py-1.5 text-sm text-white transition-colors hover:bg-primary-hover"
          >
            Try again
          </button>
        </div>
      ) : null}

      {status === "ready" && people.length === 0 ? (
        <div className="flex flex-col items-center rounded-2xl border border-border bg-surface px-6 py-14 text-center">
          <span className="flex h-14 w-14 items-center justify-center rounded-full bg-primary-soft text-primary">
            <Users className="h-7 w-7" />
          </span>
          <h3 className="mt-4 text-base font-semibold text-foreground">
            You’re all caught up
          </h3>
          <p className="mt-1 max-w-sm text-sm text-muted">
            No new people to suggest right now. Try searching above.
          </p>
        </div>
      ) : null}

      {people.length > 0 ? (
        <>
          <CardGrid>
            {people.map((u) => (
              <PersonCard key={u.id} user={u} onFollowed={onFollowed} />
            ))}
          </CardGrid>
          {nextCursor ? (
            <div className="mt-4 flex justify-center">
              <button
                type="button"
                onClick={onLoadMore}
                disabled={loadingMore}
                className="rounded-full border border-border bg-surface px-5 py-2 text-sm text-muted transition-colors hover:text-foreground disabled:opacity-50"
              >
                {loadingMore ? "Loading…" : "Load more"}
              </button>
            </div>
          ) : null}
        </>
      ) : null}
    </section>
  );
}

function TrendingTopicsSection() {
  const [items, setItems] = useState<TrendingTopic[]>([]);
  const [status, setStatus] = useState<Status>("loading");

  const load = useCallback((signal?: AbortSignal) => {
    setStatus("loading");
    getTrendingTopics({ limit: TOPICS_LIMIT }, signal)
      .then((response) => {
        setItems(response.items);
        setStatus("ready");
      })
      .catch((err) => {
        if (isAbort(err)) return;
        setStatus("error");
      });
  }, []);

  useEffect(() => {
    const c = new AbortController();
    load(c.signal);
    return () => c.abort();
  }, [load]);

  return (
    <section>
      <div className="mb-3 flex items-center gap-3">
        <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-primary-soft text-primary">
          <TrendingUp className="h-5 w-5" />
        </span>
        <div className="min-w-0">
          <h2 className="text-sm font-semibold text-foreground">
            Trending topics
          </h2>
          <p className="truncate text-xs text-muted">
            Conversations people are having on Together.
          </p>
        </div>
      </div>

      {status === "loading" ? (
        <div className="flex flex-wrap gap-2">
          {[0, 1, 2, 3].map((i) => (
            <span
              key={i}
              className="h-8 w-24 animate-pulse rounded-full bg-background"
            />
          ))}
        </div>
      ) : null}

      {status === "error" ? (
        <div className="rounded-2xl border border-border bg-surface p-6 text-center">
          <p className="text-sm text-muted">Couldn’t load trending topics.</p>
          <button
            type="button"
            onClick={() => load()}
            className="mt-3 rounded-full bg-primary px-4 py-1.5 text-sm text-white transition-colors hover:bg-primary-hover"
          >
            Try again
          </button>
        </div>
      ) : null}

      {status === "ready" && items.length === 0 ? (
        <div className="rounded-2xl border border-border bg-surface px-6 py-10 text-center">
          <p className="text-sm text-muted">No trending topics to show yet.</p>
        </div>
      ) : null}

      {items.length > 0 ? (
        <div className="flex flex-wrap gap-2">
          {items.map((item) => (
            <Link
              key={item.slug}
              href={`/topic/${encodeURIComponent(item.slug)}`}
              className="rounded-full border border-border bg-surface px-3.5 py-1.5 text-sm text-muted"
            >
              #{item.slug}
              <span className="ml-1.5 text-xs text-muted-soft">
                {item.postsCount}
              </span>
            </Link>
          ))}
        </div>
      ) : null}
    </section>
  );
}

function countryLabel(code: string): string {
  try {
    const n = new Intl.DisplayNames(["en"], { type: "region" }).of(
      code.toUpperCase(),
    );
    if (n && n !== code.toUpperCase()) return n;
  } catch {
    // fall through
  }
  return code;
}

/**
 * "Explore the world" — people-discovery weighted toward other countries
 * (backend mode=world). Country chips are derived from the real users that come
 * back (never a hardcoded list) and accumulate as more load; selecting one
 * reloads the section filtered to that country.
 */
function WorldSection() {
  const [activeCountry, setActiveCountry] = useState(""); // "" = all countries
  const [items, setItems] = useState<DiscoverUser[]>([]);
  const [status, setStatus] = useState<Status>("loading");
  const [nextCursor, setNextCursor] = useState("");
  const [loadingMore, setLoadingMore] = useState(false);
  const loadingMoreRef = useRef(false);
  const [countries, setCountries] = useState<string[]>([]);
  const seenRef = useRef<Set<string>>(new Set());

  // Accumulate the set of real countries seen so the chip row never shrinks
  // (e.g. after filtering to a single country and back).
  const accrueCountries = useCallback((users: DiscoverUser[]) => {
    setCountries((prev) => {
      const set = new Set(prev);
      for (const u of users) if (u.countryCode) set.add(u.countryCode);
      return Array.from(set).sort((a, b) =>
        countryLabel(a).localeCompare(countryLabel(b)),
      );
    });
  }, []);

  const load = useCallback(
    (country: string, signal?: AbortSignal) => {
      setStatus("loading");
      seenRef.current = new Set();
      getDiscoverUsers(
        { mode: "world", country: country || undefined, limit: FOR_YOU_LIMIT },
        signal,
      )
        .then((page) => {
          setItems(dedupe(page.items, seenRef.current));
          setNextCursor(page.nextCursor);
          accrueCountries(page.items);
          setStatus("ready");
        })
        .catch((err) => {
          if (isAbort(err)) return;
          setStatus("error");
        });
    },
    [accrueCountries],
  );

  useEffect(() => {
    const c = new AbortController();
    load(activeCountry, c.signal);
    return () => c.abort();
  }, [activeCountry, load]);

  const loadMore = () => {
    if (loadingMoreRef.current) return;
    const cursor = nextCursor;
    if (!cursor) return;
    loadingMoreRef.current = true;
    setLoadingMore(true);
    getDiscoverUsers({
      mode: "world",
      country: activeCountry || undefined,
      limit: FOR_YOU_LIMIT,
      cursor,
    })
      .then((page) => {
        setItems((prev) => [...prev, ...dedupe(page.items, seenRef.current)]);
        setNextCursor(page.nextCursor);
        accrueCountries(page.items);
      })
      .catch(() => {
        // Keep what we have.
      })
      .finally(() => {
        loadingMoreRef.current = false;
        setLoadingMore(false);
      });
  };

  const removeFollowed = (id: string) =>
    setItems((prev) => prev.filter((p) => p.id !== id));

  const chipBase =
    "shrink-0 whitespace-nowrap rounded-full px-3.5 py-1.5 text-sm font-medium transition-colors";

  return (
    <section>
      <div className="mb-3 flex items-center gap-3">
        <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-primary-soft text-primary">
          <Globe className="h-5 w-5" />
        </span>
        <div className="min-w-0">
          <h2 className="text-sm font-semibold text-foreground">
            Explore the world
          </h2>
          <p className="truncate text-xs text-muted">
            Meet people from around the world.
          </p>
        </div>
      </div>

      {/* Country chips */}
      <div className="mb-4 flex gap-2 overflow-x-auto pb-1 [-ms-overflow-style:none] [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
        <button
          type="button"
          onClick={() => setActiveCountry("")}
          aria-pressed={activeCountry === ""}
          className={`${chipBase} ${
            activeCountry === ""
              ? "bg-primary text-white"
              : "border border-border text-muted hover:text-foreground"
          }`}
        >
          All countries
        </button>
        {countries.map((code) => (
          <button
            key={code}
            type="button"
            onClick={() => setActiveCountry(code)}
            aria-pressed={activeCountry === code}
            className={`${chipBase} ${
              activeCountry === code
                ? "bg-primary text-white"
                : "border border-border text-muted hover:text-foreground"
            }`}
          >
            {countryLabel(code)}
          </button>
        ))}
      </div>

      {status === "loading" ? <CardSkeletons /> : null}

      {status === "error" ? (
        <div className="rounded-2xl border border-border bg-surface p-8 text-center">
          <p className="text-sm text-muted">Couldn’t load people.</p>
          <button
            type="button"
            onClick={() => load(activeCountry)}
            className="mt-3 rounded-full bg-primary px-4 py-1.5 text-sm text-white transition-colors hover:bg-primary-hover"
          >
            Try again
          </button>
        </div>
      ) : null}

      {status === "ready" && items.length === 0 ? (
        <div className="rounded-2xl border border-border bg-surface px-6 py-12 text-center">
          <p className="text-sm text-muted">
            {activeCountry
              ? `No people to explore in ${countryLabel(activeCountry)} right now.`
              : "No people to explore right now."}
          </p>
        </div>
      ) : null}

      {items.length > 0 ? (
        <>
          <CardGrid>
            {items.map((u) => (
              <PersonCard key={u.id} user={u} onFollowed={removeFollowed} />
            ))}
          </CardGrid>
          {nextCursor ? (
            <div className="mt-4 flex justify-center">
              <button
                type="button"
                onClick={loadMore}
                disabled={loadingMore}
                className="rounded-full border border-border bg-surface px-5 py-2 text-sm text-muted transition-colors hover:text-foreground disabled:opacity-50"
              >
                {loadingMore ? "Loading…" : "Load more"}
              </button>
            </div>
          ) : null}
        </>
      ) : null}
    </section>
  );
}

/**
 * "Popular people" — the most-followed users (backend mode=popular), excluding
 * self / blocked / already-followed. Shows a subtle follower count for context.
 */
function PopularSection() {
  const [items, setItems] = useState<DiscoverUser[]>([]);
  const [status, setStatus] = useState<Status>("loading");
  const [nextCursor, setNextCursor] = useState("");
  const [loadingMore, setLoadingMore] = useState(false);
  const loadingMoreRef = useRef(false);
  const seenRef = useRef<Set<string>>(new Set());

  const load = useCallback((signal?: AbortSignal) => {
    setStatus("loading");
    seenRef.current = new Set();
    getDiscoverUsers({ mode: "popular", limit: FOR_YOU_LIMIT }, signal)
      .then((page) => {
        setItems(dedupe(page.items, seenRef.current));
        setNextCursor(page.nextCursor);
        setStatus("ready");
      })
      .catch((err) => {
        if (isAbort(err)) return;
        setStatus("error");
      });
  }, []);

  useEffect(() => {
    const c = new AbortController();
    load(c.signal);
    return () => c.abort();
  }, [load]);

  const loadMore = () => {
    if (loadingMoreRef.current) return;
    const cursor = nextCursor;
    if (!cursor) return;
    loadingMoreRef.current = true;
    setLoadingMore(true);
    getDiscoverUsers({ mode: "popular", limit: FOR_YOU_LIMIT, cursor })
      .then((page) => {
        setItems((prev) => [...prev, ...dedupe(page.items, seenRef.current)]);
        setNextCursor(page.nextCursor);
      })
      .catch(() => {
        // Keep what we have.
      })
      .finally(() => {
        loadingMoreRef.current = false;
        setLoadingMore(false);
      });
  };

  const removeFollowed = (id: string) =>
    setItems((prev) => prev.filter((p) => p.id !== id));

  return (
    <section>
      <div className="mb-3 flex items-center gap-3">
        <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-primary-soft text-primary">
          <TrendingUp className="h-5 w-5" />
        </span>
        <div className="min-w-0">
          <h2 className="text-sm font-semibold text-foreground">
            Popular people
          </h2>
          <p className="truncate text-xs text-muted">
            The most-followed people on Together.
          </p>
        </div>
      </div>

      {status === "loading" ? <CardSkeletons /> : null}

      {status === "error" ? (
        <div className="rounded-2xl border border-border bg-surface p-8 text-center">
          <p className="text-sm text-muted">Couldn’t load popular people.</p>
          <button
            type="button"
            onClick={() => load()}
            className="mt-3 rounded-full bg-primary px-4 py-1.5 text-sm text-white transition-colors hover:bg-primary-hover"
          >
            Try again
          </button>
        </div>
      ) : null}

      {status === "ready" && items.length === 0 ? (
        <div className="rounded-2xl border border-border bg-surface px-6 py-12 text-center">
          <p className="text-sm text-muted">No popular people to show yet.</p>
        </div>
      ) : null}

      {items.length > 0 ? (
        <>
          <CardGrid>
            {items.map((u) => (
              <PersonCard
                key={u.id}
                user={u}
                onFollowed={removeFollowed}
                showFollowers
              />
            ))}
          </CardGrid>
          {nextCursor ? (
            <div className="mt-4 flex justify-center">
              <button
                type="button"
                onClick={loadMore}
                disabled={loadingMore}
                className="rounded-full border border-border bg-surface px-5 py-2 text-sm text-muted transition-colors hover:text-foreground disabled:opacity-50"
              >
                {loadingMore ? "Loading…" : "Load more"}
              </button>
            </div>
          ) : null}
        </>
      ) : null}
    </section>
  );
}

function SearchSection({
  query,
  status,
  results,
  topicStatus,
  topics,
}: {
  query: string;
  status: SearchStatus;
  results: SearchUserItem[];
  topicStatus: SearchStatus;
  topics: TrendingTopic[];
}) {
  return (
    <section>
      <SectionHeading>Results for “{query}”</SectionHeading>

      {status === "loading" ? (
        <div className="flex items-center gap-2 px-1 py-6 text-sm text-muted">
          <Loader2 className="h-4 w-4 animate-spin" />
          Searching…
        </div>
      ) : null}

      {status === "error" ? (
        <p className="px-1 py-6 text-sm text-muted">Couldn’t run search.</p>
      ) : null}

      {status === "ready" && results.length === 0 ? (
        <p className="px-1 py-6 text-sm text-muted-soft">
          No people found for “{query}”.
        </p>
      ) : null}

      {results.length > 0 ? (
        <CardGrid>
          {results.map((u) => (
            <PersonCard key={u.id} user={u} />
          ))}
        </CardGrid>
      ) : null}

      {topicStatus === "loading" ? (
        <div className="mt-6 flex items-center gap-2 text-sm text-muted">
          <Loader2 className="h-4 w-4 animate-spin" />
          Searching topics…
        </div>
      ) : null}

      {topicStatus === "error" ? (
        <p className="mt-6 text-sm text-muted">Couldn’t load topics.</p>
      ) : null}

      {topics.length > 0 ? (
        <div className="mt-6">
          <SectionHeading>Topics</SectionHeading>
          <div className="flex flex-wrap gap-2">
            {topics.map((topic) => (
              <Link
                key={topic.slug}
                href={`/topic/${encodeURIComponent(topic.slug)}`}
                className="rounded-full border border-border bg-surface px-3.5 py-1.5 text-sm text-muted"
              >
                #{topic.slug}
                <span className="ml-1.5 text-xs text-muted-soft">
                  {topic.postsCount}
                </span>
              </Link>
            ))}
          </div>
        </div>
      ) : null}
    </section>
  );
}
