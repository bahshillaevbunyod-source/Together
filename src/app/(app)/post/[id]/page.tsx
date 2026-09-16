"use client";

import { useCallback } from "react";
import { useParams } from "next/navigation";

import { Feed } from "@/components/feed/Feed";
import { FeedProvider, type FeedPageFetcher } from "@/lib/feed-context";
import { getPost } from "@/lib/api";

export default function PostPermalinkPage() {
  const params = useParams<{ id: string }>();
  const id = Array.isArray(params.id) ? params.id[0] : params.id;

  // The existing single-post API performs all visibility, follow, and block
  // checks. Returning a one-item FeedPage lets this route reuse PostCard
  // without creating a second renderer or bypassing its media/actions.
  const fetchPage = useCallback<FeedPageFetcher>(
    async (_params, signal) => ({
      items: [await getPost(id, signal)],
      nextCursor: "",
    }),
    [id],
  );

  return (
    <div className="mx-auto flex w-full max-w-2xl flex-col gap-4">
      <header>
        <h1 className="text-xl font-semibold text-foreground">Post</h1>
      </header>
      <FeedProvider fetchPage={fetchPage}>
        <Feed
          loadingMessage="Loading post…"
          errorMessage="This post isn’t available."
          emptyMessage="This post isn’t available."
        />
      </FeedProvider>
    </div>
  );
}
