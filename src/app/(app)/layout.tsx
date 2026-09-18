"use client";

import { useCallback, useEffect, useState } from "react";
import Image from "next/image";
import { useRouter } from "next/navigation";

import { AppShell } from "@/components/layout/AppShell";
import { useAuth } from "@/lib/auth-context";
import { getProfile } from "@/lib/api";
import { LanguageProvider } from "@/lib/language-context";
import { RealtimeProvider } from "@/lib/realtime-context";
// Pre-AppShell gate states render before the LanguageProvider mounts, so they
// use the canonical English dictionary (English fallback + LTR).
import en from "@/lib/i18n/locales/en";

type ProfileStatus = "loading" | "ready" | "error";

export default function AppGroupLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  const { status, refresh } = useAuth();
  const router = useRouter();
  const [profileStatus, setProfileStatus] = useState<ProfileStatus>("loading");
  const [platformLanguage, setPlatformLanguage] = useState<string | null>(null);

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
    // Only redirect once auth is definitively resolved — never while loading,
    // so there's no flicker or premature bounce.
    if (status === "unauthenticated") {
      router.replace("/login");
    }
  }, [status, router]);

  useEffect(() => {
    if (status !== "authenticated") return;
    const controller = new AbortController();
    void loadProfile(controller.signal);
    return () => controller.abort();
  }, [loadProfile, status]);

  useEffect(() => {
    if (status === "authenticated" && profileStatus === "ready" && platformLanguage === null) {
      router.replace("/language");
    }
  }, [platformLanguage, profileStatus, router, status]);

  // While loading, or during the redirect for an unauthenticated user, don't
  // render the app shell (avoids flashing protected UI).
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

  if (profileStatus === "error") {
    return (
      <div className="flex min-h-screen flex-col items-center justify-center gap-3 bg-background">
        <span className="text-sm text-muted">{en["language.loadError"]}</span>
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

  if (profileStatus !== "ready" || platformLanguage === null) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-background">
        <span className="text-sm text-muted">{en["language.loading"]}</span>
      </div>
    );
  }

  return (
    <LanguageProvider platformLanguage={platformLanguage}>
      <RealtimeProvider>
        <AppShell>{children}</AppShell>
      </RealtimeProvider>
    </LanguageProvider>
  );
}
