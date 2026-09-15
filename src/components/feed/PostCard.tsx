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
  Pencil,
  Send,
  Share2,
  Trash2,
  UserPlus,
} from "lucide-react";
import type { Post } from "@/types/post";
import { PostMedia } from "@/components/feed/PostMedia";
import { formatCount, formatTimeAgo } from "@/lib/format";
import {
  ApiError,
  createComment,
  deletePost,
  getComments,
  likePost,
  savePost,
  unlikePost,
  unsavePost,
  updatePost,
} from "@/lib/api";
import { mapApiPost } from "@/lib/map-post";
import { useAuth } from "@/lib/auth-context";
import { useFeed } from "@/lib/feed-context";

type LocalComment = {
  id: string;
  author: string;
  content: string;
  createdAt: string;
  translatedContent: string | null;
  sourceLanguage: string | null;
  targetLanguage: string | null;
};

export function PostCard({
  post,
  onUnsave,
}: {
  post: Post;
  /**
   * Called after the viewer successfully unsaves this post. Used by the
   * bookmarks page to drop the card immediately; unset elsewhere (e.g. the
   * home feed), where unsaving only toggles the bookmark icon.
   */
  onUnsave?: (id: string) => void;
}) {
  const { author, createdAt, location, content, hashtags, media, stats } = post;

  const { user } = useAuth();
  const { replacePost, removePost } = useFeed();
  const isOwn = !!user?.id && author.id === user.id;

  // Own-post menu + edit + delete state.
  const [menuOpen, setMenuOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);
  const [editing, setEditing] = useState(false);
  const [editText, setEditText] = useState(content);
  const [savingEdit, setSavingEdit] = useState(false);
  const [editError, setEditError] = useState<string | null>(null);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  const [liked, setLiked] = useState(post.viewerState.liked);
  const [saved, setSaved] = useState(post.viewerState.saved);
  const [likes, setLikes] = useState(stats.likes);

  // Translation display (shows what the backend provided; never translates here).
  const [showOriginal, setShowOriginal] = useState(false);
  const translated = post.translatedContent;
  const hasTranslation =
    translated != null &&
    translated.trim().length > 0 &&
    translated !== content;
  const primaryText =
    hasTranslation && !showOriginal ? (translated as string) : content;
  const langHint =
    hasTranslation && post.sourceLanguage && post.targetLanguage
      ? `${post.sourceLanguage.toUpperCase()} → ${post.targetLanguage.toUpperCase()}`
      : null;

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

  // Close the own-post menu on outside click.
  useEffect(() => {
    if (!menuOpen) return;
    const onClick = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        setMenuOpen(false);
      }
    };
    document.addEventListener("mousedown", onClick);
    return () => document.removeEventListener("mousedown", onClick);
  }, [menuOpen]);

  const startEdit = () => {
    setMenuOpen(false);
    setEditError(null);
    setEditText(content); // edit the original text, never a translation
    setEditing(true);
  };

  const cancelEdit = () => {
    setEditing(false);
    setEditError(null);
  };

  const saveEdit = async () => {
    const next = editText.trim();
    if (!next || savingEdit) return;
    if (next === content.trim()) {
      cancelEdit(); // no change
      return;
    }
    setSavingEdit(true);
    setEditError(null);
    try {
      const updated = await updatePost(post.id, { content: next });
      replacePost(mapApiPost(updated)); // media preserved by the backend
      setEditing(false);
    } catch (err) {
      setEditError(
        err instanceof ApiError && err.status === 401
          ? "Please sign in to edit."
          : "Couldn’t save changes. Please try again.",
      );
    } finally {
      setSavingEdit(false);
    }
  };

  const confirmDeletePost = async () => {
    if (deleting) return;
    setDeleting(true);
    setDeleteError(null);
    try {
      await deletePost(post.id);
      removePost(post.id); // drop from the feed immediately
    } catch (err) {
      setDeleteError(
        err instanceof ApiError && err.status === 401
          ? "Please sign in to delete."
          : "Couldn’t delete post. Please try again.",
      );
      setDeleting(false); // keep the dialog open to retry (card still mounted)
    }
  };

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
        onUnsave?.(post.id); // let the bookmarks page drop this card
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
              translatedContent: c.translatedContent,
              sourceLanguage: c.sourceLanguage,
              targetLanguage: c.targetLanguage,
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
          translatedContent: created.translatedContent,
          sourceLanguage: created.sourceLanguage,
          targetLanguage: created.targetLanguage,
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
        {isOwn ? (
          <div className="relative" ref={menuRef}>
            <button
              type="button"
              aria-label="Post options"
              aria-haspopup="menu"
              aria-expanded={menuOpen}
              onClick={() => setMenuOpen((v) => !v)}
              className={`flex h-8 w-8 items-center justify-center rounded-full transition-colors hover:bg-background ${
                menuOpen ? "text-foreground" : "text-muted-soft"
              }`}
            >
              <MoreHorizontal className="h-5 w-5" />
            </button>
            {menuOpen ? (
              <div
                role="menu"
                className="absolute right-0 top-full z-20 mt-2 w-40 overflow-hidden rounded-xl border border-border bg-surface p-1 shadow-lg"
              >
                <button
                  type="button"
                  role="menuitem"
                  onClick={startEdit}
                  className="flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-sm text-foreground transition-colors hover:bg-background"
                >
                  <Pencil className="h-4 w-4 text-muted" />
                  Edit
                </button>
                <button
                  type="button"
                  role="menuitem"
                  onClick={() => {
                    setMenuOpen(false);
                    setDeleteError(null);
                    setConfirmDelete(true);
                  }}
                  className="flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-sm text-red-600 transition-colors hover:bg-red-50"
                >
                  <Trash2 className="h-4 w-4" />
                  Delete
                </button>
              </div>
            ) : null}
          </div>
        ) : (
          <button
            type="button"
            aria-label="More"
            className="flex h-8 w-8 items-center justify-center rounded-full text-muted-soft transition-colors hover:bg-background"
          >
            <MoreHorizontal className="h-5 w-5" />
          </button>
        )}
      </div>

      {/* Text (or inline editor for own posts) */}
      {editing ? (
        <div className="mt-3">
          <textarea
            value={editText}
            onChange={(e) => setEditText(e.target.value)}
            rows={3}
            autoFocus
            className="w-full resize-none rounded-xl border border-border bg-background px-3 py-2 text-sm leading-relaxed text-foreground focus:outline-none focus:ring-2 focus:ring-primary"
          />
          {editError ? (
            <p className="mt-1 text-xs text-red-500" role="alert">
              {editError}
            </p>
          ) : null}
          <div className="mt-2 flex items-center justify-end gap-2">
            <button
              type="button"
              onClick={cancelEdit}
              disabled={savingEdit}
              className="rounded-full px-4 py-1.5 text-sm font-medium text-muted transition-colors hover:bg-background disabled:opacity-50"
            >
              Cancel
            </button>
            <button
              type="button"
              onClick={saveEdit}
              disabled={savingEdit || editText.trim().length === 0}
              className="rounded-full bg-primary px-5 py-1.5 text-sm font-medium text-white transition-colors hover:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:bg-primary"
            >
              {savingEdit ? "Saving…" : "Save"}
            </button>
          </div>
        </div>
      ) : (
        <p className="mt-3 text-sm leading-relaxed text-foreground">
          {primaryText}
          {hashtags && hashtags.length > 0 ? (
            <>
              {" "}
              <span className="text-primary">{hashtags.join(" ")}</span>
            </>
          ) : null}
        </p>
      )}

      {hasTranslation && !editing ? (
        <div className="mt-1 flex items-center gap-2 text-xs text-muted-soft">
          {langHint && !showOriginal ? <span>{langHint}</span> : null}
          <button
            type="button"
            onClick={() => setShowOriginal((v) => !v)}
            className="text-primary transition-opacity hover:opacity-80"
          >
            {showOriginal ? "Show translation" : "Show original"}
          </button>
        </div>
      ) : null}

      {/* Media (single natural-ratio image, or a carousel for 2+) */}
      <PostMedia media={media} />

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
                <CommentItem key={comment.id} comment={comment} />
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

      {/* Delete confirmation */}
      {confirmDelete ? (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4"
          role="dialog"
          aria-modal="true"
          aria-label="Delete post"
          onClick={() => {
            if (!deleting) setConfirmDelete(false);
          }}
        >
          <div
            className="w-full max-w-sm rounded-2xl border border-border bg-surface p-5 shadow-xl"
            onClick={(e) => e.stopPropagation()}
          >
            <h2 className="text-base font-semibold text-foreground">
              Delete post?
            </h2>
            <p className="mt-1 text-sm text-muted">
              This can’t be undone. The post and its media will be removed.
            </p>
            {deleteError ? (
              <p className="mt-2 text-xs text-red-500" role="alert">
                {deleteError}
              </p>
            ) : null}
            <div className="mt-4 flex items-center justify-end gap-2">
              <button
                type="button"
                onClick={() => setConfirmDelete(false)}
                disabled={deleting}
                className="rounded-full px-4 py-1.5 text-sm font-medium text-muted transition-colors hover:bg-background disabled:opacity-50"
              >
                Cancel
              </button>
              <button
                type="button"
                onClick={confirmDeletePost}
                disabled={deleting}
                className="rounded-full bg-red-600 px-5 py-1.5 text-sm font-medium text-white transition-colors hover:bg-red-700 disabled:cursor-not-allowed disabled:opacity-50"
              >
                {deleting ? "Deleting…" : "Delete"}
              </button>
            </div>
          </div>
        </div>
      ) : null}
    </article>
  );
}

