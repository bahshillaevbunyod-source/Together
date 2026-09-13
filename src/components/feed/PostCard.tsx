"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import Image from "next/image";
import {
  Bookmark,
  Check,
  Heart,
  Link2,
  MessageCircle,
  MoreHorizontal,
  Send,
  Share2,
  UserPlus,
} from "lucide-react";
import type { Post } from "@/types/post";
import { formatCount, formatTimeAgo } from "@/lib/format";
import {
  createComment,
  getComments,
  likePost,
  savePost,
  unlikePost,
  unsavePost,
} from "@/lib/api";

type LocalComment = {
  id: string;
  author: string;
  content: string;
  createdAt: string;
};

export function PostCard({ post }: { post: Post }) {
  const { author, createdAt, location, content, hashtags, media, stats } = post;
  const cover = media[0];

  const [liked, setLiked] = useState(post.viewerState.liked);
  const [saved, setSaved] = useState(post.viewerState.saved);
  const [likes, setLikes] = useState(stats.likes);

  const [showComments, setShowComments] = useState(false);
  const [comments, setComments] = useState<LocalComment[]>([]);
  const [commentCount, setCommentCount] = useState(stats.comments);
  const [commentText, setCommentText] = useState("");
  const [commentsStatus, setCommentsStatus] = useState<
    "idle" | "loading" | "ready" | "error"
  >("idle");
  const [submitting, setSubmitting] = useState(false);

  const [shareOpen, setShareOpen] = useState(false);
  const [copied, setCopied] = useState(false);
  const shareRef = useRef<HTMLDivElement>(null);

  // Guards against duplicate requests from rapid clicks.
  const likePending = useRef(false);
  const savePending = useRef(false);

  // Close the share popover on outside click.
  useEffect(() => {
    if (!shareOpen) return;
    const onClick = (e: MouseEvent) => {
      if (shareRef.current && !shareRef.current.contains(e.target as Node)) {
        setShareOpen(false);
      }
    };
    document.addEventListener("mousedown", onClick);
    return () => document.removeEventListener("mousedown", onClick);
  }, [shareOpen]);

  const handleCopyLink = async () => {
    const url = `${window.location.origin}/post/${post.id}`;
    try {
      await navigator.clipboard.writeText(url);
    } catch {
      // Clipboard may be unavailable; still show the success hint.
    }
    setCopied(true);
    window.setTimeout(() => {
      setCopied(false);
      setShareOpen(false);
    }, 1200);
  };

  const toggleLike = async () => {
    if (likePending.current) return;
    likePending.current = true;

    const next = !liked;
    const prevLiked = liked;
    const prevLikes = likes;

    // Optimistic update.
    setLiked(next);
    setLikes((c) => c + (next ? 1 : -1));

    try {
      const state = next ? await likePost(post.id) : await unlikePost(post.id);
      // Reconcile with the authoritative server state.
      setLiked(state.liked);
      setLikes(state.likesCount);
    } catch {
      // Roll back on failure.
      setLiked(prevLiked);
      setLikes(prevLikes);
    } finally {
      likePending.current = false;
    }
  };

  const toggleSave = async () => {
    if (savePending.current) return;
    savePending.current = true;

    const next = !saved;
    const prevSaved = saved;

    // Optimistic update.
    setSaved(next);

    try {
      if (next) {
        await savePost(post.id);
      } else {
        await unsavePost(post.id);
      }
    } catch {
      // Roll back on failure.
      setSaved(prevSaved);
    } finally {
      savePending.current = false;
    }
  };

  const canSend = commentText.trim().length > 0 && !submitting;

  const loadComments = useCallback(
    (signal?: AbortSignal) => {
      setCommentsStatus("loading");
      getComments(post.id, {}, signal)
        .then((page) => {
          setComments(
            page.items.map((c) => ({
              id: c.id,
              author: c.author.displayName,
              content: c.content,
              createdAt: c.createdAt,
            })),
          );
          setCommentsStatus("ready");
        })
        .catch((err) => {
          if (err instanceof DOMException && err.name === "AbortError") return;
          setCommentsStatus("error");
        });
    },
    [post.id],
  );

  // Load comments the first time the section is opened.
  useEffect(() => {
    if (!showComments || commentsStatus !== "idle") return;
    const controller = new AbortController();
    loadComments(controller.signal);
    return () => controller.abort();
  }, [showComments, commentsStatus, loadComments]);

  const submitComment = async (e: React.FormEvent) => {
    e.preventDefault();
    const text = commentText.trim();
    if (!text || submitting) return;

    setSubmitting(true);
    try {
      const created = await createComment(post.id, text);
      setComments((prev) => [
        ...prev,
        {
          id: created.id,
          author: created.author.displayName,
          content: created.content,
          createdAt: created.createdAt,
        },
      ]);
      setCommentCount((c) => c + 1);
      setCommentText("");
    } catch {
      // Keep the text so the user can retry.
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <article className="rounded-2xl border border-border bg-surface p-4 shadow-sm">
      {/* Author */}
      <div className="flex items-start gap-3">
        <Image
          src={author.avatar}
          alt={author.name}
          width={44}
          height={44}
          className="h-11 w-11 rounded-full object-cover"
        />
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-1.5">
            <span className="font-semibold text-foreground">{author.name}</span>
            {author.flag ? <span aria-hidden>{author.flag}</span> : null}
          </div>
          <div className="text-xs text-muted">
            {formatTimeAgo(createdAt)} · {location}
          </div>
        </div>
        <button
          type="button"
          aria-label="More"
          className="flex h-8 w-8 items-center justify-center rounded-full text-muted-soft transition-colors hover:bg-background"
        >
          <MoreHorizontal className="h-5 w-5" />
        </button>
      </div>

      {/* Text */}
      <p className="mt-3 text-sm leading-relaxed text-foreground">
        {content}
        {hashtags && hashtags.length > 0 ? (
          <>
            {" "}
            <span className="text-primary">{hashtags.join(" ")}</span>
          </>
        ) : null}
      </p>

      {/* Photo */}
      {cover ? (
        <div className="relative mt-3 aspect-[4/3] overflow-hidden rounded-xl border border-border">
          <Image
            src={cover.src}
            alt={cover.alt}
            fill
            sizes="(max-width: 1024px) 100vw, 620px"
            className="object-cover"
          />
        </div>
      ) : null}

      {/* Actions */}
      <div className="mt-3 flex items-center gap-6 text-sm text-muted">
        <button
          type="button"
          onClick={toggleLike}
          aria-pressed={liked}
          className={`flex items-center gap-2 transition-colors ${
            liked ? "text-primary" : "hover:text-foreground"
          }`}
        >
          <Heart className={`h-5 w-5 ${liked ? "fill-current" : ""}`} />
          {formatCount(likes)}
        </button>
        <button
          type="button"
          onClick={() => setShowComments((prev) => !prev)}
          aria-expanded={showComments}
          className={`flex items-center gap-2 transition-colors ${
            showComments ? "text-primary" : "hover:text-foreground"
          }`}
        >
          <MessageCircle className="h-5 w-5" />
          {formatCount(commentCount)}
        </button>
        <div className="relative" ref={shareRef}>
          <button
            type="button"
            onClick={() => setShareOpen((prev) => !prev)}
            aria-expanded={shareOpen}
            aria-haspopup="menu"
            className={`flex items-center gap-2 transition-colors ${
              shareOpen ? "text-primary" : "hover:text-foreground"
            }`}
          >
            <Share2 className="h-5 w-5" />
            {formatCount(stats.shares)}
          </button>

          {shareOpen ? (
            <div
              role="menu"
              className="absolute left-0 top-full z-20 mt-2 w-52 overflow-hidden rounded-xl border border-border bg-surface p-1 shadow-lg"
            >
              <button
                type="button"
                role="menuitem"
                onClick={handleCopyLink}
                className="flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-sm text-foreground transition-colors hover:bg-background"
              >
                {copied ? (
                  <Check className="h-4 w-4 text-emerald-500" />
                ) : (
                  <Link2 className="h-4 w-4 text-muted" />
                )}
                {copied ? "Copied" : "Copy link"}
              </button>
              <button
                type="button"
                role="menuitem"
                onClick={() => setShareOpen(false)}
                className="flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-sm text-foreground transition-colors hover:bg-background"
              >
                <Send className="h-4 w-4 text-muted" />
                Send in message
              </button>
              <button
                type="button"
                role="menuitem"
                onClick={() => setShareOpen(false)}
                className="flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-sm text-foreground transition-colors hover:bg-background"
              >
                <UserPlus className="h-4 w-4 text-muted" />
                Share to profile
              </button>
            </div>
          ) : null}
        </div>
        <button
          type="button"
          onClick={toggleSave}
          aria-pressed={saved}
          aria-label="Save"
          className={`ml-auto flex h-8 w-8 items-center justify-center rounded-full transition-colors ${
            saved ? "text-primary" : "hover:text-foreground"
          }`}
        >
          <Bookmark className={`h-5 w-5 ${saved ? "fill-current" : ""}`} />
        </button>
      </div>

      {/* Comments */}
      {showComments ? (
        <div className="mt-3 border-t border-border pt-3">
          {commentsStatus === "loading" ? (
            <p className="mb-3 text-sm text-muted">Loading comments…</p>
          ) : null}

          {commentsStatus === "error" ? (
            <div className="mb-3 flex items-center gap-3">
              <p className="text-sm text-muted">Couldn’t load comments.</p>
              <button
                type="button"
                onClick={() => loadComments()}
                className="text-sm text-primary hover:underline"
              >
                Try again
              </button>
            </div>
          ) : null}

          {commentsStatus === "ready" && comments.length === 0 ? (
            <p className="mb-3 text-sm text-muted-soft">No comments yet.</p>
          ) : null}

          {comments.length > 0 ? (
            <ul className="mb-3 flex flex-col gap-2">
              {comments.map((comment) => (
                <li key={comment.id} className="flex gap-2">
                  <span className="h-8 w-8 shrink-0 rounded-full bg-background" />
                  <div className="rounded-2xl bg-background px-3 py-2">
                    <div className="flex items-center gap-1.5">
                      <span className="text-xs font-semibold text-foreground">
                        {comment.author}
                      </span>
                      <span className="text-xs text-muted-soft">
                        {formatTimeAgo(comment.createdAt)}
                      </span>
                    </div>
                    <div className="text-sm text-foreground">
                      {comment.content}
                    </div>
                  </div>
                </li>
              ))}
            </ul>
          ) : null}

          <form onSubmit={submitComment} className="flex items-center gap-2">
            <span className="h-8 w-8 shrink-0 rounded-full bg-background" />
            <input
              type="text"
              value={commentText}
              onChange={(e) => setCommentText(e.target.value)}
              placeholder="Write a comment…"
              className="h-9 flex-1 rounded-full bg-background px-4 text-sm text-foreground placeholder:text-muted-soft focus:outline-none"
            />
            <button
              type="submit"
              disabled={!canSend}
              aria-label="Send comment"
              className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-primary text-white transition-colors hover:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:bg-primary"
            >
              <Send className="h-4 w-4" />
            </button>
          </form>
        </div>
      ) : null}
    </article>
  );
}
