"use client";

import { useCallback } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";

import { Feed } from "@/components/feed/Feed";
import { FeedProvider, type FeedPageFetcher } from "@/lib/feed-context";
import { getTopicPosts } from "@/lib/api";
import { useLanguage } from "@/lib/language-context";

export default function TopicPage() {
  const { t } = useLanguage();
  const params = useParams<{ slug: string }>();
  const slug = params.slug;

  const fetchPage = useCallback<FeedPageFetcher>(
    (pageParams, signal) => getTopicPosts(slug, pageParams, signal),
    [slug],
  );

  return (
    <div className="mx-auto w-full max-w-3xl">
      <header className="mb-5">
        <Link
          href="/discover"
          className="text-sm text-muted transition-colors hover:text-foreground"
        >
          Discover
        </Link>
        <h1 className="mt-2 text-2xl font-bold tracking-tight text-foreground">
          #{slug}
        </h1>
        <p className="mt-1 text-sm text-muted">Posts in this topic.</p>
      </header>

      <FeedProvider key={slug} fetchPage={fetchPage}>
        <Feed
          loadingMessage={t("feed.topicLoading")}
          errorMessage={t("feed.topicError")}
          emptyMessage={t("feed.topicEmpty")}
        />
      </FeedProvider>
    </div>
  );
}
