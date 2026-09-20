"use client";

import { useCallback, useEffect, useMemo, useState } from "react";

import { ApiError, getProfile, updateProfile } from "@/lib/api";
import { LANGUAGES } from "@/lib/languages";
import { useLanguage, type TranslationKey } from "@/lib/language-context";

type Status = "loading" | "ready" | "error";

// Backend semantics: preferredLanguage = null means "fall back to native
// language" (and, with auto-translate, effectively no translation target).
const NATIVE_VALUE = ""; // select value representing null / use-native

// Map on stable HTTP status so the displayed copy stays translatable without
// changing which error surfaces for a given failure.
function friendlySettingsErrorKey(err: unknown): TranslationKey {
  if (err instanceof ApiError) {
    if (err.status === 400) return "settings.error400";
    if (err.status === 401) return "edit.sessionExpired";
  }
  return "settings.saveError";
}

export default function SettingsPage() {
  const { t, setLanguage } = useLanguage();
  const [status, setStatus] = useState<Status>("loading");
  const [platformLanguage, setPlatformLanguage] = useState<string>("");
  const [preferredLanguage, setPreferredLanguage] = useState<string>(NATIVE_VALUE);
  const [autoTranslate, setAutoTranslate] = useState(false);
  const [nativeLanguage, setNativeLanguage] = useState("");
  const [isPrivate, setIsPrivate] = useState(false);

  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);

  // Platform language applies immediately on change (its own save), separate
  // from the translation form's Save button.
  const [platformSaving, setPlatformSaving] = useState(false);
  const [platformError, setPlatformError] = useState<string | null>(null);

  const load = useCallback((signal?: AbortSignal) => {
    setStatus("loading");
    getProfile(signal)
      .then((p) => {
        setPlatformLanguage(p.platformLanguage ?? "");
        setPreferredLanguage(p.preferredLanguage ?? NATIVE_VALUE);
        setAutoTranslate(p.autoTranslateEnabled);
        setNativeLanguage(p.nativeLanguage);
        setIsPrivate(p.isPrivate);
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

  // If the stored preferred language isn't in the starter list, surface it so
  // the select reflects the real value instead of silently mismatching.
  const options = useMemo(() => {
    if (
      preferredLanguage &&
      !LANGUAGES.some((l) => l.code === preferredLanguage)
    ) {
      return [{ code: preferredLanguage, label: preferredLanguage }, ...LANGUAGES];
    }
    return LANGUAGES;
  }, [preferredLanguage]);

  // Platform-language options come only from the canonical catalog; a stored
  // value outside it (unlikely) is surfaced so the select reflects reality.
  const platformOptions = useMemo(() => {
    if (platformLanguage && !LANGUAGES.some((l) => l.code === platformLanguage)) {
      return [{ code: platformLanguage, label: platformLanguage }, ...LANGUAGES];
    }
    return LANGUAGES;
  }, [platformLanguage]);

  // Change the whole UI language. Persists via the existing partial PATCH
  // (platformLanguage only — never touches native/preferred/autoTranslate) and,
  // on success, switches the live LanguageProvider immediately. On failure the
  // previous value is kept so UI state and persisted state never diverge.
  const onPlatformChange = async (next: string) => {
    if (!next || next === platformLanguage || platformSaving) return;
    const previous = platformLanguage;
    setPlatformLanguage(next); // optimistic; reverted on failure
    setPlatformSaving(true);
    setPlatformError(null);
    try {
      const updated = await updateProfile({ platformLanguage: next });
      const applied = updated.platformLanguage ?? next;
      setPlatformLanguage(applied);
      setLanguage(applied); // live UI switch (LanguageProvider source of truth)
    } catch (err) {
      setPlatformLanguage(previous); // keep previous; no divergence
      setPlatformError(t(friendlySettingsErrorKey(err)));
    } finally {
      setPlatformSaving(false);
    }
  };

  const onSave = async (e: React.FormEvent) => {
    e.preventDefault();
    if (saving) return;
    setSaving(true);
    setError(null);
    setSaved(false);
    try {
      const updated = await updateProfile({
        preferredLanguage:
          preferredLanguage === NATIVE_VALUE ? null : preferredLanguage,
        autoTranslateEnabled: autoTranslate,
        isPrivate,
      });
      // Re-sync from the response so the form is never stale after saving.
      setPreferredLanguage(updated.preferredLanguage ?? NATIVE_VALUE);
      setAutoTranslate(updated.autoTranslateEnabled);
      setNativeLanguage(updated.nativeLanguage);
      setIsPrivate(updated.isPrivate);
      setSaved(true);
    } catch (err) {
      setError(t(friendlySettingsErrorKey(err))); // keep entered values
    } finally {
      setSaving(false);
    }
  };

  if (status === "loading") {
    return (
      <p className="py-16 text-center text-sm text-muted">{t("settings.loading")}</p>
    );
  }

  if (status === "error") {
    return (
      <div className="mx-auto max-w-2xl rounded-2xl border border-border bg-surface p-8 text-center shadow-sm">
        <p className="text-sm text-muted">{t("settings.loadError")}</p>
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
      <section className="rounded-2xl border border-border bg-surface p-6 shadow-sm">
        <h1 className="text-lg font-bold tracking-tight text-foreground">
          {t("navigation.settings")}
        </h1>

        {/* Platform language = the language of Together's interface. Distinct
            from the preferred (content translation) language below. Applies
            immediately on selection. */}
        <label className="mt-4 flex flex-col gap-1.5">
          <span className="text-sm font-medium text-foreground">
            {t("settings.platformLanguage")}
          </span>
          <span className="text-xs text-muted">
            {t("settings.platformLanguageDescription")}
          </span>
          <select
            value={platformLanguage}
            disabled={platformSaving}
            onChange={(e) => void onPlatformChange(e.target.value)}
            className="h-11 w-full rounded-lg border border-border bg-background px-3 text-sm text-foreground focus:border-primary focus:outline-none focus:ring-2 focus:ring-primary/20 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {platformOptions.map((l) => (
              <option key={l.code} value={l.code}>
                {l.label} — {l.code}
              </option>
            ))}
          </select>
          {platformError ? (
            <span className="text-xs text-red-500" role="alert">
              {platformError}
            </span>
          ) : null}
        </label>

        <h2 className="mt-6 border-t border-border pt-4 text-sm text-muted">
          {t("settings.translation")}
        </h2>

        <form onSubmit={onSave} className="mt-5 flex flex-col gap-5">
          {/* Preferred language */}
          <label className="flex flex-col gap-1.5">
            <span className="text-sm font-medium text-foreground">
              {t("settings.preferredLanguage")}
            </span>
            <select
              value={preferredLanguage}
              onChange={(e) => {
                setPreferredLanguage(e.target.value);
                setSaved(false);
                setError(null);
              }}
              className="h-11 w-full rounded-lg border border-border bg-background px-3 text-sm text-foreground focus:border-primary focus:outline-none focus:ring-2 focus:ring-primary/20"
            >
              <option value={NATIVE_VALUE}>{t("settings.useNativeLanguage")}</option>
              {options.map((l) => (
                <option key={l.code} value={l.code}>
                  {l.label} — {l.code}
                </option>
              ))}
            </select>
          </label>

          {/* Auto translate */}
          <label className="flex items-start gap-3">
            <input
              type="checkbox"
              checked={autoTranslate}
              onChange={(e) => {
                setAutoTranslate(e.target.checked);
                setSaved(false);
                setError(null);
              }}
              className="mt-0.5 h-4 w-4 rounded border-border text-primary focus:ring-primary/20"
            />
            <span>
              <span className="block text-sm font-medium text-foreground">
                {t("settings.autoTranslate")}
              </span>
              <span className="block text-xs text-muted">
                {t("settings.autoTranslateDescription")}
              </span>
            </span>
          </label>

          {/* Private account */}
          <label className="flex items-start gap-3 border-t border-border pt-4">
            <input
              type="checkbox"
              checked={isPrivate}
              onChange={(e) => {
                setIsPrivate(e.target.checked);
                setSaved(false);
                setError(null);
              }}
              className="mt-0.5 h-4 w-4 rounded border-border text-primary focus:ring-primary/20"
            />
            <span>
              <span className="block text-sm font-medium text-foreground">
                {t("settings.privateAccount")}
              </span>
              <span className="block text-xs text-muted">
                {t("settings.privateAccountDescription")}
              </span>
            </span>
          </label>

          {/* Native language (informational) */}
          <div className="border-t border-border pt-4 text-sm">
            <span className="text-muted-soft">{t("profile.nativeLanguage")}</span>
            <div className="text-foreground">{nativeLanguage}</div>
          </div>

          {error ? (
            <p className="text-sm text-red-500" role="alert">
              {error}
            </p>
          ) : null}
          {saved && !error ? (
            <p className="text-sm text-emerald-600" role="status">
              {t("settings.saved")}
            </p>
          ) : null}

          <div>
            <button
              type="submit"
              disabled={saving}
              className="rounded-full bg-primary px-5 py-2 text-sm font-medium text-white transition-colors hover:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:bg-primary"
            >
              {saving ? t("post.saving") : t("post.save")}
            </button>
          </div>
        </form>
      </section>
    </div>
  );
}
