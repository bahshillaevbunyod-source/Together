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

type NavItem = {
  label: string;
  icon: LucideIcon;
  /** Route this item navigates to; items without one are not yet wired. */
  href?: string;
  badge?: string;
};

const navItems: NavItem[] = [
  { label: "Home", icon: Home, href: "/" },
  { label: "Discover", icon: Search },
  { label: "Messages", icon: MessageCircle, href: "/messages", badge: "3" },
  { label: "Calls", icon: Phone },
  { label: "Groups", icon: Users },
  { label: "Explore", icon: Compass },
  { label: "Events", icon: Calendar },
  { label: "Bookmarks", icon: Bookmark, href: "/bookmarks" },
  { label: "Profile", icon: User, href: "/profile" },
  { label: "Settings", icon: Settings, href: "/settings" },
];

export function Sidebar() {
  const pathname = usePathname();

  return (
    <aside className="hidden w-64 shrink-0 lg:block">
      <div className="sticky top-[5.5rem] flex h-[calc(100vh-7rem)] flex-col">
        <nav className="flex flex-col gap-1">
          {navItems.map(({ label, icon: Icon, href, badge }) => {
            const active = href ? pathname === href : false;
            const className = `flex items-center gap-3 rounded-xl px-3 py-2.5 text-sm font-medium transition-colors ${
              active
                ? "bg-primary-soft text-primary"
                : "text-muted hover:bg-primary-soft/60 hover:text-foreground"
            }`;
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

            return href ? (
              <Link key={label} href={href} className={className}>
                {inner}
              </Link>
            ) : (
              <button key={label} type="button" className={className}>
                {inner}
              </button>
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
              <h3 className="text-base font-semibold">Meet the World</h3>
              <p className="mt-1 text-xs leading-snug text-white/80">
                New people. New stories. A kinder world.
              </p>
              <button
                type="button"
                className="mt-3 inline-flex items-center gap-1.5 rounded-full bg-foreground px-3 py-1.5 text-xs font-medium text-white transition-opacity hover:opacity-90"
              >
                Explore Now
                <ArrowRight className="h-3.5 w-3.5" />
              </button>
            </div>
          </div>
        </div>
      </div>
    </aside>
  );
}
