"use client";

import { PostCard } from "./PostCard";
import { useFeed } from "@/lib/feed-context";
import { useLanguage } from "@/lib/language-context";

type FeedProps = {
  loadingMessage?: string;
  errorMessage?: string;
  emptyMessage?: string;
};

export function Feed({
  loadingMessage,
  errorMessage,
  emptyMessage,
}: FeedProps) {
  const { posts, status, nextCursor, loadingMore, reload, loadMore } = useFeed();
  const { t } = useLanguage();

  // Callers may pass localized overrides; otherwise fall back to the feed's own
  // translated defaults.
  const loadingText = loadingMessage ?? t("feed.loading");
  const errorText = errorMessage ?? t("feed.error");
  const emptyText = emptyMessage ?? t("feed.empty");

  if (status === "loading") {
    return (
      <section className="flex flex-col gap-4">
        <p className="py-8 text-center text-sm text-muted">{loadingText}</p>
      </section>
    );
  }

  if (status === "error") {
    return (
      <section className="flex flex-col gap-4">
        <div className="rounded-2xl border border-border bg-surface p-6 text-center">
          <p className="text-sm text-muted">{errorText}</p>
          <button
            type="button"
            onClick={reload}
            className="mt-3 rounded-full bg-primary px-4 py-1.5 text-sm text-white transition-colors hover:bg-primary-hover"
          >
            {t("search.tryAgain")}
          </button>
        </div>
      </section>
    );
  }

  if (posts.length === 0) {
    return (
      <section className="flex flex-col gap-4">
        <p className="py-8 text-center text-sm text-muted">{emptyText}</p>
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
          {loadingMore ? t("feed.loadingMore") : t("feed.loadMore")}
        </button>
      ) : null}
    </section>
  );
}
