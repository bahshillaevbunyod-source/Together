import { StoriesRow } from "@/components/feed/StoriesRow";
import { Composer } from "@/components/feed/Composer";
import { Feed } from "@/components/feed/Feed";
import { FeedProvider } from "@/lib/feed-context";

export default function Home() {
  return (
    <div className="flex flex-col gap-4">
      <StoriesRow />
      <FeedProvider>
        <Composer />
        <Feed />
      </FeedProvider>
    </div>
  );
}
