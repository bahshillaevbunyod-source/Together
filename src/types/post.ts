export type PostAuthor = {
  name: string;
  avatar: string;
  /** Optional country flag emoji shown next to the name. */
  flag?: string;
};

export type PostMedia = {
  type: "image";
  src: string;
  alt: string;
};

export type PostStats = {
  likes: number;
  comments: number;
  shares: number;
};

export type PostViewerState = {
  liked: boolean;
  saved: boolean;
};

export type Post = {
  id: string;
  author: PostAuthor;
  /** ISO 8601 timestamp — rendered as a relative label. */
  createdAt: string;
  location: string;
  content: string;
  hashtags?: string[];
  media: PostMedia[];
  stats: PostStats;
  viewerState: PostViewerState;
  /** Translation of `content` for the viewer (null when none). */
  translatedContent?: string | null;
  sourceLanguage?: string | null;
  targetLanguage?: string | null;
};
