"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import Image from "next/image";
import { useRouter } from "next/navigation";

import { ApiError, getProfile, updateProfile } from "@/lib/api";
import { useAuth } from "@/lib/auth-context";
import { LANGUAGES } from "@/lib/languages";
// Pre-AppShell onboarding renders before any LanguageProvider exists, so it uses
// the canonical English dictionary (English fallback + LTR). The selected
// language must not become the active platform language until the PATCH succeeds.
import en from "@/lib/i18n/locales/en";

type ProfileStatus = "loading" | "ready" | "error";

export default function LanguagePage() {
  const { status, refresh } = useAuth();
  const router = useRouter();
  const [profileStatus, setProfileStatus] = useState<ProfileStatus>("loading");
  const [platformLanguage, setPlatformLanguage] = useState<string | null>(null);
  const [query, setQuery] = useState("");
  const [selectedLanguage, setSelectedLanguage] = useState("");
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);

  const loadProfile = useCallback(async (signal?: AbortSignal) => {
    setProfileStatus("loading");
    try {
      const profile = await getProfile(signal);
      setPlatformLanguage(profile.platformLanguage);
      setProfileStatus("ready");
    } catch (err) {
      if (err instanceof DOMException && err.name === "AbortError") return;
      setProfileStatus("error");
    }
  }, []);

  useEffect(() => {
    if (status !== "authenticated") return;
    const controller = new AbortController();
    void loadProfile(controller.signal);
    return () => controller.abort();
  }, [loadProfile, status]);

  useEffect(() => {
    if (status === "unauthenticated") {
      router.replace("/login");
    }
  }, [router, status]);

  useEffect(() => {
    if (status === "authenticated" && profileStatus === "ready" && platformLanguage !== null) {
      router.replace("/");
    }
  }, [platformLanguage, profileStatus, router, status]);

  const languages = useMemo(() => {
    const normalizedQuery = query.trim().toLowerCase();
    if (!normalizedQuery) return LANGUAGES;
    return LANGUAGES.filter(
      (language) =>
        language.label.toLowerCase().includes(normalizedQuery) ||
        language.code.includes(normalizedQuery),
    );
  }, [query]);

  const onContinue = async () => {
    if (!selectedLanguage || saving) return;
    setSaving(true);
    setSaveError(null);
    try {
      await updateProfile({
        platformLanguage: selectedLanguage,
        autoTranslateEnabled: true,
      });
      router.replace("/");
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setSaveError(en["edit.sessionExpired"]);
      } else {
        setSaveError(en["language.saveError"]);
      }
    } finally {
      setSaving(false);
    }
  };

  if (status !== "authenticated") {
    return (
      <div className="flex min-h-screen flex-col items-center justify-center gap-3 bg-background">
        <Image
          src="/images/together-logo.png"
          alt="Together"
          width={56}
          height={56}
          priority
          className="h-14 w-14 animate-pulse object-contain"
        />
        {status === "error" ? (
          <>
            <span className="text-sm text-muted">{en["language.connectionError"]}</span>
            <button
              type="button"
              onClick={() => void refresh()}
              className="rounded-full bg-primary px-4 py-1.5 text-sm text-white transition-colors hover:bg-primary-hover"
            >
              {en["search.tryAgain"]}
            </button>
          </>
        ) : (
          <span className="text-sm text-muted">{en["language.loading"]}</span>
        )}
      </div>
    );
  }

  if (profileStatus === "loading" || platformLanguage !== null) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-background">
        <span className="text-sm text-muted">{en["language.loading"]}</span>
      </div>
    );
  }

  if (profileStatus === "error") {
    return (
      <div className="flex min-h-screen flex-col items-center justify-center gap-3 bg-background px-4 text-center">
        <p className="text-sm text-muted">{en["language.loadError"]}</p>
        <button
          type="button"
          onClick={() => void loadProfile()}
          className="rounded-full bg-primary px-4 py-1.5 text-sm text-white transition-colors hover:bg-primary-hover"
        >
          {en["search.tryAgain"]}
        </button>
      </div>
    );
  }

  return (
    <main className="flex min-h-screen items-center justify-center bg-background px-4 py-10">
      <section className="w-full max-w-xl rounded-2xl border border-border bg-surface p-6 shadow-sm sm:p-8">
        <div className="flex flex-col items-center text-center">
          <Image
            src="/images/together-logo.png"
            alt="Together"
            width={48}
            height={48}
            priority
            className="h-12 w-12 object-contain"
          />
          <h1 className="mt-4 text-xl font-bold tracking-tight text-foreground">
            {en["language.chooseTitle"]}
          </h1>
          <p className="mt-2 max-w-md text-sm leading-6 text-muted">
            {en["language.description"]}
          </p>
        </div>

        <label className="mt-6 block">
          <span className="sr-only">{en["language.search"]}</span>
          <input
            type="search"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder={en["language.search"]}
            className="h-11 w-full rounded-lg border border-border bg-background px-4 text-sm text-foreground placeholder:text-muted-soft focus:border-primary focus:outline-none focus:ring-2 focus:ring-primary/20"
          />
        </label>

        <div className="mt-3 grid max-h-72 grid-cols-1 gap-2 overflow-y-auto sm:grid-cols-2">
          {languages.map((language) => {
            const selected = selectedLanguage === language.code;
            return (
              <button
                key={language.code}
                type="button"
                onClick={() => {
                  setSelectedLanguage(language.code);
                  setSaveError(null);
                }}
                aria-pressed={selected}
                className={`rounded-lg border px-4 py-3 text-left text-sm transition-colors ${
                  selected
                    ? "border-primary bg-primary/10 text-foreground"
                    : "border-border bg-background text-foreground hover:border-primary/50"
                }`}
              >
                <span className="font-medium">{language.label}</span>
                <span className="ml-2 text-muted-soft">{language.code}</span>
              </button>
            );
          })}
        </div>
        {languages.length === 0 ? (
          <p className="mt-4 text-sm text-muted">{en["language.noMatches"]}</p>
        ) : null}

        {saveError ? (
          <p className="mt-4 text-sm text-red-500" role="alert">
            {saveError}
          </p>
        ) : null}

        <button
          type="button"
          disabled={!selectedLanguage || saving}
          onClick={() => void onContinue()}
          className="mt-6 w-full rounded-full bg-primary px-5 py-2.5 text-sm font-medium text-white transition-colors hover:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:bg-primary"
        >
          {saving ? en["post.saving"] : en["language.continue"]}
        </button>
      </section>
    </main>
  );
}
