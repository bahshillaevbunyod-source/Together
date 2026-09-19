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
};

export type StoryPage = {
  items: Story[];
  nextCursor: string;
};
