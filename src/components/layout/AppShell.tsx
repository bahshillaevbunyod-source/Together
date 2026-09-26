import { Header } from "./Header";
import { MobileNavigation, Sidebar } from "./Sidebar";
import { RightSidebar } from "./RightSidebar";

export function AppShell({ children }: { children: React.ReactNode }) {
  return (
    <div className="min-h-dvh bg-background text-foreground">
      <Header />
      <div className="app-shell-cols mx-auto flex w-full max-w-[1536px] gap-6 pl-[max(1rem,env(safe-area-inset-left))] pr-[max(1rem,env(safe-area-inset-right))] pt-4 pb-[calc(5rem+env(safe-area-inset-bottom))] sm:px-6 sm:pt-6 lg:py-6">
        <Sidebar />
        <main className="min-h-[calc(100dvh-7rem)] min-w-0 flex-1">{children}</main>
        <RightSidebar />
      </div>
      <MobileNavigation />
    </div>
  );
}
