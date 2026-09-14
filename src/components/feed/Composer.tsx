"use client";

import { useEffect, useRef, useState } from "react";
import {
  AlertCircle,
  BarChart3,
  ChevronDown,
  Globe,
  Image as ImageIcon,
  Mic,
  Smile,
  Video,
  X,
  type LucideIcon,
} from "lucide-react";
import {
  ApiError,
  confirmMediaUpload,
  createPost,
  requestMediaUploadUrl,
  uploadFileToPresignedUrl,
} from "@/lib/api";
import { mapApiPost } from "@/lib/map-post";
import { useFeed } from "@/lib/feed-context";

// Client-side image constraints, mirroring the backend media policy. The picker
// validates against these directly (never trusting the input `accept` alone).
const ALLOWED_IMAGE_TYPES = ["image/jpeg", "image/png", "image/webp"];
const MAX_IMAGE_BYTES = 15 * 1024 * 1024; // 15 MiB
const MAX_IMAGES = 8; // product limit: at most 8 images per post

type Action = {
  label: string;
  icon: LucideIcon;
  color: string;
};

const actions: Action[] = [
  { label: "Photo", icon: ImageIcon, color: "text-emerald-500" },
  { label: "Video", icon: Video, color: "text-rose-500" },
  { label: "Voice", icon: Mic, color: "text-violet-500" },
  { label: "Poll", icon: BarChart3, color: "text-sky-500" },
  { label: "Feeling", icon: Smile, color: "text-amber-500" },
];

type Visibility = "everyone" | "friends" | "private";

const visibilityLabels: Record<Visibility, string> = {
  everyone: "Everyone",
  friends: "Friends",
  private: "Only me",
};

// Map the composer's audience choice to the backend's visibility values.
const backendVisibility: Record<Visibility, string> = {
  everyone: "public",
  friends: "followers",
  private: "private",
};

// One selected image. `storageKey` is set only after a successful R2 PUT, and
// `confirmed` only after a successful confirm — so a failed submit can be retried
// without re-uploading or re-confirming completed images.
type SelectedImage = {
  id: string;
  file: File;
  previewUrl: string;
  storageKey: string | null;
  confirmed: boolean;
};

