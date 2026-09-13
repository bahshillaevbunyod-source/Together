import type { ApiPost } from "@/lib/api";
import type { Post } from "@/types/post";

/**
 * Neutral fallback avatar (a gray circle) used when a user has no avatar. It is
 * an inline data URI so it needs no asset file or remote host.
 */
const DEFAULT_AVATAR =
  "data:image/svg+xml;utf8," +
  encodeURIComponent(
    '<svg xmlns="http://www.w3.org/2000/svg" width="44" height="44"><circle cx="22" cy="22" r="22" fill="#d4d4d8"/></svg>',
  );

/**
 * Adapt a backend post into the `Post` view model the existing PostCard renders.
 * The visual shape is preserved: the secondary meta line shows the author's
 * handle (the backend has no post location), and `shares` is 0 (not tracked
 * server-side yet). Only image media is carried, since PostCard renders images.
 */
export function mapApiPost(api: ApiPost): Post {
  const images = api.media
    .filter((m) => m.type === "image")
    .sort((a, b) => a.sortOrder - b.sortOrder)
    .map((m) => ({
      type: "image" as const,
      src: m.url,
      alt: api.content ?? "Post image",
    }));

  return {
    id: api.id,
    author: {
      name: api.author.displayName,
      avatar: api.author.avatarUrl ?? DEFAULT_AVATAR,
    },
    createdAt: api.createdAt,
    location: `@${api.author.username}`,
    content: api.content ?? "",
    media: images,
    stats: {
      likes: api.likesCount,
      comments: api.commentsCount,
      shares: 0,
    },
    viewerState: {
      liked: api.likedByMe,
      saved: api.savedByMe,
    },
  };
}
