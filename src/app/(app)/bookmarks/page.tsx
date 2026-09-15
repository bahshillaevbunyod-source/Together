"use client";

import { BookmarksFeed } from "@/components/feed/BookmarksFeed";
import { FeedProvider } from "@/lib/feed-context";
import { getBookmarks } from "@/lib/api";

export default function BookmarksPage() {
  return (
    <div className="mx-auto flex w-full max-w-2xl flex-col gap-4">
      <header>
        <h1 className="text-xl font-semibold text-foreground">Bookmarks</h1>
        <p className="mt-0.5 text-sm text-muted">Posts you’ve saved for later.</p>
      </header>
      <FeedProvider fetchPage={getBookmarks}>
        <BookmarksFeed />
      </FeedProvider>
    </div>
  );
}