export function Composer() {
  const { prependPost } = useFeed();
  const [content, setContent] = useState("");
  const [visibility] = useState<Visibility>("everyone");
  const [images, setImages] = useState<SelectedImage[]>([]);
  const [posting, setPosting] = useState(false);
  const [uploadingIndex, setUploadingIndex] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);

  const fileInputRef = useRef<HTMLInputElement>(null);
  const errorTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  // Mirror of `images` for unmount cleanup (avoids stale closure over state).
  const imagesRef = useRef<SelectedImage[]>([]);
  useEffect(() => {
    imagesRef.current = images;
  }, [images]);

  const clearErrorTimer = () => {
    if (errorTimerRef.current) {
      clearTimeout(errorTimerRef.current);
      errorTimerRef.current = null;
    }
  };

  // Show a contextual error and auto-dismiss it after a few seconds. Only the
  // latest error's timer is ever live, so no stale timer can clear a newer one.
  const showError = (message: string) => {
    clearErrorTimer();
    setError(message);
    errorTimerRef.current = setTimeout(() => {
      setError(null);
      errorTimerRef.current = null;
    }, 4500);
  };

  const clearError = () => {
    clearErrorTimer();
    setError(null);
  };

  // Revoke every remaining preview object URL on unmount.
  useEffect(
    () => () => {
      clearErrorTimer();
      imagesRef.current.forEach((im) => URL.revokeObjectURL(im.previewUrl));
    },
    [],
  );

  const openImagePicker = () => {
    if (posting) return; // don't start a new selection mid-upload
    fileInputRef.current?.click();
  };

  const onFilesSelected = (e: React.ChangeEvent<HTMLInputElement>) => {
    const files = Array.from(e.target.files ?? []);
    // Allow re-selecting the same file(s) later by clearing the input value.
    e.target.value = "";
    if (posting || files.length === 0) return;

    const remaining = MAX_IMAGES - images.length;
    if (remaining <= 0) {
      showError(`You can add up to ${MAX_IMAGES} photos.`);
      return;
    }

    const valid: File[] = [];
    let hadInvalid = false;
    for (const f of files) {
      if (ALLOWED_IMAGE_TYPES.includes(f.type) && f.size <= MAX_IMAGE_BYTES) {
        valid.push(f);
      } else {
        hadInvalid = true;
      }
    }

    // Keep only what fits under the 8-image cap; the rest are dropped.
    const accepted = valid.slice(0, remaining);
    const truncated = valid.length > remaining;

    if (accepted.length > 0) {
      const additions = accepted.map((file) => ({
        id: crypto.randomUUID(),
        file,
        previewUrl: URL.createObjectURL(file),
        storageKey: null,
        confirmed: false,
      }));
      setImages((prev) => [...prev, ...additions]);
    }

    // Feedback priority: invalid types/sizes first, then the count cap.
    if (hadInvalid) {
      showError("Use JPG, PNG, or WebP images up to 15 MB each.");
    } else if (truncated) {
      showError(`You can add up to ${MAX_IMAGES} photos.`);
    } else if (accepted.length > 0) {
      clearError();
    }
  };

  const removeImage = (id: string) => {
    if (posting) return; // don't mutate the selection mid-upload
    setImages((prev) => {
      const target = prev.find((im) => im.id === id);
      if (target) URL.revokeObjectURL(target.previewUrl);
      return prev.filter((im) => im.id !== id);
    });
    clearError();
  };

  // Persist per-image upload progress back into state so a retry reuses it.
  const patchImage = (id: string, patch: Partial<SelectedImage>) => {
    setImages((prev) => prev.map((im) => (im.id === id ? { ...im, ...patch } : im)));
  };

  const trimmed = content.trim();
  const canPost = !posting && (trimmed.length > 0 || images.length > 0);

  const submitPost = async () => {
    if (posting || (!trimmed && images.length === 0)) return;

    setPosting(true);
    clearError();
    // Distinguishes an upload failure from a post-creation failure.
    let reachedCreatePost = false;
    try {
      // Upload sequentially in selection order, reusing any already-completed
      // work (from a previous failed attempt) so nothing is uploaded twice.
      const working = images.map((im) => ({ ...im }));
      for (let i = 0; i < working.length; i++) {
        const im = working[i];
        setUploadingIndex(i);
        if (im.confirmed && im.storageKey) continue; // already done

        if (!im.storageKey) {
          const presign = await requestMediaUploadUrl({
            type: "image",
            mimeType: im.file.type,
            sizeBytes: im.file.size,
          });
          await uploadFileToPresignedUrl(presign.uploadUrl, im.file);
          im.storageKey = presign.storageKey; // only after PUT succeeds
          patchImage(im.id, { storageKey: im.storageKey });
        }

        await confirmMediaUpload(im.storageKey);
        im.confirmed = true;
        patchImage(im.id, { confirmed: true });
      }

      const storageKeys = working
        .map((im) => im.storageKey)
        .filter((k): k is string => k !== null);

      reachedCreatePost = true;
      const created = await createPost({
        ...(trimmed ? { content: trimmed } : {}),
        visibility: backendVisibility[visibility],
        ...(storageKeys.length > 0 ? { storageKeys } : {}),
      });

      // Success: prepend the real returned post and reset everything.
      prependPost(mapApiPost(created));
      working.forEach((im) => URL.revokeObjectURL(im.previewUrl));
      setImages([]);
      setContent("");
      if (fileInputRef.current) fileInputRef.current.value = "";
      clearError();
    } catch (err) {
      // Preserve text, previews, and every completed storageKey/confirmed flag so
      // a retry continues where it left off. Never delete R2 objects on failure.
      if (err instanceof ApiError && err.status === 401) {
        showError("Please sign in to post.");
      } else if (!reachedCreatePost) {
        showError("Couldn’t upload images. Please try again.");
      } else {
        showError("Couldn’t create post. Please try again.");
      }
    } finally {
      setPosting(false);
      setUploadingIndex(null);
    }
  };

  const postLabel = posting
    ? uploadingIndex !== null && images.length > 0
      ? `Uploading ${uploadingIndex + 1} of ${images.length}…`
      : "Posting…"
    : "Post";

  return (
    <section className="rounded-2xl border border-border bg-surface p-4 shadow-sm">
      <div className="flex items-center gap-3">
        <span className="h-10 w-10 shrink-0 rounded-full bg-background" />
        <input
          type="text"
          value={content}
          onChange={(e) => {
            setContent(e.target.value);
            if (error) clearError(); // clear the error as the user edits
          }}
          placeholder="What's on your mind?"
          className="h-11 flex-1 rounded-full bg-background px-4 text-sm text-foreground placeholder:text-muted-soft focus:outline-none"
        />
      </div>

      {error ? (
        <div
          role="alert"
          aria-live="assertive"
          className="mt-2 flex items-center gap-2 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700"
        >
          <AlertCircle className="h-4 w-4 shrink-0 text-red-500" aria-hidden />
          <span>{error}</span>
        </div>
      ) : null}

      {/* Selected image previews (1–8), in posting order. */}
      {images.length > 0 ? (
        <div className="mt-3 grid grid-cols-4 gap-2 sm:grid-cols-4">
          {images.map((im, i) => (
            <div
              key={im.id}
              className="relative aspect-square overflow-hidden rounded-lg border border-border bg-background"
            >
              {/* eslint-disable-next-line @next/next/no-img-element */}
              <img
                src={im.previewUrl}
                alt={`Selected image ${i + 1}`}
                className="h-full w-full object-cover"
              />
              <button
                type="button"
                onClick={() => removeImage(im.id)}
                disabled={posting}
                aria-label={`Remove image ${i + 1}`}
                className="absolute right-1 top-1 flex h-6 w-6 items-center justify-center rounded-full bg-foreground/70 text-white transition-opacity hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
              >
                <X className="h-3.5 w-3.5" />
              </button>
            </div>
          ))}
        </div>
      ) : null}

      {/* Hidden multi-image picker opened by the Photo button. */}
      <input
        ref={fileInputRef}
        type="file"
        multiple
        accept="image/jpeg,image/png,image/webp"
        onChange={onFilesSelected}
        className="hidden"
      />

      <div className="mt-4 flex flex-wrap items-center gap-1 border-t border-border pt-3">
        {actions.map(({ label, icon: Icon, color }) => (
          <button
            key={label}
            type="button"
            onClick={label === "Photo" ? openImagePicker : undefined}
            disabled={label === "Photo" && posting}
            className="flex items-center gap-2 rounded-lg px-3 py-2 text-sm font-medium text-muted transition-colors hover:bg-background disabled:cursor-not-allowed disabled:opacity-50"
          >
            <Icon className={`h-4 w-4 ${color}`} />
            {label}
          </button>
        ))}

        <div className="ml-auto flex items-center gap-2">
          <button
            type="button"
            className="flex items-center gap-1.5 rounded-full border border-border px-3 py-1.5 text-sm text-muted transition-colors hover:bg-background"
          >
            <Globe className="h-4 w-4" />
            {visibilityLabels[visibility]}
            <ChevronDown className="h-4 w-4 text-muted-soft" />
          </button>
          <button
            type="button"
            onClick={submitPost}
            disabled={!canPost}
            className="rounded-full bg-primary px-5 py-1.5 text-sm font-medium text-white transition-colors hover:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:bg-primary"
          >
            {postLabel}
          </button>
        </div>
      </div>
    </section>
  );
}
