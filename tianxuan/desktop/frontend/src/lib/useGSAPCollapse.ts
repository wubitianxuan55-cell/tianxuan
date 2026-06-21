import { useLayoutEffect, useRef } from "react";
import gsap from "gsap";
import { DUR_BASE, EASE_OUT, prefersReducedMotion } from "./gsapAnimations";

/**
 * useGSAPCollapse — animate a container's height between 0 and its
 * scrollHeight whenever `open` flips.  Replaces the old CSS max-height
 * hack with a precise pixel-level GSAP tween.
 *
 * Usage:
 *   const ref = useRef<HTMLDivElement>(null);
 *   useGSAPCollapse(ref, open);
 *   return <div ref={ref} style={{ overflow: "hidden" }}>{children}</div>;
 *
 * The container should have `overflow: hidden` in CSS.
 */
export function useGSAPCollapse(
  ref: React.RefObject<HTMLElement | null>,
  open: boolean,
  opts?: {
    duration?: number;
    ease?: string;
    /** Called after the open animation completes. */
    onOpenComplete?: () => void;
    /** Called after the close animation completes. */
    onCloseComplete?: () => void;
    /** When closing, use this height as the starting point instead of
     *  measuring scrollHeight (which may have already shrunk due to
     *  content being conditionally removed). */
    prevHeight?: number;
  },
) {
  const prevOpen = useRef<boolean | null>(null);
  const onOpenRef = useRef(opts?.onOpenComplete);
  const onCloseRef = useRef(opts?.onCloseComplete);
  const durRef = useRef(opts?.duration ?? DUR_BASE);
  const easeRef = useRef(opts?.ease ?? EASE_OUT);
  const prevHeightRef = useRef(opts?.prevHeight);
  onOpenRef.current = opts?.onOpenComplete;
  onCloseRef.current = opts?.onCloseComplete;
  durRef.current = opts?.duration ?? DUR_BASE;
  easeRef.current = opts?.ease ?? EASE_OUT;
  prevHeightRef.current = opts?.prevHeight;

  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;

    // Skip the very first render — don't animate from 0→auto on mount.
    // Use a direct style write (no GSAP overhead) for the initial state.
    if (prevOpen.current === null) {
      prevOpen.current = open;
      el.style.height = open ? "auto" : "0px";
      return;
    }

    // No change — nothing to do.
    if (prevOpen.current === open) return;
    prevOpen.current = open;

    const reduced = prefersReducedMotion();
    const dur = reduced ? 0.001 : durRef.current;
    const ease = easeRef.current;

    // Kill any in-flight GSAP animations on this element.
    gsap.killTweensOf(el);

    if (open) {
      // Phase 1 — measure the target (auto) height without visible change.
      gsap.set(el, { height: "auto" });
      const targetHeight = el.scrollHeight;
      // Phase 2 — animate from 0 to the measured target height.
      gsap.fromTo(
        el,
        { height: 0 },
        {
          height: targetHeight,
          duration: dur,
          ease,
          clearProps: "height",
          onComplete: () => onOpenRef.current?.(),
        },
      );
    } else {
      // Close: if caller provided a pre-swap height use it as the start,
      // otherwise measure the current scrollHeight.
      const startHeight = prevHeightRef.current && prevHeightRef.current > 0
        ? prevHeightRef.current
        : (gsap.set(el, { height: "auto" }), el.scrollHeight);
      gsap.fromTo(
        el,
        { height: startHeight },
        {
          height: 0,
          duration: dur,
          ease,
          onComplete: () => onCloseRef.current?.(),
        },
      );
    }
  }, [open, ref]);
}
