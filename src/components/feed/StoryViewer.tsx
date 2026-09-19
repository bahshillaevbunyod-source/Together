"use client";

import Image from "next/image";
import { ChevronLeft, ChevronRight, Trash2, X } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";

import { deleteStory, viewStory } from "@/lib/api";
import { useAuth } from "@/lib/auth-context";
import { useLanguage } from "@/lib/language-context";
import type { Story } from "@/types/story";

type StoryGroup = {
  author: Story["author"];
  stories: Story[];
  hasUnseenStory: boolean;
};

export function StoryViewer({
  group,
  onClose,
  onViewed,
  onDeleted,
}: {
  group: StoryGroup;
  onClose: () => void;
  onViewed: (storyID: string) => void;
  onDeleted: (storyID: string) => void;
}) {
  const { t } = useLanguage();
  const { user } = useAuth();
  const viewerRef = useRef<HTMLDivElement>(null);
  const [index, setIndex] = useState(0);
  const [deleting, setDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState(false);

  const stories = useMemo(
    () =>
      [...group.stories].sort((a, b) => {
        const timeDifference = Date.parse(a.createdAt) - Date.parse(b.createdAt);
        return timeDifference || a.id.localeCompare(b.id);
      }),
    [group.stories],
  );
  const story = stories[index] ?? null;
  const isOwnStory = group.author.id === user?.id;

  useEffect(() => {
    viewerRef.current?.focus();
  }, []);

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
      if (event.key === "ArrowLeft") setIndex((current) => Math.max(0, current - 1));
      if (event.key === "ArrowRight") {
        setIndex((current) => Math.min(stories.length - 1, current + 1));
      }
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [onClose, stories.length]);

  useEffect(() => {
    if (!story) return;
    if (!story.viewed) {
      onViewed(story.id);
      void viewStory(story.id).catch(() => undefined);
    }
  }, [onViewed, story]);

  useEffect(() => {
    document.body.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = "";
    };
  }, []);

  const removeCurrent = async () => {
    if (!story || deleting) return;
    if (!window.confirm(t("stories.deleteConfirm"))) return;
    setDeleting(true);
    setDeleteError(false);
    try {
      await deleteStory(story.id);
      onDeleted(story.id);
      if (stories.length <= 1) {
        onClose();
      } else {
        setIndex((current) => Math.min(current, stories.length - 2));
      }
    } catch {
      setDeleteError(true);
    } finally {
      setDeleting(false);
    }
  };

  if (!story) return null;

  return (
    <div
      ref={viewerRef}
      tabIndex={-1}
      className="fixed inset-0 z-50 flex flex-col bg-black text-white outline-none"
      role="dialog"
      aria-modal="true"
      aria-label={t("stories.viewerLabel", {
        name: group.author.displayName || group.author.username,
      })}
      onClick={onClose}
    >
      <div className="flex items-center gap-3 p-4" onClick={(event) => event.stopPropagation()}>
        <div className="flex min-w-0 flex-1 items-center gap-3">
          <Image
            src={group.author.avatarUrl ?? "data:image/svg+xml;utf8," + encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" width="40" height="40"><circle cx="20" cy="20" r="20" fill="#52525b"/></svg>')}
            alt={group.author.displayName || group.author.username}
            width={40}
            height={40}
            unoptimized={Boolean(group.author.avatarUrl)}
            className="h-10 w-10 shrink-0 rounded-full object-cover"
          />
          <span className="truncate text-sm font-medium">
            {group.author.displayName || `@${group.author.username}`}
          </span>
        </div>
        {isOwnStory ? (
          <button
            type="button"
            aria-label={t("post.delete")}
            onClick={removeCurrent}
            disabled={deleting}
            className="rounded-full p-2 text-white/80 hover:bg-white/10 disabled:opacity-50"
          >
            <Trash2 className="h-5 w-5" />
          </button>
        ) : null}
        <button
          type="button"
          aria-label={t("profile.close")}
          onClick={onClose}
          className="rounded-full p-2 text-white/80 hover:bg-white/10"
        >
          <X className="h-6 w-6" />
        </button>
      </div>

      <div className="flex gap-1 px-4" aria-label={t("stories.progress", { current: index + 1, total: stories.length })}>
        {stories.map((item, itemIndex) => (
          <span
            key={item.id}
            className={`h-1 flex-1 rounded-full ${itemIndex <= index ? "bg-white" : "bg-white/30"}`}
          />
        ))}
      </div>

      <div className="relative flex min-h-0 flex-1 items-center justify-center p-4" onClick={(event) => event.stopPropagation()}>
        {story.media.url ? (
          story.media.type === "video" ? (
            <video
              src={story.media.url}
              controls
              autoPlay
              playsInline
              className="max-h-full max-w-full rounded-xl object-contain"
            />
          ) : (
            <Image
              src={story.media.url}
              alt={t("stories.mediaAlt", { name: group.author.displayName || group.author.username })}
              fill
              unoptimized
              className="object-contain"
            />
          )
        ) : (
          <p className="text-sm text-white/70">{t("stories.mediaUnavailable")}</p>
        )}

        {index > 0 ? (
          <button
            type="button"
            aria-label={t("stories.previous")}
            onClick={() => setIndex((current) => Math.max(0, current - 1))}
            className="absolute left-2 rounded-full bg-black/40 p-2 hover:bg-black/60"
          >
            <ChevronLeft className="h-6 w-6" />
          </button>
        ) : null}
        {index < stories.length - 1 ? (
          <button
            type="button"
            aria-label={t("stories.next")}
            onClick={() => setIndex((current) => Math.min(stories.length - 1, current + 1))}
            className="absolute right-2 rounded-full bg-black/40 p-2 hover:bg-black/60"
          >
            <ChevronRight className="h-6 w-6" />
          </button>
        ) : null}
      </div>

      {deleteError ? (
        <p className="px-4 pb-4 text-center text-sm text-red-300" role="alert">
          {t("stories.deleteError")}
        </p>
      ) : null}
    </div>
  );
}
