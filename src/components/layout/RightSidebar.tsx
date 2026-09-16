import { WorldMapCard } from "@/components/right-sidebar/WorldMapCard";
import { SuggestedPeople } from "@/components/right-sidebar/SuggestedPeople";

export function RightSidebar() {
  return (
    <aside className="hidden w-[400px] shrink-0 xl:block">
      <div className="sticky top-[5.5rem] flex flex-col gap-3">
        <WorldMapCard />
        <SuggestedPeople />
      </div>
    </aside>
  );
}
