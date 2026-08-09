import { useEffect, useState } from "react";

// Global clock — a single 1s interval shared by all ToolCards, replacing N
// independent setInterval calls (one per running tool). Components subscribe
// via useNow() and compute their own elapsed time from their own start ref.
let listeners: Array<() => void> = [];
let timerId: ReturnType<typeof setInterval> | null = null;

function subscribe(onChange: () => void) {
  listeners.push(onChange);
  if (!timerId) {
    timerId = setInterval(() => {
      for (const fn of listeners) fn();
    }, 1000);
  }
  return () => {
    listeners = listeners.filter(f => f !== onChange);
    if (listeners.length === 0 && timerId !== null) {
      clearInterval(timerId);
      timerId = null;
    }
  };
}

/**
 * Returns the current Unix timestamp in seconds, refreshed every 1s while
 * `active` (default true). Pass `active=false` to opt out of the shared
 * interval — e.g. a running-state timer that should only tick while running,
 * avoiding needless re-renders when idle.
 */
export function useNow(active = true): number {
  const [now, setNow] = useState(() => Math.floor(Date.now() / 1000));
  useEffect(() => {
    if (!active) return;
    return subscribe(() => setNow(Math.floor(Date.now() / 1000)));
  }, [active]);
  return now;
}
