"use client";

import Image from "next/image";
import { NotificationsBell } from "./NotificationsBell";
import { HeaderUser } from "./HeaderUser";
import { GlobalSearch } from "./GlobalSearch";
import { useLanguage } from "@/lib/language-context";

export function Header() {
  const { t } = useLanguage();

  return (
    <header className="sticky top-0 z-30 w-full border-b border-border bg-surface pt-[env(safe-area-inset-top)]">
      <div className="app-header-bar mx-auto flex h-16 max-w-[1536px] items-center gap-2 pl-[max(1rem,env(safe-area-inset-left))] pr-[max(1rem,env(safe-area-inset-right))] sm:gap-4 sm:px-6">
        {/* Logo / wordmark */}
        <div className="flex min-w-0 shrink-0 items-center gap-2 lg:w-64 lg:gap-3">
          <Image
            src="/images/together-logo.png"
            alt="Together"
            width={40}
            height={40}
            priority
            className="h-10 w-10 object-contain"
          />
          <div className="hidden min-w-0 leading-tight sm:block">
            <div className="text-lg font-bold tracking-tight text-foreground">
              Together
            </div>
            <div className="text-xs text-muted">{t("header.tagline")}</div>
          </div>
        </div>

        {/* Search */}
        <GlobalSearch />

        {/* Actions */}
        <div className="flex shrink-0 items-center gap-2 sm:gap-3">
          <NotificationsBell />

          <HeaderUser />
        </div>
      </div>
    </header>
  );
}
