import type { ApiNotification } from "@/lib/api";

/**
 * Returns only destinations backed by the current notification contract. A
 * missing actor/object deliberately produces no link: the row can still be
 * marked read without taking the user to a malformed or unavailable URL.
 */
export function notificationDestination(
  notification: ApiNotification,
): string | null {
  switch (notification.type) {
    case "follow":
      return notification.actor?.username
        ? `/u/${encodeURIComponent(notification.actor.username)}`
        : null;
    case "follow_request":
      // The request inbox belongs to the current user's profile. The query is
      // consumed by the profile page and leaves public accounts on the normal
      // profile view, which is a safe fallback for a stale request event.
      return "/profile?followRequests=1";
    case "post_like":
    case "post_comment":
      return notification.postId
        ? `/post/${encodeURIComponent(notification.postId)}`
        : null;
    default:
      return null;
  }
}

/** Notify mounted notification surfaces that server-side read state changed. */
export function notifyNotificationReadStateChanged(): void {
  if (typeof window !== "undefined") {
    window.dispatchEvent(new Event("together:notification-read-state-changed"));
  }
}
