"use client";

import { Bookmark } from "lucide-react";

import { PostCard } from "./PostCard";
import { useFeed } from "@/lib/feed-context";

/**
 * Renders the viewer's saved posts using the shared feed context (backed by
 * getBookmarks). Reuses PostCard; passing onUnsave={removePost} makes unsaving
 * drop the card from the list immediately.
 */
export function BookmarksFeed() {
  const { posts, status, nextCursor, loadingMore, reload, loadMore, removePost } =
    useFeed();

  if (status === "loading") {
    return (
      <section className="flex flex-col gap-4" aria-busy="true">
        {[0, 1, 2].map((i) => (
          <div
            key={i}
            className="animate-pulse rounded-2xl border border-border bg-surface p-4 shadow-sm"
          >
            <div className="flex items-center gap-3">
              <div className="h-11 w-11 rounded-full bg-background" />
              <div className="flex-1 space-y-2">
                <div className="h-3 w-32 rounded bg-background" />
                <div className="h-2.5 w-20 rounded bg-background" />
              </div>
            </div>
            <div className="mt-4 space-y-2">
              <div className="h-3 w-full rounded bg-background" />
              <div className="h-3 w-4/5 rounded bg-background" />
            </div>
          </div>
        ))}
      </section>
    );
  }

  if (status === "error") {
    return (
      <section className="flex flex-col gap-4">
        <div className="rounded-2xl border border-border bg-surface p-6 text-center">
          <p className="text-sm text-muted">Couldn’t load your bookmarks.</p>
          <button
            type="button"
            onClick={reload}
            className="mt-3 rounded-full bg-primary px-4 py-1.5 text-sm text-white transition-colors hover:bg-primary-hover"
          >
            Try again
          </button>
        </div>
      </section>
    );
  }

  if (posts.length === 0) {
    return (
      <section className="flex flex-col gap-4">
        <div className="flex flex-col items-center rounded-2xl border border-border bg-surface px-6 py-14 text-center">
          <span className="flex h-14 w-14 items-center justify-center rounded-full bg-primary-soft text-primary">
            <Bookmark className="h-7 w-7" />
          </span>
          <h2 className="mt-4 text-base font-semibold text-foreground">
            No bookmarks yet
          </h2>
          <p className="mt-1 max-w-sm text-sm text-muted">
            Tap the bookmark icon on any post to save it here for later.
          </p>
        </div>
      </section>
    );
  }

  return (
    <section className="flex flex-col gap-4">
      {posts.map((post) => (
        <PostCard key={post.id} post={post} onUnsave={removePost} />
      ))}

      {nextCursor ? (
        <button
          type="button"
          onClick={loadMore}
          disabled={loadingMore}
          className="mx-auto rounded-full border border-border bg-surface px-5 py-2 text-sm text-muted transition-colors hover:text-foreground disabled:opacity-50"
        >
          {loadingMore ? "Loading…" : "Load more"}
        </button>
      ) : null}
    </section>
  );
}
