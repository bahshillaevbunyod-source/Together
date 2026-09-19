"use client";

import Image from "next/image";
import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  ArrowRight,
  Bookmark,
  Calendar,
  Compass,
  Home,
  MessageCircle,
  Phone,
  Search,
  Settings,
  User,
  Users,
  type LucideIcon,
} from "lucide-react";

import { useLanguage, type TranslationKey } from "@/lib/language-context";

type NavItem = {
  labelKey: TranslationKey;
  icon: LucideIcon;
  /** Route this item navigates to; items without one are not yet wired. */
  href?: string;
  badge?: string;
};

const navItems: NavItem[] = [
  { labelKey: "navigation.home", icon: Home, href: "/" },
  { labelKey: "navigation.discover", icon: Search, href: "/discover" },
  { labelKey: "navigation.messages", icon: MessageCircle, href: "/messages" },
  { labelKey: "navigation.calls", icon: Phone },
  { labelKey: "navigation.groups", icon: Users },
  { labelKey: "navigation.explore", icon: Compass },
  { labelKey: "navigation.events", icon: Calendar },
  { labelKey: "navigation.bookmarks", icon: Bookmark, href: "/bookmarks" },
  { labelKey: "navigation.profile", icon: User, href: "/profile" },
  { labelKey: "navigation.settings", icon: Settings, href: "/settings" },
];

// The real destinations are shared by the desktop sidebar and mobile nav so
// responsive navigation cannot drift from the established route structure.
const primaryNavItems = navItems.filter((item) => item.href);

export function Sidebar() {
  const pathname = usePathname();
  const { t } = useLanguage();

  return (
    <aside className="hidden w-64 shrink-0 lg:block">
      <div className="sticky top-[5.5rem] flex h-[calc(100vh-7rem)] flex-col">
        <nav className="flex flex-col gap-1">
          {navItems.map(({ labelKey, icon: Icon, href, badge }) => {
            const label = t(labelKey);
            const active = href ? pathname === href : false;
            const base =
              "flex items-center gap-3 rounded-xl px-3 py-2.5 text-sm font-medium";
            const inner = (
              <>
                <Icon className="h-5 w-5 shrink-0" />
                <span>{label}</span>
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
              <button
                type="button"
                disabled
                aria-disabled="true"
                className="mt-3 inline-flex cursor-not-allowed items-center gap-1.5 rounded-full bg-foreground/80 px-3 py-1.5 text-xs font-medium text-white opacity-70"
              >
                {t("world.exploreNow")}
                <ArrowRight className="h-3.5 w-3.5" />
              </button>
            </div>
          </div>
        </div>
      </div>
    </aside>
  );
}

export function MobileNavigation() {
  const pathname = usePathname();
  const { t } = useLanguage();

  return (
    <nav
      aria-label={t("navigation.primary")}
      className="fixed inset-x-0 bottom-0 z-30 border-t border-border bg-surface/95 px-2 pb-[max(0.5rem,env(safe-area-inset-bottom))] pt-1.5 backdrop-blur lg:hidden"
    >
      <div className="mx-auto flex max-w-lg items-stretch justify-between">
        {primaryNavItems.map(({ labelKey, icon: Icon, href }) => {
          const label = t(labelKey);
          const active = pathname === href;
          return (
            <Link
              key={labelKey}
              href={href as string}
              aria-current={active ? "page" : undefined}
              className={`flex min-w-0 flex-1 flex-col items-center gap-0.5 rounded-xl px-1 py-1.5 text-[10px] font-medium transition-colors ${
                active
                  ? "bg-primary-soft text-primary"
                  : "text-muted hover:bg-primary-soft/60 hover:text-foreground"
              }`}
            >
              <Icon className="h-5 w-5 shrink-0" />
              <span className="truncate">{label}</span>
            </Link>
          );
        })}
      </div>
    </nav>
  );
}
