"use client";

import Image from "next/image";
import { X } from "lucide-react";
import { useEffect, useRef, useState } from "react";

import {
  ApiError,
  confirmMediaUpload,
  createStory,
  requestMediaUploadUrl,
  uploadFileToPresignedUrl,
} from "@/lib/api";
import { useLanguage } from "@/lib/language-context";
import {
  checkPostStoryMedia,
  MAX_STORY_VIDEO_SECONDS,
  MEDIA_ACCEPT,
  readVideoDuration,
  storageUploadErrorKey,
  withEffectiveType,
} from "@/lib/media-rules";

export function StoryComposer({
  onClose,
  onCreated,
  inline = false,
}: {
  onClose: () => void;
  onCreated: () => void;
  /** Render inside the unified create dialog instead of its own overlay. */
  inline?: boolean;
}) {
  const { t } = useLanguage();
  const inputRef = useRef<HTMLInputElement>(null);
  const [file, setFile] = useState<File | null>(null);
  const [previewURL, setPreviewURL] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [uploading, setUploading] = useState(false);
  const [checking, setChecking] = useState(false);
  // Ignores a stale duration check when another file was picked meanwhile.
  const pickRef = useRef(0);

  useEffect(() => {
    if (!file) {
      setPreviewURL(null);
      return;
    }
    const url = URL.createObjectURL(file);
    setPreviewURL(url);
    return () => URL.revokeObjectURL(url);
  }, [file]);

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !uploading) onClose();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [onClose, uploading]);

  const selectFile = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const picked = event.target.files?.[0] ?? null;
    event.target.value = "";
    if (!picked) return;
    const pick = ++pickRef.current;
    const check = checkPostStoryMedia(picked, true);
    if (!check.ok) {
      setFile(null);
      setError(t(`media.${check.reason}`));
      return;
    }
    const next = withEffectiveType(picked);
    if (check.kind === "video") {
      // Enforce the 60-second limit before a potentially large upload.
      setChecking(true);
      setError(null);
      setFile(null);
      try {
        const seconds = await readVideoDuration(next);
        if (pick !== pickRef.current) return;
        if (seconds > MAX_STORY_VIDEO_SECONDS + 0.5) {
          setError(t("media.videoTooLong"));
          return;
        }
      } catch {
        if (pick !== pickRef.current) return;
        setError(t("media.videoMetadataError"));
        return;
      } finally {
        if (pick === pickRef.current) setChecking(false);
      }
    }
    setError(null);
    setFile(next);
  };

  const submit = async () => {
    if (!file || uploading) return;
    setUploading(true);
    setError(null);
    let stage: "presign" | "upload" | "confirm" | "create" = "presign";
    try {
      const type = file.type.startsWith("video/") ? "video" : "image";
      const presign = await requestMediaUploadUrl({
        type,
        mimeType: file.type,
        sizeBytes: file.size,
        purpose: "story",
        visibility: "private",
      });
      stage = "upload";
      await uploadFileToPresignedUrl(presign.uploadUrl, file);
      stage = "confirm";
      const confirmed = await confirmMediaUpload(presign.storageKey);
      stage = "create";
      await createStory(confirmed.storageKey);
      onCreated();
    } catch (err) {
      setError(
        stage === "presign" && err instanceof ApiError && err.status === 400
          ? t("media.rejected")
          : stage === "upload"
            ? t(storageUploadErrorKey(err))
            : stage === "confirm"
              ? t("media.confirmFailed")
              : t("stories.createError"),
      );
    } finally {
      setUploading(false);
    }
  };

  return (
    <div
      className={inline ? "" : "fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4"}
      role={inline ? undefined : "dialog"}
      aria-modal={inline || undefined}
      aria-label={t("stories.add")}
      onClick={() => {
        if (!inline && !uploading) onClose();
      }}
    >
      <div
        className="flex w-full max-w-md flex-col gap-4 rounded-2xl border border-border bg-surface p-5 shadow-xl"
        onClick={(event) => event.stopPropagation()}
      >
        <div className="flex items-center justify-between">
          <h2 className="text-base font-semibold text-foreground">{t("stories.add")}</h2>
          <button
            type="button"
            aria-label={t("profile.close")}
            onClick={onClose}
            disabled={uploading}
            className="flex h-8 w-8 items-center justify-center rounded-full text-muted hover:bg-background disabled:opacity-50"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        <input
          ref={inputRef}
          type="file"
          accept={MEDIA_ACCEPT}
          className="hidden"
          onChange={selectFile}
        />

        {previewURL ? (
          <div className="relative aspect-video overflow-hidden rounded-xl bg-black">
            {file?.type.startsWith("video/") ? (
              <video src={previewURL} controls playsInline className="h-full w-full object-contain" />
            ) : (
              <Image
                src={previewURL}
                alt={t("stories.mediaAlt", { name: t("stories.add") })}
                fill
                unoptimized
                className="object-contain"
              />
            )}
          </div>
        ) : (
          <button
            type="button"
            onClick={() => inputRef.current?.click()}
            disabled={checking}
            className="flex min-h-32 flex-col items-center justify-center gap-2 rounded-xl border-2 border-dashed border-border px-4 text-center text-sm text-muted hover:bg-background focus:outline-none focus-visible:ring-2 focus-visible:ring-primary"
          >
            <span className="font-medium text-foreground">{t("stories.chooseMedia")}</span>
            <span className="text-xs text-muted-soft">{checking ? t("media.checkingVideo") : t("stories.mediaRules")}</span>
          </button>
        )}

        {file && !uploading ? (
          <button
            type="button"
            onClick={() => inputRef.current?.click()}
            className="self-start text-sm text-primary hover:underline"
          >
            {t("stories.chooseMedia")}
          </button>
        ) : null}

        {error ? (
          <p role="alert" className="text-sm text-red-600">{error}</p>
        ) : null}

        <div className="flex justify-end gap-2">
          <button
            type="button"
            onClick={onClose}
            disabled={uploading}
            className="rounded-full border border-border px-4 py-2 text-sm text-foreground hover:bg-background disabled:opacity-50"
          >
            {t("post.cancel")}
          </button>
          <button
            type="button"
            onClick={submit}
            disabled={!file || uploading || checking}
            className="rounded-full bg-primary px-4 py-2 text-sm font-medium text-white hover:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-50"
          >
            {uploading ? t("stories.uploading") : t("stories.upload")}
          </button>
        </div>
      </div>
    </div>
  );
}
