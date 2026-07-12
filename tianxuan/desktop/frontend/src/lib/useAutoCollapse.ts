import { useCallback, useRef, useState } from "react";
import { useCompact } from "../hooks/useCompact";

/**
 * useAutoCollapse — shared state machine for auto-expand/collapse widgets.
 *
 * Pattern: user can toggle manually; when they haven't touched it, the
 * widget auto-opens when `hasRunning` is true and auto-closes otherwise.
 * In compact mode, default is always collapsed regardless of running.
 *
 * Returns { open, toggleOpen } — pass `open` to useGSAPCollapse and
 * bind `toggleOpen` to the click handler.
 */
export function useAutoCollapse(hasRunning: boolean) {
  const compact = useCompact();
  const [openState, setOpenState] = useState(false);

  // Track user override as a ref so we don't lose it across re-renders
  // when hasRunning flips. (Reasonix pattern: prevents auto-close from
  // overriding a user's explicit open.)
  const userOverridden = useRef(false);

  const open = userOverridden.current
    ? openState
    : compact
      ? false
      : hasRunning;

  const toggleOpen = useCallback(() => {
    userOverridden.current = true;
    setOpenState((v) => !v);
  }, []);

  return { open, toggleOpen };
}
