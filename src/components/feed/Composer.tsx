"use client";

import { useState } from "react";
import {
  BarChart3,
  ChevronDown,
  Globe,
  Image as ImageIcon,
  Mic,
  Smile,
  Video,
  type LucideIcon,
} from "lucide-react";
import { createPost } from "@/lib/api";
import { mapApiPost } from "@/lib/map-post";
import { useFeed } from "@/lib/feed-context";

type Action = {
  label: string;
  icon: LucideIcon;
  color: string;
};

const actions: Action[] = [
  { label: "Photo", icon: ImageIcon, color: "text-emerald-500" },
  { label: "Video", icon: Video, color: "text-rose-500" },
  { label: "Voice", icon: Mic, color: "text-violet-500" },
  { label: "Poll", icon: BarChart3, color: "text-sky-500" },
  { label: "Feeling", icon: Smile, color: "text-amber-500" },
];

type Visibility = "everyone" | "friends" | "private";

const visibilityLabels: Record<Visibility, string> = {
  everyone: "Everyone",
  friends: "Friends",
  private: "Only me",
};

// Map the composer's audience choice to the backend's visibility values.
const backendVisibility: Record<Visibility, string> = {
  everyone: "public",
  friends: "followers",
  private: "private",
};

type ComposerDraft = {
  content: string;
  visibility: Visibility;
  media: string[];
  feeling: string | null;
};

const emptyDraft: ComposerDraft = {
  content: "",
  visibility: "everyone",
  media: [],
  feeling: null,
};

export function Composer() {
  const { prependPost } = useFeed();
  const [draft, setDraft] = useState<ComposerDraft>(emptyDraft);
  const [posting, setPosting] = useState(false);

  const canPost =
    !posting &&
    (draft.content.trim().length > 0 || draft.media.length > 0);

  const submitPost = async () => {
    const content = draft.content.trim();
    if (!content || posting) return; // media upload not wired yet: text required

    setPosting(true);
    try {
      const created = await createPost({
        content,
        visibility: backendVisibility[draft.visibility],
      });
      prependPost(mapApiPost(created));
      setDraft(emptyDraft);
    } catch {
      // Keep the draft so the user can retry.
    } finally {
      setPosting(false);
    }
  };

  return (
    <section className="rounded-2xl border border-border bg-surface p-4 shadow-sm">
      <div className="flex items-center gap-3">
        <span className="h-10 w-10 shrink-0 rounded-full bg-background" />
        <input
          type="text"
          value={draft.content}
          onChange={(e) =>
            setDraft((d) => ({ ...d, content: e.target.value }))
          }
          placeholder="What's on your mind?"
          className="h-11 flex-1 rounded-full bg-background px-4 text-sm text-foreground placeholder:text-muted-soft focus:outline-none"
        />
      </div>

      <div className="mt-4 flex flex-wrap items-center gap-1 border-t border-border pt-3">
        {actions.map(({ label, icon: Icon, color }) => (
          <button
            key={label}
            type="button"
            className="flex items-center gap-2 rounded-lg px-3 py-2 text-sm font-medium text-muted transition-colors hover:bg-background"
          >
            <Icon className={`h-4 w-4 ${color}`} />
            {label}
          </button>
        ))}

        <div className="ml-auto flex items-center gap-2">
          <button
            type="button"
            className="flex items-center gap-1.5 rounded-full border border-border px-3 py-1.5 text-sm text-muted transition-colors hover:bg-background"
          >
            <Globe className="h-4 w-4" />
            {visibilityLabels[draft.visibility]}
            <ChevronDown className="h-4 w-4 text-muted-soft" />
          </button>
          <button
            type="button"
            onClick={submitPost}
            disabled={!canPost}
            className="rounded-full bg-primary px-5 py-1.5 text-sm font-medium text-white transition-colors hover:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:bg-primary"
          >
            {posting ? "Posting…" : "Post"}
          </button>
        </div>
      </div>
    </section>
  );
}
