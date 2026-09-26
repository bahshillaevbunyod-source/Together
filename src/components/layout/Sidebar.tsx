"use client";

import Image from "next/image";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useRef, useState } from "react";
import {
  ArrowRight,
  Bookmark,
  Calendar,
  Globe2,
  Home,
  Megaphone,
  MessageCircle,
  MoreHorizontal,
  Phone,
  Search,
  Settings,
  User,
  Users,
  X,
  type LucideIcon,
} from "lucide-react";

import { useLanguage, type TranslationKey } from "@/lib/language-context";

type NavItem = {
  labelKey: TranslationKey;
  icon: LucideIcon;
  /** Route this item navigates to; items without one are not yet wired. */
  href?: string;
  badge?: string;
  /**
   * Mobile placement: "primary" items are tabs in the bottom bar; "more" items
   * live in the bottom bar's More sheet. Every routed item is reachable.
   */
  mobile: "primary" | "more";
  /** Also active on nested routes (e.g. /events/[eventId]). */
  matchSubroutes?: boolean;
};

const navItems: NavItem[] = [
  { labelKey: "navigation.home", icon: Home, href: "/", mobile: "primary" },
  { labelKey: "navigation.discover", icon: Search, href: "/discover", mobile: "more" },
  { labelKey: "navigation.messages", icon: MessageCircle, href: "/messages", mobile: "primary" },
  { labelKey: "navigation.calls", icon: Phone, href: "/calls", mobile: "more" },
  { labelKey: "navigation.groups", icon: Users, href: "/groups", mobile: "more" },
  { labelKey: "navigation.channels", icon: Megaphone, href: "/channels", mobile: "more" },
  { labelKey: "navigation.world", icon: Globe2, href: "/world", matchSubroutes: true, mobile: "primary" },
  { labelKey: "navigation.events", icon: Calendar, href: "/events", matchSubroutes: true, mobile: "primary" },
  { labelKey: "navigation.bookmarks", icon: Bookmark, href: "/bookmarks", mobile: "more" },
  { labelKey: "navigation.profile", icon: User, href: "/profile", mobile: "more" },
  { labelKey: "navigation.settings", icon: Settings, href: "/settings", mobile: "more" },
];

// The real destinations are shared by the desktop sidebar and mobile nav so
// responsive navigation cannot drift from the established route structure.
const primaryNavItems = navItems.filter((item) => item.href && item.mobile === "primary");
const moreNavItems = navItems.filter((item) => item.href && item.mobile === "more");

function isActive(pathname: string, item: NavItem): boolean {
  if (!item.href) return false;
  if (pathname === item.href) return true;
  return Boolean(item.matchSubroutes) && pathname.startsWith(`${item.href}/`);
}

export function Sidebar() {
  const pathname = usePathname();
  const { t } = useLanguage();

  return (
    <aside className="hidden w-64 shrink-0 lg:block">
      <div className="sticky top-[5.5rem] flex h-[calc(100vh-7rem)] flex-col">
        <nav className="flex flex-col gap-1">
          {navItems.map((item) => {
            const { labelKey, icon: Icon, href, badge } = item;
            const label = t(labelKey);
            const active = isActive(pathname, item);
            const base =
              "app-nav-row flex items-center gap-3 rounded-xl px-3 py-2.5 text-sm font-medium";
            const inner = (
              <>
                <Icon className="h-5 w-5 shrink-0" />
                <span className="app-nav-label">{label}</span>
                {badge ? (
                  <span className="ml-auto flex h-5 min-w-5 items-center justify-center rounded-full bg-red-500 px-1.5 text-xs font-semibold text-white">
                    {badge}
                  </span>
                ) : null}
              </>
            );

            // Items without a route are not built yet: keep them visible for
            // layout but honestly non-interactive (never a control that silently
            // does nothing).
            if (!href) {
              return (
                <span
                  key={labelKey}
                  aria-disabled="true"
                  className={`${base} cursor-not-allowed text-muted opacity-50`}
                >
                  {inner}
                </span>
              );
            }

            return (
              <Link
                key={labelKey}
                href={href}
                className={`${base} transition-colors ${
                  active
                    ? "bg-primary-soft text-primary"
                    : "text-muted hover:bg-primary-soft/60 hover:text-foreground"
                }`}
              >
                {inner}
              </Link>
            );
          })}
        </nav>

        {/* Meet the World */}
        <div className="mt-auto pt-4">
          <div className="relative overflow-hidden rounded-2xl p-4 text-white">
            <Image
              src="/images/meet-the-world.webp"
              alt=""
              fill
              sizes="256px"
              className="object-cover"
            />
            <div className="absolute inset-0 bg-gradient-to-t from-black/60 to-black/20" />
            <div className="relative">
              <h3 className="text-base font-semibold">{t("world.meetTheWorld")}</h3>
              <p className="mt-1 text-xs leading-snug text-white/80">
                {t("world.meetDescription")}
              </p>
              <Link
                href="/world"
                className="mt-3 inline-flex items-center gap-1.5 rounded-full bg-foreground/80 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-foreground"
              >
                {t("world.exploreNow")}
                <ArrowRight className="h-3.5 w-3.5 rtl:rotate-180" />
              </Link>
            </div>
          </div>
        </div>
      </div>
    </aside>
  );
}


const tabClass = (active: boolean) =>
  `flex min-h-12 min-w-0 flex-1 flex-col items-center justify-center gap-0.5 rounded-xl px-0 py-1 text-[11px] font-medium transition-colors min-[360px]:text-xs ${
    active ? "text-primary" : "text-muted hover:text-foreground"
  }`;

