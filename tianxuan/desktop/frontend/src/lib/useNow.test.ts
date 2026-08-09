// @vitest-environment happy-dom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, renderHook } from "@testing-library/react";
import type { useNow as UseNowFn } from "./useNow";

let useNow: typeof UseNowFn;

beforeEach(async () => {
  // 重置模块：useNow 的 listeners/timerId 是模块级状态，跨测试会泄漏
  vi.resetModules();
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-08-09T10:00:00.000Z"));
  ({ useNow } = await import("./useNow"));
});

afterEach(() => {
  vi.useRealTimers();
});

describe("useNow", () => {
  it("ticks every second while active (default)", () => {
    const { result } = renderHook(() => useNow());
    const t0 = result.current;
    act(() => {
      vi.advanceTimersByTime(1000);
    });
    expect(result.current).toBe(t0 + 1);
    act(() => {
      vi.advanceTimersByTime(2000);
    });
    expect(result.current).toBe(t0 + 3);
  });

  it("does not tick while inactive", () => {
    const { result } = renderHook(() => useNow(false));
    const t0 = result.current;
    act(() => {
      vi.advanceTimersByTime(5000);
    });
    expect(result.current).toBe(t0);
  });

  it("starts ticking when active flips from false to true", () => {
    const { result, rerender } = renderHook(({ active }) => useNow(active), {
      initialProps: { active: false },
    });
    const t0 = result.current;
    rerender({ active: true });
    act(() => {
      vi.advanceTimersByTime(1000);
    });
    expect(result.current).toBe(t0 + 1);
  });

  it("stops ticking when active flips from true to false", () => {
    const { result, rerender } = renderHook(({ active }) => useNow(active), {
      initialProps: { active: true },
    });
    const t0 = result.current;
    rerender({ active: false });
    act(() => {
      vi.advanceTimersByTime(4000);
    });
    expect(result.current).toBe(t0);
  });
});
