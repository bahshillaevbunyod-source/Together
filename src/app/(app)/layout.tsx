"use client";

import { useCallback, useEffect, useState } from "react";
import Image from "next/image";
import { useRouter } from "next/navigation";

import { AppShell } from "@/components/layout/AppShell";
import { useAuth } from "@/lib/auth-context";
import { getProfile } from "@/lib/api";
import { RealtimeProvider } from "@/lib/realtime-context";

type ProfileStatus = "loading" | "ready" | "error";

export default function AppGroupLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  const { status, refresh } = useAuth();
  const router = useRouter();
  const [profileStatus, setProfileStatus] = useState<ProfileStatus>("loading");
  const [preferredLanguage, setPreferredLanguage] = useState<string | null>(null);

  const loadProfile = useCallback(async (signal?: AbortSignal) => {
    setProfileStatus("loading");
    try {
      const profile = await getProfile(signal);
      setPreferredLanguage(profile.preferredLanguage);
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
    if (status === "authenticated" && profileStatus === "ready" && preferredLanguage === null) {
      router.replace("/language");
    }
  }, [preferredLanguage, profileStatus, router, status]);

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
            <span className="text-sm text-muted">Together couldn’t connect.</span>
            <button
              type="button"
              onClick={() => void refresh()}
              className="rounded-full bg-primary px-4 py-1.5 text-sm text-white transition-colors hover:bg-primary-hover"
            >
              Retry
            </button>
          </>
        ) : (
          <span className="text-sm text-muted">Loading Together…</span>
        )}
      </div>
    );
  }

  if (profileStatus === "error") {
    return (
      <div className="flex min-h-screen flex-col items-center justify-center gap-3 bg-background">
        <span className="text-sm text-muted">Together couldn’t load your language preferences.</span>
        <button
          type="button"
          onClick={() => void loadProfile()}
          className="rounded-full bg-primary px-4 py-1.5 text-sm text-white transition-colors hover:bg-primary-hover"
        >
          Retry
        </button>
      </div>
    );
  }

  if (profileStatus !== "ready" || preferredLanguage === null) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-background">
        <span className="text-sm text-muted">Loading Together…</span>
      </div>
    );
  }

  return (
    <RealtimeProvider>
      <AppShell>{children}</AppShell>
    </RealtimeProvider>
  );
}
