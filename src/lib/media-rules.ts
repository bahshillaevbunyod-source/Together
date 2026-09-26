/**
 * Client-side media rules for posts and stories. They mirror the backend
 * policy (backend/internal/server/media_upload.go, postMediaLimits), which is
 * the real enforcement: the server re-checks type and size at upload-url time,
 * the presigned PUT binds the approved size, and create re-validates the
 * stored object. These checks only reject early, before a large upload.
 */

import { StorageUploadError } from "@/lib/api";

export const MiB = 1024 * 1024;

/** Post and story photos. (Profile photos use their own 60 MiB rule.) */
export const MAX_POST_IMAGE_BYTES = 100 * MiB;
/** Story videos. */
export const MAX_STORY_VIDEO_BYTES = 250 * MiB;
export const MAX_STORY_VIDEO_SECONDS = 60;

export const IMAGE_TYPES = ["image/jpeg", "image/png", "image/webp"] as const;
/** video/quicktime is the iPhone camera default (.mov). */
export const VIDEO_TYPES = ["video/mp4", "video/quicktime", "video/webm"] as const;

export const IMAGE_ACCEPT = IMAGE_TYPES.join(",");
export const MEDIA_ACCEPT = [...IMAGE_TYPES, ...VIDEO_TYPES, ".mov"].join(",");

const EXT_TYPES: Record<string, string> = {
  jpg: "image/jpeg",
  jpeg: "image/jpeg",
  png: "image/png",
  webp: "image/webp",
  mp4: "video/mp4",
  m4v: "video/mp4",
  mov: "video/quicktime",
  webm: "video/webm",
  heic: "image/heic",
  heif: "image/heif",
};

/**
 * The file's MIME type, inferred from the extension when the browser reports
 * none (some Android pickers do). Lower-cased, parameters stripped.
 */
export function effectiveMimeType(file: File): string {
  const reported = file.type.split(";")[0].trim().toLowerCase();
  if (reported) return reported;
  const ext = file.name.split(".").pop()?.toLowerCase() ?? "";
  return EXT_TYPES[ext] ?? "";
}

/**
 * Returns the file itself, or a copy carrying the inferred MIME type when the
 * browser left it empty — the presigned PUT signs Content-Type, so the body
 * must be sent with the same type the server approved.
 */
export function withEffectiveType(file: File): File {
  const type = effectiveMimeType(file);
  if (!type || file.type === type) return file;
  return new File([file], file.name, { type, lastModified: file.lastModified });
}

export type MediaCheck =
  | { ok: true; kind: "image" | "video"; mimeType: string }
  | { ok: false; reason: "imageUnsupported" | "videoUnsupported" | "imageTooLarge" | "videoTooLarge" };

/** Type and size rules for post/story media (duration is checked separately). */
export function checkPostStoryMedia(file: File, allowVideo: boolean): MediaCheck {
  const mimeType = effectiveMimeType(file);
  if (mimeType.startsWith("video/")) {
    if (!allowVideo || !(VIDEO_TYPES as readonly string[]).includes(mimeType)) {
      return { ok: false, reason: allowVideo ? "videoUnsupported" : "imageUnsupported" };
    }
    if (file.size <= 0 || file.size > MAX_STORY_VIDEO_BYTES) return { ok: false, reason: "videoTooLarge" };
    return { ok: true, kind: "video", mimeType };
  }
  if (!(IMAGE_TYPES as readonly string[]).includes(mimeType)) return { ok: false, reason: "imageUnsupported" };
  if (file.size <= 0 || file.size > MAX_POST_IMAGE_BYTES) return { ok: false, reason: "imageTooLarge" };
  return { ok: true, kind: "image", mimeType };
}

/**
 * Reads a local video's duration (seconds) from its metadata without playing
 * or attaching anything to the DOM. The object URL is always revoked and the
 * element released. Rejects when the browser cannot read the metadata.
 */
export function readVideoDuration(file: File, timeoutMs = 15000): Promise<number> {
  return new Promise((resolve, reject) => {
    const url = URL.createObjectURL(file);
    const video = document.createElement("video");
    let settled = false;
    const finish = (fn: () => void) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      video.onloadedmetadata = null;
      video.ondurationchange = null;
      video.onerror = null;
      video.removeAttribute("src");
      video.load(); // release the decoder/resource
      URL.revokeObjectURL(url);
      fn();
    };
    const timer = setTimeout(() => finish(() => reject(new Error("metadata timeout"))), timeoutMs);
    video.preload = "metadata";
    video.muted = true;
    const settle = () => {
      const d = video.duration;
      if (Number.isFinite(d) && d > 0) finish(() => resolve(d));
    };
    video.onloadedmetadata = () => {
      const d = video.duration;
      if (d === Infinity) {
        // Browser-recorded WebM often has no duration header: seeking far past
        // the end makes the browser compute the real duration.
        video.ondurationchange = settle;
        video.currentTime = Number.MAX_SAFE_INTEGER;
        return;
      }
      if (Number.isFinite(d) && d > 0) settle();
      else finish(() => reject(new Error("no duration")));
    };
    video.onerror = () => finish(() => reject(new Error("metadata error")));
    video.src = url;
  });
}

/**
 * Message for a failed storage PUT: "interrupted" only when the device is
 * really offline; a storage HTTP error, or a request blocked while online,
 * is a storage-side problem the user cannot fix by reconnecting.
 */
export function storageUploadErrorKey(err: unknown): "media.uploadFailed" | "media.storageFailed" {
  const offline = typeof navigator !== "undefined" && navigator.onLine === false;
  if (err instanceof StorageUploadError && err.kind === "rejected") return "media.storageFailed";
  return offline ? "media.uploadFailed" : "media.storageFailed";
}
