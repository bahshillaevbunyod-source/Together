"use client";

import Image from "next/image";
import { Plus } from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";

import { getStories } from "@/lib/api";
import { useAuth } from "@/lib/auth-context";
import { useLanguage } from "@/lib/language-context";
import type { Story } from "@/types/story";
import { StoryComposer } from "@/components/feed/StoryComposer";
import { StoryViewer } from "@/components/feed/StoryViewer";

type StoryStatus = "loading" | "ready" | "error";

type StoryGroup = {
  author: Story["author"];
  stories: Story[];
  hasUnseenStory: boolean;
};

const FALLBACK_AVATAR =
  "data:image/svg+xml;utf8," +
  encodeURIComponent(
    '<svg xmlns="http://www.w3.org/2000/svg" width="60" height="60"><circle cx="30" cy="30" r="30" fill="#d4d4d8"/></svg>',
  );

function groupStories(items: Story[], ownUserID: string | undefined): StoryGroup[] {
  const groups = new Map<string, StoryGroup>();
  for (const item of items) {
    const existing = groups.get(item.author.id);
    if (existing) {
      existing.stories.push(item);
      existing.hasUnseenStory ||= !item.viewed;
      continue;
    }
    groups.set(item.author.id, {
      author: item.author,
      stories: [item],
      hasUnseenStory: !item.viewed,
    });
  }

  return [...groups.values()].sort((a, b) => {
    if (a.author.id === ownUserID) return -1;
    if (b.author.id === ownUserID) return 1;
    return 0;
  });
}

export function StoriesRow() {
  const { t } = useLanguage();
  const { user } = useAuth();
  const [items, setItems] = useState<Story[]>([]);
  const [status, setStatus] = useState<StoryStatus>("loading");
  const [composerOpen, setComposerOpen] = useState(false);
  const [selectedAuthorID, setSelectedAuthorID] = useState<string | null>(null);

  const load = useCallback(async (signal?: AbortSignal) => {
    setStatus("loading");
    try {
      const all: Story[] = [];
      let cursor: string | undefined;
      const seenCursors = new Set<string>();
      while (true) {
        const page = await getStories({ cursor, limit: 50 }, signal);
        all.push(...page.items);
        if (!page.nextCursor || seenCursors.has(page.nextCursor)) break;
        seenCursors.add(page.nextCursor);
        cursor = page.nextCursor;
      }
      setItems(all);
      setStatus("ready");
    } catch (error) {
      if (error instanceof DOMException && error.name === "AbortError") return;
      setStatus("error");
    }
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  }, [load]);

  const groups = useMemo(() => groupStories(items, user?.id), [items, user?.id]);
  const selectedGroup = selectedAuthorID
    ? groups.find((group) => group.author.id === selectedAuthorID) ?? null
    : null;

  const markViewed = useCallback((storyID: string) => {
    setItems((current) =>
      current.map((item) => (item.id === storyID ? { ...item, viewed: true } : item)),
    );
  }, []);

  const removeStory = useCallback((storyID: string) => {
    setItems((current) => current.filter((item) => item.id !== storyID));
  }, []);

  const setLiked = useCallback((storyID: string, liked: boolean) => {
    setItems((current) =>
      current.map((item) => (item.id === storyID ? { ...item, likedByMe: liked } : item)),
    );
  }, []);

  // Step to the neighbouring person in the row; false when there is none.
  const stepGroup = useCallback(
    (delta: 1 | -1) => {
      const i = groups.findIndex((group) => group.author.id === selectedAuthorID);
      const next = i >= 0 ? groups[i + delta] : undefined;
      if (!next) return false;
      setSelectedAuthorID(next.author.id);
      return true;
    },
    [groups, selectedAuthorID],
  );

  return (
    <section className="rounded-2xl border border-border bg-surface p-4 shadow-sm">
      <div className="flex min-h-[5.5rem] items-start gap-4 overflow-x-auto">
        {/* Add story */}
        <button
          type="button"
          aria-label={t("stories.add")}
          onClick={() => setComposerOpen(true)}
          className="flex w-16 shrink-0 flex-col items-center gap-2"
        >
          <span className="flex h-16 w-16 items-center justify-center rounded-full border-2 border-dashed border-border text-primary">
            <Plus className="h-6 w-6" />
          </span>
          <span className="text-xs text-muted">{t("stories.add")}</span>
        </button>

        {status === "loading" ? (
          <div className="flex gap-4" aria-label={t("stories.loading")}>
            {Array.from({ length: 5 }, (_, index) => (
              <span
                key={index}
                className="h-16 w-16 shrink-0 animate-pulse rounded-full bg-border"
              />
            ))}
          </div>
        ) : null}

        {status === "error" ? (
          <div className="flex min-w-48 items-center gap-2 text-sm text-muted">
            <span>{t("stories.loadError")}</span>
            <button
              type="button"
              onClick={() => void load()}
              className="shrink-0 text-primary hover:underline focus:outline-none focus-visible:ring-2 focus-visible:ring-primary"
            >
              {t("search.tryAgain")}
            </button>
          </div>
        ) : null}

        {/* People stories, grouped by author from the real feed. */}
        {status === "ready" && groups.map((group) => (
          <button
            key={group.author.id}
            type="button"
            onClick={() => setSelectedAuthorID(group.author.id)}
            aria-label={t("stories.viewerLabel", {
              name: group.author.displayName || group.author.username,
            })}
            className="flex w-16 shrink-0 flex-col items-center gap-2"
          >
            <span
              className={`h-16 w-16 rounded-full p-[2px] ${
                group.hasUnseenStory
                  ? "bg-gradient-to-tr from-[#EAF3FF] via-[#67B8FF] to-[#2F6BFF]"
                  : "bg-border"
              }`}
            >
              <span className="block h-full w-full rounded-full bg-surface p-[2px]">
                <Image
                  src={group.author.avatarUrl ?? FALLBACK_AVATAR}
                  alt={group.author.displayName || group.author.username}
                  width={60}
                  height={60}
                  unoptimized={Boolean(group.author.avatarUrl)}
                  className="h-full w-full rounded-full object-cover"
                />
              </span>
            </span>
            <span className="w-16 truncate text-center text-xs text-muted">
              {group.author.displayName || `@${group.author.username}`}
            </span>
          </button>
        ))}
      </div>

      {composerOpen ? (
        <StoryComposer
          onClose={() => setComposerOpen(false)}
          onCreated={() => {
            setComposerOpen(false);
            void load();
          }}
        />
      ) : null}

      {selectedGroup ? (
        <StoryViewer
          group={selectedGroup}
          onClose={() => setSelectedAuthorID(null)}
          onViewed={markViewed}
          onDeleted={removeStory}
          onLikedChange={setLiked}
          onNextGroup={() => stepGroup(1)}
          onPrevGroup={() => stepGroup(-1)}
        />
      ) : null}
    </section>
  );
}
