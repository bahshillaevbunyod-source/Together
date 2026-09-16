"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  Bookmark,
  Home,
  MessageCircle,
  Search,
  Settings,
  User,
  type LucideIcon,
} from "lucide-react";

type NavItem = {
  label: string;
  icon: LucideIcon;
  href: string;
};

const navItems: NavItem[] = [
  { label: "Home", icon: Home, href: "/" },
  { label: "Discover", icon: Search, href: "/discover" },
  { label: "Messages", icon: MessageCircle, href: "/messages" },
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
          {navItems.map(({ label, icon: Icon, href }) => {
            const active = pathname === href;
            const className = `flex items-center gap-3 rounded-xl px-3 py-2.5 text-sm font-medium transition-colors ${
              active
                ? "bg-primary-soft text-primary"
                : "text-muted hover:bg-primary-soft/60 hover:text-foreground"
            }`;
            const inner = (
              <>
                <Icon className="h-5 w-5 shrink-0" />
                <span>{label}</span>
              </>
            );

            return (
              <Link key={label} href={href} className={className}>
                {inner}
              </Link>
            );
          })}
        </nav>
      </div>
    </aside>
  );
}
