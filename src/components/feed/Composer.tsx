"use client";

import { useEffect, useRef, useState } from "react";
import Image from "next/image";
import {
  AlertCircle,
  Check,
  ChevronDown,
  Globe,
  Image as ImageIcon,
  Lock,
  Users,
  X,
  type LucideIcon,
} from "lucide-react";
import { StoryComposer } from "./StoryComposer";
import {
  ApiError,
  confirmMediaUpload,
  createPost,
  requestMediaUploadUrl,
  uploadFileToPresignedUrl,
} from "@/lib/api";
import { mapApiPost } from "@/lib/map-post";
import { useFeed } from "@/lib/feed-context";
import { useAuth } from "@/lib/auth-context";
import { checkPostStoryMedia, IMAGE_ACCEPT, storageUploadErrorKey, withEffectiveType } from "@/lib/media-rules";
import { useLanguage, type TranslationKey } from "@/lib/language-context";

// Client-side image constraints, mirroring the backend media policy. The picker
// validates against these directly (never trusting the input `accept` alone).
const MAX_IMAGES = 8; // product limit: at most 8 images per post
const FALLBACK_AVATAR =
  "data:image/svg+xml;utf8," +
  encodeURIComponent(
    '<svg xmlns="http://www.w3.org/2000/svg" width="40" height="40"><circle cx="20" cy="20" r="20" fill="#d4d4d8"/></svg>',
  );

type Action = {
  id: "photo";
  labelKey: TranslationKey;
  icon: LucideIcon;
  color: string;
};

