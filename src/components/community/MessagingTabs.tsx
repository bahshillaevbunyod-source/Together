"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

import { useLanguage, type TranslationKey } from "@/lib/language-context";

const TABS: { href: string; labelKey: TranslationKey }[] = [
  { href: "/messages", labelKey: "navigation.messages" },
  { href: "/groups", labelKey: "navigation.groups" },
  { href: "/channels", labelKey: "navigation.channels" },
  { href: "/calls", labelKey: "navigation.calls" },
];

/**
 * Compact segmented switch between direct messages, groups and channels. It is
 * the mobile entry point to Groups/Channels/Calls (the bottom bar stays unchanged)
 * and a quick switch on desktop.
 */
export function MessagingTabs() {
  const pathname = usePathname();
  const { t } = useLanguage();
  return (
    <nav
      aria-label={t("messages.sectionsAria")}
      className="flex rounded-full bg-background p-0.5"
    >
      {TABS.map(({ href, labelKey }) => {
        const active = pathname === href;
        return (
          <Link
            key={href}
            href={href}
            aria-current={active ? "page" : undefined}
            className={`flex h-9 min-w-0 flex-1 items-center justify-center rounded-full px-1 text-center text-[11px] font-medium transition-colors min-[360px]:text-xs sm:h-auto sm:px-2 sm:py-1 ${
              active
                ? "bg-surface text-foreground shadow-sm"
                : "text-muted hover:text-foreground"
            }`}
          >
            <span className="truncate">{t(labelKey)}</span>
          </Link>
        );
      })}
    </nav>
  );
}
