import type { SessionMeta } from "./types";

export function sessionTitle(
  session: SessionMeta,
  fallback: string,
): string {
  return session.title || session.preview || fallback;
}

export function sessionTime(ms: number): string {
  const d = new Date(ms);
  const now = new Date();
  const opts: Intl.DateTimeFormatOptions = { month: "short", day: "numeric" };
  // Add year when the session is from a different year than now.
  if (d.getFullYear() !== now.getFullYear()) opts.year = "numeric";
  return d.toLocaleDateString([], opts);
}
