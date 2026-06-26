import { useEffect, useRef } from "react";
import gsap from "gsap";
import { DUR_SLOW, EASE_OUT, prefersReducedMotion } from "./gsapAnimations";

// 入场动画 Hook —— 基于 GSAP 的 data-entrance 属性系统。
// 首次挂载（和 resetKey 变化时）预填充已见集合，历史项永远不播动画；
// 仅在 deps 变化时扫描新增元素并播放入场动画。
// (Design adopted from DeepSeek-Reasonix-V1.12)
export function useEntranceAnimation<T extends HTMLElement>(
  resetKey?: unknown,
  deps?: unknown,
  selector = "[data-entrance]",
) {
  const ref = useRef<T | null>(null);
  const seen = useRef(new Set<string>());
  const timerRef = useRef<number | null>(null);
  const firstRun = useRef(true);
  const prevResetKey = useRef(resetKey);

  // 会话切换时重置
  if (prevResetKey.current !== resetKey) {
    prevResetKey.current = resetKey;
    seen.current = new Set();
    firstRun.current = true;
    if (timerRef.current !== null) {
      clearTimeout(timerRef.current);
      timerRef.current = null;
    }
  }

  // 单次 effect：首次挂载时预填充已见集合（不播动画）。
  // 后续 deps 变化时只对新增元素播放入场动画。
  useEffect(() => {
    const container = ref.current;
    if (!container) return;

    const entries: HTMLElement[] = [];
    container.querySelectorAll(selector).forEach((el) => {
      const id = el.getAttribute("data-entrance");
      if (id && !seen.current.has(id)) {
        seen.current.add(id);
        // 首次运行：仅记录ID，不对历史项播动画
        if (firstRun.current) return;
        entries.push(el as HTMLElement);
      }
    });

    if (firstRun.current) {
      firstRun.current = false;
      return; // 预填充完毕，不播动画
    }

    if (entries.length === 0) return;

    if (prefersReducedMotion()) {
      // 无动画模式：直接显示
      for (const el of entries) {
        gsap.set(el, { opacity: 1, y: 0, clearProps: "all" });
      }
      return;
    }

    // 批量处理，requestAnimationFrame 后再启动避免 layout thrashing
    timerRef.current = window.setTimeout(() => {
      timerRef.current = null;
      gsap.fromTo(
        entries,
        { opacity: 0, y: 8 },
        { opacity: 1, y: 0, duration: DUR_SLOW, ease: EASE_OUT, stagger: 0.04, overwrite: "auto" },
      );
    }, 0);
  }, [deps, selector]);

  return ref;
}
