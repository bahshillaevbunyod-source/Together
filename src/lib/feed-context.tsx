"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from "react";

import { getFeed } from "@/lib/api";
import { mapApiPost } from "@/lib/map-post";
import type { Post } from "@/types/post";

export type FeedStatus = "loading" | "ready" | "error";

interface FeedState {
  posts: Post[];
  status: FeedStatus;
  nextCursor: string;
  loadingMore: boolean;
  reload: () => void;
  loadMore: () => void;
  /** Insert a freshly created post at the top of the feed (no reload). */
  prependPost: (post: Post) => void;
  /** Replace an existing post in place (e.g. after an edit). */
  replacePost: (post: Post) => void;
  /** Remove a post from the feed (e.g. after a delete). */
  removePost: (id: string) => void;
}

const FeedContext = createContext<FeedState | undefined>(undefined);

export function FeedProvider({ children }: { children: ReactNode }) {
  const [posts, setPosts] = useState<Post[]>([]);
  const [status, setStatus] = useState<FeedStatus>("loading");
  const [nextCursor, setNextCursor] = useState("");
  const [loadingMore, setLoadingMore] = useState(false);

  const loadFirstPage = useCallback((signal?: AbortSignal) => {
    setStatus("loading");
    getFeed({}, signal)
      .then((page) => {
        setPosts(page.items.map(mapApiPost));
        setNextCursor(page.nextCursor);
        setStatus("ready");
      })
      .catch((err) => {
        if (err instanceof DOMException && err.name === "AbortError") return;
        setStatus("error");
      });
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    loadFirstPage(controller.signal);
    return () => controller.abort();
  }, [loadFirstPage]);

  const loadMore = useCallback(() => {
    setNextCursor((cursor) => {
      if (!cursor) return cursor;
      setLoadingMore(true);
      getFeed({ cursor })
        .then((page) => {
          setPosts((prev) => [...prev, ...page.items.map(mapApiPost)]);
          setNextCursor(page.nextCursor);
        })
        .catch(() => {
          // Keep what we have; the button stays available to retry.
        })
        .finally(() => setLoadingMore(false));
      return cursor;
    });
  }, []);

  const prependPost = useCallback((post: Post) => {
    setPosts((prev) => [post, ...prev]);
  }, []);

  const replacePost = useCallback((post: Post) => {
    setPosts((prev) => prev.map((p) => (p.id === post.id ? post : p)));
  }, []);

  const removePost = useCallback((id: string) => {
    setPosts((prev) => prev.filter((p) => p.id !== id));
  }, []);

  const reload = useCallback(() => loadFirstPage(), [loadFirstPage]);

  return (
    <FeedContext.Provider
      value={{
        posts,
        status,
        nextCursor,
        loadingMore,
        reload,
        loadMore,
        prependPost,
        replacePost,
        removePost,
      }}
    >
      {children}
    </FeedContext.Provider>
  );
}

export function useFeed(): FeedState {
  const ctx = useContext(FeedContext);
  if (ctx === undefined) {
    throw new Error("useFeed must be used within a FeedProvider");
  }
  return ctx;
}
