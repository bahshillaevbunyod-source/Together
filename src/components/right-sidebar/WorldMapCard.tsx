import Image from "next/image";
import { ArrowRight } from "lucide-react";

export function WorldMapCard() {
  return (
    <section className="mr-1 mt-2 rounded-2xl border border-border bg-surface p-5 shadow-sm">
      <div className="flex items-center justify-between gap-2">
        <h2 className="text-sm font-semibold text-foreground">
          People online around the world
        </h2>
        <span className="flex shrink-0 items-center gap-1.5 text-xs text-muted">
          <span className="h-2 w-2 rounded-full bg-emerald-500" />
          12,436 online
        </span>
      </div>

      <div className="relative mt-5 aspect-[7/5] overflow-hidden rounded-xl border border-border bg-background">
        <Image
          src="/images/right-sidebar/global-people-map.png"
          alt="People online around the world"
          fill
          sizes="340px"
          className="object-cover"
        />
        <button
          type="button"
          className="absolute bottom-4 left-4 flex items-center gap-1.5 rounded-full border border-border bg-surface px-3.5 py-2 text-xs font-medium text-foreground shadow-sm transition-colors hover:bg-background"
        >
          Explore Map
          <ArrowRight className="h-3.5 w-3.5" />
        </button>
      </div>
    </section>
  );
}
