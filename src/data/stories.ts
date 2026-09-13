import type { Story } from "@/types/story";

// Mock stories data. Swap this module for an API response later — the shape
// matches the `Story` type, so `StoriesRow` needs no changes.
export const stories: Story[] = [
  { id: "1", user: "Yuki", avatar: "/images/avatars/yuki-tanaka.webp", hasUnseenStory: true },
  { id: "2", user: "Timur", avatar: "/images/avatars/timur-yusupov.webp", hasUnseenStory: true },
  { id: "3", user: "David", avatar: "/images/avatars/david-okafor.webp", hasUnseenStory: true },
  { id: "4", user: "Priya", avatar: "/images/avatars/priya-sharma.webp", hasUnseenStory: true },
  { id: "5", user: "Camila", avatar: "/images/avatars/camila-souza.webp", hasUnseenStory: true },
  { id: "6", user: "Omar", avatar: "/images/avatars/omar-al-farsi.webp", hasUnseenStory: true },
  { id: "7", user: "Sophie", avatar: "/images/avatars/sophie-laurent.webp", hasUnseenStory: true },
];
