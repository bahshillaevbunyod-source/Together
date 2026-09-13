import type { Post } from "@/types/post";

// Mock feed data. Swap this module for an API response later — the shape
// matches the `Post` type, so `Feed` / `PostCard` need no changes.
export const posts: Post[] = [
  {
    id: "1",
    author: {
      name: "Alex Morozov",
      avatar: "/images/avatars/alex-morozov.webp",
      flag: "🇲🇪",
    },
    createdAt: new Date(Date.now() - 2 * 60 * 60 * 1000).toISOString(),
    location: "Kotor, Montenegro",
    content: "Sunsets like this make every mile worth it.",
    hashtags: ["#together", "#travel", "#montenegro"],
    media: [
      {
        type: "image",
        src: "/images/posts/alex-montenegro.webp",
        alt: "Sunset over the Montenegro coast",
      },
    ],
    stats: { likes: 2400, comments: 124, shares: 86 },
    viewerState: { liked: false, saved: false },
  },
];
