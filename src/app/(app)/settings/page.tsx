"use client";

import { useCallback, useEffect, useMemo, useState } from "react";

import { ApiError, getProfile, updateProfile } from "@/lib/api";
import { LANGUAGES } from "@/lib/languages";

type Status = "loading" | "ready" | "error";

// Backend semantics: preferredLanguage = null means "fall back to native
// language" (and, with auto-translate, effectively no translation target).
const NATIVE_VALUE = ""; // select value representing null / use-native

function friendlySettingsError(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.status === 400) return "Please check your translation settings.";
    if (err.status === 401)
      return "Your session has expired. Please sign in again.";
  }
  return "Couldn’t save settings. Try again.";
}

export default function SettingsPage() {
  const [status, setStatus] = useState<Status>("loading");
  const [preferredLanguage, setPreferredLanguage] = useState<string>(NATIVE_VALUE);
  const [autoTranslate, setAutoTranslate] = useState(false);
  const [nativeLanguage, setNativeLanguage] = useState("");

  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);

  const load = useCallback((signal?: AbortSignal) => {
    setStatus("loading");
    getProfile(signal)
      .then((p) => {
        setPreferredLanguage(p.preferredLanguage ?? NATIVE_VALUE);
        setAutoTranslate(p.autoTranslateEnabled);
        setNativeLanguage(p.nativeLanguage);
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
      });
      // Re-sync from the response so the form is never stale after saving.
      setPreferredLanguage(updated.preferredLanguage ?? NATIVE_VALUE);
      setAutoTranslate(updated.autoTranslateEnabled);
      setNativeLanguage(updated.nativeLanguage);
      setSaved(true);
    } catch (err) {
      setError(friendlySettingsError(err)); // keep entered values
    } finally {
      setSaving(false);
    }
  };

  if (status === "loading") {
    return (
      <p className="py-16 text-center text-sm text-muted">Loading settings…</p>
    );
  }

  if (status === "error") {
    return (
      <div className="mx-auto max-w-2xl rounded-2xl border border-border bg-surface p-8 text-center shadow-sm">
        <p className="text-sm text-muted">Couldn’t load your settings.</p>
        <button
          type="button"
          onClick={() => load()}
          className="mt-3 rounded-full bg-primary px-4 py-1.5 text-sm text-white transition-colors hover:bg-primary-hover"
        >
          Try again
        </button>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-2xl">
      <section className="rounded-2xl border border-border bg-surface p-6 shadow-sm">
        <h1 className="text-lg font-bold tracking-tight text-foreground">
          Settings
        </h1>
        <h2 className="mt-1 text-sm text-muted">Translation</h2>

        <form onSubmit={onSave} className="mt-5 flex flex-col gap-5">
          {/* Preferred language */}
          <label className="flex flex-col gap-1.5">
            <span className="text-sm font-medium text-foreground">
              Preferred translation language
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
              <option value={NATIVE_VALUE}>Use native language</option>
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
                Auto translate
              </span>
              <span className="block text-xs text-muted">
                Incoming content can be automatically translated into your
                preferred language.
              </span>
            </span>
          </label>

          {/* Native language (informational) */}
          <div className="border-t border-border pt-4 text-sm">
            <span className="text-muted-soft">Native language</span>
            <div className="text-foreground">{nativeLanguage}</div>
          </div>

          {error ? (
            <p className="text-sm text-red-500" role="alert">
              {error}
            </p>
          ) : null}
          {saved && !error ? (
            <p className="text-sm text-emerald-600" role="status">
              Settings saved.
            </p>
          ) : null}

          <div>
            <button
              type="submit"
              disabled={saving}
              className="rounded-full bg-primary px-5 py-2 text-sm font-medium text-white transition-colors hover:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:bg-primary"
            >
              {saving ? "Saving…" : "Save"}
            </button>
          </div>
        </form>
      </section>
    </div>
  );
}