/**
 * Mobile bottom bar: four primary destinations plus a More tab that opens a
 * sheet with every other real destination. Fixed above the safe area; content
 * clearance is handled by AppShell's bottom padding.
 */
export function MobileNavigation() {
  const pathname = usePathname();
  const { t } = useLanguage();
  const [moreOpen, setMoreOpen] = useState(false);
  const moreButtonRef = useRef<HTMLButtonElement | null>(null);
  const sheetRef = useRef<HTMLDivElement | null>(null);
  const moreActive = moreNavItems.some((item) => isActive(pathname, item));

  // Close the sheet whenever navigation happens.
  useEffect(() => {
    setMoreOpen(false);
  }, [pathname]);

  // Escape closes; focus moves into the sheet and back to More on close;
  // background scroll is locked only while the sheet is open.
  useEffect(() => {
    if (!moreOpen) return;
    const button = moreButtonRef.current;
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    sheetRef.current?.querySelector<HTMLElement>("a,button")?.focus();
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setMoreOpen(false);
    };
    document.addEventListener("keydown", onKey);
    return () => {
      document.body.style.overflow = previousOverflow;
      document.removeEventListener("keydown", onKey);
      button?.focus();
    };
  }, [moreOpen]);

  return (
    <>
      <nav
        aria-label={t("navigation.primary")}
        className="fixed inset-x-0 bottom-0 z-30 border-t border-border bg-surface/95 pb-[env(safe-area-inset-bottom)] pl-[max(0.25rem,env(safe-area-inset-left))] pr-[max(0.25rem,env(safe-area-inset-right))] pt-1 backdrop-blur lg:hidden"
      >
        <div className="app-mobile-nav mx-auto flex max-w-lg items-stretch justify-between gap-0 pb-1">
          {primaryNavItems.map((item) => {
            const { labelKey, icon: Icon, href } = item;
            const active = isActive(pathname, item);
            return (
              <Link key={labelKey} href={href as string} aria-current={active ? "page" : undefined} className={tabClass(active)}>
                <span className={`flex h-7 w-12 items-center justify-center rounded-full transition-colors ${active ? "bg-primary-soft" : ""}`}>
                  <Icon className="h-5 w-5 shrink-0" aria-hidden />
                </span>
                <span className="line-clamp-2 max-w-full hyphens-auto break-words text-center leading-tight">{t(labelKey)}</span>
              </Link>
            );
          })}
          <button
            ref={moreButtonRef}
            type="button"
            onClick={() => setMoreOpen((open) => !open)}
            aria-expanded={moreOpen}
            aria-haspopup="dialog"
            aria-controls="mobile-more-sheet"
            className={tabClass(moreActive || moreOpen)}
          >
            <span className={`flex h-7 w-12 items-center justify-center rounded-full transition-colors ${moreActive || moreOpen ? "bg-primary-soft" : ""}`}>
              <MoreHorizontal className="h-5 w-5 shrink-0" aria-hidden />
            </span>
            <span className="line-clamp-2 max-w-full hyphens-auto break-words text-center leading-tight">{t("navigation.more")}</span>
          </button>
        </div>
      </nav>

      {moreOpen ? (
        <div className="fixed inset-0 z-50 flex items-end bg-black/40 lg:hidden" onClick={() => setMoreOpen(false)}>
          <div
            id="mobile-more-sheet"
            ref={sheetRef}
            role="dialog"
            aria-modal="true"
            aria-label={t("navigation.more")}
            onClick={(e) => e.stopPropagation()}
            className="max-h-[85dvh] w-full overflow-y-auto rounded-t-3xl border-t border-border bg-surface pb-[max(1rem,env(safe-area-inset-bottom))] pl-[max(1rem,env(safe-area-inset-left))] pr-[max(1rem,env(safe-area-inset-right))] pt-2 shadow-xl"
          >
            <div className="mx-auto mb-2 h-1 w-10 rounded-full bg-border" aria-hidden />
            <div className="mb-2 flex items-center justify-between">
              <h2 className="text-base font-semibold text-foreground">{t("navigation.more")}</h2>
              <button
                type="button"
                onClick={() => setMoreOpen(false)}
                aria-label={t("profile.close")}
                className="flex h-11 w-11 items-center justify-center rounded-full text-muted transition-colors hover:bg-background hover:text-foreground"
              >
                <X className="h-5 w-5" aria-hidden />
              </button>
            </div>
            <ul className="grid grid-cols-3 gap-2">
              {moreNavItems.map((item) => {
                const { labelKey, icon: Icon, href } = item;
                const active = isActive(pathname, item);
                return (
                  <li key={labelKey}>
                    <Link
                      href={href as string}
                      aria-current={active ? "page" : undefined}
                      onClick={() => setMoreOpen(false)}
                      className={`flex min-h-20 flex-col items-center justify-center gap-1.5 rounded-2xl px-2 py-3 text-center text-xs font-medium transition-colors ${
                        active ? "bg-primary-soft text-primary" : "bg-background text-foreground hover:bg-primary-soft/60"
                      }`}
                    >
                      <Icon className="h-6 w-6 shrink-0" aria-hidden />
                      <span className="line-clamp-2 w-full break-words leading-tight">{t(labelKey)}</span>
                    </Link>
                  </li>
                );
              })}
            </ul>
          </div>
        </div>
      ) : null}
    </>
  );
}
