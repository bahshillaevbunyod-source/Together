"use client";

import Image from "next/image";
import { ChevronLeft, ChevronRight, Trash2, X } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";

import { deleteStory, viewStory } from "@/lib/api";
import { useAuth } from "@/lib/auth-context";
import { useLanguage } from "@/lib/language-context";
import { formatTimeAgo } from "@/lib/format";
import type { Story } from "@/types/story";

type StoryGroup = {
  author: Story["author"];
  stories: Story[];
  hasUnseenStory: boolean;
};

const FALLBACK_AVATAR =
  "data:image/svg+xml;utf8," +
  encodeURIComponent(
    '<svg xmlns="http://www.w3.org/2000/svg" width="40" height="40"><circle cx="20" cy="20" r="20" fill="#cbd5e1"/></svg>',
  );

// How long an image story stays before auto-advancing to the next one.
const AUTO_ADVANCE_MS = 5000;

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
  const authorName = group.author.displayName || group.author.username;

  const goPrev = () => setIndex((current) => Math.max(0, current - 1));
  const goNext = () => setIndex((current) => Math.min(stories.length - 1, current + 1));

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

  // Record a view once per unseen story.
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

  // Auto-advance image stories only, and never past the last one — so existing
  // manual navigation / close behavior is never disrupted.
  useEffect(() => {
    if (!story || story.media.type === "video" || !story.media.url) return;
    if (deleting) return;
    if (index >= stories.length - 1) return;
    const timer = window.setTimeout(
      () => setIndex((current) => Math.min(stories.length - 1, current + 1)),
      AUTO_ADVANCE_MS,
    );
    return () => window.clearTimeout(timer);
  }, [story, index, stories.length, deleting]);

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

  const prevStory = index > 0 ? stories[index - 1] : null;
  const nextStory = index < stories.length - 1 ? stories[index + 1] : null;

  // A small blurred preview card for the adjacent story (desktop only).
  const sidePreview = (item: Story | null, onSelect: () => void, side: "prev" | "next") =>
    item && item.media.url ? (
      <button
        type="button"
        aria-label={t(side === "prev" ? "stories.previous" : "stories.next")}
        onClick={onSelect}
        className="hidden h-40 w-24 shrink-0 overflow-hidden rounded-2xl opacity-70 shadow-md ring-1 ring-black/5 transition hover:opacity-100 lg:block"
      >
        <span className="relative block h-full w-full bg-neutral-200">
          <Image
            src={item.media.url}
            alt=""
            aria-hidden
            fill
            unoptimized
            className="object-cover blur-[1px]"
          />
        </span>
      </button>
    ) : (
      <span aria-hidden className="hidden h-40 w-24 shrink-0 lg:block" />
    );

  return (
    <div
      ref={viewerRef}
      tabIndex={-1}
      className="fixed inset-0 z-50 flex items-center justify-center gap-4 bg-neutral-900/40 p-4 outline-none backdrop-blur-md sm:gap-6"
      role="dialog"
      aria-modal="true"
      aria-label={t("stories.viewerLabel", { name: authorName })}
      onClick={onClose}
    >
      {/* Previous preview (desktop) */}
      {sidePreview(prevStory, goPrev, "prev")}

      {/* Portrait story card */}
      <div
        className="relative aspect-[9/16] max-h-[86vh] w-full max-w-[420px] shrink-0 overflow-hidden rounded-[20px] bg-neutral-900 shadow-2xl ring-1 ring-black/10"
        onClick={(event) => event.stopPropagation()}
      >
        {/* Media */}
        {story.media.url ? (
          story.media.type === "video" ? (
            <video
              key={story.id}
              src={story.media.url}
              controls
              autoPlay
              playsInline
              className="absolute inset-0 h-full w-full bg-black object-contain"
            />
          ) : (
            <>
              {/* Blurred, dimmed full-frame background so landscape photos keep
                  the portrait frame without stretching. */}
              <Image
                src={story.media.url}
                alt=""
                aria-hidden
                fill
                unoptimized
                className="scale-110 object-cover blur-2xl brightness-75"
              />
              {/* Sharp, aspect-preserved original centered above. */}
              <Image
                src={story.media.url}
                alt={t("stories.mediaAlt", { name: authorName })}
                fill
                unoptimized
                className="object-contain"
              />
            </>
          )
        ) : (
          <div className="flex h-full w-full items-center justify-center bg-neutral-800 px-6 text-center">
            <p className="text-sm text-white/80">{t("stories.mediaUnavailable")}</p>
          </div>
        )}

        {/* Top scrim for legible controls over any photo */}
        <div className="pointer-events-none absolute inset-x-0 top-0 h-28 bg-gradient-to-b from-black/55 to-transparent" />

        {/* Segmented progress */}
        <div
          className="absolute inset-x-0 top-0 flex gap-[3px] px-4 pt-3"
          aria-label={t("stories.progress", {
            current: index + 1,
            total: stories.length,
          })}
        >
          {stories.map((item, itemIndex) => (
            <span
              key={item.id}
              className={`h-[2px] flex-1 rounded-full ${
                itemIndex <= index ? "bg-white/90" : "bg-white/35"
              }`}
            />
          ))}
        </div>

        {/* Header: avatar / name / timestamp / delete / close */}
        <div className="absolute inset-x-0 top-0 flex items-center gap-2.5 px-3 pt-6 text-white">
          <Image
            src={group.author.avatarUrl ?? FALLBACK_AVATAR}
            alt={authorName}
            width={36}
            height={36}
            unoptimized={Boolean(group.author.avatarUrl)}
            className="h-9 w-9 shrink-0 rounded-full object-cover ring-2 ring-white/70"
          />
          <div className="min-w-0 flex-1 leading-tight">
            <div className="truncate text-sm font-semibold drop-shadow">
              {authorName}
            </div>
            <div className="truncate text-[11px] text-white/80 drop-shadow">
              {formatTimeAgo(story.createdAt)}
            </div>
          </div>
          {isOwnStory ? (
            <button
              type="button"
              aria-label={t("post.delete")}
              onClick={removeCurrent}
              disabled={deleting}
              className="rounded-full p-2 text-white/90 transition hover:bg-white/15 disabled:opacity-50"
            >
              <Trash2 className="h-5 w-5" />
            </button>
          ) : null}
          <button
            type="button"
            aria-label={t("profile.close")}
            onClick={onClose}
            className="rounded-full p-2 text-white/90 transition hover:bg-white/15"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* Tap zones for quick prev/next (mobile-friendly) */}
        <button
          type="button"
          aria-label={t("stories.previous")}
          onClick={goPrev}
          disabled={index === 0}
          className="absolute inset-y-0 left-0 w-1/3 cursor-default focus:outline-none disabled:cursor-default"
        />
        <button
          type="button"
          aria-label={t("stories.next")}
          onClick={goNext}
          disabled={index >= stories.length - 1}
          className="absolute inset-y-0 right-0 w-1/3 cursor-default focus:outline-none disabled:cursor-default"
        />

        {/* Visible chevrons */}
        {index > 0 ? (
          <button
            type="button"
            aria-label={t("stories.previous")}
            onClick={goPrev}
            className="absolute left-2 top-1/2 -translate-y-1/2 rounded-full bg-black/35 p-1.5 text-white transition hover:bg-black/55"
          >
            <ChevronLeft className="h-5 w-5" />
          </button>
        ) : null}
        {index < stories.length - 1 ? (
          <button
            type="button"
            aria-label={t("stories.next")}
            onClick={goNext}
            className="absolute right-2 top-1/2 -translate-y-1/2 rounded-full bg-black/35 p-1.5 text-white transition hover:bg-black/55"
          >
            <ChevronRight className="h-5 w-5" />
          </button>
        ) : null}

        {deleteError ? (
          <p
            className="absolute inset-x-0 bottom-0 bg-black/40 px-4 py-2 text-center text-sm text-red-200"
            role="alert"
          >
            {t("stories.deleteError")}
          </p>
        ) : null}
      </div>

      {/* Next preview (desktop) */}
      {sidePreview(nextStory, goNext, "next")}
    </div>
  );
}
