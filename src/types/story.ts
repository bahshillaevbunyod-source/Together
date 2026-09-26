export type StoryMediaType = "image" | "video" | string;

export type StoryAuthor = {
  id: string;
  username: string;
  displayName: string;
  avatarUrl: string | null;
};

export type StoryMedia = {
  type: StoryMediaType;
  url: string;
  mimeType: string;
  width: number | null;
  height: number | null;
  durationMs: number | null;
};

export type Story = {
  id: string;
  author: StoryAuthor;
  media: StoryMedia;
  createdAt: string;
  viewed: boolean;
  /** The current user's like on this story. */
  likedByMe: boolean;
  /** Other people's views; present only on the current user's own stories. */
  viewCount?: number;
};

export type StoryViewer = {
  user: StoryAuthor;
  viewedAt: string;
  liked: boolean;
};

export type StoryViewerPage = {
  items: StoryViewer[];
  total: number;
  nextCursor: string;
};

export type StoryPage = {
  items: Story[];
  nextCursor: string;
};
