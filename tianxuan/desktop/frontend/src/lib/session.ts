import type { SessionMeta } from "./types";

export function sessionTitle(
  session: SessionMeta,
  fallback: string,
): string {
  return session.title || session.preview || fallback;
}

export function sessionTime(ms: number): string {
  return new Date(ms).toLocaleDateString([], {
    month: "short",
    day: "numeric",
  });
}
