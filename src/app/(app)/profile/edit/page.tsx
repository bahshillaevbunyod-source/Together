"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { AlertCircle, Camera, User } from "lucide-react";

import {
  ApiError,
  confirmMediaUpload,
  getProfile,
  requestMediaUploadUrl,
  updateProfile,
  uploadFileToPresignedUrl,
} from "@/lib/api";
import { LANGUAGES } from "@/lib/languages";
import { useLanguage, type TranslationKey } from "@/lib/language-context";

type Status = "loading" | "ready" | "error";

// Client-side avatar constraints (mirrors the backend media policy).
const ALLOWED_IMAGE_TYPES = ["image/jpeg", "image/png", "image/webp"];
const MAX_IMAGE_BYTES = 60 * 1024 * 1024; // 60 MiB (backend maxAvatarImageBytes)

// Map backend failures to a safe, friendly translation key (keyed on the stable
// HTTP status, never on English copy). The caller resolves it via t().
function friendlyProfileErrorKey(err: unknown): TranslationKey {
  if (err instanceof ApiError) {
    switch (err.status) {
      case 400:
        return "edit.error400";
      case 401:
        return "edit.sessionExpired";
      case 409:
        return "edit.error409";
    }
  }
  return "edit.errorGeneric";
}

export default function EditProfilePage() {
  const router = useRouter();
  const { t } = useLanguage();

  const [status, setStatus] = useState<Status>("loading");
  const [displayName, setDisplayName] = useState("");
  const [bio, setBio] = useState("");
  const [countryCode, setCountryCode] = useState("");
  const [city, setCity] = useState("");
  const [nativeLanguage, setNativeLanguage] = useState("");

  // Avatar: existing persisted URL + a pending local selection (uploaded on Save).
  const [avatarUrl, setAvatarUrl] = useState<string | null>(null);
  const [avatarFile, setAvatarFile] = useState<File | null>(null);
  const [avatarPreview, setAvatarPreview] = useState<string | null>(null);
  const [avatarRemoved, setAvatarRemoved] = useState(false);
  const [avatarError, setAvatarError] = useState<string | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const inputClass =
    "h-12 w-full min-w-0 rounded-xl border border-border bg-background px-3 text-base text-foreground placeholder:text-muted-soft transition-colors focus:border-primary focus:outline-none focus:ring-2 focus:ring-primary/20 sm:h-11 sm:text-sm";
  const labelClass = "text-sm font-medium text-foreground";

  // Revoke the local preview object URL when replaced / on unmount.
  useEffect(() => {
    if (!avatarPreview) return;
    return () => URL.revokeObjectURL(avatarPreview);
  }, [avatarPreview]);

  const load = useCallback((signal?: AbortSignal) => {
    setStatus("loading");
    getProfile(signal)
      .then((p) => {
        setDisplayName(p.displayName);
        setBio(p.bio ?? "");
        setCountryCode(p.countryCode ?? "");
        setCity(p.city ?? "");
        setNativeLanguage(p.nativeLanguage);
        setAvatarUrl(p.avatarUrl);
        setStatus("ready");
      })
      .catch((err) => {
        if (err instanceof DOMException && err.name === "AbortError") return;
        setStatus("error");
      });
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    load(controller.signal);
    return () => controller.abort();
  }, [load]);

  // The image currently shown in the avatar preview (local selection wins).
  const shownAvatar = avatarPreview ?? (avatarRemoved ? null : avatarUrl);

  const openPicker = () => {
    if (saving) return;
    fileInputRef.current?.click();
  };

  const onAvatarSelected = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0] ?? null;
    e.target.value = ""; // allow re-selecting the same file later
    if (!file || saving) return;

    if (!ALLOWED_IMAGE_TYPES.includes(file.type) || file.size > MAX_IMAGE_BYTES) {
      setAvatarError(t("edit.avatarInvalid"));
      return;
    }
    setAvatarError(null);
    setAvatarFile(file);
    setAvatarPreview(URL.createObjectURL(file)); // effect revokes any previous URL
    setAvatarRemoved(false);
  };

  const removePhoto = () => {
    if (saving) return;
    setAvatarFile(null);
    setAvatarPreview(null);
    setAvatarRemoved(true);
    setAvatarError(null);
  };

  const onSave = async (e: React.FormEvent) => {
    e.preventDefault();
    if (saving) return;

    const name = displayName.trim();
    const lang = nativeLanguage.trim();
    if (!name) {
      setError(t("edit.displayNameRequired"));
      return;
    }
    if (lang.length < 2) {
      setError(t("edit.nativeLanguageRequired"));
      return;
    }

    const trimOrNull = (v: string) => (v.trim() === "" ? null : v.trim());

    setSaving(true);
    setError(null);
    try {
      // Resolve the avatar change: upload a new file, clear it, or leave as-is.
      let avatarField: { avatarUrl?: string | null } = {};
      if (avatarFile) {
        try {
          const presign = await requestMediaUploadUrl({
            type: "image",
            mimeType: avatarFile.type,
            sizeBytes: avatarFile.size,
            purpose: "avatar",
          });
          await uploadFileToPresignedUrl(presign.uploadUrl, avatarFile);
          const confirmed = await confirmMediaUpload(presign.storageKey);
          avatarField = { avatarUrl: confirmed.publicUrl };
        } catch {
          setAvatarError(t("edit.avatarUploadError"));
          setSaving(false);
          return;
        }
      } else if (avatarRemoved) {
        avatarField = { avatarUrl: null };
      }

      await updateProfile({
        displayName: name,
        nativeLanguage: lang,
        bio: trimOrNull(bio),
        city: trimOrNull(city),
        countryCode:
          countryCode.trim() === "" ? null : countryCode.trim().toUpperCase(),
        ...avatarField,
      });
      router.push("/profile");
    } catch (err) {
      setError(t(friendlyProfileErrorKey(err))); // keep entered values on failure
      setSaving(false);
    }
  };

  if (status === "loading") {
    return (
      <div className="mx-auto max-w-2xl">
        <div className="rounded-2xl border border-border bg-surface p-6 shadow-sm">
          <div className="h-5 w-32 animate-pulse rounded bg-background" />
          <div className="mt-6 flex items-center gap-4">
            <div className="h-24 w-24 animate-pulse rounded-full bg-background" />
            <div className="h-9 w-32 animate-pulse rounded-full bg-background" />
          </div>
          <div className="mt-6 flex flex-col gap-4">
            {[0, 1, 2, 3].map((i) => (
              <div key={i} className="h-11 w-full animate-pulse rounded-lg bg-background" />
            ))}
          </div>
        </div>
      </div>
    );
  }

  if (status === "error") {
    return (
      <div className="mx-auto max-w-2xl rounded-2xl border border-border bg-surface p-8 text-center shadow-sm">
        <p className="text-sm text-muted">{t("profile.loadError")}</p>
        <button
          type="button"
          onClick={() => load()}
          className="mt-3 rounded-full bg-primary px-4 py-1.5 text-sm text-white transition-colors hover:bg-primary-hover"
        >
          {t("search.tryAgain")}
        </button>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-2xl">
      <section className="overflow-hidden rounded-2xl border border-border bg-surface shadow-sm">
        {/* Header */}
        <div className="border-b border-border px-4 py-4 sm:px-6 sm:py-5">
          <h1 className="text-lg font-bold tracking-tight text-foreground">
            {t("profile.edit")}
          </h1>
          <p className="mt-0.5 text-sm text-muted">
            {t("edit.subtitle", { brand: "Together" })}
          </p>
        </div>

        <form onSubmit={onSave} className="flex flex-col gap-5 px-4 py-5 sm:gap-6 sm:px-6 sm:py-6">
          {/* Avatar */}
          <div className="flex flex-col items-center gap-4 sm:flex-row sm:items-center">
            <button
              type="button"
              onClick={openPicker}
              disabled={saving}
              aria-label={t("edit.changePhotoAria")}
              className="group relative h-24 w-24 shrink-0 overflow-hidden rounded-full border border-border bg-background disabled:cursor-not-allowed"
            >
              {shownAvatar ? (
                // eslint-disable-next-line @next/next/no-img-element
                <img
                  src={shownAvatar}
                  alt={t("edit.photoAlt")}
                  className="h-full w-full object-cover"
                />
              ) : (
                <span className="flex h-full w-full items-center justify-center text-muted-soft">
                  <User className="h-9 w-9" />
                </span>
              )}
              {/* Camera overlay on hover/focus */}
              <span className="absolute inset-0 flex items-center justify-center bg-black/40 text-white opacity-0 transition-opacity group-hover:opacity-100 group-focus-visible:opacity-100">
                <Camera className="h-6 w-6" />
              </span>
            </button>

            <div className="flex w-full min-w-0 flex-col items-center gap-2 sm:w-auto sm:flex-1 sm:items-start">
              <div className="flex flex-wrap items-center justify-center gap-2 sm:justify-start">
                <button
                  type="button"
                  onClick={openPicker}
                  disabled={saving}
                  className="h-10 rounded-full border border-border px-4 text-sm font-medium text-foreground transition-colors hover:bg-background disabled:opacity-50"
                >
                  {t("edit.changePhoto")}
                </button>
                {shownAvatar ? (
                  <button
                    type="button"
                    onClick={removePhoto}
                    disabled={saving}
                    className="h-10 rounded-full px-4 text-sm font-medium text-red-600 transition-colors hover:bg-red-50 disabled:opacity-50"
                  >
                    {t("edit.removePhoto")}
                  </button>
                ) : null}
              </div>
              <p className="text-center text-xs text-muted-soft sm:text-start">{t("edit.photoConstraints")}</p>
              {avatarError ? (
                <div
                  role="alert"
                  className="flex w-full items-start gap-2 rounded-xl border border-red-200 bg-red-50 px-3 py-2 text-xs leading-snug text-red-700"
                >
                  <AlertCircle className="mt-px h-3.5 w-3.5 shrink-0 text-red-500" aria-hidden />
                  <span className="min-w-0 break-words">{avatarError}</span>
                </div>
              ) : null}
            </div>

            <input
              ref={fileInputRef}
              type="file"
              accept="image/jpeg,image/png,image/webp"
              onChange={onAvatarSelected}
              className="hidden"
            />
          </div>

          <div className="h-px bg-border" />

          {/* Display name */}
          <label className="flex flex-col gap-1.5">
            <span className={labelClass}>{t("edit.displayNameLabel")}</span>
            <input
              className={inputClass}
              type="text"
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
              placeholder={t("edit.displayNamePlaceholder")}
            />
          </label>

          {/* Bio */}
          <label className="flex flex-col gap-1.5">
            <span className={labelClass}>{t("edit.bioLabel")}</span>
            <textarea
              value={bio}
              onChange={(e) => setBio(e.target.value)}
              placeholder={t("edit.bioPlaceholder")}
              rows={3}
              className="w-full min-w-0 resize-none rounded-xl border border-border bg-background px-3 py-2 text-base text-foreground placeholder:text-muted-soft transition-colors focus:border-primary focus:outline-none focus:ring-2 focus:ring-primary/20 sm:text-sm"
            />
          </label>

          {/* City + Country */}
          <div className="flex flex-col gap-4 sm:flex-row">
            <label className="flex flex-1 flex-col gap-1.5">
              <span className={labelClass}>{t("edit.city")}</span>
              <input
                className={inputClass}
                type="text"
                value={city}
                onChange={(e) => setCity(e.target.value)}
                placeholder={t("edit.city")}
              />
            </label>
            <label className="flex flex-col gap-1.5 sm:w-40">
              <span className={labelClass}>{t("edit.countryLabel")}</span>
              <input
                className={inputClass}
                type="text"
                value={countryCode}
                onChange={(e) => setCountryCode(e.target.value)}
                placeholder="US"
                maxLength={2}
              />
            </label>
          </div>

          {/* Native language */}
          <label className="flex flex-col gap-1.5">
            <span className={labelClass}>{t("profile.nativeLanguage")}</span>
            <select
              className={inputClass}
              value={nativeLanguage}
              onChange={(e) => setNativeLanguage(e.target.value)}
            >
              {LANGUAGES.some((l) => l.code === nativeLanguage) ? null : (
                <option value={nativeLanguage}>{nativeLanguage || t("edit.selectLanguage")}</option>
              )}
              {LANGUAGES.map((l) => (
                <option key={l.code} value={l.code}>
                  {l.label}
                </option>
              ))}
            </select>
          </label>

          {error ? (
            <div
              role="alert"
              className="flex items-start gap-2 rounded-xl border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700"
            >
              <AlertCircle className="mt-0.5 h-4 w-4 shrink-0 text-red-500" aria-hidden />
              <span className="min-w-0 break-words">{error}</span>
            </div>
          ) : null}

          {/* Actions */}
          <div className="flex flex-col-reverse gap-2 border-t border-border pt-5 sm:flex-row sm:items-center sm:justify-end sm:gap-3">
            <Link
              href="/profile"
              className="flex h-11 items-center justify-center rounded-full border border-border px-5 text-sm font-medium text-muted transition-colors hover:bg-background hover:text-foreground sm:h-10"
            >
              {t("post.cancel")}
            </Link>
            <button
              type="submit"
              disabled={saving}
              className="h-11 rounded-full bg-primary px-6 text-sm font-medium text-white transition-colors hover:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:bg-primary sm:h-10"
            >
              {saving ? t("post.saving") : t("edit.saveChanges")}
            </button>
          </div>
        </form>
      </section>
    </div>
  );
}
