import Image from "next/image";
import { NotificationsBell } from "./NotificationsBell";
import { HeaderUser } from "./HeaderUser";
import { GlobalSearch } from "./GlobalSearch";

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
        <GlobalSearch />

        {/* Actions */}
        <div className="flex shrink-0 items-center gap-3">
          <NotificationsBell />

          <HeaderUser />
        </div>
      </div>
    </header>
  );
}
