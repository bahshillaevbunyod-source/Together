"use client";

import { BookmarksFeed } from "@/components/feed/BookmarksFeed";
import { FeedProvider } from "@/lib/feed-context";
import { getBookmarks } from "@/lib/api";
import { useLanguage } from "@/lib/language-context";

export default function BookmarksPage() {
  const { t } = useLanguage();

  return (
    <div className="mx-auto flex w-full max-w-2xl flex-col gap-4">
      <header>
        <h1 className="text-xl font-semibold text-foreground">
          {t("navigation.bookmarks")}
        </h1>
        <p className="mt-0.5 text-sm text-muted">{t("bookmarks.description")}</p>
      </header>
      <FeedProvider fetchPage={getBookmarks}>
        <BookmarksFeed />
      </FeedProvider>
    </div>
  );
}
