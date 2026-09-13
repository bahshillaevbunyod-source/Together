import { Header } from "./Header";
import { Sidebar } from "./Sidebar";
import { RightSidebar } from "./RightSidebar";

export function AppShell({ children }: { children: React.ReactNode }) {
  return (
    <div className="min-h-screen bg-background text-foreground">
      <Header />
      <div className="mx-auto flex w-full max-w-[1536px] gap-6 px-6 py-6">
        <Sidebar />
        <main className="min-h-[calc(100vh-7rem)] min-w-0 flex-1">{children}</main>
        <RightSidebar />
      </div>
    </div>
  );
}