const actions: Action[] = [
  { id: "photo", labelKey: "composer.action.photo", icon: ImageIcon, color: "text-emerald-500" },
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
  const { user } = useAuth();
  const [content, setContent] = useState("");
  const [visibility, setVisibility] = useState<Visibility>("everyone");
  const [visibilityOpen, setVisibilityOpen] = useState(false);
  const [images, setImages] = useState<SelectedImage[]>([]);
  const [posting, setPosting] = useState(false);
  const [uploadingIndex, setUploadingIndex] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [open, setOpen] = useState(false);
  const [mode, setMode] = useState<"post" | "story">("post");

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

  useEffect(() => {
    if (!open) return;
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !posting) setOpen(false);
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [open, posting]);

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

    // Posts are photo-only (V1). Each file is checked against the shared
    // post/story rules (100 MiB); the server enforces the same policy.
    const valid: File[] = [];
    const reasons = new Set<string>();
    for (const f of files) {
      const check = checkPostStoryMedia(f, false);
      if (check.ok) valid.push(withEffectiveType(f));
      else reasons.add(check.reason);
    }
    const hadInvalid = reasons.size > 0;

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
      showError(
        reasons.size > 1
          ? t("composer.invalidImages")
          : reasons.has("imageTooLarge")
            ? t("media.imageTooLarge")
            : t("media.imageUnsupported"),
      );
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
    // Which step failed decides the message: rejected request, storage
    // upload, server-side verification, or post creation.
    let stage: "presign" | "upload" | "confirm" | "create" = "presign";
    try {
      // Upload sequentially in selection order, reusing any already-completed
      // work (from a previous failed attempt) so nothing is uploaded twice.
      const working = images.map((im) => ({ ...im }));
      for (let i = 0; i < working.length; i++) {
        const im = working[i];
        setUploadingIndex(i);
        if (im.confirmed && im.storageKey) continue; // already done

        if (!im.storageKey) {
          stage = "presign";
          const presign = await requestMediaUploadUrl({
            type: "image",
            mimeType: im.file.type,
            sizeBytes: im.file.size,
            // Route the upload to the bucket matching the chosen visibility.
            visibility: backendVisibility[visibility],
          });
          stage = "upload";
          await uploadFileToPresignedUrl(presign.uploadUrl, im.file);
          im.storageKey = presign.storageKey; // only after PUT succeeds
          patchImage(im.id, { storageKey: im.storageKey });
        }

        stage = "confirm";
        await confirmMediaUpload(im.storageKey);
        im.confirmed = true;
        patchImage(im.id, { confirmed: true });
      }

      const storageKeys = working
        .map((im) => im.storageKey)
        .filter((k): k is string => k !== null);

      stage = "create";
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
      setOpen(false);
    } catch (err) {
      // Preserve text, previews, and every completed storageKey/confirmed flag so
      // a retry continues where it left off. Never delete R2 objects on failure.
      if (err instanceof ApiError && err.status === 401) {
        showError(t("composer.signIn"));
      } else if (stage === "presign") {
        showError(err instanceof ApiError && err.status === 400 ? t("media.rejected") : t("composer.uploadFailed"));
      } else if (stage === "upload") {
        showError(t(storageUploadErrorKey(err)));
      } else if (stage === "confirm") {
        showError(t("media.confirmFailed"));
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

  if (!open) {
    return (
      <section className="rounded-2xl border border-border bg-surface p-3 shadow-sm">
        <button
          type="button"
          onClick={() => { setMode("post"); setOpen(true); }}
          className="flex w-full items-center gap-3 rounded-xl p-2 text-left transition-colors hover:bg-background focus:outline-none focus-visible:ring-2 focus-visible:ring-primary"
        >
          <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-primary-soft text-lg font-semibold text-primary">+</span>
          <span className="flex-1 rounded-full bg-background px-4 py-2.5 text-sm text-muted-soft">{t("composer.placeholder")}</span>
        </button>
      </section>
    );
  }

  return (
    <div className="fixed inset-0 z-50 flex items-end justify-center bg-black/40 p-0 sm:items-center sm:p-4" role="dialog" aria-modal="true" aria-label={t("post.title")} onMouseDown={() => { if (!posting) setOpen(false); }}>
    <section className="max-h-[94dvh] w-full overflow-y-auto rounded-t-2xl border border-border bg-surface p-4 shadow-xl sm:max-w-2xl sm:rounded-2xl sm:p-5" onMouseDown={(event) => event.stopPropagation()}>
      <div className="mb-4 flex items-center justify-between border-b border-border pb-3">
        <div className="flex gap-1 rounded-lg bg-background p-1" role="tablist" aria-label={t("post.title")}>
          <button type="button" role="tab" aria-selected={mode === "post"} onClick={() => setMode("post")} disabled={posting} className={`rounded-md px-3 py-1.5 text-sm font-medium ${mode === "post" ? "bg-surface text-foreground shadow-sm" : "text-muted"}`}>{t("post.title")}</button>
          <button type="button" role="tab" aria-selected={mode === "story"} onClick={() => setMode("story")} disabled={posting} className={`rounded-md px-3 py-1.5 text-sm font-medium ${mode === "story" ? "bg-surface text-foreground shadow-sm" : "text-muted"}`}>{t("stories.add")}</button>
        </div>
        <button type="button" aria-label={t("profile.close")} onClick={() => setOpen(false)} disabled={posting} className="flex h-8 w-8 items-center justify-center rounded-full text-muted hover:bg-background disabled:opacity-50"><X className="h-5 w-5" /></button>
      </div>
      {mode === "story" ? <StoryComposer inline onClose={() => setOpen(false)} onCreated={() => setOpen(false)} /> : <>
      <div className="mb-4 flex items-center gap-3">
        <Image src={user?.avatarUrl ?? FALLBACK_AVATAR} alt={user?.displayName ?? ""} width={40} height={40} unoptimized={Boolean(user?.avatarUrl)} className="h-10 w-10 rounded-full object-cover" />
        <div className="min-w-0 leading-tight"><div className="truncate text-sm font-semibold text-foreground">{user?.displayName ?? ""}</div><div className="truncate text-xs text-muted">{user?.username ? `@${user.username}` : ""}</div></div>
      </div>
      <div className="flex items-center gap-3">
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
        accept={IMAGE_ACCEPT}
        onChange={onFilesSelected}
        className="hidden"
      />

      <div className="mt-4 flex flex-wrap items-center gap-1 border-t border-border pt-3">
        {actions.map(({ id, labelKey, icon: Icon, color }) => {
          return (
            <button
              key={id}
              type="button"
              onClick={openImagePicker}
              disabled={posting}
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
      </>}
    </section>
    </div>
  );
}
