import Image from "next/image";
import { Plus } from "lucide-react";
import { stories } from "@/data/stories";

export function StoriesRow() {
  return (
    <section>
      <div className="flex gap-4 overflow-x-auto">
        {/* Add story */}
        <button
          type="button"
          className="flex w-16 shrink-0 flex-col items-center gap-2"
        >
          <span className="flex h-16 w-16 items-center justify-center rounded-full border-2 border-dashed border-border text-primary">
            <Plus className="h-6 w-6" />
          </span>
          <span className="text-xs text-muted">Add story</span>
        </button>

        {/* People stories */}
        {stories.map((story) => (
          <button
            key={story.id}
            type="button"
            className="flex w-16 shrink-0 flex-col items-center gap-2"
          >
            <span
              className={`h-16 w-16 rounded-full p-[2px] ${
                story.hasUnseenStory
                  ? "bg-gradient-to-tr from-[#EAF3FF] via-[#67B8FF] to-[#2F6BFF]"
                  : "bg-border"
              }`}
            >
              <span className="block h-full w-full rounded-full bg-surface p-[2px]">
                <Image
                  src={story.avatar}
                  alt={story.user}
                  width={60}
                  height={60}
                  className="h-full w-full rounded-full object-cover"
                />
              </span>
            </span>
            <span className="w-16 truncate text-center text-xs text-muted">
              {story.user}
            </span>
          </button>
        ))}
      </div>
    </section>
  );
}
