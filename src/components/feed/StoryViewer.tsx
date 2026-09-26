"use client";

import Image from "next/image";
import {
  ChevronLeft,
  ChevronRight,
  Eye,
  Heart,
  Pause,
  Send,
  Trash2,
  Volume2,
  VolumeX,
  X,
} from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import {
  ApiError,
  deleteStory,
  getStoryViewers,
  replyToStory,
  setStoryLiked,
  viewStory,
} from "@/lib/api";
import { useAuth } from "@/lib/auth-context";
import { useLanguage } from "@/lib/language-context";
import { formatTimeAgo } from "@/lib/format";
import type { Story, StoryViewer as StoryViewerEntry } from "@/types/story";

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

// How long an image story stays before auto-advancing.
const IMAGE_DURATION_MS = 5000;
// A press longer than this is a "hold to pause", not a tap.
const HOLD_MS = 220;

/**
 * Full-screen story viewer: segmented progress, custom (non-native) video
 * playback with a sound toggle, tap left/right to navigate, press-and-hold to
 * pause, automatic advance (and on to the next person's stories), plus real
 * interactions — like and reply on other people's stories, "Viewed by" with
 * the viewer list on your own.
 */
export function StoryViewer({
  group,
  onClose,
  onViewed,
  onDeleted,
  onLikedChange,
  onNextGroup,
  onPrevGroup,
}: {
  group: StoryGroup;
  onClose: () => void;
  onViewed: (storyID: string) => void;
  onDeleted: (storyID: string) => void;
  onLikedChange?: (storyID: string, liked: boolean) => void;
  /** Move to the next person's stories; returns false when there is none. */
  onNextGroup?: () => boolean;
  /** Move to the previous person's stories; returns false when there is none. */
  onPrevGroup?: () => boolean;
}) {
  const { t, locale } = useLanguage();
  const { user } = useAuth();
  const rootRef = useRef<HTMLDivElement>(null);
  const videoRef = useRef<HTMLVideoElement>(null);
  const bgCanvasRef = useRef<HTMLCanvasElement>(null);

  const stories = useMemo(
    () =>
      [...group.stories].sort((a, b) => {
        const d = Date.parse(a.createdAt) - Date.parse(b.createdAt);
        return d || a.id.localeCompare(b.id);
      }),
    [group.stories],
  );
  const [index, setIndex] = useState(() => {
    const firstUnseen = stories.findIndex((s) => !s.viewed);
    return firstUnseen >= 0 ? firstUnseen : 0;
  });
  const story = stories[Math.min(index, stories.length - 1)] ?? null;
  const isOwnStory = group.author.id === user?.id;
  const authorName = group.author.displayName || group.author.username;

  // Playback state.
  const [progress, setProgress] = useState(0); // 0..1 of the current story
  // Image elapsed fraction, kept in a ref so pause/resume continues from it.
  const imageProgressRef = useRef(0);
  const [held, setHeld] = useState(false); // press-and-hold / hidden-tab pause
  const [muted, setMuted] = useState(true); // autoplay-safe default
  const [mediaFailed, setMediaFailed] = useState(false);
  const [mediaReady, setMediaReady] = useState(false);

  // Interaction state.
  const [reply, setReply] = useState("");
  const [replyFocused, setReplyFocused] = useState(false);
  const [sending, setSending] = useState(false);
  const [notice, setNotice] = useState<{ kind: "ok" | "error"; text: string } | null>(null);
  const [liked, setLiked] = useState(false);
  const [liking, setLiking] = useState(false);
  const [sheetOpen, setSheetOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [preview, setPreview] = useState<{ total: number; items: StoryViewerEntry[] } | null>(null);

  const paused = held || replyFocused || sheetOpen || deleting || sending;

  // --- navigation ---------------------------------------------------------
  const goNext = useCallback(() => {
    if (index < stories.length - 1) setIndex(index + 1);
    else if (!onNextGroup?.()) onClose();
  }, [index, stories.length, onNextGroup, onClose]);

  const goPrev = useCallback(() => {
    if (index > 0) setIndex(index - 1);
    else if (!onPrevGroup?.()) {
      // First story of the first person: restart it.
      setProgress(0);
      imageProgressRef.current = 0;
      if (videoRef.current) videoRef.current.currentTime = 0;
    }
  }, [index, onPrevGroup]);

  // Reset per-story state when the story changes (also when a new person opens).
  useEffect(() => {
    setProgress(0);
    imageProgressRef.current = 0;
    setMediaFailed(false);
    setMediaReady(false);
    setReply("");
    setNotice(null);
    setSheetOpen(false);
    setLiked(Boolean(story?.likedByMe));
    setPreview(null);
  }, [story?.id, story?.likedByMe]);

  // When the person changes, start at their first unseen story.
  useEffect(() => {
    const firstUnseen = stories.findIndex((s) => !s.viewed);
    setIndex(firstUnseen >= 0 ? firstUnseen : 0);
    // Only on a new person, not when their stories are marked viewed.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [group.author.id]);

  // Record a view (never for your own story; the server ignores it anyway).
  useEffect(() => {
    if (!story || isOwnStory || story.viewed) return;
    onViewed(story.id);
    void viewStory(story.id).catch(() => undefined);
  }, [isOwnStory, onViewed, story]);

  // Owner: load the first viewers for "Viewed by N" and the avatar stack.
  useEffect(() => {
    if (!story || !isOwnStory) return;
    const controller = new AbortController();
    getStoryViewers(story.id, { limit: 3 }, controller.signal)
      .then((page) => setPreview({ total: page.total, items: page.items }))
      .catch(() => undefined);
    return () => controller.abort();
  }, [isOwnStory, story]);

  // --- image timing (pausable) ------------------------------------------
  useEffect(() => {
    if (!story || story.media.type === "video" || mediaFailed || !mediaReady || paused) return;
    let last = performance.now();
    let raf = 0;
    const tick = (now: number) => {
      const next = Math.min(1, imageProgressRef.current + (now - last) / IMAGE_DURATION_MS);
      last = now;
      imageProgressRef.current = next;
      setProgress(next);
      if (next >= 1) {
        goNext();
        return;
      }
      raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
  }, [story, mediaFailed, mediaReady, paused, goNext]);

  // Media that cannot be shown still advances after the image duration.
  useEffect(() => {
    if (!mediaFailed || paused) return;
    const timer = window.setTimeout(goNext, IMAGE_DURATION_MS);
    return () => window.clearTimeout(timer);
  }, [mediaFailed, paused, goNext]);

  // --- video playback -----------------------------------------------------
  useEffect(() => {
    const video = videoRef.current;
    if (!video || story?.media.type !== "video") return;
    video.muted = muted;
    if (paused) {
      video.pause();
    } else {
      void video.play().catch(() => {
        // Autoplay with sound refused by the browser: continue muted.
        if (!video.muted) setMuted(true);
      });
    }
  }, [paused, muted, story?.id, story?.media.type, mediaReady]);

  // Video progress + ambient background frame.
  useEffect(() => {
    const video = videoRef.current;
    if (!video || story?.media.type !== "video") return;
    let raf = 0;
    let lastPaint = 0;
    const loop = (now: number) => {
      if (video.duration > 0 && Number.isFinite(video.duration)) {
        setProgress(Math.min(1, video.currentTime / video.duration));
      }
      // Repaint the blurred background about twice a second (tiny canvas).
      const canvas = bgCanvasRef.current;
      if (canvas && now - lastPaint > 500 && video.readyState >= 2) {
        lastPaint = now;
        try {
          canvas.getContext("2d")?.drawImage(video, 0, 0, canvas.width, canvas.height);
        } catch {
          // Display-only; ignore.
        }
      }
      raf = requestAnimationFrame(loop);
    };
    raf = requestAnimationFrame(loop);
    return () => cancelAnimationFrame(raf);
  }, [story?.id, story?.media.type]);

  // Pause while the tab is hidden.
  useEffect(() => {
    const onVis = () => setHeld(document.hidden);
    document.addEventListener("visibilitychange", onVis);
    return () => document.removeEventListener("visibilitychange", onVis);
  }, []);

  // Body scroll lock + initial focus.
  useEffect(() => {
    const previous = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    rootRef.current?.focus();
    return () => {
      document.body.style.overflow = previous;
    };
  }, []);

  // Keyboard: Escape closes (the sheet first), arrows navigate, space pauses.
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      const typing = (event.target as HTMLElement | null)?.tagName === "INPUT";
      if (event.key === "Escape") {
        if (sheetOpen) setSheetOpen(false);
        else onClose();
        return;
      }
      if (typing) return;
      if (event.key === "ArrowLeft") goPrev();
      if (event.key === "ArrowRight") goNext();
      if (event.key === " ") {
        event.preventDefault();
        setHeld((h) => !h);
      }
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [goNext, goPrev, onClose, sheetOpen]);

  // --- gestures: tap left/right, press-and-hold to pause -------------------
  const press = useRef<{ x: number; timer: number; holding: boolean } | null>(null);
  const onPointerDown = (event: React.PointerEvent<HTMLDivElement>) => {
    if (event.button !== 0) return;
    const rect = event.currentTarget.getBoundingClientRect();
    const x = (event.clientX - rect.left) / rect.width;
    const timer = window.setTimeout(() => {
      if (press.current) {
        press.current.holding = true;
        setHeld(true);
      }
    }, HOLD_MS);
    press.current = { x, timer, holding: false };
  };
  const endPress = (navigate: boolean) => {
    const current = press.current;
    press.current = null;
    if (!current) return;
    window.clearTimeout(current.timer);
    if (current.holding) {
      setHeld(false);
      return;
    }
    if (navigate) {
      if (current.x < 0.3) goPrev();
      else goNext();
    }
  };

  // --- actions ------------------------------------------------------------
  const toggleLike = async () => {
    if (!story || liking) return;
    const next = !liked;
    setLiked(next); // optimistic, rolled back on failure
    setLiking(true);
    try {
      await setStoryLiked(story.id, next);
      onLikedChange?.(story.id, next);
    } catch {
      setLiked(!next);
      setNotice({ kind: "error", text: t("stories.likeError") });
    } finally {
      setLiking(false);
    }
  };

  const sendReply = async (event: React.FormEvent) => {
    event.preventDefault();
    const content = reply.trim();
    if (!story || !content || sending) return;
    setSending(true);
    setNotice(null);
    try {
      await replyToStory(story.id, content);
      setReply("");
      setNotice({ kind: "ok", text: t("stories.replySent") });
      (document.activeElement as HTMLElement | null)?.blur();
    } catch (err) {
      setNotice({
        kind: "error",
        text: err instanceof ApiError && err.status === 403 ? t("profile.messageError") : t("stories.replyError"),
      });
    } finally {
      setSending(false);
    }
  };

  const removeCurrent = async () => {
    if (!story || deleting) return;
    setDeleting(true); // pauses playback while the confirm is open
    if (!window.confirm(t("stories.deleteConfirm"))) {
      setDeleting(false);
      return;
    }
    try {
      await deleteStory(story.id);
      onDeleted(story.id);
      if (stories.length <= 1) onClose();
      else setIndex((current) => Math.min(current, stories.length - 2));
    } catch {
      setNotice({ kind: "error", text: t("stories.deleteError") });
    } finally {
      setDeleting(false);
    }
  };

  // Auto-hide the success notice.
  useEffect(() => {
    if (notice?.kind !== "ok") return;
    const timer = window.setTimeout(() => setNotice(null), 2500);
    return () => window.clearTimeout(timer);
  }, [notice]);

  if (!story) return null;

  const isVideo = story.media.type === "video";
  const hasMedia = Boolean(story.media.url) && !mediaFailed;
  const viewCount = preview?.total ?? story.viewCount ?? 0;

  return (
    <div
      ref={rootRef}
      tabIndex={-1}
      role="dialog"
      aria-modal="true"
      aria-label={t("stories.viewerLabel", { name: authorName })}
      className="fixed inset-0 z-50 flex items-center justify-center bg-black outline-none sm:bg-neutral-950/90 sm:backdrop-blur-md"
      onClick={(event) => {
        if (event.target === event.currentTarget) onClose();
      }}
    >
      {/* Desktop previous */}
      <button
        type="button"
        aria-label={t("stories.previous")}
        onClick={goPrev}
        className="mr-4 hidden h-11 w-11 shrink-0 items-center justify-center rounded-full bg-white/10 text-white transition hover:bg-white/20 lg:flex"
      >
        <ChevronLeft className="h-6 w-6 rtl:rotate-180" />
      </button>

      {/* Story canvas: full screen on phones, a portrait card on larger screens */}
      <div className="relative h-dvh w-full overflow-hidden bg-neutral-900 sm:aspect-[9/16] sm:h-[min(92dvh,860px)] sm:w-auto sm:rounded-[22px] sm:shadow-2xl sm:ring-1 sm:ring-white/10">
        {/* Ambient background: a blurred, dimmed copy of the media fills the
            canvas so landscape media never sits in dead black space. */}
        {hasMedia ? (
          isVideo ? (
            <canvas
              ref={bgCanvasRef}
              width={36}
              height={64}
              aria-hidden
              className="absolute inset-0 h-full w-full scale-125 opacity-80 blur-2xl brightness-[0.55]"
            />
          ) : (
            <Image
              src={story.media.url}
              alt=""
              aria-hidden
              fill
              unoptimized
              className="scale-125 object-cover opacity-80 blur-2xl brightness-[0.55]"
            />
          )
        ) : null}

        {/* Foreground media, aspect preserved (never stretched or cropped) */}
        {hasMedia ? (
          isVideo ? (
            <video
              key={story.id}
              ref={videoRef}
              src={story.media.url}
              playsInline
              muted={muted}
              autoPlay
              preload="auto"
              disablePictureInPicture
              controls={false}
              onLoadedData={() => setMediaReady(true)}
              onEnded={() => goNext()}
              onError={() => setMediaFailed(true)}
              className="absolute inset-0 h-full w-full object-contain"
            />
          ) : (
            <Image
              key={story.id}
              src={story.media.url}
              alt={t("stories.mediaAlt", { name: authorName })}
              fill
              unoptimized
              priority
              onLoad={() => setMediaReady(true)}
              onError={() => setMediaFailed(true)}
              className="object-contain"
            />
          )
        ) : (
          <div className="absolute inset-0 flex items-center justify-center px-8 text-center">
            <p className="text-sm text-white/80">{t("stories.mediaUnavailable")}</p>
          </div>
        )}

        {/* Gesture layer: tap left = previous, tap right = next, hold = pause */}
        <div
          aria-hidden
          className="absolute inset-0 touch-manipulation select-none"
          onPointerDown={onPointerDown}
          onPointerUp={() => endPress(true)}
          onPointerCancel={() => endPress(false)}
          onPointerLeave={() => endPress(false)}
          onContextMenu={(event) => event.preventDefault()}
        />

        {/* Legibility scrims */}
        <div className="pointer-events-none absolute inset-x-0 top-0 h-32 bg-gradient-to-b from-black/60 to-transparent" />
        <div className="pointer-events-none absolute inset-x-0 bottom-0 h-40 bg-gradient-to-t from-black/70 to-transparent" />

        {/* Top: progress + author + controls */}
        <div className="absolute inset-x-0 top-0 px-3 pt-[max(0.625rem,env(safe-area-inset-top))] text-white">
          <div
            className="flex gap-1"
            role="progressbar"
            aria-label={t("stories.progress", { current: index + 1, total: stories.length })}
            aria-valuemin={0}
            aria-valuemax={stories.length}
            aria-valuenow={index + progress}
          >
            {stories.map((item, i) => (
              <span key={item.id} className="h-[3px] flex-1 overflow-hidden rounded-full bg-white/35">
                <span
                  className="block h-full rounded-full bg-white"
                  style={{ width: `${i < index ? 100 : i === index ? progress * 100 : 0}%` }}
                />
              </span>
            ))}
          </div>
          <div className="mt-2.5 flex items-center gap-2.5">
            <Image
              src={group.author.avatarUrl ?? FALLBACK_AVATAR}
              alt=""
              width={36}
              height={36}
              unoptimized={Boolean(group.author.avatarUrl)}
              className="h-9 w-9 shrink-0 rounded-full object-cover ring-2 ring-white/70"
            />
            <div className="min-w-0 flex-1 leading-tight">
              <div className="flex min-w-0 items-baseline gap-1.5">
                <span className="truncate text-sm font-semibold drop-shadow">{authorName}</span>
                <span className="shrink-0 text-xs text-white/75 drop-shadow">
                  {formatTimeAgo(story.createdAt, locale)}
                </span>
              </div>
              <div className="truncate text-xs text-white/70 drop-shadow">@{group.author.username}</div>
            </div>
            {held ? <Pause className="h-4 w-4 shrink-0 text-white/80" aria-hidden /> : null}
            {isVideo && hasMedia ? (
              <button
                type="button"
                aria-label={muted ? t("stories.unmuteSound") : t("stories.muteSound")}
                aria-pressed={!muted}
                onClick={() => setMuted((m) => !m)}
                className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full text-white transition hover:bg-white/15"
              >
                {muted ? <VolumeX className="h-5 w-5" /> : <Volume2 className="h-5 w-5" />}
              </button>
            ) : null}
            <button
              type="button"
              aria-label={t("profile.close")}
              onClick={onClose}
              className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full text-white transition hover:bg-white/15"
            >
              <X className="h-6 w-6" />
            </button>
          </div>
        </div>

        {/* Keyboard / screen-reader navigation (visible when focused) */}
        <button
          type="button"
          onClick={goPrev}
          className="sr-only focus:not-sr-only focus:absolute focus:start-3 focus:top-1/2 focus:rounded-full focus:bg-black/60 focus:px-3 focus:py-2 focus:text-sm focus:text-white"
        >
          {t("stories.previous")}
        </button>
        <button
          type="button"
          onClick={goNext}
          className="sr-only focus:not-sr-only focus:absolute focus:end-3 focus:top-1/2 focus:rounded-full focus:bg-black/60 focus:px-3 focus:py-2 focus:text-sm focus:text-white"
        >
          {t("stories.next")}
        </button>

        {/* Bottom bar */}
        <div className="absolute inset-x-0 bottom-0 px-3 pb-[max(0.75rem,env(safe-area-inset-bottom))] pt-2 text-white">
          {notice ? (
            <p
              role={notice.kind === "error" ? "alert" : "status"}
              className={`mb-2 break-words rounded-xl px-3 py-2 text-center text-sm ${
                notice.kind === "error" ? "bg-red-600/90" : "bg-black/60"
              }`}
            >
              {notice.text}
            </p>
          ) : null}

          {isOwnStory ? (
            <div className="flex items-center gap-2">
              <button
                type="button"
                onClick={() => setSheetOpen(true)}
                className="flex min-h-11 min-w-0 flex-1 items-center gap-2.5 rounded-full px-2 text-start transition hover:bg-white/10"
              >
                {preview && preview.items.length > 0 ? (
                  <span className="flex shrink-0 -space-x-2 rtl:space-x-reverse" aria-hidden>
                    {preview.items.map((v) => (
                      <Image
                        key={v.user.id}
                        src={v.user.avatarUrl ?? FALLBACK_AVATAR}
                        alt=""
                        width={28}
                        height={28}
                        unoptimized={Boolean(v.user.avatarUrl)}
                        className="h-7 w-7 rounded-full object-cover ring-2 ring-neutral-900"
                      />
                    ))}
                  </span>
                ) : (
                  <Eye className="h-5 w-5 shrink-0" aria-hidden />
                )}
                <span className="truncate text-sm font-medium">{t("stories.viewedBy", { count: viewCount })}</span>
              </button>
              <button
                type="button"
                aria-label={t("post.delete")}
                onClick={() => void removeCurrent()}
                disabled={deleting}
                className="flex h-11 w-11 shrink-0 items-center justify-center rounded-full transition hover:bg-white/15 disabled:opacity-50"
              >
                <Trash2 className="h-5 w-5" />
              </button>
            </div>
          ) : (
            <form onSubmit={sendReply} className="flex items-center gap-2">
              <input
                value={reply}
                onChange={(event) => setReply(event.target.value)}
                onFocus={() => setReplyFocused(true)}
                onBlur={() => setReplyFocused(false)}
                maxLength={2000}
                enterKeyHint="send"
                placeholder={t("stories.replyPlaceholder", { name: authorName })}
                aria-label={t("stories.replyPlaceholder", { name: authorName })}
                disabled={sending}
                className="h-11 min-w-0 flex-1 rounded-full border border-white/40 bg-black/25 px-4 text-base text-white placeholder:text-white/70 focus:border-white focus:outline-none sm:text-sm"
              />
              {reply.trim() ? (
                <button
                  type="submit"
                  aria-label={t("stories.sendReply")}
                  disabled={sending}
                  className="flex h-11 w-11 shrink-0 items-center justify-center rounded-full bg-primary text-white transition hover:bg-primary-hover disabled:opacity-50"
                >
                  <Send className="h-5 w-5 rtl:-scale-x-100" />
                </button>
              ) : (
                <button
                  type="button"
                  aria-label={liked ? t("stories.unlike") : t("stories.like")}
                  aria-pressed={liked}
                  onClick={() => void toggleLike()}
                  disabled={liking}
                  className="flex h-11 w-11 shrink-0 items-center justify-center rounded-full transition hover:bg-white/15"
                >
                  <Heart className={`h-7 w-7 transition ${liked ? "fill-red-500 text-red-500" : "text-white"}`} />
                </button>
              )}
            </form>
          )}
        </div>

        {sheetOpen && isOwnStory ? (
          <StoryViewersSheet storyId={story.id} onClose={() => setSheetOpen(false)} />
        ) : null}
      </div>

      {/* Desktop next */}
      <button
        type="button"
        aria-label={t("stories.next")}
        onClick={goNext}
        className="ml-4 hidden h-11 w-11 shrink-0 items-center justify-center rounded-full bg-white/10 text-white transition hover:bg-white/20 lg:flex"
      >
        <ChevronRight className="h-6 w-6 rtl:rotate-180" />
      </button>
    </div>
  );
}

/** Owner-only list of who viewed the story, with a heart for likes. */
function StoryViewersSheet({ storyId, onClose }: { storyId: string; onClose: () => void }) {
  const { t, locale } = useLanguage();
  const [items, setItems] = useState<StoryViewerEntry[]>([]);
  const [total, setTotal] = useState(0);
  const [cursor, setCursor] = useState("");
  const [status, setStatus] = useState<"loading" | "ready" | "error">("loading");
  const [loadingMore, setLoadingMore] = useState(false);

  const load = useCallback(
    (signal?: AbortSignal) => {
      setStatus("loading");
      getStoryViewers(storyId, { limit: 30 }, signal)
        .then((page) => {
          setItems(page.items);
          setTotal(page.total);
          setCursor(page.nextCursor);
          setStatus("ready");
        })
        .catch((err) => {
          if (err instanceof DOMException && err.name === "AbortError") return;
          setStatus("error");
        });
    },
    [storyId],
  );

  useEffect(() => {
    const controller = new AbortController();
    load(controller.signal);
    return () => controller.abort();
  }, [load]);

  const loadMore = async () => {
    if (!cursor || loadingMore) return;
    setLoadingMore(true);
    try {
      const page = await getStoryViewers(storyId, { limit: 30, cursor });
      setItems((prev) => [...prev, ...page.items]);
      setCursor(page.nextCursor);
      setTotal(page.total);
    } catch {
      // Keep what is shown; the button stays available.
    } finally {
      setLoadingMore(false);
    }
  };

  return (
    <div className="absolute inset-0 z-10 flex flex-col justify-end bg-black/40" onClick={onClose}>
      <section
        role="dialog"
        aria-modal="true"
        aria-label={t("stories.viewers")}
        onClick={(event) => event.stopPropagation()}
        className="flex max-h-[70%] flex-col rounded-t-3xl bg-surface text-foreground shadow-2xl"
      >
        <header className="flex items-center justify-between gap-3 border-b border-border px-4 py-3">
          <h2 className="min-w-0 truncate text-base font-semibold">
            {t("stories.viewers")}
            {status === "ready" ? <span className="ms-1.5 text-sm font-normal text-muted">{total}</span> : null}
          </h2>
          <button
            type="button"
            aria-label={t("profile.close")}
            onClick={onClose}
            className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full text-muted hover:bg-background"
          >
            <X className="h-5 w-5" />
          </button>
        </header>
        <div className="min-h-0 flex-1 overflow-y-auto px-2 pb-[max(0.75rem,env(safe-area-inset-bottom))] pt-1">
          {status === "loading" ? (
            <ul className="space-y-1 px-2 py-2" aria-hidden>
              {[0, 1, 2].map((i) => (
                <li key={i} className="flex items-center gap-3 py-2">
                  <span className="h-10 w-10 animate-pulse rounded-full bg-background" />
                  <span className="h-3 w-32 animate-pulse rounded bg-background" />
                </li>
              ))}
            </ul>
          ) : status === "error" ? (
            <div className="px-4 py-8 text-center">
              <p className="text-sm text-muted">{t("stories.viewersError")}</p>
              <button
                type="button"
                onClick={() => load()}
                className="mt-3 h-10 rounded-full bg-primary px-4 text-sm font-medium text-white"
              >
                {t("search.tryAgain")}
              </button>
            </div>
          ) : items.length === 0 ? (
            <p className="px-4 py-10 text-center text-sm text-muted">{t("stories.noViews")}</p>
          ) : (
            <>
              <ul>
                {items.map((v) => (
                  <li key={v.user.id} className="flex items-center gap-3 rounded-xl px-2 py-2">
                    <Image
                      src={v.user.avatarUrl ?? FALLBACK_AVATAR}
                      alt=""
                      width={40}
                      height={40}
                      unoptimized={Boolean(v.user.avatarUrl)}
                      className="h-10 w-10 shrink-0 rounded-full object-cover"
                    />
                    <div className="min-w-0 flex-1 leading-tight">
                      <div className="truncate text-sm font-semibold">{v.user.displayName || v.user.username}</div>
                      <div className="truncate text-xs text-muted">
                        @{v.user.username} · {formatTimeAgo(v.viewedAt, locale)}
                      </div>
                    </div>
                    {v.liked ? (
                      <Heart className="h-5 w-5 shrink-0 fill-red-500 text-red-500" aria-label={t("stories.liked")} />
                    ) : null}
                  </li>
                ))}
              </ul>
              {cursor ? (
                <button
                  type="button"
                  onClick={() => void loadMore()}
                  disabled={loadingMore}
                  className="mx-auto my-2 block h-10 rounded-full px-4 text-sm font-medium text-primary hover:bg-background disabled:opacity-50"
                >
                  {t("feed.loadMore")}
                </button>
              ) : null}
            </>
          )}
        </div>
      </section>
    </div>
  );
}
