import Image from "next/image";
import { Plus, Search } from "lucide-react";
import { NotificationsBell } from "./NotificationsBell";
import { HeaderUser } from "./HeaderUser";

export function Header() {
  return (
    <header className="sticky top-0 z-30 h-16 w-full border-b border-border bg-surface">
      <div className="mx-auto flex h-16 max-w-[1536px] items-center gap-4 px-6">
        {/* Logo / wordmark */}
        <div className="flex w-64 shrink-0 items-center gap-3">
          <Image
            src="/images/together-logo.png"
            alt="Together"
            width={40}
            height={40}
            priority
            className="h-10 w-10 object-contain"
          />
          <div className="leading-tight">
            <div className="text-lg font-bold tracking-tight text-foreground">
              Together
            </div>
            <div className="text-xs text-muted">Different people. One world.</div>
          </div>
        </div>

        {/* Search */}
        <div className="flex flex-1 justify-center">
          <div className="relative w-full max-w-[560px]">
            <Search className="pointer-events-none absolute left-4 top-1/2 h-5 w-5 -translate-y-1/2 text-muted-soft" />
            <input
              type="text"
              placeholder="Search people, places, interests…"
              className="h-11 w-full rounded-full border border-border bg-background pl-11 pr-16 text-sm text-foreground placeholder:text-muted-soft focus:border-primary focus:outline-none focus:ring-2 focus:ring-primary/20"
            />
            <span className="absolute right-3 top-1/2 -translate-y-1/2 rounded-md border border-border bg-surface px-2 py-0.5 text-xs text-muted-soft">
              Ctrl K
            </span>
          </div>
        </div>

        {/* Actions */}
        <div className="flex shrink-0 items-center gap-3">
          <button
            type="button"
            className="flex items-center gap-2 rounded-full bg-primary px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-primary-hover"
          >
            <Plus className="h-4 w-4" />
            Create
          </button>

          <NotificationsBell />

          <HeaderUser />
        </div>
      </div>
    </header>
  );
}
