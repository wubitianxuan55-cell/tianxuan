import { describe, it, expect } from "vitest";

// Replicate the store's per-turn usage accumulation logic in a pure function.
// This mirrors the case "usage" handler in store.ts without Zustand/React.

interface WireUsage {
  promptTokens: number;
  completionTokens: number;
  totalTokens: number;
  cacheHitTokens: number;
  cacheMissTokens: number;
  sessionCacheHitTokens: number;
  sessionCacheMissTokens: number;
  source?: string;
  turn?: number;
  costUsd?: number;
}

interface UsageAcc {
  combined: WireUsage | undefined;
  main: WireUsage | undefined;
  sub: WireUsage | undefined;
  steps: WireUsage[];
}

function accBySource(prev: WireUsage | undefined, u: WireUsage): WireUsage {
  if (!prev) return { ...u };
  return {
    promptTokens: prev.promptTokens + u.promptTokens,
    completionTokens: prev.completionTokens + u.completionTokens,
    totalTokens: prev.totalTokens + u.totalTokens,
    cacheHitTokens: prev.cacheHitTokens + u.cacheHitTokens,
    cacheMissTokens: prev.cacheMissTokens + u.cacheMissTokens,
    sessionCacheHitTokens: u.sessionCacheHitTokens > 0 ? u.sessionCacheHitTokens : prev.sessionCacheHitTokens,
    sessionCacheMissTokens: u.sessionCacheMissTokens > 0 ? u.sessionCacheMissTokens : prev.sessionCacheMissTokens,
  };
}

function applyUsage(acc: UsageAcc, u: WireUsage): UsageAcc {
  const isSub = u.source === "subagent";
  return {
    combined: accBySource(acc.combined, u),
    main: isSub ? acc.main : accBySource(acc.main, u),
    sub: isSub ? accBySource(acc.sub, u) : acc.sub,
    steps: [...acc.steps, { ...u }],
  };
}

function emptyAcc(): UsageAcc {
  return { combined: undefined, main: undefined, sub: undefined, steps: [] };
}

describe("store usage accumulation", () => {
  const mkUsage = (overrides: Partial<WireUsage> = {}): WireUsage => ({
    promptTokens: 100, completionTokens: 50, totalTokens: 150,
    cacheHitTokens: 80, cacheMissTokens: 20,
    sessionCacheHitTokens: 0, sessionCacheMissTokens: 0,
    ...overrides,
  });

  it("accumulates main-only usage correctly", () => {
    const events = [
      mkUsage({ promptTokens: 100, completionTokens: 50, totalTokens: 150, source: "main" }),
      mkUsage({ promptTokens: 200, completionTokens: 100, totalTokens: 300, source: "main" }),
    ];
    const acc = events.reduce(applyUsage, emptyAcc());
    expect(acc.combined?.promptTokens).toBe(300);
    expect(acc.combined?.totalTokens).toBe(450);
    expect(acc.main?.promptTokens).toBe(300);
    expect(acc.sub).toBeUndefined();
    expect(acc.steps.length).toBe(2);
  });

  it("accumulates subagent-only usage correctly", () => {
    const events = [
      mkUsage({ promptTokens: 50, totalTokens: 80, source: "subagent" }),
      mkUsage({ promptTokens: 30, totalTokens: 50, source: "subagent" }),
    ];
    const acc = events.reduce(applyUsage, emptyAcc());
    expect(acc.combined?.promptTokens).toBe(80);
    expect(acc.main).toBeUndefined();
    expect(acc.sub?.promptTokens).toBe(80);
  });

  it("splits mixed main + subagent usage", () => {
    const events = [
      mkUsage({ promptTokens: 500, totalTokens: 600, source: "main" }),
      mkUsage({ promptTokens: 200, totalTokens: 300, source: "main" }),
      mkUsage({ promptTokens: 100, totalTokens: 150, source: "subagent" }),
      mkUsage({ promptTokens: 50,  totalTokens: 80,  source: "subagent" }),
    ];
    const acc = events.reduce(applyUsage, emptyAcc());
    expect(acc.combined?.promptTokens).toBe(850);
    expect(acc.main?.promptTokens).toBe(700);
    expect(acc.sub?.promptTokens).toBe(150);
    expect(acc.steps.length).toBe(4);
    expect(acc.steps[0].source).toBe("main");
    expect(acc.steps[2].source).toBe("subagent");
  });

  it("handles empty events", () => {
    const acc = emptyAcc();
    expect(acc.combined).toBeUndefined();
    expect(acc.main).toBeUndefined();
    expect(acc.sub).toBeUndefined();
    expect(acc.steps).toEqual([]);
  });

  it("treats missing source as main", () => {
    const events = [
      mkUsage({ promptTokens: 100, totalTokens: 150 }), // no source
    ];
    const acc = events.reduce(applyUsage, emptyAcc());
    expect(acc.main?.promptTokens).toBe(100);
    expect(acc.sub).toBeUndefined();
  });
});
