"use client";

import { useEffect, useRef, useState } from "react";
import {
  AlertCircle,
  BarChart3,
  Check,
  ChevronDown,
  Globe,
  Image as ImageIcon,
  Lock,
  Mic,
  Smile,
  Users,
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
import { useLanguage, type TranslationKey } from "@/lib/language-context";

// Client-side image constraints, mirroring the backend media policy. The picker
// validates against these directly (never trusting the input `accept` alone).
const ALLOWED_IMAGE_TYPES = ["image/jpeg", "image/png", "image/webp"];
const MAX_IMAGE_BYTES = 15 * 1024 * 1024; // 15 MiB
const MAX_IMAGES = 8; // product limit: at most 8 images per post

type Action = {
  id: "photo" | "video" | "voice" | "poll" | "feeling";
  labelKey: TranslationKey;
  icon: LucideIcon;
  color: string;
};

const actions: Action[] = [
  { id: "photo", labelKey: "composer.action.photo", icon: ImageIcon, color: "text-emerald-500" },
  { id: "video", labelKey: "composer.action.video", icon: Video, color: "text-rose-500" },
  { id: "voice", labelKey: "composer.action.voice", icon: Mic, color: "text-violet-500" },
  { id: "poll", labelKey: "composer.action.poll", icon: BarChart3, color: "text-sky-500" },
  { id: "feeling", labelKey: "composer.action.feeling", icon: Smile, color: "text-amber-500" },
];

type Visibility = "everyone" | "friends" | "private";

// Translation keys for each audience choice's label and hint.
const visibilityLabelKey: Record<Visibility, TranslationKey> = {
  everyone: "composer.visibility.public",
  friends: "composer.visibility.followers",
  private: "composer.visibility.private",
};
const visibilityHintKey: Record<Visibility, TranslationKey> = {
  everyone: "composer.visibility.publicHint",
  friends: "composer.visibility.followersHint",
  private: "composer.visibility.privateHint",
};

// Map the composer's audience choice to the backend's visibility values.
const backendVisibility: Record<Visibility, string> = {
  everyone: "public",
  friends: "followers",
  private: "private",
};

// Audience options in display order, each with its icon. Labels/hints are
// resolved from the i18n dictionary at render time.
const visibilityOptions: { value: Visibility; icon: LucideIcon }[] = [
  { value: "everyone", icon: Globe },
  { value: "friends", icon: Users },
  { value: "private", icon: Lock },
];

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
  const { t } = useLanguage();
  const [content, setContent] = useState("");
  const [visibility, setVisibility] = useState<Visibility>("everyone");
  const [visibilityOpen, setVisibilityOpen] = useState(false);
  const [images, setImages] = useState<SelectedImage[]>([]);
  const [posting, setPosting] = useState(false);
  const [uploadingIndex, setUploadingIndex] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);

  const fileInputRef = useRef<HTMLInputElement>(null);
  const visibilityMenuRef = useRef<HTMLDivElement>(null);
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

  // Close the visibility menu on outside click.
  useEffect(() => {
    if (!visibilityOpen) return;
    const onClick = (e: MouseEvent) => {
      if (
        visibilityMenuRef.current &&
        !visibilityMenuRef.current.contains(e.target as Node)
      ) {
        setVisibilityOpen(false);
      }
    };
    document.addEventListener("mousedown", onClick);
    return () => document.removeEventListener("mousedown", onClick);
  }, [visibilityOpen]);

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
      showError(t("composer.tooManyPhotos"));
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
      showError(t("composer.invalidImages"));
    } else if (truncated) {
      showError(t("composer.tooManyPhotos"));
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
            // Route the upload to the bucket matching the chosen visibility.
            visibility: backendVisibility[visibility],
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
        showError(t("composer.signIn"));
      } else if (!reachedCreatePost) {
        showError(t("composer.uploadFailed"));
      } else {
        showError(t("composer.createFailed"));
      }
    } finally {
      setPosting(false);
      setUploadingIndex(null);
    }
  };

  const postLabel = posting
    ? uploadingIndex !== null && images.length > 0
      ? t("composer.uploading", { current: uploadingIndex + 1, total: images.length })
      : t("composer.posting")
    : t("composer.post");

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
          placeholder={t("composer.placeholder")}
          className="h-11 flex-1 rounded-full bg-background px-4 text-sm text-foreground placeholder:text-muted-soft"
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
                alt={t("composer.selectedImage", { index: i + 1 })}
                className="h-full w-full object-cover"
              />
              <button
                type="button"
                onClick={() => removeImage(im.id)}
                disabled={posting}
                aria-label={t("composer.removeImage", { index: i + 1 })}
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
        {actions.map(({ id, labelKey, icon: Icon, color }) => {
          // Only Photo is implemented; the others stay visible for layout but are
          // honestly disabled (non-interactive, no fake click) until built.
          const isPhoto = id === "photo";
          return (
            <button
              key={id}
              type="button"
              onClick={isPhoto ? openImagePicker : undefined}
              disabled={!isPhoto || posting}
              className="flex items-center gap-2 rounded-lg px-3 py-2 text-sm font-medium text-muted transition-colors hover:bg-background disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:bg-transparent"
            >
              <Icon className={`h-4 w-4 ${color}`} />
              {t(labelKey)}
            </button>
          );
        })}

        <div className="ml-auto flex items-center gap-2">
          <div className="relative" ref={visibilityMenuRef}>
            <button
              type="button"
              onClick={() => setVisibilityOpen((v) => !v)}
              disabled={posting}
              aria-haspopup="menu"
              aria-expanded={visibilityOpen}
              aria-label={t("composer.audience", {
                audience: t(visibilityLabelKey[visibility]),
              })}
              className="flex items-center gap-1.5 rounded-full border border-border px-3 py-1.5 text-sm text-muted transition-colors hover:bg-background disabled:cursor-not-allowed disabled:opacity-50"
            >
              {(() => {
                const Icon =
                  visibilityOptions.find((o) => o.value === visibility)?.icon ??
                  Globe;
                return <Icon className="h-4 w-4" />;
              })()}
              {t(visibilityLabelKey[visibility])}
              <ChevronDown className="h-4 w-4 text-muted-soft" />
            </button>
            {visibilityOpen ? (
              <div
                role="menu"
                className="absolute bottom-full right-0 z-20 mb-2 w-56 overflow-hidden rounded-xl border border-border bg-surface p-1 shadow-lg"
              >
                {visibilityOptions.map(({ value, icon: Icon }) => (
                  <button
                    key={value}
                    type="button"
                    role="menuitemradio"
                    aria-checked={visibility === value}
                    onClick={() => {
                      setVisibility(value);
                      setVisibilityOpen(false);
                    }}
                    className="flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-left transition-colors hover:bg-background"
                  >
                    <Icon className="h-4 w-4 shrink-0 text-muted" />
                    <span className="min-w-0 flex-1">
                      <span className="block text-sm font-medium text-foreground">
                        {t(visibilityLabelKey[value])}
                      </span>
                      <span className="block text-xs text-muted-soft">
                        {t(visibilityHintKey[value])}
                      </span>
                    </span>
                    {visibility === value ? (
                      <Check className="h-4 w-4 shrink-0 text-primary" />
                    ) : null}
                  </button>
                ))}
              </div>
            ) : null}
          </div>
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
