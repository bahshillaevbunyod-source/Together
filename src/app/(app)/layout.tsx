"use client";

import { useEffect } from "react";
import Image from "next/image";
import { useRouter } from "next/navigation";

import { AppShell } from "@/components/layout/AppShell";
import { useAuth } from "@/lib/auth-context";
import { RealtimeProvider } from "@/lib/realtime-context";

export default function AppGroupLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  const { status } = useAuth();
  const router = useRouter();

  useEffect(() => {
    // Only redirect once auth is definitively resolved — never while loading,
    // so there's no flicker or premature bounce.
    if (status === "unauthenticated") {
      router.replace("/login");
    }
  }, [status, router]);

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