// CommentItem renders one comment. When a translation is present it shows the
// translated text by default with a per-comment "Show original" toggle. It never
// translates in the browser — only displays what the backend provided.
function CommentItem({ comment }: { comment: LocalComment }) {
  const [showOriginal, setShowOriginal] = useState(false);

  const translated = comment.translatedContent;
  const hasTranslation =
    translated != null &&
    translated.trim().length > 0 &&
    translated !== comment.content;
  const primary =
    hasTranslation && !showOriginal ? (translated as string) : comment.content;
  const langHint =
    hasTranslation && comment.sourceLanguage && comment.targetLanguage
      ? `${comment.sourceLanguage.toUpperCase()} → ${comment.targetLanguage.toUpperCase()}`
      : null;

  return (
    <li className="flex gap-2">
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
        <div className="text-sm text-foreground">{primary}</div>
        {hasTranslation ? (
          <div className="mt-0.5 flex items-center gap-2 text-[10px] text-muted-soft">
            {langHint && !showOriginal ? <span>{langHint}</span> : null}
            <button
              type="button"
              onClick={() => setShowOriginal((v) => !v)}
              className="text-primary transition-opacity hover:opacity-80"
            >
              {showOriginal ? "Show translation" : "Show original"}
            </button>
          </div>
        ) : null}
      </div>
    </li>
  );
}
