import { useEffect, useRef, useState } from "react";
import { prefersReducedMotion } from "./gsapAnimations";

// useMountTransition keeps a conditionally-rendered overlay mounted long enough
// to play an exit animation. Callers flip `open`; the hook returns whether the
// node should still be in the DOM (`mounted`) and its transition `status`,
// which the component maps to a `data-state` attribute for CSS enter/exit.
export type MountStatus = "open" | "closing";

export function useMountTransition(
  open: boolean,
  duration: number,
): { mounted: boolean; status: MountStatus } {
  const [mounted, setMounted] = useState(open);
  const [status, setStatus] = useState<MountStatus>(open ? "open" : "closing");
  const timerRef = useRef<number | null>(null);

  const clearTimer = () => {
    if (timerRef.current !== null) {
      window.clearTimeout(timerRef.current);
      timerRef.current = null;
    }
  };

  useEffect(() => {
    if (open) {
      clearTimer();
      setMounted(true);
      const raf = requestAnimationFrame(() => setStatus("open"));
      return () => cancelAnimationFrame(raf);
    }
    if (!mounted) return;
    setStatus("closing");
    const wait = prefersReducedMotion() ? 0 : duration;
    clearTimer();
    timerRef.current = window.setTimeout(() => {
      timerRef.current = null;
      setMounted(false);
    }, wait);
    return undefined;
  }, [open, mounted, duration]);

  useEffect(() => () => clearTimer(), []);

  return { mounted, status };
}

// useDeferredClose suits overlays whose mount is owned by a parent (rendered as
// `{cond && <Panel onClose={...} />}`). The panel defers the parent's unmount
// so the exit animation can play first.
export function useDeferredClose(
  onClose: () => void,
  duration: number,
): { status: MountStatus; requestClose: () => void } {
  const [closing, setClosing] = useState(false);
  const timerRef = useRef<number | null>(null);
  const onCloseRef = useRef(onClose);
  onCloseRef.current = onClose;

  const requestClose = () => {
    if (timerRef.current !== null) return;
    setClosing(true);
    const wait = prefersReducedMotion() ? 0 : duration;
    timerRef.current = window.setTimeout(() => {
      timerRef.current = null;
      onCloseRef.current();
    }, wait);
  };

  useEffect(
    () => () => {
      if (timerRef.current !== null) window.clearTimeout(timerRef.current);
    },
    [],
  );

  return { status: closing ? "closing" : "open", requestClose };
}
