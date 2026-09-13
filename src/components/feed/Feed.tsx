"use client";

import { PostCard } from "./PostCard";
import { useFeed } from "@/lib/feed-context";

export function Feed() {
  const { posts, status, nextCursor, loadingMore, reload, loadMore } = useFeed();

  if (status === "loading") {
    return (
      <section className="flex flex-col gap-4">
        <p className="py-8 text-center text-sm text-muted">Loading feed…</p>
      </section>
    );
  }

  if (status === "error") {
    return (
      <section className="flex flex-col gap-4">
        <div className="rounded-2xl border border-border bg-surface p-6 text-center">
          <p className="text-sm text-muted">Couldn’t load your feed.</p>
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
        <p className="py-8 text-center text-sm text-muted">
          Your feed is empty. Follow people to see their posts here.
        </p>
      </section>
    );
  }

  return (
    <section className="flex flex-col gap-4">
      {posts.map((post) => (
        <PostCard key={post.id} post={post} />
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
